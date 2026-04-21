package main

import (
	"flag"
	"fmt"
	"smart-task-platform/internal/bootstrap"
	"time"

	"go.uber.org/zap"
)

var (
	configFile = flag.String("c", "", "Config file storage path")
	port       = flag.String("port", "8080", "Application port")
	host       = flag.String("host", "127.0.0.1", "Application host")
)

func main() {
	flag.Parse()
	// 如果读取配置文件出现问题，那么就直接退出程序
	cfg := bootstrap.InitConfig(*configFile)
	if cfg == nil {
		fmt.Println("Failed to initialize configuration")
		return
	}

	bootstrap.InitLogger(&cfg.Logger)
	bootstrap.InitDB(cfg.Logger.Mode, &cfg.MySQL)
	bootstrap.InitRedis(&cfg.Redis)
	zap.L().Debug("初始化成功")
	time.Sleep(10 * time.Second)
}
