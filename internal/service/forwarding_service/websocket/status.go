// internal/service/forwarding_service/websocket/status.go
// package websocket
// 定义 websocket 登录状态
package websocket

// ClientStatus 表示 WebSocket 连接状态
type ClientStatus int32

const (
	// ClientStatusConnecting 表示连接正在建立。
	ClientStatusConnecting ClientStatus = iota + 1

	// ClientStatusOnline 表示连接在线可用。
	ClientStatusOnline

	// ClientStatusClosing 表示连接正在关闭。
	ClientStatusClosing

	// ClientStatusClosed 表示连接已经关闭。
	ClientStatusClosed
)

// String 返回当前状态的 string
func (s ClientStatus) String() string {
	switch s {
	case ClientStatusConnecting:
		return "connecting"
	case ClientStatusOnline:
		return "online"
	case ClientStatusClosing:
		return "closing"
	case ClientStatusClosed:
		return "closed"
	default:
		return "unknown"
	}
}
