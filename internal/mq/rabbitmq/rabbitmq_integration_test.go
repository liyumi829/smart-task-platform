// internal/mq/rabbitmq/rabbitmq_integration_test.go
// package rabbitmq
// 功能：RabbitMQ 集成测试，用于验证连接、拓扑声明、发布、订阅、ACK、NACK、死信、Publisher 自动重连、Subscriber 自动重连。

package rabbitmq

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"smart-task-platform/internal/bootstrap"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// newTestRabbitMQConfig 创建测试用 RabbitMQ 配置。
func newTestRabbitMQConfig(t *testing.T) bootstrap.RabbitMQConfig {
	t.Helper()

	suffix := fmt.Sprintf("test.%d", time.Now().UnixNano())

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

		ExchangeName: fmt.Sprintf("smart-task-platform.notification.exchange.%s", suffix),
		ExchangeType: "direct",
		RoutingKey:   fmt.Sprintf("smart-task-platform.notification.routing.%s", suffix),
		QueueName:    fmt.Sprintf("smart-task-platform.notification.queue.%s", suffix),

		RetryExchangeName: fmt.Sprintf("smart-task-platform.notification.retry.exchange.%s", suffix),
		RetryRoutingKey:   fmt.Sprintf("smart-task-platform.notification.retry.routing.%s", suffix),
		RetryQueueName:    fmt.Sprintf("smart-task-platform.notification.retry.queue.%s", suffix),
		RetryDelay:        500 * time.Millisecond,

		DeadLetterExchangeName: fmt.Sprintf("smart-task-platform.notification.dlx.%s", suffix),
		DeadLetterRoutingKey:   fmt.Sprintf("smart-task-platform.notification.dead.routing.%s", suffix),
		DeadLetterQueueName:    fmt.Sprintf("smart-task-platform.notification.dlq.%s", suffix),

		PublishTimeout: 5 * time.Second,

		ConsumerTag:           fmt.Sprintf("smart-task-platform-test-consumer-%s", suffix),
		PrefetchCount:         4,
		SubscribeCloseTimeout: 5 * time.Second,
	}
}

// setupTestLogger 初始化测试日志，并允许通过 zap.L() 打印日志。
func setupTestLogger(t *testing.T) *zap.Logger {
	t.Helper()

	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	zap.ReplaceGlobals(logger)

	return zap.L()
}

// requireRabbitMQ 确保 RabbitMQ 可连接，不可连接时跳过测试。
func requireRabbitMQ(t *testing.T, config bootstrap.RabbitMQConfig) {
	t.Helper()

	conn, err := amqp.Dial(config.URL())
	if err != nil {
		t.Skipf("RabbitMQ is not available, skip integration test: %v", err)
		return
	}

	_ = conn.Close()
}

// openTestRabbitMQConnection 创建测试 RabbitMQ 连接。
func openTestRabbitMQConnection(t *testing.T, config bootstrap.RabbitMQConfig) *amqp.Connection {
	t.Helper()

	conn, err := amqp.Dial(config.URL())
	if err != nil {
		t.Fatalf("RabbitMQ dial failed: %v", err)
	}

	return conn
}

// cleanupRabbitMQTopology 清理测试创建的队列和交换机。
func cleanupRabbitMQTopology(t *testing.T, config bootstrap.RabbitMQConfig) {
	t.Helper()

	conn, err := amqp.Dial(config.URL())
	if err != nil {
		t.Logf("RabbitMQ cleanup dial failed: %v", err)
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	ch, err := conn.Channel()
	if err != nil {
		t.Logf("RabbitMQ cleanup channel create failed: %v", err)
		return
	}
	defer func() {
		_ = ch.Close()
	}()

	// 先删除队列，再删除交换机。
	_, _ = ch.QueueDelete(config.QueueName, false, false, false)
	_, _ = ch.QueueDelete(config.RetryQueueName, false, false, false)
	_, _ = ch.QueueDelete(config.DeadLetterQueueName, false, false, false)

	_ = ch.ExchangeDelete(config.ExchangeName, false, false)
	_ = ch.ExchangeDelete(config.RetryExchangeName, false, false)
	_ = ch.ExchangeDelete(config.DeadLetterExchangeName, false, false)
}

// waitMessage 等待订阅消息。
func waitMessage(ctx context.Context, messages <-chan *ConsumedMessage) (*ConsumedMessage, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case message, ok := <-messages:
			if !ok {
				return nil, fmt.Errorf("message channel closed")
			}

			return message, nil
		}
	}
}

