// Package bootstrap
// 服务器启动相关的配置信息

package bootstrap

type ServerConfig struct {
	// 运行模式，dev/release
	Mode string `yaml:"mode"`

	// 服务器主机地址
	Host string `yaml:"host"`

	// 服务器端口
	Port string `yaml:"port"`
}

func (c *ServerConfig) setDefault() {
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == "" {
		c.Port = "8080"
	}
}
