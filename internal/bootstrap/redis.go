// Package bootstrap
// Redis 的配置信息以及初始化
package bootstrap

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisConfig Redis 配置
type RedisConfig struct {
	// Redis 地址（集群支持多个）
	Addrs []string `yaml:"addrs"`

	// 密码
	Password string `yaml:"password"`

	// 数据库编号（集群只能用0）
	DB int `yaml:"db"`

	// 最大连接数
	PoolSize int `yaml:"pool_size"`

	// 最小空闲连接数
	MinIdleConns int `yaml:"min_idle_conns"`
}

func InitRedis(c *RedisConfig) redis.UniversalClient {
	c.setDefault()

	// 创建 Redis 客户端配置
	opt := &redis.UniversalOptions{
		Addrs:        c.Addrs,
		Password:     c.Password,
		DB:           c.DB,
		PoolSize:     c.PoolSize,
		MinIdleConns: c.MinIdleConns,
	}

	// 初始化客户端
	client := redis.NewUniversalClient(opt)

	// 使用 Ping 检查 Redis 连接是否真正成功
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 执行 Ping 命令检测连接
	_, err := client.Ping(ctx).Result()
	if err != nil {
		// 连接失败：打印错误日志
		zap.L().Fatal("Redis 客户端连接失败",
			zap.Strings("addrs", c.Addrs),
			zap.Int("db", c.DB),
			zap.Error(err),
		)
	}

	// 连接成功：打印成功日志
	zap.L().Info("Redis 客户端初始化成功",
		zap.Strings("addrs", c.Addrs),
		zap.Int("db", c.DB),
		zap.Int("pool_size", c.PoolSize),
	)

	return client
}

// setDefault 设置 Redis 配置默认值
func (c *RedisConfig) setDefault() {
	// 地址默认本地单节点
	if len(c.Addrs) == 0 {
		c.Addrs = []string{
			"127.0.0.1:6379",
		}
	}
	// 数据库默认 0
	if c.DB <= 0 {
		c.DB = 0
	}
	// 最大连接数
	if c.PoolSize <= 0 {
		c.PoolSize = 100
	}
	// 最小空闲连接
	if c.MinIdleConns <= 0 {
		c.MinIdleConns = 10
	}
}
