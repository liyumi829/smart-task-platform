// internal/service/forwarding_service/websocket/manager.go
// package websocket
// 功能：管理 websocket Client 的生命周期

package websocket

import (
	"sync"

	"go.uber.org/zap"
)

// Manager 管理 WebSocket 连接。
// 当前系统单点登录，因此核心索引是 user_id -> client。
type Manager struct {
	mu      sync.RWMutex       // 保护 clients 资源
	clients map[uint64]*Client // userID -> Client 映射
	logger  *zap.Logger        // 日志器
}

// NewManager 创建连接管理器。
func NewManager(logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Manager{
		clients: make(map[uint64]*Client),
		logger: logger.With(
			zap.String("component", "websocket_manager"),
		),
	}
}

// Register 注册一个客户端连接。
// 如果用户已有旧连接，则关闭旧连接并替换为新连接
func (m *Manager) Register(client *Client) error {
	if client == nil {
		m.logger.Error("register failed: client is nil")
		return ErrNilClient
	}

	if client.UserID() == 0 {
		m.logger.Error("register failed: invalid user id")
		return ErrInvalidUserID
	}

	var oldClient *Client // 旧连接，这里定义便于后面进行关闭旧连接

	m.mu.Lock()
	if existed, ok := m.clients[client.UserID()]; ok && existed != nil && existed != client {
		oldClient = existed     // 赋值旧连接
		m.removeLocked(existed) // 将旧连接删除
		m.logger.Warn("old client connection replaced, will close old connection",
			zap.Uint64("user_id", client.UserID()),
			zap.String("old_conn_id", oldClient.ConnID()),
			zap.String("new_conn_id", client.ConnID()),
		)
	}
	client.SetOnline()                  // 设置新连接为在线状态
	m.clients[client.UserID()] = client // 将ID进行管理起来
	m.mu.Unlock()

	m.logger.Info("client registered successfully",
		zap.Uint64("user_id", client.UserID()),
		zap.String("conn_id", client.ConnID()),
	)

	if oldClient != nil {
		oldClient.Close() // 将旧连接进行关闭
	}

	return nil
}

// UnregisterByUserID 根据用户 ID 注销连接
func (m *Manager) UnregisterByUserID(userID uint64) {
	if userID == 0 {
		m.logger.Warn("unregister by user id failed: invalid user id")
		return
	}

	var client *Client

	m.mu.Lock()
	client = m.clients[userID]
	if client != nil {
		m.removeLocked(client)
	}
	m.mu.Unlock()

	if client != nil {
		m.logger.Info("unregister client by user id",
			zap.Uint64("user_id", userID),
			zap.String("conn_id", client.ConnID()),
		)
		client.Close()
	} else {
		m.logger.Debug("unregister by user id: client not found",
			zap.Uint64("user_id", userID),
		)
	}
}

// UnregisterByClient 注销指定客户端。
// 该方法可避免旧连接退出时误删新连接。
func (m *Manager) UnregisterByClient(client *Client) {
	if client == nil {
		m.logger.Warn("unregister by client failed: client is nil")
		return
	}

	userID := client.UserID()
	connID := client.ConnID()

	m.mu.Lock()
	current, ok := m.clients[client.UserID()]
	if ok && current == client {
		m.removeLocked(client)
		m.logger.Info("client removed from manager",
			zap.Uint64("user_id", userID),
			zap.String("conn_id", connID),
		)
	} else {
		m.logger.Debug("skip unregister: client is not current active connection",
			zap.Uint64("user_id", userID),
			zap.String("conn_id", connID),
		)
	}
	m.mu.Unlock()

	m.logger.Info("closing client connection",
		zap.Uint64("user_id", userID),
		zap.String("conn_id", connID),
	)
	client.Close()
}

// GetByUserID 根据用户 ID 获取连接。
func (m *Manager) GetByUserID(userID uint64) (*Client, bool) {
	if userID == 0 {
		m.logger.Warn("get client failed: invalid user id")
		return nil, false
	}

	m.mu.RLock()
	client, ok := m.clients[userID]
	m.mu.RUnlock()

	if !ok || client == nil || !client.IsOnline() {
		m.logger.Debug("client not found or offline",
			zap.Uint64("user_id", userID),
		)
		return nil, false
	}

	m.logger.Debug("get client successfully",
		zap.Uint64("user_id", userID),
		zap.String("conn_id", client.ConnID()),
	)
	return client, true
}

// CloseAll 关闭所有连接
func (m *Manager) UnregisterAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for userID, client := range m.clients {
		client.Close()
		delete(m.clients, userID)
	}

}

//=======
// helper
//=======

// removeLocked 在持锁状态下删除连接索引。
func (m *Manager) removeLocked(client *Client) {
	if client == nil {
		return
	}

	delete(m.clients, client.UserID())
}
