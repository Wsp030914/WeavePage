package models

// 文件说明：这个文件定义任务与文档聚合根模型。
// 实现方式：在同一个持久化结构里统一承载任务、普通文档、会议纪要、日记和回收站相关字段。
// 这样做的好处是任务型与文档型能力可以共享主表、缓存和实时事件机制，同时通过 doc_type 区分语义。

import (
	"time"

	"gorm.io/gorm"
)

// Task 是任务/文档主模型。
// 它既承载基础任务字段，也承载文档类型、协作模式、正文、回收站和成员快照等扩展信息。
type Task struct {
	ID                int              `gorm:"primaryKey"                           json:"id"`
	UserID            int              `gorm:"not null;index:idx_user_sort,priority:1;index:idx_user_proj_sort,priority:1;uniqueIndex:ux_task_user_proj_title,priority:1" json:"user_id"`
	ProjectID         int              `gorm:"not null;index;index:idx_user_proj_sort,priority:2;index:idx_tasks_project_sort_priority,priority:1;index:idx_tasks_project_status_sort_priority,priority:1;uniqueIndex:ux_task_user_proj_title,priority:2" json:"project_id"`
	Title             string           `gorm:"size:200;not null;uniqueIndex:ux_task_user_proj_title,priority:3" json:"title"`
	DeletedTitle      string           `gorm:"size:200"                             json:"deleted_title,omitempty"`
	ContentMD         string           `gorm:"type:longtext"                         json:"content_md"`
	DocType           string           `gorm:"size:32;not null;default:'document';index" json:"doc_type"`
	CollaborationMode string           `gorm:"size:32;not null;default:'collaborative';index" json:"collaboration_mode"`
	Status            string           `gorm:"type:enum('todo','done');not null;default:'todo';index:idx_tasks_due_watch,priority:1;index:idx_tasks_project_status_sort_priority,priority:2" json:"status"`
	Priority          int              `gorm:"type:tinyint;not null;default:3;index:idx_tasks_project_sort_priority,priority:3;index:idx_tasks_project_status_sort_priority,priority:4" json:"priority"`
	SortOrder         int64            `gorm:"not null;default:0;index:idx_user_sort,priority:2;index:idx_user_proj_sort,priority:3;index:idx_tasks_project_sort_priority,priority:2;index:idx_tasks_project_status_sort_priority,priority:3" json:"sort_order"`
	DueAt             *time.Time       `gorm:"index:idx_tasks_due_watch,priority:2" json:"due_at"`
	Version           int              `gorm:"not null;default:1"                   json:"version"`
	CreatedAt         time.Time        `json:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at"`
	DeletedAt         gorm.DeletedAt   `gorm:"index"                                json:"deleted_at,omitempty" swaggertype:"string" format:"date-time"`
	DeletedBy         *int             `gorm:"index"                                json:"deleted_by,omitempty"`
	Notified          bool             `gorm:"not null;default:false;index:idx_tasks_due_watch,priority:3" json:"notified"`
	Members           []TaskMemberInfo `gorm:"-" json:"members"`
}

// TaskIDScore 是任务排序缓存所需的最小字段集合。
type TaskIDScore struct {
	ID        int
	SortOrder int64
	Priority  int
}

// BeforeCreate 在任务首次入库前补齐默认排序、版本和文档属性。
// 这样做可以保证不同入口创建的任务在持久化层拥有一致的默认值。
func (t *Task) BeforeCreate(tx *gorm.DB) error {
	if t.SortOrder == 0 {
		t.SortOrder = time.Now().UnixNano()
	}
	if t.Version <= 0 {
		t.Version = 1
	}
	if t.DocType == "" {
		t.DocType = DocTypeDocument
	}
	if t.CollaborationMode == "" {
		t.CollaborationMode = CollaborationModeCollaborative
	}
	return nil
}

// CalculateScore 计算任务在排序缓存中的分值。
// 当前直接复用 SortOrder，是为了让缓存排序与数据库排序保持一致。
func (t *Task) CalculateScore() float64 {
	return float64(t.SortOrder)
}

const (
	TaskTodo = "todo"
	TaskDone = "done"
)

const (
	DocTypeDocument = "document"
	DocTypeMeeting  = "meeting"
	DocTypeDiary    = "diary"
	DocTypeTodo     = "todo"
)

const (
	CollaborationModeCollaborative = "collaborative"
	CollaborationModePrivate       = "private"
)
