// internal/service/forwarding_service/websocket/client.go
// package websocket
// 实现 websocket 客户端连接的定义，便于管理
package websocket

import (
	"smart-task-platform/internal/pkg/utils"
	"sync"
	"sync/atomic"
	"time"

	gws "github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	DefaultSendBufferSize = 128              // DefaultSendBufferSize 默认发送队列长度
	DefaultWriteWait      = 10 * time.Second // DefaultWriteWait 默认写超时时间
	DefaultPongWait       = 60 * time.Second // DefaultPongWait 默认 pong 等待时间
	DefaultPingPeriod     = 30 * time.Second // DefaultPingPeriod 默认 ping 周期
	DefaultReadLimit      = 1024 * 8         // DefaultReadLimit 默认最大读取大小
)

// Client 表示一个 WebSocket 连接
// 当前系统为单点登录，因此一个用户只维护一个 Client
type Client struct {
	userID       uint64       // 用户ID
	connID       string       // 连接ID
	sessionID    string       // 会话ID
	conn         *gws.Conn    // websocket 连接
	send         chan []byte  // 发送缓冲区通道
	connectedAt  time.Time    // 发生连接的时间
	lastActiveAt atomic.Int64 // 上次活跃时间戳
	status       atomic.Int32 // 登录状态
	closeOnce    sync.Once    // 关闭

	writeWait  time.Duration // 写数据超时时长
	pongWait   time.Duration // 等待pong超时时长
	pingPeriod time.Duration // ping 的时间间隔
	readLimit  int64         // 每次读取数据的大小

	logger *zap.Logger // 日志器
}

// NewClient 创建一个 WebSocket 客户端连接
func NewClient(userID uint64, sessionID string, conn *gws.Conn, sendBufferSize int, logger *zap.Logger) (*Client, error) {
	if userID == 0 {
		return nil, ErrInvalidUserID
	}

	if conn == nil {
		return nil, ErrNilConnection
	}

	if logger == nil {
		logger = zap.NewNop() // 空日志器
	}

	if sendBufferSize <= 0 {
		sendBufferSize = DefaultSendBufferSize
	}

	now := time.Now()

	connID := utils.Uuid()
	c := &Client{
		userID:      userID,
		connID:      connID,
		sessionID:   sessionID,
		conn:        conn,
		send:        make(chan []byte, sendBufferSize),
		connectedAt: now,
		writeWait:   DefaultWriteWait,
		pongWait:    DefaultPongWait,
		pingPeriod:  DefaultPingPeriod,
		readLimit:   DefaultReadLimit,
		logger: logger.With(
			zap.String("component", "websocket_client"),
			zap.Uint64("user_id", userID),
			zap.String("conn_id", connID),
			zap.String("session_id", sessionID),
		),
	}

	c.lastActiveAt.Store(now.UnixNano())
	c.status.Store(int32(ClientStatusConnecting))

	c.logger.Info("websocket client created successfully",
		zap.Time("connected_at", now),
		zap.Duration("ping_period", c.pingPeriod),
		zap.Duration("pong_wait", c.pongWait),
	)

	return c, nil
}

// 退出登录接口
type Unregister interface {

	// 注销登录的客户端 -- 等幂
	UnregisterByClient(client *Client)
}

// Start 启动 Client 的管理
//   - readPump
//   - writePump
func (c *Client) Start(manage Unregister) {
	go c.readPump(manage)
	go c.writePump(manage)
}

// readPump 持续读取客户端消息并处理 pong
// 当前阶段不处理业务消息，只负责保活与断线清理
func (c *Client) readPump(manager Unregister) {
	if c == nil || c.conn == nil || manager == nil {
		return
	}

	c.logger.Info("read pump started")
	defer func() {
		c.logger.Info("read pump exit, unregister client")
		manager.UnregisterByClient(c)
	}()

	c.conn.SetReadLimit(c.readLimit)                   // 设置每次读取的数据大小
	c.conn.SetReadDeadline(time.Now().Add(c.pongWait)) // 设置读取的超时时间
	c.conn.SetPongHandler(func(appData string) error { // 设置 pong 的处理函数
		c.RefreshActiveTime()                              // 刷新活跃时间
		c.conn.SetReadDeadline(time.Now().Add(c.pongWait)) // 设置下一次的pong的读取超时时间
		c.logger.Debug("received pong from client, refresh active time")
		return nil
	})

	// 阻塞等待连接事件，只要连接正常就一直活着；只要连接断开 / 出错，立刻退出循环，执行清理
	for {
		// 当前只消费连接层事件，不处理业务消息内容
		// 读取到数据没有发生错误说明是 客户端发送其它数据
		if _, _, err := c.conn.ReadMessage(); err != nil {
			c.logger.Warn("read message error, connection lost", zap.Error(err))
			return // 发生错误直接返回
		}
		c.RefreshActiveTime() // 刷新活跃时间 一般不会走到这里
		c.logger.Debug("received client message, refresh active time")
	}
}

