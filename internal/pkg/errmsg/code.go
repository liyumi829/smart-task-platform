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
	InvalidParams  = 10001 // 请求参数错误
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
	InvalidUsernameFormat  = 20007 // 用户名格式不正确
	InvalidEmailFormat     = 20008 // 邮箱格式不正确
	InvalidAccountFormat   = 20009 // 账号格式不正确
	InvalidPasswordFormat  = 20010 // 密码格式不正确
	InvalidNicknameFormat  = 20011 // 昵称格式不正确
	InvalidAvatarURLFormat = 20012 // 头像 URL 格式不正确
	UserAlreadyExists      = 20013 // 用户已存在
	UserNotFound           = 20014 // 用户不存在
	UserDisabled           = 20015 // 用户已被禁用
	UserLoggedIn           = 20016 // 用户已登录
	PasswordIncorrect      = 20017 // 账号或密码错误
	OldPasswordIncorrect   = 20018 // 旧密码错误
	NewPasswordSameAsOld   = 20019 // 新密码不能与旧密码相同

	// =========================
	// 项目模块 30000+
	// =========================

	ProjectNotFound            = 30001 // 项目不存在
	ProjectArchived            = 30002 // 项目已归档
	ProjectNoPermission        = 30003 // 无项目操作权限
	InvalidProjectNameFormat   = 30004 // 项目名称格式不正确
	InvalidProjectParams       = 30005 // 项目参数不正确
	InvalidTimeParam           = 30006 // 项目时间参数不正确
	InvalidTimeRange           = 30007 // 项目时间范围不正确
	InvalidProjectStatus       = 30008 // 项目状态不正确
	InvalidProjectDescription  = 30009 // 项目描述格式不正确
	EmptyProjectMemberRole     = 30010 // 项目成员角色不能为空
	InvalidProjectMemberRole   = 30011 // 项目成员角色不正确
	ProjectMemberAlreadyExists = 30012 // 项目成员已存在
	ExceededAdminMemberLimit   = 30013 // 项目管理员数量已达上限

	// =========================
	// 任务模块 40000+
	// =========================

	TaskNotFound                 = 40001 // 任务不存在
	TaskNoPermission             = 40002 // 无任务操作权限
	EmptyTaskTitle               = 40003 // 任务标题为空
	InvalidTaskTitleFormat       = 40004 // 任务标题格式不正确
	InvalidTaskDescriptionFormat = 40005 // 任务描述格式不正确
	InvalidTaskPriorityFormat    = 40006 // 任务优先级格式不正确
	InvalidTaskStatusFormat      = 40007 // 任务状态格式不正确
	InvalidTaskTimeFormat        = 40008 // 任务时间格式不正确
	AssigneeNotFound             = 40009 // 负责人用户不存在
	AssigneeNotProjectMember     = 40010 // 负责人不是项目成员
	InvalidTaskSortByFormat      = 40011 // 任务排序规则格式不正确
	InvalidTaskSortOrderFormat   = 40012 // 任务排序顺序格式不正确
	InvalidTaskSortItem          = 40013 // 任务排序表项不合法
	EmptyTaskSortItem            = 40014 // 任务排序表项为空
)

// codeMsgMap 错误码与错误信息映射
var codeMsgMap = map[int]string{
	// =========================
	// 成功
	// =========================
	Success: "Success",

	// =========================
	// 通用错误信息
	// =========================
	ServerError:    "Internal server error",
	InvalidParams:  "Invalid request parameters",
	NotFound:       "Resource not found",
	Unauthorized:   "Authentication required",
	Forbidden:      "Permission denied",
	TooManyRequest: "Too many requests, please try again later",

	// =========================
	// Token / 认证错误信息
	// =========================
	AccessTokenMissing:  "Access token is required",
	AccessTokenInvalid:  "Invalid access token",
	AccessTokenExpired:  "Access token has expired",
	RefreshTokenMissing: "Refresh token is required",
	RefreshTokenInvalid: "Invalid refresh token",
	RefreshTokenExpired: "Refresh token has expired",

	// =========================
	// 用户参数错误信息
	// =========================
	InvalidUsernameFormat:  "Invalid username format",
	InvalidEmailFormat:     "Invalid email format",
	InvalidAccountFormat:   "Invalid account format",
	InvalidPasswordFormat:  "Invalid password format",
	InvalidNicknameFormat:  "Invalid nickname format",
	InvalidAvatarURLFormat: "Invalid avatar URL format",

	// =========================
	// 用户业务错误信息
	// =========================
	UserAlreadyExists:    "User already exists",
	UserNotFound:         "User not found",
	UserDisabled:         "User has been disabled",
	UserLoggedIn:         "User is already logged in",
	PasswordIncorrect:    "Incorrect account or password",
	OldPasswordIncorrect: "Old password is incorrect",
	NewPasswordSameAsOld: "New password cannot be the same as the old password",

	// =========================
	// 项目基础错误信息
	// =========================
	ProjectNotFound:           "Project not found",
	ProjectArchived:           "Project has been archived",
	ProjectNoPermission:       "No permission to operate this project",
	InvalidProjectNameFormat:  "Invalid project name format",
	InvalidProjectParams:      "Invalid project parameters",
	InvalidTimeParam:          "Invalid project time parameter",
	InvalidTimeRange:          "Invalid project time range",
	InvalidProjectStatus:      "Invalid project status",
	InvalidProjectDescription: "Invalid project description",

	// =========================
	// 项目成员错误信息
	// =========================
	EmptyProjectMemberRole:     "Project member role is required",
	InvalidProjectMemberRole:   "Invalid project member role",
	ProjectMemberAlreadyExists: "Project member already exists",
	ExceededAdminMemberLimit:   "Project admin member limit has been reached",

	// =========================
	// 任务错误信息
	// =========================
	TaskNotFound:                 "Task not found",
	TaskNoPermission:             "No permission to operate this task",
	EmptyTaskTitle:               "Task title cannot be empty",
	InvalidTaskTitleFormat:       "Invalid task title format",
	InvalidTaskDescriptionFormat: "Invalid task description format",
	InvalidTaskPriorityFormat:    "Invalid task priority format",
	InvalidTaskStatusFormat:      "Invalid task status format",
	InvalidTaskTimeFormat:        "Invalid task time format",
	AssigneeNotFound:             "Assignee user not found",
	AssigneeNotProjectMember:     "Assignee is not a project member",
	InvalidTaskSortByFormat:      "Invalid task sort by format",
	InvalidTaskSortOrderFormat:   "Invalid task sort order format",
	InvalidTaskSortItem:          "Invalid task sort item",
	EmptyTaskSortItem:            "Task sort item cannot be empty",
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