// waitDeadLetterMessage 等待死信队列消息。
func waitDeadLetterMessage(ctx context.Context, config bootstrap.RabbitMQConfig) (*amqp.Delivery, error) {
	conn, err := amqp.Dial(config.URL())
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = conn.Close()
	}()

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = ch.Close()
	}()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-ticker.C:
			delivery, ok, err := ch.Get(config.DeadLetterQueueName, false)
			if err != nil {
				return nil, err
			}

			if ok {
				_ = delivery.Ack(false)
				return &delivery, nil
			}
		}
	}
}

// newTestPublisher 创建测试 Publisher。
func newTestPublisher(t *testing.T, config bootstrap.RabbitMQConfig, conn *amqp.Connection) *Publisher {
	t.Helper()

	publisher, err := NewPublisher(config, zap.L(), conn)
	if err != nil {
		t.Fatalf("RabbitMQ publisher create failed: %v", err)
	}

	return publisher
}

// newTestSubscriber 创建测试 Subscriber。
func newTestSubscriber(t *testing.T, config bootstrap.RabbitMQConfig, conn *amqp.Connection) *Subscriber {
	t.Helper()

	subscriber, err := NewSubscriber(config, zap.L(), conn)
	if err != nil {
		t.Fatalf("RabbitMQ subscriber create failed: %v", err)
	}

	return subscriber
}

// TestRabbitMQPublisherPublishAndGet 测试 Publisher 发布消息后，可以直接从队列拉取到消息。
func TestRabbitMQPublisherPublishAndGet(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-publisher-get-001",
		EventID:   "test-event-publisher-get-001",
		EventType: "notification.created",
		Payload:   []byte(`{"user_id":1,"content":"publisher get test"}`),
	})
	if err != nil {
		t.Fatalf("RabbitMQ publish failed: %v", err)
	}

	conn := openTestRabbitMQConnection(t, config)
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

	delivery, ok, err := ch.Get(config.QueueName, false)
	if err != nil {
		t.Fatalf("RabbitMQ queue get failed: %v", err)
	}
	if !ok {
		t.Fatal("expected message in queue, got empty")
	}
	defer func() {
		_ = delivery.Ack(false)
	}()

	if delivery.MessageId != "test-publisher-get-001" {
		t.Fatalf("unexpected message_id, got=%s", delivery.MessageId)
	}

	zap.L().Info("✅ RabbitMQ publisher publish and get test passed",
		zap.String("message_id", delivery.MessageId),
	)
}

// TestRabbitMQSubscribeAck 测试 Subscribe 返回消费数据，业务层手动 ACK。
func TestRabbitMQSubscribeAck(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	subscriber := newTestSubscriber(t, config, nil)
	defer func() {
		_ = subscriber.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	messages, err := subscriber.Subscribe(ctx)
	if err != nil {
		t.Fatalf("RabbitMQ subscribe failed: %v", err)
	}

	err = publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-subscribe-ack-001",
		EventID:   "test-event-subscribe-ack-001",
		EventType: "notification.subscribe.ack",
		Payload:   []byte(`{"user_id":2,"content":"subscribe ack test"}`),
	})
	if err != nil {
		t.Fatalf("RabbitMQ publish failed: %v", err)
	}

	message, err := waitMessage(ctx, messages)
	if err != nil {
		t.Fatalf("RabbitMQ wait message failed: %v", err)
	}

	if message.MessageID != "test-subscribe-ack-001" {
		t.Fatalf("unexpected message_id, got=%s", message.MessageID)
	}

	if err := message.Ack(); err != nil {
		t.Fatalf("RabbitMQ message ack failed: %v", err)
	}

	zap.L().Info("✅ RabbitMQ subscribe ack test passed",
		zap.String("message_id", message.MessageID),
		zap.String("event_id", message.EventID),
	)
}