// writePump 负责写业务消息和发送 ping 保活消息
func (c *Client) writePump(manager Unregister) {
	if c == nil || c.conn == nil || manager == nil {
		return
	}

	c.logger.Info("write pump started", zap.Duration("ping_period", c.pingPeriod))
	ticker := time.NewTicker(c.pingPeriod) // 每次发送 ping 包的时间间隔
	defer func() {                         // 清理资源
		ticker.Stop()
		c.logger.Info("write pump exit, unregister client")
		manager.UnregisterByClient(c)
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(c.writeWait)) // 设置写的超时时间
			if !ok {                                             // 如果缓冲区关闭了
				c.logger.Warn("send channel closed, send close frame to client")
				c.conn.WriteMessage(gws.CloseMessage, []byte{}) // 向客户端发送关闭连接的消息
				return
			}

			// 正常处理缓冲区数据
			// 发送文本信息
			if err := c.conn.WriteMessage(gws.TextMessage, message); err != nil {
				c.logger.Error("write message failed", zap.Error(err), zap.Int("size", len(message)))
				return
			}
			c.RefreshActiveTime() // 刷新活跃时间
			c.logger.Debug("message sent to client successfully", zap.Int("size", len(message)))

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(c.writeWait))
			if err := c.conn.WriteMessage(gws.PingMessage, nil); err != nil {
				c.logger.Error("send ping failed", zap.Error(err))
				return
			}
			c.logger.Debug("ping sent to client")
		}
	}
}

// Close 关闭连接
//
// 生命周期被外部的 Manager 管理
func (c *Client) Close() {
	if c == nil {
		return
	}

	c.closeOnce.Do(func() {
		c.logger.Info("websocket client closing...")

		c.status.Store(int32(ClientStatusClosing))
		close(c.send)  // 关闭缓冲区
		c.conn.Close() // 关闭连接
		c.status.Store(int32(ClientStatusClosed))

		c.logger.Info("websocket client closed successfully")
	})
}

// TrySend 尝试发送消息到缓冲区
func (c *Client) TrySend(message []byte) error {
	if c == nil || !c.IsOnline() {
		c.logger.Warn("try send message failed, client closed or offline")
		return ErrClientClosed
	}

	select {
	case c.send <- message:
		c.logger.Debug("message enqueued to send buffer successfully", zap.Int("message_size", len(message)))
		return nil
	default:
		c.logger.Warn("send buffer full, message discarded", zap.Int("queue_size", len(c.send)))
		return ErrSendBufferFull
	}
}

// UserID 返回用户 ID
func (c *Client) UserID() uint64 {
	if c == nil {
		return 0
	}
	return c.userID
}

// ConnID 返回连接 ID
func (c *Client) ConnID() string {
	if c == nil {
		return ""
	}
	return c.connID
}

// SessionID 返回会话 ID
func (c *Client) SessionID() string {
	if c == nil {
		return ""
	}
	return c.sessionID
}

// Conn 返回底层连接
func (c *Client) Conn() *gws.Conn {
	if c == nil {
		return nil
	}
	return c.conn
}

// ConnectedAt 返回连接建立时间
func (c *Client) ConnectedAt() time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.connectedAt
}

// LastActiveAt 返回最近活跃时间
func (c *Client) LastActiveAt() time.Time {
	if c == nil {
		return time.Time{}
	}
	return time.Unix(0, c.lastActiveAt.Load())
}

// RefreshActiveTime 刷新最近活跃时间
func (c *Client) RefreshActiveTime() {
	if c == nil {
		return
	}
	c.lastActiveAt.Store(time.Now().UnixNano())
}

// Status 返回连接状态
func (c *Client) Status() ClientStatus {
	if c == nil {
		return ClientStatusClosed
	}
	return ClientStatus(c.status.Load())
}

// SetOnline 设置为在线状态
func (c *Client) SetOnline() {
	if c == nil {
		return
	}
	c.status.Store(int32(ClientStatusOnline))
	c.logger.Info("websocket client status changed to online")
}

// IsOnline 判断是否在线
func (c *Client) IsOnline() bool {
	return c != nil && c.Status() == ClientStatusOnline
}
