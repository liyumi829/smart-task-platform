// internal/service/forwarding_service/websocket/errors.go
// package websocket
// 定义 websocket 连接的错误
package websocket

import "errors"

var (
	ErrNilClient      = errors.New("websocket client is nil")       // ErrNilClient 表示客户端为空
	ErrNilConnection  = errors.New("websocket connection is nil")   // ErrNilConnection 表示底层连接为空
	ErrInvalidUserID  = errors.New("invalid user id")               // ErrInvalidUserID 表示用户 ID 非法
	ErrInvalidToken   = errors.New("invalid token")                 // ErrInvalidToken 表示 token 非法
	ErrInvalidSession = errors.New("invalid session")               // ErrInvalidSession 表示会话非法或不存在
	ErrEmptySessionID = errors.New("empty session id")              // ErrEmptySessionID 表示会话 ID 为空
	ErrUserOffline    = errors.New("user is offline")               // ErrUserOffline 表示用户离线
	ErrClientClosed   = errors.New("websocket client is closed")    // ErrClientClosed 表示连接已关闭
	ErrSendBufferFull = errors.New("websocket send buffer is full") // ErrSendBufferFull 表示发送缓冲区已满
)
