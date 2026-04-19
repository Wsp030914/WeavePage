package repo

import (
	"ToDoList/server/models"
	"context"

	"gorm.io/gorm"
)

type TaskEventRepository interface {
	Create(ctx context.Context, event *models.TaskEvent) (*models.TaskEvent, error)
	ListByProjectAfterID(ctx context.Context, projectID int, afterID int64, limit int) ([]models.TaskEvent, error)
}

type taskEventRepo struct {
	db *gorm.DB
}

func NewTaskEventRepository(db *gorm.DB) TaskEventRepository {
	return &taskEventRepo{db: db}
}

func (r *taskEventRepo) Create(ctx context.Context, event *models.TaskEvent) (*models.TaskEvent, error) {
	if err := r.db.WithContext(ctx).Create(event).Error; err != nil {
		return nil, err
	}
	return event, nil
}

func (r *taskEventRepo) ListByProjectAfterID(ctx context.Context, projectID int, afterID int64, limit int) ([]models.TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	var events []models.TaskEvent
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND id > ?", projectID, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}
