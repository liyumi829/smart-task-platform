// Package logger
// 全局日志器的配置文件信息：日志器的运行模式、日志的存储路径、文件名称
package logger

// 实例配置
type LoggerConfig struct {
	// 运行模式 prod/debug 默认运行模式 debug
	Mode string

	// 存储文件的名称
	StorageName string

	// 存储路径 如果是debug 不填即可，如果不填的prod模式，默认在当前工作路径下
	StoragePath string
}
