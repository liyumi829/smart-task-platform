// internal/api/handler/forwarding_handler.go
// package handler
// 实现升级 http 协议为 websocket 协议

package handler

import (
	"net/http"
	"smart-task-platform/internal/api/contextx"

	"github.com/gin-gonic/gin"
	gws "github.com/gorilla/websocket"
)

// 管理 websocket 连接
type wesocketManager interface {
	HandleWebSocket(userID uint64, sessionID string, conn *gws.Conn)
}

const (
	// DefaultWebSocketReadBufferSize 默认 websocket 读缓冲大小。
	DefaultWebSocketReadBufferSize = 1024

	// DefaultWebSocketWriteBufferSize 默认 websocket 写缓冲大小。
	DefaultWebSocketWriteBufferSize = 1024
)

// WebSocketHandler 从 http 协议升级到 websocket 协议处理器
type WebSocketHandler struct {
	forwardingService wesocketManager
	upgrader          gws.Upgrader // websocket 升级器
}

// NewWebsocketHandler 实例化一个处理器
func NewWebsocketHandler(forwardingService wesocketManager) *WebSocketHandler {
	return &WebSocketHandler{
		forwardingService: forwardingService,
		upgrader: gws.Upgrader{
			ReadBufferSize:  DefaultWebSocketReadBufferSize,
			WriteBufferSize: DefaultWebSocketWriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				// 默认放行，生产环境建议通过配置限制可信域名。
				return true
			},
		},
	}
}

// Upgrade 对用户的 http 协议进行升级为 websocket 协议
// GET /ws
func (h *WebSocketHandler) Upgrade(c *gin.Context) {
	// 不携带参数，之前已经对 c 进行了 jwt 鉴权和 redis 会话验证
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil) // 将http连接升级为 websocket 连接
	if err != nil {
		return
	}
	userID := contextx.GetUserID(c)
	sessionID := contextx.GetSessionID(c)
	h.forwardingService.HandleWebSocket(userID, sessionID, conn)
}
