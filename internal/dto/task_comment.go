// internal/dto/task_comment.go
// Package dto 任务评论的数据传输对象

package dto

import "time"

// ProjectTaskCommentUri 路径参数（GET / DELETE）
//
// - 这里的ProjectID和TaskID都是必传的，不使用指针
type ProjectTaskCommentUri struct {
	ProjectID uint64 `uri:"projectId" binding:"required"`  // 必须：项目ID
	TaskID    uint64 `uri:"taskId" binding:"required"`     // 必须：任务ID
	CommentID uint64 `uri:"commentId" binding:"omitempty"` // 必须：评论ID（DELETE时必传）
}

type TaskCommentBaseFields struct {
	ID            uint64             `json:"id"`               // 评论 ID
	TaskID        uint64             `json:"task_id"`          // 任务 ID
	Content       string             `json:"content"`          // 评论内容
	AuthorID      uint64             `json:"author_id"`        // 评论人 ID
	Author        *UserPublicProfile `json:"author"`           // 评论人公开信息
	ParentID      *uint64            `json:"parent_id"`        // 父评论 ID，nil 表示一级评论
	ReplyToUserID *uint64            `json:"reply_to_user_id"` // 被回复用户 ID，nil 表示没有回复对象
	ReplyToUser   *UserPublicProfile `json:"reply_to_user"`    // 被回复用户公开信息
	CreatedAt     time.Time          `json:"created_at"`       // 创建时间
}

// CreateTaskCommentReq 创建任务评论请求
type CreateTaskCommentReq struct {
	// 评论内容
	Content string `json:"content" binding:"required"`

	// 父评论 ID
	//
	// nil 表示一级评论；
	// 非 nil 表示回复某条评论；
	// 不建议用 0 表示无父评论，因为 0 不是一个真实 ID，语义不如 nil 清晰。
	ParentID *uint64 `json:"parent_id" binding:"omitempty"`

	// 被回复用户 ID
	//
	// 推荐由后端根据 ParentID 查询父评论后自动推导，不建议前端传，避免伪造 reply_to_user_id。
	// 如果业务确实需要前端指定被回复用户，再打开这个字段。
	// ReplyToUserID *uint64 `json:"reply_to_user_id" binding:"required"`
}

// CreateTaskCommentResp 创建任务评论响应
type CreateTaskCommentResp struct {
	*TaskCommentBaseFields
}

// ListTaskCommentsQuery 任务评论列表查询
type ListTaskCommentsQuery struct {
	PageQuery
}

// TaskCommentListItem 评论表项
type TaskCommentListItem struct {
	*TaskCommentBaseFields
	ParentDeleted bool `json:"parent_deleted"`
}

// ListTaskCommentResp 任务评论列表响应
type ListTaskCommentsResp = PageResp[*TaskCommentListItem]

// RemoveTaskCommentReq 删除任务的请求
type RemoveTaskCommentReq struct{} // 占位

// RemoveTaskCommentResp 删除任务的响应
type RemoveTaskCommentResp struct{} // 占位
