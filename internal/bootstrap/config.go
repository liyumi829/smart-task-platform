// Package bootsrtrap
// 从 yaml 配置文件中读取配置文件信息
package bootstrap

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	// 运行模式
	ModeDev     = "dev"
	ModeRelease = "release"
)

// Config 项目的最终配置信息
type Config struct {
	// 服务器配置
	Server ServerConfig `yaml:"server"`

	// 日志配置
	Logger LoggerConfig `yaml:"log"`

	// 数据库配置
	MySQL MySQLConfig `yaml:"mysql"`

	// Redis 配置
	Redis RedisConfig `yaml:"redis"`

	// JWT 配置
	JWT JWTConfig `yaml:"jwt"`

	// RabbitMQ 配置
	RabbitMQ RabbitMQConfig `yaml:"rabbitmq"`

	// OutBoxManager 配置
	OutboxWorkerManager OutboxWorkerManagerConfig `yaml:"outbox_worker_manager"`

	// HandleWorkerManager 配置
	HandleWorkerManager HandleWorkerManagerConfig `yaml:"handle_worker_manager"`
}

func (c *Config) setDefault() {
	c.Server.setDefault()              // 服务器
	c.Logger.setDefault()              // 日志
	c.MySQL.setDefault()               // MySQL
	c.Redis.setDefault()               // Redis
	c.JWT.setDefault()                 // JWT
	c.RabbitMQ.setDefault()            // RabbitMQ
	c.OutboxWorkerManager.setDefault() // OutboxWorkerManager
	c.HandleWorkerManager.setDefault() // HandleWorkerManager

	// 统一配置
	c.HandleWorkerManager.RetryExchangeName = c.RabbitMQ.RetryExchangeName
	c.HandleWorkerManager.RetryRoutingKey = c.RabbitMQ.RetryRoutingKey
}

func InitConfig(file string) (*Config, error) {
	var cfg Config
	if file == "" {
		return nil, fmt.Errorf("empty config file path")
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 解析成功
	cfg.setDefault() // 合并默认配置
	return &cfg, nil
}
