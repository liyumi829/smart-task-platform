// internal/model/task.go tasks表对象映射

package model

import (
	"time"

	"gorm.io/gorm"
)

// ==============================
// Task 状态 / 优先级 枚举常量
// ==============================

const (
	TaskStatusTodo       = "todo"        // 待处理
	TaskStatusInProgress = "in_progress" // 进行中
	TaskStatusDone       = "done"        // 已完成
	TaskStatusCancelled  = "cancelled"   // 已取消

	TaskPriorityLow    = "low"    // 低
	TaskPriorityMedium = "medium" // 中
	TaskPriorityHigh   = "high"   // 高
	TaskPriorityUrgent = "urgent" // 紧急
)

// ==============================
// Task 优先级等级（数字越小优先级越高）
// ==============================

var TaskPriorityLevel map[string]int = map[string]int{
	TaskPriorityUrgent: 0,
	TaskPriorityHigh:   1,
	TaskPriorityMedium: 2,
	TaskPriorityLow:    3,
}

// ==============================
// Task 表名 / 列名 常量
// ==============================

const (
	TaskTableName              = "tasks"               // 表名
	TaskColumnID               = "id"                  // 任务ID
	TaskColumnProjectID        = "project_id"          // 所属项目ID
	TaskColumnTitle            = "title"               // 任务标题
	TaskColumnDescription      = "description"         // 任务描述
	TaskColumnStatus           = "status"              // 任务状态
	TaskColumnPriority         = "priority"            // 任务优先级
	TaskColumnCreatorID        = "creator_id"          // 创建人ID
	TaskColumnAssigneeID       = "assignee_id"         // 负责人ID
	TaskColumnDueDate          = "due_date"            // 截止时间
	TaskColumnStartTime        = "start_time"          // 开始时间
	TaskColumnCompletedAt      = "completed_at"        // 完成时间
	TaskColumnAISummary        = "ai_summary"          // AI任务摘要
	TaskColumnSortOrder        = "sort_order"          // 排序值
	TaskColumnCreatedAt        = "created_at"          // 创建时间
	TaskColumnUpdatedAt        = "updated_at"          // 更新时间
	TaskColumnDeletedAt        = "deleted_at"          // 软删除时间
	TaskColumnPriorityOrder    = "priority_order"      // 优先级排序
	TaskColumnStatusOrder      = "status_order"        // 状态排序
	TaskColumnDueDateNullOrder = "due_date_null_order" // 日志NULL排序
)

// ==============================
// Task 关联关系常量（结构体字段名）
// ==============================

const (
	TaskAssocProject  = "Project"  // 关联项目
	TaskAssocCreator  = "Creator"  // 关联创建人
	TaskAssocAssignee = "Assignee" // 关联负责人
)

