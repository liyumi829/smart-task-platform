// internal/service/forwarding_service/worker/outbox_worker_flow_rabbitmq_integration_test.go
// Package worker
// 功能：整合测试完整消息链路。
// 1. SQLite OutboxMessage 被 OutboxWorkerManager 拉取
// 2. OutboxWorkerManager 通过真实 RabbitMQ 发布消息
// 3. HandleWorkerManager 通过真实 RabbitMQ 消费消息
// 4. 消费成功后 ACK，OutboxMessage 状态变为 sent
//
// 说明：
// - 不使用单独的 outbox worker 直接跑流程，而是使用 OutboxWorkerManager
// - 不自定义假的 publish 实现，直接使用项目内已实现的真实 RabbitMQ Publisher / Subscriber
// - Repository 使用真实 SQLite + GORM，方便观察数据库状态流转

package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"smart-task-platform/internal/bootstrap"
	"smart-task-platform/internal/model"
	mqrabbit "smart-task-platform/internal/mq/rabbitmq"
	"smart-task-platform/internal/repository"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//==============================
// 测试状态常量
//==============================

const (
	flowOutboxStatusPending    = "pending"
	flowOutboxStatusProcessing = "processing"
	flowOutboxStatusSent       = "sent"
	flowOutboxStatusFailed     = "failed"
)

//==============================
// 测试配置
//==============================

// newFlowTestLogger 创建测试日志
func newFlowTestLogger(t *testing.T) *zap.Logger {
	t.Helper()

	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	zap.ReplaceGlobals(logger)

	return zap.L()
}

// newFlowRabbitMQConfig 创建 RabbitMQ 测试配置
func newFlowRabbitMQConfig(t *testing.T) bootstrap.RabbitMQConfig {
	t.Helper()

	suffix := fmt.Sprintf("flow.%d", time.Now().UnixNano())

	host := os.Getenv("TEST_RABBITMQ_HOST")
	if host == "" {
		host = "localhost"
	}

	user := os.Getenv("TEST_RABBITMQ_USER")
	if user == "" {
		user = "root"
	}

	password := os.Getenv("TEST_RABBITMQ_PASSWORD")
	if password == "" {
		password = "967096489"
	}

	return bootstrap.RabbitMQConfig{
		Host:     host,
		User:     user,
		Password: password,

		ConnectTimeout: 5 * time.Second,

		ExchangeName: fmt.Sprintf("smart-task-platform.flow.exchange.%s", suffix),
		ExchangeType: "direct",
		RoutingKey:   fmt.Sprintf("smart-task-platform.flow.routing.%s", suffix),
		QueueName:    fmt.Sprintf("smart-task-platform.flow.queue.%s", suffix),

		RetryExchangeName: fmt.Sprintf("smart-task-platform.flow.retry.exchange.%s", suffix),
		RetryRoutingKey:   fmt.Sprintf("smart-task-platform.flow.retry.routing.%s", suffix),
		RetryQueueName:    fmt.Sprintf("smart-task-platform.flow.retry.queue.%s", suffix),
		RetryDelay:        500 * time.Millisecond,

		DeadLetterExchangeName: fmt.Sprintf("smart-task-platform.flow.dlx.%s", suffix),
		DeadLetterRoutingKey:   fmt.Sprintf("smart-task-platform.flow.dead.routing.%s", suffix),
		DeadLetterQueueName:    fmt.Sprintf("smart-task-platform.flow.dlq.%s", suffix),

		PublishTimeout: 5 * time.Second,

		ConsumerTag:           fmt.Sprintf("smart-task-platform-flow-consumer-%s", suffix),
		PrefetchCount:         4,
		SubscribeCloseTimeout: 5 * time.Second,
	}
}

// requireFlowRabbitMQ 确认 RabbitMQ 可连接
func requireFlowRabbitMQ(t *testing.T, config bootstrap.RabbitMQConfig) {
	t.Helper()

	conn, err := amqp.Dial(config.URL())
	if err != nil {
		t.Skipf("RabbitMQ unavailable, skip test: %v", err)
		return
	}

	_ = conn.Close()
}

