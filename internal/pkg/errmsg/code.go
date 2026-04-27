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
	AccessTokenMissing     = 20001 // 访问 Token 缺失
	AccessTokenInvalid     = 20002 // 访问 Token 无效
	AccessTokenExpired     = 20003 // 访问 Token 已过期
	RefreshTokenMissing    = 20004 // 刷新 Token 缺失
	RefreshTokenInvalid    = 20005 // 刷新 Token 无效
	RefreshTokenExpired    = 20006 // 刷新 Token 已过期
	InvalidUsernameFormat  = 20007 // 无效的用户名格式
	InvalidEmailFormat     = 20008 // 无效的邮箱地址格式
	InvalidAccountFormat   = 20009 // 无效的账户格式
	InvalidPasswordFormat  = 20010 // 无效的密码格式
	InvalidNicknameFormat  = 20011 // 无效的昵称格式
	InvalidAvatarURLFormat = 20012 // 无效的头像URL格式
	UserAlreadyExists      = 20013 // 用户已存在
	UserNotFound           = 20014 // 用户不存在
	UserDisabled           = 20015 // 用户被禁用
	UserLoggedIn           = 20016 // 用户已登录
	PasswordIncorrect      = 20017 // 账户/密码错误
	OldPasswordIncorrect   = 20018 // 输入的旧密码不正确
	NewPasswordSameAsOld   = 20019 // 新密码和旧密码相同

	// =========================
	// 项目模块 30000+
	// =========================
	ProjectNotFound           = 30001 // 项目不存在
	ProjectArchived           = 30002 // 项目已归档
	ProjectNoPermission       = 30003 // 无项目操作权限
	InvalidProjectNameFormat  = 30004 // 非法项目名称格式
	InvalidProjectParams      = 30005 // 非法项目用参数
	InvalidTimeParam          = 30006 // 非法的项目时间参数
	InvalidTimeRange          = 30007 // 非法时间范围
	InvalidProjectStatus      = 30008 // 非法的项目状态
	InvalidProjectDescription = 30009 // 非法的项目描述
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

	// 项目
	ProjectNotFound:           "Project not found",
	ProjectArchived:           "Project archived",
	ProjectNoPermission:       "No permission to operate project",
	InvalidTimeRange:          "Invalid time range",
	InvalidProjectNameFormat:  "Invalid project name",
	InvalidProjectParams:      "Invalid project param",
	InvalidTimeParam:          "Invalid time param",
	InvalidProjectStatus:      "Invalid project status",
	InvalidProjectDescription: "Invalid project description",
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