// Task 任务表模型
type Task struct {
	ID          uint64     `gorm:"column:id;primaryKey;autoIncrement;comment:任务ID" json:"id"`
	ProjectID   uint64     `gorm:"column:project_id;not null;index:idx_tasks_project_id;index:idx_tasks_project_status,priority:1;comment:所属项目ID" json:"project_id"`
	Title       string     `gorm:"column:title;type:varchar(200);not null;comment:任务标题" json:"title"`
	Description *string    `gorm:"column:description;type:text;comment:任务描述" json:"description"`
	Status      string     `gorm:"column:status;type:varchar(20);not null;default:todo;index:idx_tasks_status;index:idx_tasks_project_status,priority:2;comment:任务状态" json:"status"`
	Priority    string     `gorm:"column:priority;type:varchar(20);not null;default:medium;index:idx_tasks_priority;comment:任务优先级" json:"priority"`
	CreatorID   uint64     `gorm:"column:creator_id;not null;index:idx_tasks_creator_id;comment:创建人ID" json:"creator_id"`
	AssigneeID  *uint64    `gorm:"column:assignee_id;default:null;index:idx_tasks_assignee_id;comment:负责人ID" json:"assignee_id"`
	DueDate     *time.Time `gorm:"column:due_date;default:null;index:idx_tasks_due_date;comment:截止时间" json:"due_date"`
	StartTime   *time.Time `gorm:"column:start_time;default:null;comment:开始时间" json:"start_time"`
	CompletedAt *time.Time `gorm:"column:completed_at;default:null;comment:完成时间" json:"completed_at"`
	AISummary   string     `gorm:"column:ai_summary;type:text;comment:AI任务摘要" json:"ai_summary"`
	SortOrder   int        `gorm:"column:sort_order;not null;default:0;index:idx_tasks_sort_order;comment:排序值" json:"sort_order"`

	// PriorityOrder 优先级排序值
	//
	// urgent    -> 1
	// high      -> 2
	// medium    -> 3
	// low       -> 4
	//
	// 该字段用于替代 ORDER BY FIELD(priority, ...)
	PriorityOrder int `gorm:"column:priority_order;not null;default:3;comment:优先级排序值：1 urgent，2 high，3 medium，4 low" json:"priority_order"`

	// StatusOrder 状态排序值
	//
	// todo        -> 1
	// in_progress -> 2
	// done        -> 3
	// cancelled   -> 4
	//
	// 该字段用于替代 ORDER BY FIELD(status, ...)
	StatusOrder int `gorm:"column:status_order;not null;default:1;comment:状态排序值：1 todo，2 in_progress，3 done，4 cancelled" json:"status_order"`

	// DueDateNullOrder 截止时间 NULL 排序值
	//
	// due_date 非空 -> 0
	// due_date 为空 -> 1
	//
	// 该字段用于替代 ORDER BY due_date IS NULL
	DueDateNullOrder int `gorm:"column:due_date_null_order;not null;default:1;comment:截止时间NULL排序值：0非空，1为空" json:"due_date_null_order"`

	CreatedAt time.Time      `gorm:"column:created_at;not null;autoCreateTime;comment:创建时间" json:"created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;autoUpdateTime;comment:更新时间" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index;comment:软删除时间" json:"-"`

	Project  *Project `gorm:"foreignKey:ProjectID;references:ID" json:"project,omitempty"`   // Project 所属项目
	Creator  *User    `gorm:"foreignKey:CreatorID;references:ID" json:"creator,omitempty"`   // Creator 创建人
	Assignee *User    `gorm:"foreignKey:AssigneeID;references:ID" json:"assignee,omitempty"` // Assignee 负责人
}

// TableName 指定任务表名
func (Task) TableName() string {
	return "tasks"
}

// BeforeSave 保存任务前维护排序冗余字段
func (t *Task) BeforeSave(_ *gorm.DB) error {
	t.PriorityOrder = BuildTaskPriorityOrder(t.Priority)
	t.StatusOrder = BuildTaskStatusOrder(t.Status)
	t.DueDateNullOrder = BuildTaskDueDateNullOrder(t.DueDate)
	return nil
}

// BuildTaskPriorityOrder 构建任务优先级排序值
func BuildTaskPriorityOrder(priority string) int {
	switch priority {
	case TaskPriorityUrgent:
		return 1
	case TaskPriorityHigh:
		return 2
	case TaskPriorityMedium:
		return 3
	case TaskPriorityLow:
		return 4
	default:
		return 3
	}
}

// BuildTaskStatusOrder 构建任务状态排序值
func BuildTaskStatusOrder(status string) int {
	switch status {
	case TaskStatusTodo:
		return 1
	case TaskStatusInProgress:
		return 2
	case TaskStatusDone:
		return 3
	case TaskStatusCancelled:
		return 4
	default:
		return 1
	}
}

// BuildTaskDueDateNullOrder 构建截止时间 NULL 排序值
func BuildTaskDueDateNullOrder(dueDate *time.Time) int {
	if dueDate == nil {
		return 1
	}
	return 0
}