// cleanupFlowRabbitMQTopology 清理测试拓扑
func cleanupFlowRabbitMQTopology(t *testing.T, config bootstrap.RabbitMQConfig) {
	t.Helper()

	conn, err := amqp.Dial(config.URL())
	if err != nil {
		t.Logf("cleanup RabbitMQ dial failed: %v", err)
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	ch, err := conn.Channel()
	if err != nil {
		t.Logf("cleanup RabbitMQ channel failed: %v", err)
		return
	}
	defer func() {
		_ = ch.Close()
	}()

	_, _ = ch.QueueDelete(config.QueueName, false, false, false)
	_, _ = ch.QueueDelete(config.RetryQueueName, false, false, false)
	_, _ = ch.QueueDelete(config.DeadLetterQueueName, false, false, false)

	_ = ch.ExchangeDelete(config.ExchangeName, false, false)
	_ = ch.ExchangeDelete(config.RetryExchangeName, false, false)
	_ = ch.ExchangeDelete(config.DeadLetterExchangeName, false, false)
}

// ====================== 用例1：Outbox->Handle全链路真实SQLite+RabbitMQ批量消息 ======================
func TestOutboxManagerToHandleManager_FullFlow_RealSQLiteAndRabbitMQ(t *testing.T) {
	logger := newFlowTestLogger(t)
	rabbitCfg := newFlowRabbitMQConfig(t)
	requireFlowRabbitMQ(t, rabbitCfg)

	// 后置清理MQ拓扑
	t.Cleanup(func() { cleanupFlowRabbitMQTopology(t, rabbitCfg) })

	db := newFlowSQLiteDB(t)
	outboxRepo := &flowOutboxRepository{db: db}
	// 构造多条待投递消息
	payloadList := [][]byte{
		[]byte(`{"user_id":1001,"content":"full flow message 1"}`),
		[]byte(`{"user_id":1002,"content":"full flow message 2"}`),
		[]byte(`{"user_id":1003,"content":"full flow message 3"}`),
	}
	var createList []*model.OutboxMessage
	event2Payload := make(map[string]string)
	event2Type := make(map[string]string)

	for _, body := range payloadList {
		msg := insertFlowOutboxMessage(t, db, rabbitCfg.ExchangeName, rabbitCfg.RoutingKey, body, 0, 3)
		createList = append(createList, msg)
		event2Payload[msg.EventID] = string(body)
		event2Type[msg.EventID] = msg.EventType
	}

	handledCh := make(chan *ConsumedMessage, len(createList))

	handle := func(msg *ConsumedMessage) HandlerResult {
		wantPayload, ok := event2Payload[msg.EventID]
		if !ok {
			return RetryResult(fmt.Sprintf("unknown event_id: %s", msg.EventID))
		}
		if msg.EventType != event2Type[msg.EventID] {
			return RetryResult(fmt.Sprintf("event_type mismatch: got=%s want=%s", msg.EventType, event2Type[msg.EventID]))
		}
		if string(msg.Payload) != wantPayload {
			return RetryResult("payload mismatch")
		}
		handledCh <- msg
		return AckResult()
	}

	// 初始化消费管理器
	handleMgr := NewHandleWorkerManager(
		HandleWorkerManagerConfig{
			WorkerCount:    2,
			WorkerIDPrefix: "flow-handle-worker",
			StopTimeout:    5 * time.Second,
			HandleWorkerConfig: bootstrap.HandleWorkerConfig{
				MaxRetries:        3,
				RetryExchangeName: rabbitCfg.RetryExchangeName,
				RetryRoutingKey:   rabbitCfg.RetryRoutingKey,
			},
		},
		PublisherFactoryFunc(func(ctx context.Context, wid string, idx int) (CloseablePublisher, error) {
			return mqrabbit.NewPublisher(rabbitCfg, logger, nil)
		}),
		SubscriberFactoryFunc(func(ctx context.Context) (CloseableSubscriber, error) {
			return mqrabbit.NewSubscriber(rabbitCfg, logger, nil)
		}),
		logger,
	)
	stopHandle := startBlockingHandleManager(t, handleMgr, handle)
	defer stopHandle()

	// 初始化发件管理器
	resetOff := false
	outboxMgr := NewOutboxWorkerManager(
		OutboxWorkerManagerConfig{
			WorkerCount:    2,
			WorkerIDPrefix: "flow-outbox-worker",
			StopTimeout:    5 * time.Second,
			OutboxWorkerConfig: bootstrap.OutboxWorkerConfig{
				PollInterval:          100 * time.Millisecond,
				BatchSize:             10,
				ProcessingTimeout:     time.Minute,
				RetryBackoff:          500 * time.Millisecond,
				ResetTimeoutOnStartup: &resetOff,
			},
		},
		outboxRepo.TxManager(),
		outboxRepo,
		PublisherFactoryFunc(func(ctx context.Context, wid string, idx int) (CloseablePublisher, error) {
			return mqrabbit.NewPublisher(rabbitCfg, logger, nil)
		}),
		logger,
	)
	stopOutbox := startBlockingOutboxManager(t, outboxMgr)
	defer stopOutbox()

	// 等待全部消息消费完成
	consumedList := drainConsumedMessages(t, handledCh, len(createList), 10*time.Second)
	consumeEventSet := make(map[string]struct{})
	for _, m := range consumedList {
		require.NotEmpty(t, m.MessageID)
		consumeEventSet[m.EventID] = struct{}{}
	}

	// 逐条校验落库状态
	for _, origin := range createList {
		_, exist := consumeEventSet[origin.EventID]
		require.True(t, exist, "event not consumed: %s", origin.EventID)

		saved := waitFlowOutboxStatus(t, db, origin.ID, flowOutboxStatusSent, 5*time.Second)
		require.NotNil(t, saved.SentAt)
		require.Nil(t, saved.LockedBy)
		require.Nil(t, saved.LockedAt)
		require.Nil(t, saved.ErrorMessage)
		require.EqualValues(t, 0, saved.RetryCount)
	}
	t.Logf("full flow test pass, total msg: %d", len(createList))
}

// ====================== 用例2：发布持续失败→耗尽重试流转Failed ======================
func TestOutboxManager_RealSQLite_PublishFailed_MarkFailed(t *testing.T) {
	logger := newFlowTestLogger(t)
	db := newFlowSQLiteDB(t)
	outboxRepo := &flowOutboxRepository{db: db}

	originMsg := insertFlowOutboxMessage(
		t, db, "fake.exchange", "fake.routing", []byte(`{"content":"publish failed retry"}`), 0, 3,
	)

	resetOff := false
	txMgr := repository.NewTxManager(db)
	outboxMgr := NewOutboxWorkerManager(
		OutboxWorkerManagerConfig{
			WorkerCount:    1,
			WorkerIDPrefix: "flow-retry-worker",
			StopTimeout:    5 * time.Second,
			OutboxWorkerConfig: bootstrap.OutboxWorkerConfig{
				PollInterval:          50 * time.Millisecond,
				BatchSize:             10,
				ProcessingTimeout:     time.Minute,
				RetryBackoff:          50 * time.Millisecond,
				ResetTimeoutOnStartup: &resetOff,
			},
		},
		txMgr,
		outboxRepo,
		PublisherFactoryFunc(func(_ context.Context, _ string, _ int) (CloseablePublisher, error) {
			return &failedPublishMessagePublisher{}, nil
		}),
		logger,
	)

	stopOutbox := startBlockingOutboxManager(t, outboxMgr)
	defer stopOutbox()

	// 等待流转失败状态
	final := waitFlowOutboxStatus(t, db, originMsg.ID, flowOutboxStatusFailed, 8*time.Second)
	require.EqualValues(t, 3, final.RetryCount)
	require.NotNil(t, final.ErrorMessage)
	require.NotEmpty(t, *final.ErrorMessage)
	require.Nil(t, final.LockedBy)
	require.Nil(t, final.LockedAt)
	require.Nil(t, final.SentAt)

	t.Logf("retry failed test pass, outbox_id=%d retry_cnt=%d", final.ID, final.RetryCount)
}

// ====================== 用例3：ResetTimeoutOnStartup启动重置长时间卡住Processing脏数据 ======================
func TestOutboxManager_RealSQLite_ResetTimeoutProcessingOnStartup(t *testing.T) {
	logger := newFlowTestLogger(t)
	db := newFlowSQLiteDB(t)
	outboxRepo := &flowOutboxRepository{db: db}

	now := time.Now()
	lockWorker := "dead-worker"
	lockTime := now.Add(-10 * time.Minute)
	// 预插入一条长时间卡在processing的脏消息
	dirtyMsg := &model.OutboxMessage{
		EventID:       fmt.Sprintf("flow-timeout-event-%d", now.UnixNano()),
		EventType:     "notification.created",
		ExchangeName:  "fake.exchange",
		RoutingKey:    "fake.routing",
		Payload:       datatypes.JSON([]byte(`{"content":"reset timeout processing message"}`)),
		Status:        flowOutboxStatusProcessing,
		RetryCount:    2,
		MaxRetryCount: 3,
		LockedBy:      &lockWorker,
		LockedAt:      &lockTime,
		CreatedAt:     now.Add(-10 * time.Minute),
		UpdatedAt:     now.Add(-10 * time.Minute),
	}
	require.NoError(t, db.Create(dirtyMsg).Error)

	// 开启启动自动重置超时processing开关
	resetOn := true
	txMgr := repository.NewTxManager(db)
	outboxMgr := NewOutboxWorkerManager(
		OutboxWorkerManagerConfig{
			WorkerCount:    1,
			WorkerIDPrefix: "flow-reset-timeout-worker",
			StopTimeout:    5 * time.Second,
			OutboxWorkerConfig: bootstrap.OutboxWorkerConfig{
				PollInterval:          50 * time.Millisecond,
				BatchSize:             10,
				ProcessingTimeout:     time.Minute,
				RetryBackoff:          50 * time.Millisecond,
				ResetTimeoutOnStartup: &resetOn,
			},
		},
		txMgr,
		outboxRepo,
		PublisherFactoryFunc(func(_ context.Context, _ string, _ int) (CloseablePublisher, error) {
			return &failedPublishMessagePublisher{}, nil
		}),
		logger,
	)

	stopOutbox := startBlockingOutboxManager(t, outboxMgr)
	defer stopOutbox()

	final := waitFlowOutboxStatus(t, db, dirtyMsg.ID, flowOutboxStatusFailed, 8*time.Second)
	require.EqualValues(t, 3, final.RetryCount)
	require.Nil(t, final.LockedBy)
	require.Nil(t, final.LockedAt)
	require.NotNil(t, final.ErrorMessage)
	require.NotEmpty(t, *final.ErrorMessage)

	t.Logf("reset timeout startup test pass, outbox_id=%d", final.ID)
}

//==============================
// SQLite Repository
//==============================

// flowOutboxRepository 使用真实 SQLite / GORM 操作 OutboxMessage
type flowOutboxRepository struct {
	db *gorm.DB
}

func (r *flowOutboxRepository) TxManager() *repository.TxManager {
	return repository.NewTxManager(r.db)
}

// ClaimPending 抢占 pending 消息，并更新为 processing
func (r *flowOutboxRepository) ClaimPending(
	ctx context.Context,
	tx *gorm.DB,
	param *repository.ClaimPendingParam,
) ([]*model.OutboxMessage, error) {
	if param == nil {
		return nil, errors.New("claim pending param is nil")
	}

	if tx == nil {
		tx = r.db
	}

	var messages []*model.OutboxMessage

	err := tx.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		if err := db.
			Where("status = ?", flowOutboxStatusPending).
			Where("(next_retry_at IS NULL OR next_retry_at <= ?)", param.Now).
			Order("created_at ASC").
			Limit(param.Limit).
			Find(&messages).Error; err != nil {
			return err
		}

		if len(messages) == 0 {
			return nil
		}

		ids := make([]uint64, 0, len(messages))
		for _, message := range messages {
			ids = append(ids, message.ID)
		}

		if err := db.Model(&model.OutboxMessage{}).
			Where("id IN ?", ids).
			Where("status = ?", flowOutboxStatusPending).
			Updates(map[string]any{
				"status":     flowOutboxStatusProcessing,
				"locked_by":  param.WorkerID,
				"locked_at":  param.LockedAt,
				"updated_at": param.Now,
			}).Error; err != nil {
			return err
		}

		return db.Where("id IN ?", ids).Order("id ASC").Find(&messages).Error
	})

	if err != nil {
		return nil, err
	}

	return messages, nil
}

