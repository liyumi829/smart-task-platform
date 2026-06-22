package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/router"
	"smart-task-platform/internal/bootstrap"
	"smart-task-platform/internal/cache"
	mq "smart-task-platform/internal/mq/rabbitmq"
	redispkg "smart-task-platform/internal/pkg/redis"
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service"
	"smart-task-platform/internal/service/cachesvc"
	"smart-task-platform/internal/service/forwarding_service/forwarding"
	"smart-task-platform/internal/service/forwarding_service/websocket"
	"smart-task-platform/internal/service/forwarding_service/worker"

	"net/http"
	_ "net/http/pprof"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var (
	configFile = flag.String("c", "", "Config file storage path")
	port       = flag.String("port", "8080", "Application port")
	host       = flag.String("host", "127.0.0.1", "Application host")
)

func main() {
	// pprof 只监听本机，避免暴露到公网。
	go func() {
		log.Println("pprof listening on 127.0.0.1:6060")
		_ = http.ListenAndServe("127.0.0.1:6060", nil)
	}()
	flag.Parse()
	// 如果读取配置文件出现问题，那么就直接退出程序
	log.Printf("config file: %s", *configFile)
	cfg, err := bootstrap.InitConfig(*configFile)
	if err != nil {
		fmt.Printf("Failed to initialize configuration, error: %v", err)
		return
	}

	// 命令行参数优先于配置文件
	if *host != "127.0.0.1" {
		cfg.Server.Host = *host
	}
	if *port != "8080" {
		cfg.Server.Port = *port
	}

	// 统一运行模式
	if cfg.Server.Mode != "" {
		cfg.Logger.Mode = cfg.Server.Mode
	}

	// 初始化日志、数据库 等组件以及中间件
	bootstrap.InitLogger(&cfg.Logger)                     // 初始化全局日志器
	db := bootstrap.InitDB(cfg.Server.Mode, &cfg.MySQL)   // MySQL 数据库连接
	redis := bootstrap.InitRedis(&cfg.Redis)              // Redis 连接
	jwtManager := bootstrap.InitJWT(&cfg.JWT)             // JWT 管理器
	authStore := redispkg.NewRedisAuthStore(redis)        // 认证存储管理器
	RabbitMQConn := bootstrap.InitRabbitMQ(&cfg.RabbitMQ) // RabbitMQ 连接, 仅仅用于声明
	RabbitMQConn.Close()                                  // 连接声明后立即关闭，生产者/消费者会在各自的工厂函数里重新建立连接

	// 事务管理器、初始化仓储
	txManager := repository.NewTxManager(db)                       // 事务管理器
	userRepo := repository.NewUserRepository(db)                   // 用户表仓储
	projectRepo := repository.NewProjectRepository(db)             // 项目表仓储
	projectMemberRepo := repository.NewProjectMemberRepository(db) // 项目成员表仓储
	taskRepo := repository.NewTaskRepository(db)                   // 任务表仓储
	taskCommentRepo := repository.NewTaskCommentRepository(db)     // 任务评论表仓储
	taskActivityRepo := repository.NewTaskActivityRepository(db)   // 任务活动表仓储
	notificationRepo := repository.NewNotificationRepository(db)   // 通知表仓储
	outboxRepo := repository.NewOutboxMessageRepository(db)        // 消息仓储

	// 缓存服务
	cacheStore := cache.NewRedisCacheStore(redis)
	cacheService := cachesvc.NewCacheService(cacheStore, userRepo, projectRepo, projectMemberRepo, taskRepo, nil)

	// 初始化服务
	authService := service.NewAuthService(txManager, userRepo, authStore, jwtManager)                                                    // 鉴权服务
	userService := service.NewUserService(txManager, userRepo, cacheService)                                                             // 用户服务
	projectService := service.NewProjectService(txManager, projectRepo, projectMemberRepo, cacheService)                                 // 项目服务
	projectMemberService := service.NewProjectMemberService(txManager, projectRepo, projectMemberRepo, taskRepo, cacheService)           // 项目成员服务
	taskActivityService := service.NewTaskActivityService(taskActivityRepo, cacheService)                                                // 任务
	notificationService := service.NewNotificationService(txManager, notificationRepo, outboxRepo, nil)                                  // 通知服务
	taskService := service.NewTaskService(txManager, taskRepo, taskActivityRepo, cacheService, notificationService)                      // 任务服务
	taskCommentService := service.NewTaskCommentService(txManager, taskCommentRepo, taskActivityRepo, cacheService, notificationService) // 任务评论服务
	// 初始化转发服务生产者/消费者
	outboxWorkerManager := worker.NewOutboxWorkerManager(
		cfg.OutboxWorkerManager, txManager, outboxRepo, worker.PublisherFactoryFunc(func(ctx context.Context, workerID string, workerIndex int) (worker.CloseablePublisher, error) {
			return mq.NewPublisher(cfg.RabbitMQ, zap.L(), nil)
		}), zap.L())
	handleWorkerManager := worker.NewHandleWorkerManager(cfg.HandleWorkerManager,
		worker.PublisherFactoryFunc(func(ctx context.Context, workerID string, workerIndex int) (worker.CloseablePublisher, error) {
			return mq.NewPublisher(cfg.RabbitMQ, zap.L(), nil)
		}),
		worker.SubscriberFactoryFunc(func(ctx context.Context) (worker.CloseableSubscriber, error) {
			return mq.NewSubscriber(cfg.RabbitMQ, zap.L(), nil)
		}), zap.L())
	// 初始化转发服务 websocket 连接管理
	websocketManager := websocket.NewManager(zap.L())
	// 初始化转发服务
	forwardingService, err := forwarding.NewNotificationForwardService(websocketManager, handleWorkerManager, outboxWorkerManager)
	if err != nil {
		log.Fatalf("Failed to initialize forwarding service: %v", err)
	}
	forwardingService.Start()

	// 初始化 Handler
	authHandler := handler.NewAuthHandler(authService)                            // 鉴权 Handler
	userHandler := handler.NewUserHandler(userService)                            // 用户 Handler
	projectHandler := handler.NewProjectHandler(projectService)                   // 项目 Handler
	projectMemberHandler := handler.NewProjectMemberHandler(projectMemberService) // 项目成员 Handler
	taskHandler := handler.NewTaskHandler(taskService)                            // 任务 Handler
	taskCommentHandler := handler.NewTaskCommentHandler(taskCommentService)       // 任务评论 Handler
	taskActivityHandler := handler.NewTaskActivityHandler(taskActivityService)    // 任务活动 Handler
	notificationHandler := handler.NewNotificationHandler(notificationService)    // 通知 Handler
	websocketHandler := handler.NewWebsocketHandler(forwardingService)            // 升级 Handler

	// 选择日志模式：
	switch cfg.Server.Mode {
	case bootstrap.ModeDev:
		gin.SetMode(gin.DebugMode)
		log.Printf("gin logger mode: %s\n", bootstrap.ModeDev)
	case bootstrap.ModeRelease:
		gin.SetMode(gin.ReleaseMode)
		log.Printf("gin logger mode: %s\n", bootstrap.ModeRelease)
	default:
		log.Printf("unknown logger mode: %s, defaulting to debug\n", cfg.Logger.Mode)
		gin.SetMode(gin.DebugMode)
	}
	// 初始化 Gin
	r := gin.New()
	r.Use(gin.Recovery())

	// 根据环境决定是否开启Gin访问日志
	if cfg.Server.Mode == bootstrap.ModeDev {
		r.Use(gin.Logger()) // 开发环境输出请求日志
	}

	// 注册路由
	router.Register(r, jwtManager, authStore,
		authHandler,
		userHandler,
		projectHandler,
		projectMemberHandler,
		taskHandler,
		taskCommentHandler,
		taskActivityHandler,
		notificationHandler,
		websocketHandler,
	)

	// 启动服务
	addr := net.JoinHostPort(*host, *port)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server start failed: %v", err)
	}
}
