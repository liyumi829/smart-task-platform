// Package errmsg
// 主要设置项目中的退出码以及错误信息
package errmsg

// 错误码设计说明：
// 0      : 成功
// 10000+ : 通用错误
// 20000+ : 认证/用户模块
// 30000+ : 项目模块
// 40000+ : 任务模块
// 50000+ : 评论/通知/协作模块
// 60000+ : AI 模块

const (
	// Success 成功
	Success = 0

	// =========================
	// 通用错误码 10000+
	// =========================
	ServerError    = 10000 // 服务器内部错误
	InvalidParams  = 10001 // 参数错误
	NotFound       = 10002 // 资源不存在
	Unauthorized   = 10003 // 未登录或认证失败
	Forbidden      = 10004 // 无权限访问
	TooManyRequest = 10005 // 请求过于频繁

	// =========================
	// 认证/用户模块 20000+
	// =========================
	UserAlreadyExists      = 20001 // 用户已存在
	UserNotFound           = 20002 // 用户不存在
	UserDisabled           = 20003 // 用户被禁用
	UserLoggedIn           = 20004 // 用户已登录
	AccessTokenInvalid     = 20005 // 访问 Token 无效
	AccessTokenExpired     = 20006 // 访问 Token 已过期
	AccessTokenMissing     = 20007 // 访问 Token 缺失
	RefreshTokenInvalid    = 20008 // 刷新 Token 无效
	RefreshTokenExpired    = 20009 // 刷新 Token 已过期
	RefreshTokenMissing    = 20010 // 刷新 Token 缺失
	InvalidUsernameFormat  = 20011 // 无效的用户名格式
	InvalidEmailFormat     = 20012 // 无效的邮箱地址格式
	InvalidAccountFormat   = 20013 // 无效的账户格式
	InvalidPasswordFormat  = 20014 // 无效的密码格式
	InvalidNicknameFormat  = 20015 // 无效的昵称格式
	PasswordIncorrect      = 20016 // 账户/密码错误
	InvalidAvatarURLFormat = 20017 // 无效的头像URL格式
	OldPasswordIncorrect   = 20018 // 输入的旧密码不正确
	NewPasswordSameAsOld   = 20019 // 新密码和旧密码相同
)

// codeMsgMap 错误码与错误信息映射
var codeMsgMap = map[int]string{
	Success: "Success",

	// 通用
	ServerError:    "Internal server error",
	InvalidParams:  "Invalid request parameters",
	NotFound:       "Resource not found",
	Unauthorized:   "Unauthorized or authentication failed",
	Forbidden:      "Forbidden, no permission to access",
	TooManyRequest: "Too many requests",

	// 认证/用户信息
	UserAlreadyExists:      "User already exists(check email and username)",
	UserNotFound:           "User not found",
	UserDisabled:           "User is disabled",
	AccessTokenInvalid:     "Invalid access token",
	AccessTokenExpired:     "Access token expired",
	AccessTokenMissing:     "Access token is missing",
	RefreshTokenInvalid:    "Invalid refresh token",
	RefreshTokenExpired:    "Refresh token expired",
	RefreshTokenMissing:    "Refresh token is missing",
	UserLoggedIn:           "User already logged in",
	InvalidUsernameFormat:  "Invalid username format",
	InvalidEmailFormat:     "Invalid email address format",
	InvalidAccountFormat:   "Invalid account format",
	InvalidPasswordFormat:  "Invalid password format",
	InvalidNicknameFormat:  "Invalid nickname format",
	PasswordIncorrect:      "Incorrect account or password",
	InvalidAvatarURLFormat: "Invalid avatar URL format",
	OldPasswordIncorrect:   "Old password is incorrect",
	NewPasswordSameAsOld:   "New password cannot be the same as old password",
}

// GetMsg 根据错误码获取错误信息
func GetMsg(code int) string {
	if msg, ok := codeMsgMap[code]; ok {
		return msg
	}
	return codeMsgMap[ServerError]
}

// RegisterMsg 注册自定义错误信息
// 如果后续需要扩展模块错误码，可以在初始化时调用此方法注册。
func RegisterMsg(code int, msg string) {
	if msg == "" {
		return
	}
	codeMsgMap[code] = msg
}