// MarkAsPublished 发布成功后标记 sent
func (r *flowOutboxRepository) MarkAsPublished(
	ctx context.Context,
	param *repository.MarkOutboxSentParam,
) error {
	if param == nil {
		return errors.New("mark as published param is nil")
	}

	return r.db.WithContext(ctx).
		Model(&model.OutboxMessage{}).
		Where("id = ?", param.MessageID).
		Updates(map[string]any{
			"status":        flowOutboxStatusSent,
			"sent_at":       param.SentAt,
			"locked_by":     nil,
			"locked_at":     nil,
			"error_message": nil,
			"updated_at":    time.Now(),
		}).Error
}

// MarkAsRetry 发布失败后进入重试
func (r *flowOutboxRepository) MarkAsRetry(
	ctx context.Context,
	param *repository.MarkOutboxRetryParam,
) error {
	if param == nil {
		return errors.New("mark as retry param is nil")
	}

	return r.db.WithContext(ctx).
		Model(&model.OutboxMessage{}).
		Where("id = ?", param.MessageID).
		Updates(map[string]any{
			"status":        flowOutboxStatusPending,
			"retry_count":   gorm.Expr("retry_count + 1"),
			"next_retry_at": param.NextRetryAt,
			"locked_by":     nil,
			"locked_at":     nil,
			"error_message": param.ErrorMessage,
			"updated_at":    param.UpdatedAt,
		}).Error
}