// TestRabbitMQSubscribeNackRequeue 测试业务层 Nack(true) 后消息重新回到队列并再次被消费。
func TestRabbitMQSubscribeNackRequeue(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	config.PrefetchCount = 1

	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	subscriber := newTestSubscriber(t, config, nil)
	defer func() {
		_ = subscriber.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages, err := subscriber.Subscribe(ctx)
	if err != nil {
		t.Fatalf("RabbitMQ subscribe failed: %v", err)
	}

	err = publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-subscribe-nack-requeue-001",
		EventID:   "test-event-subscribe-nack-requeue-001",
		EventType: "notification.subscribe.nack.requeue",
		Payload:   []byte(`{"user_id":3,"content":"nack requeue test"}`),
	})
	if err != nil {
		t.Fatalf("RabbitMQ publish failed: %v", err)
	}

	firstMessage, err := waitMessage(ctx, messages)
	if err != nil {
		t.Fatalf("RabbitMQ wait first message failed: %v", err)
	}

	if firstMessage.MessageID != "test-subscribe-nack-requeue-001" {
		t.Fatalf("unexpected first message_id, got=%s", firstMessage.MessageID)
	}

	if err := firstMessage.Nack(true); err != nil {
		t.Fatalf("RabbitMQ first message nack requeue failed: %v", err)
	}

	secondMessage, err := waitMessage(ctx, messages)
	if err != nil {
		t.Fatalf("RabbitMQ wait second message failed: %v", err)
	}

	if secondMessage.MessageID != "test-subscribe-nack-requeue-001" {
		t.Fatalf("unexpected second message_id, got=%s", secondMessage.MessageID)
	}

	if err := secondMessage.Ack(); err != nil {
		t.Fatalf("RabbitMQ second message ack failed: %v", err)
	}

	zap.L().Info("✅ RabbitMQ subscribe nack requeue test passed",
		zap.String("message_id", secondMessage.MessageID),
	)
}

// TestRabbitMQSubscribeNackToDeadLetter 测试业务层 Nack(false) 后消息进入死信队列。
func TestRabbitMQSubscribeNackToDeadLetter(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	subscriber := newTestSubscriber(t, config, nil)
	defer func() {
		_ = subscriber.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages, err := subscriber.Subscribe(ctx)
	if err != nil {
		t.Fatalf("RabbitMQ subscribe failed: %v", err)
	}

	err = publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-subscribe-dead-letter-001",
		EventID:   "test-event-subscribe-dead-letter-001",
		EventType: "notification.subscribe.dead",
		Payload:   []byte(`{"user_id":4,"content":"dead letter test"}`),
	})
	if err != nil {
		t.Fatalf("RabbitMQ publish failed: %v", err)
	}

	message, err := waitMessage(ctx, messages)
	if err != nil {
		t.Fatalf("RabbitMQ wait message failed: %v", err)
	}

	if message.MessageID != "test-subscribe-dead-letter-001" {
		t.Fatalf("unexpected message_id, got=%s", message.MessageID)
	}

	if err := message.Nack(false); err != nil {
		t.Fatalf("RabbitMQ message nack dead letter failed: %v", err)
	}

	deadMessage, err := waitDeadLetterMessage(ctx, config)
	if err != nil {
		t.Fatalf("RabbitMQ wait dead letter message failed: %v", err)
	}

	if deadMessage.MessageId != "test-subscribe-dead-letter-001" {
		t.Fatalf("unexpected dead letter message_id, got=%s", deadMessage.MessageId)
	}

	zap.L().Info("✅ RabbitMQ subscribe nack to dead letter test passed",
		zap.String("message_id", deadMessage.MessageId),
	)
}

