// Package bootsrtrap
// 从 yaml 配置文件中读取配置文件信息
package bootstrap

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 项目的最终配置信息
type Config struct {
	// 日志配置
	Logger LoggerConfig

	// 数据库配置
	MySQL MySQLConfig

	// Redis 配置
	Redis RedisConfig
}

func (c *Config) setDefault() {
	c.Logger.setDefault() // 日志
	c.MySQL.setDefault()  // MySQL
	c.Redis.setDefault()  // Redis
}

// InitConfig 初始化配置信息
func InitConfig(file string) *Config {
	var cfg Config
	if file == "" {
		// 错误的路径
		fmt.Println("Invalid configuration file path - empty")
		return nil
	}
	// 读取配置配置文件的数据
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Configuration file does not exist", fmt.Sprintf("path: %s", file))
		}
		return nil
	}
	// 读取成功，解析数据
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Println("Failed to parse YAML file", fmt.Sprintf("path: %s", file))
		return nil
	}
	// 解析成功
	(&cfg).setDefault() // 合并默认值
	return &cfg
}