// MarkAsFailed 达到最大重试次数后标记 failed
func (r *flowOutboxRepository) MarkAsFailed(
	ctx context.Context,
	param *repository.MarkOutboxFailedParam,
) error {
	if param == nil {
		return errors.New("mark as failed param is nil")
	}

	return r.db.WithContext(ctx).
		Model(&model.OutboxMessage{}).
		Where("id = ?", param.MessageID).
		Updates(map[string]any{
			"status":        flowOutboxStatusFailed,
			"retry_count":   gorm.Expr("retry_count + 1"),
			"locked_by":     nil,
			"locked_at":     nil,
			"error_message": param.ErrorMessage,
			"updated_at":    param.UpdatedAt,
		}).Error
}

// ResetTimeoutProcessingMessages 重置超时 processing 消息
func (r *flowOutboxRepository) ResetTimeoutProcessingMessages(
	ctx context.Context,
	param *repository.ResetTimeoutProcessingMessagesParam,
) error {
	if param == nil {
		return errors.New("reset timeout param is nil")
	}

	return r.db.WithContext(ctx).
		Model(&model.OutboxMessage{}).
		Where("status = ?", flowOutboxStatusProcessing).
		Where("locked_at IS NOT NULL AND locked_at < ?", param.Before).
		Updates(map[string]any{
			"status":     flowOutboxStatusPending,
			"locked_by":  nil,
			"locked_at":  nil,
			"updated_at": param.UpdatedAt,
		}).Error
}