// TestRabbitMQPublisherMandatoryReturn 测试 mandatory=true 时，不可路由消息返回错误。
func TestRabbitMQPublisherMandatoryReturn(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := publisher.Publish(ctx, &PublishMessage{
		MessageID:  "test-publisher-return-001",
		EventID:    "test-event-publisher-return-001",
		EventType:  "notification.publisher.return",
		Exchange:   config.ExchangeName,
		RoutingKey: fmt.Sprintf("%s.unroutable", config.RoutingKey),
		Payload:    []byte(`{"user_id":5,"content":"mandatory return test"}`),
	})
	if err == nil {
		t.Fatal("expected mandatory return error, got nil")
	}

	if !errorsIsPublishReturned(err) {
		t.Fatalf("expected ErrRabbitMQPublishReturned, got=%v", err)
	}

	zap.L().Info("✅ RabbitMQ publisher mandatory return test passed",
		zap.Error(err),
	)
}

// errorsIsPublishReturned 判断是否为发布不可路由错误。
func errorsIsPublishReturned(err error) bool {
	return err == ErrRabbitMQPublishReturned
}

// TestRabbitMQPublisherAutoReconnectByClosingChannel 测试 Publisher channel 被关闭后自动重建并继续发布。
func TestRabbitMQPublisherAutoReconnectByClosingChannel(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	publisher.mu.Lock()
	if publisher.ch != nil {
		_ = publisher.ch.Close()
	}
	publisher.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-publisher-channel-reconnect-001",
		EventID:   "test-event-publisher-channel-reconnect-001",
		EventType: "notification.publisher.channel.reconnect",
		Payload:   []byte(`{"user_id":6,"content":"publisher channel reconnect test"}`),
	})
	if err != nil {
		t.Fatalf("RabbitMQ publish after channel close failed: %v", err)
	}

	zap.L().Info("✅ RabbitMQ publisher channel auto reconnect test passed")
}

// TestRabbitMQPublisherAutoReconnectByClosingConnection 测试 Publisher connection 被关闭后自动重连并继续发布。
func TestRabbitMQPublisherAutoReconnectByClosingConnection(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	publisher.mu.Lock()
	if publisher.conn != nil {
		_ = publisher.conn.Close()
	}
	publisher.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-publisher-connection-reconnect-001",
		EventID:   "test-event-publisher-connection-reconnect-001",
		EventType: "notification.publisher.connection.reconnect",
		Payload:   []byte(`{"user_id":7,"content":"publisher connection reconnect test"}`),
	})
	if err != nil {
		t.Fatalf("RabbitMQ publish after connection close failed: %v", err)
	}

	zap.L().Info("✅ RabbitMQ publisher connection auto reconnect test passed")
}

// TestRabbitMQSubscriberAutoReconnectByClosingChannel 测试 Subscriber channel 关闭后自动重建并继续消费。
func TestRabbitMQSubscriberAutoReconnectByClosingChannel(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	subscriber := newTestSubscriber(t, config, nil)
	defer func() {
		_ = subscriber.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages, err := subscriber.Subscribe(ctx)
	if err != nil {
		t.Fatalf("RabbitMQ subscribe failed: %v", err)
	}

	subscriber.mu.Lock()
	if subscriber.ch != nil {
		_ = subscriber.ch.Close()
	}
	subscriber.mu.Unlock()

	time.Sleep(800 * time.Millisecond)

	err = publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-subscriber-channel-reconnect-001",
		EventID:   "test-event-subscriber-channel-reconnect-001",
		EventType: "notification.subscriber.channel.reconnect",
		Payload:   []byte(`{"user_id":8,"content":"subscriber channel reconnect test"}`),
	})
	if err != nil {
		t.Fatalf("RabbitMQ publish failed: %v", err)
	}

	message, err := waitMessage(ctx, messages)
	if err != nil {
		t.Fatalf("RabbitMQ wait message after subscriber channel reconnect failed: %v", err)
	}

	if message.MessageID != "test-subscriber-channel-reconnect-001" {
		t.Fatalf("unexpected message_id, got=%s", message.MessageID)
	}

	if err := message.Ack(); err != nil {
		t.Fatalf("RabbitMQ message ack failed: %v", err)
	}

	zap.L().Info("✅ RabbitMQ subscriber channel auto reconnect test passed",
		zap.String("message_id", message.MessageID),
	)
}

