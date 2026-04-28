package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"smart-task-platform/internal/api/handler"
	"smart-task-platform/internal/api/router"
	"smart-task-platform/internal/bootstrap"
	redispkg "smart-task-platform/internal/pkg/redis"
	"smart-task-platform/internal/repository"
	"smart-task-platform/internal/service"

	"github.com/gin-gonic/gin"
)

var (
	configFile = flag.String("c", "", "Config file storage path")
	port       = flag.String("port", "8080", "Application port")
	host       = flag.String("host", "127.0.0.1", "Application host")
)

func main() {
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
	bootstrap.InitLogger(&cfg.Logger)                   // 初始化全局日志器
	db := bootstrap.InitDB(cfg.Server.Mode, &cfg.MySQL) // MySQL 数据库连接
	redis := bootstrap.InitRedis(&cfg.Redis)            // Redis 连接
	jwtManager := bootstrap.InitJWT(&cfg.JWT)           // JWT 管理器
	authStore := redispkg.NewRedisAuthStore(redis)      // 认证存储管理器
	// 自动迁移 数据库表结构
	if err := bootstrap.AutoMigrate(db); err != nil {
		log.Fatalf("auto migrate failed: %v", err)
	}

	// 事务管理器、初始化仓储
	txManager := repository.NewTxManager(db)                       // 事务管理器
	userRepo := repository.NewUserRepository(db)                   // 用户表
	projectRepo := repository.NewProjectRepository(db)             // 项目表
	projectMemberRepo := repository.NewProjectMemberRepository(db) // 项目成员表

	// 初始化服务
	authService := service.NewAuthService(txManager, userRepo, authStore, jwtManager)                            // 鉴权服务
	userService := service.NewUserService(txManager, userRepo)                                                   // 用户服务
	projectService := service.NewProjectService(txManager, userRepo, projectRepo, projectMemberRepo)             // 项目服务
	projectMemberService := service.NewProjectMemberService(txManager, userRepo, projectRepo, projectMemberRepo) // 项目成员服务

	// 初始化 Handler
	authHandler := handler.NewAuthHandler(authService)                            // 鉴权 Handler
	userHandler := handler.NewUserHandler(userService)                            // 用户 Handler
	projectHandler := handler.NewProjectHandler(projectService)                   // 项目 Handler
	projectMemberHandler := handler.NewProjectMemberHandler(projectMemberService) // 项目成员 Handler
	// 初始化 Gin
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	// 选择日志模式：
	switch cfg.Server.Mode {
	case bootstrap.ModeDev:
		gin.SetMode(gin.DebugMode)
	case bootstrap.ModeRelease:
		gin.SetMode(gin.ReleaseMode)
	default:
		log.Printf("unknown logger mode: %s, defaulting to debug", cfg.Logger.Mode)
		gin.SetMode(gin.DebugMode)
	}

	// 注册路由
	router.Register(r, jwtManager, authStore, authHandler, userHandler, projectHandler, projectMemberHandler)

	// 启动服务
	addr := net.JoinHostPort(*host, *port)
	if err := r.Run(addr); err != nil {
		log.Fatalf("server start failed: %v", err)
	}
}