// newFlowSQLiteDB 创建真实 SQLite 测试库
func newFlowSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "outbox_flow_test.db")

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}

	if err := db.AutoMigrate(&model.OutboxMessage{}); err != nil {
		t.Fatalf("auto migrate outbox message failed: %v", err)
	}

	return db
}

// insertFlowOutboxMessage 插入一条真实 outbox 数据
func insertFlowOutboxMessage(
	t *testing.T,
	db *gorm.DB,
	exchangeName string,
	routingKey string,
	payload []byte,
	retryCount int,
	maxRetryCount int,
) *model.OutboxMessage {
	t.Helper()

	now := time.Now()

	message := &model.OutboxMessage{
		EventID:       fmt.Sprintf("flow-event-%d", now.UnixNano()),
		EventType:     "notification.created",
		ExchangeName:  exchangeName,
		Payload:       datatypes.JSON(payload),
		RoutingKey:    routingKey,
		Status:        flowOutboxStatusPending,
		RetryCount:    retryCount,
		MaxRetryCount: maxRetryCount,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := db.Create(message).Error; err != nil {
		t.Fatalf("insert outbox message failed: %v", err)
	}

	return message
}

// getFlowOutboxMessage 查询 outbox 数据
func getFlowOutboxMessage(t *testing.T, db *gorm.DB, id uint64) *model.OutboxMessage {
	t.Helper()

	var message model.OutboxMessage
	if err := db.First(&message, "id = ?", id).Error; err != nil {
		t.Fatalf("query outbox message failed: %v", err)
	}

	return &message
}

// waitFlowOutboxStatus 等待 outbox 状态变化
func waitFlowOutboxStatus(
	t *testing.T,
	db *gorm.DB,
	id uint64,
	status string,
	timeout time.Duration,
) *model.OutboxMessage {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			message := getFlowOutboxMessage(t, db, id)
			t.Fatalf("wait outbox status timeout, want=%s got=%s", status, message.Status)
			return nil

		case <-ticker.C:
			message := getFlowOutboxMessage(t, db, id)
			if message.Status == status {
				return message
			}
		}
	}
}