// TestRabbitMQMultipleMessagesSubscribeAck 测试连续发布多条消息并全部消费 ACK。
func TestRabbitMQMultipleMessagesSubscribeAck(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	config.PrefetchCount = 5

	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	defer func() {
		_ = publisher.Close()
	}()

	subscriber := newTestSubscriber(t, config, nil)
	defer func() {
		_ = subscriber.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	messages, err := subscriber.Subscribe(ctx)
	if err != nil {
		t.Fatalf("RabbitMQ subscribe failed: %v", err)
	}

	const total = 10

	for i := 0; i < total; i++ {
		err := publisher.Publish(ctx, &PublishMessage{
			MessageID: fmt.Sprintf("test-multiple-message-%03d", i),
			EventID:   fmt.Sprintf("test-multiple-event-%03d", i),
			EventType: "notification.multiple.ack",
			Payload:   []byte(fmt.Sprintf(`{"index":%d,"content":"multiple ack test"}`, i)),
		})
		if err != nil {
			t.Fatalf("RabbitMQ publish multiple message failed: %v", err)
		}
	}

	var consumed atomic.Int32

	for consumed.Load() < total {
		message, err := waitMessage(ctx, messages)
		if err != nil {
			t.Fatalf("RabbitMQ wait multiple message failed: %v", err)
		}

		if err := message.Ack(); err != nil {
			t.Fatalf("RabbitMQ multiple message ack failed: %v", err)
		}

		consumed.Add(1)
	}

	zap.L().Info("✅ RabbitMQ multiple messages subscribe ack test passed",
		zap.Int32("consumed", consumed.Load()),
	)
}

// TestRabbitMQCloseBehavior 测试 Publisher / Subscriber 关闭后的行为。
func TestRabbitMQCloseBehavior(t *testing.T) {
	setupTestLogger(t)

	config := newTestRabbitMQConfig(t)
	requireRabbitMQ(t, config)

	t.Cleanup(func() {
		cleanupRabbitMQTopology(t, config)
	})

	publisher := newTestPublisher(t, config, nil)
	subscriber := newTestSubscriber(t, config, nil)

	if err := publisher.Close(); err != nil {
		t.Fatalf("RabbitMQ publisher close failed: %v", err)
	}

	if err := subscriber.Close(); err != nil {
		t.Fatalf("RabbitMQ subscriber close failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	publishErr := publisher.Publish(ctx, &PublishMessage{
		MessageID: "test-close-behavior-001",
		EventID:   "test-event-close-behavior-001",
		EventType: "notification.close.behavior",
		Payload:   []byte(`{"content":"close behavior test"}`),
	})
	if publishErr == nil {
		t.Fatal("expected publisher closed error, got nil")
	}

	if !IsClosedError(publishErr) {
		t.Fatalf("expected publisher closed error, got=%v", publishErr)
	}

	_, subscribeErr := subscriber.Subscribe(ctx)
	if subscribeErr == nil {
		t.Fatal("expected subscriber closed error, got nil")
	}

	zap.L().Info("✅ RabbitMQ close behavior test passed",
		zap.Error(publishErr),
		zap.Error(subscribeErr),
	)
}
