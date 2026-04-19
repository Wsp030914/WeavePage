package models

import (
	"time"

	"gorm.io/gorm"
)

type Task struct {
	ID        int              `gorm:"primaryKey"                           json:"id"`
	UserID    int              `gorm:"not null;index:idx_user_sort,priority:1;index:idx_user_proj_sort,priority:1;uniqueIndex:ux_task_user_proj_title,priority:1" json:"user_id"`
	ProjectID int              `gorm:"not null;index;index:idx_user_proj_sort,priority:2;index:idx_tasks_project_sort_priority,priority:1;index:idx_tasks_project_status_sort_priority,priority:1;uniqueIndex:ux_task_user_proj_title,priority:2" json:"project_id"`
	Title     string           `gorm:"size:200;not null;uniqueIndex:ux_task_user_proj_title,priority:3" json:"title"`
	ContentMD string           `gorm:"type:longtext"                         json:"content_md"`
	Status    string           `gorm:"type:enum('todo','done');not null;default:'todo';index:idx_tasks_due_watch,priority:1;index:idx_tasks_project_status_sort_priority,priority:2" json:"status"`
	Priority  int              `gorm:"type:tinyint;not null;default:3;index:idx_tasks_project_sort_priority,priority:3;index:idx_tasks_project_status_sort_priority,priority:4" json:"priority"`
	SortOrder int64            `gorm:"not null;default:0;index:idx_user_sort,priority:2;index:idx_user_proj_sort,priority:3;index:idx_tasks_project_sort_priority,priority:2;index:idx_tasks_project_status_sort_priority,priority:3" json:"sort_order"`
	DueAt     *time.Time       `gorm:"index:idx_tasks_due_watch,priority:2" json:"due_at"`
	Version   int              `gorm:"not null;default:1"                   json:"version"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Notified  bool             `gorm:"not null;default:false;index:idx_tasks_due_watch,priority:3" json:"notified"`
	Members   []TaskMemberInfo `gorm:"-" json:"members"`
}

type TaskIDScore struct {
	ID        int
	SortOrder int64
	Priority  int
}

func (t *Task) BeforeCreate(tx *gorm.DB) error {
	if t.SortOrder == 0 {
		t.SortOrder = time.Now().UnixNano()
	}
	if t.Version <= 0 {
		t.Version = 1
	}
	return nil
}

func (t *Task) CalculateScore() float64 {
	return float64(t.SortOrder)
}

const (
	TaskTodo = "todo"
	TaskDone = "done"
)