//==============================
// RabbitMQ 工具
//==============================

// getMessageFromQueue 从队列直接拉取一条消息
func getMessageFromQueue(t *testing.T, config bootstrap.RabbitMQConfig, queueName string, timeout time.Duration) *amqp.Delivery {
	t.Helper()

	conn, err := amqp.Dial(config.URL())
	if err != nil {
		t.Fatalf("RabbitMQ dial failed: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("RabbitMQ channel create failed: %v", err)
	}
	defer func() {
		_ = ch.Close()
	}()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			t.Fatalf("get message from queue timeout, queue=%s", queueName)
			return nil

		case <-ticker.C:
			delivery, ok, err := ch.Get(queueName, false)
			if err != nil {
				t.Fatalf("RabbitMQ queue get failed: %v", err)
			}

			if ok {
				return &delivery
			}
		}
	}
}

//==============================
// 真实 Publisher / Subscriber
//==============================

// mustCreateRealPublisher 创建真实 RabbitMQ Publisher
func mustCreateRealPublisher(
	t *testing.T,
	config bootstrap.RabbitMQConfig,
	logger *zap.Logger,
) CloseablePublisher {
	t.Helper()

	publisher, err := mqrabbit.NewPublisher(config, logger, nil)
	if err != nil {
		t.Fatalf("create RabbitMQ publisher failed: %v", err)
	}

	return publisher
}

// mustCreateRealSubscriber 创建真实 RabbitMQ Subscriber
func mustCreateRealSubscriber(
	t *testing.T,
	config bootstrap.RabbitMQConfig,
	logger *zap.Logger,
) CloseableSubscriber {
	t.Helper()

	subscriber, err := mqrabbit.NewSubscriber(config, logger, nil)
	if err != nil {
		t.Fatalf("create RabbitMQ subscriber failed: %v", err)
	}

	return subscriber
}

// failedPublishMessagePublisher 用于模拟 publish 失败
type failedPublishMessagePublisher struct{}

// Publish 始终失败
func (p *failedPublishMessagePublisher) Publish(ctx context.Context, message *PublishMessage) error {
	return errors.New("mock publish failed")
}

// Close 关闭
func (p *failedPublishMessagePublisher) Close() error {
	return nil
}

// ====================== 通用阻塞启动Helper（全局复用，无需重复编码） ======================
// startBlockingHandleManager 协程启动阻塞HandleWorkerManager.Start，就绪探测+返回关闭闭包
func startBlockingHandleManager(t *testing.T, manager *HandleWorkerManager, handle HandleMessage) func() {
	t.Helper()
	startErrCh := make(chan error, 1)

	// 协程运行阻塞Start
	go func() {
		startErrCh <- manager.Start(context.Background(), handle)
	}()

	// 轮询等待服务标记为已启动，避免未就绪就下发消息
	waitFlowCondition(t, "handle manager started", 5*time.Second, manager.IsStarted)

	// 返回关闭回调：Close + 阻塞等待Start协程退出
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// 执行关闭
		err := manager.Close(ctx)
		if err != nil && !errors.Is(err, ErrHandleWorkerManagerNotStarted) {
			t.Fatalf("close handle worker manager failed: %v", err)
		}

		// 等待阻塞Start函数返回
		select {
		case err = <-startErrCh:
			require.NoError(t, err, "handle manager start exit with unexpected error")
		case <-time.After(5 * time.Second):
			t.Fatal("wait handle worker manager Start exit timeout")
		}

		// 兜底校验：必须处于停止状态
		require.False(t, manager.IsStarted(), "handle manager should be stopped after close")
	}
}

// startBlockingOutboxManager 协程启动阻塞OutboxWorkerManager.Start，同上面规范
func startBlockingOutboxManager(t *testing.T, manager *OutboxWorkerManager) func() {
	t.Helper()
	startErrCh := make(chan error, 1)

	go func() {
		startErrCh <- manager.Start(context.Background())
	}()

	waitFlowCondition(t, "outbox manager started", 5*time.Second, manager.IsStarted)

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := manager.Close(ctx)
		if err != nil && !errors.Is(err, ErrOutboxWorkerManagerNotStarted) {
			t.Fatalf("close outbox worker manager failed: %v", err)
		}

		select {
		case err = <-startErrCh:
			require.NoError(t, err, "outbox manager start exit with unexpected error")
		case <-time.After(5 * time.Second):
			t.Fatal("wait outbox worker manager Start exit timeout")
		}

		require.False(t, manager.IsStarted(), "outbox manager should be stopped after close")
	}
}

// waitFlowCondition 通用轮询等待条件满足，带超时防卡死
func waitFlowCondition(t *testing.T, desc string, timeout time.Duration, cond func() bool) {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	timer := time.NewTimer(timeout)
	defer ticker.Stop()
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			t.Fatalf("wait [%s] timeout", desc)
		case <-ticker.C:
			if cond() {
				return
			}
		}
	}
}

// drainConsumedMessages 批量阻塞收取N条消费消息，超时失败
func drainConsumedMessages(t *testing.T, ch <-chan *ConsumedMessage, expectCnt int, timeout time.Duration) []*ConsumedMessage {
	t.Helper()
	res := make([]*ConsumedMessage, 0, expectCnt)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for len(res) < expectCnt {
		select {
		case msg := <-ch:
			require.NotNil(t, msg, "consumed message must not be nil")
			res = append(res, msg)
		case <-timer.C:
			t.Fatalf("consume wait timeout, want=%d got=%d", expectCnt, len(res))
		}
	}
	return res
}
