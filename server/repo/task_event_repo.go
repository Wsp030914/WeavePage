package repo

// 文件说明：这个文件负责某类持久化访问逻辑。
// 实现方式：把数据库查询、更新与事务细节收口在仓储层。
// 这样做的好处是业务层不直接依赖 ORM 细节。
import (
	"ToDoList/server/models"
	"context"

	"gorm.io/gorm"
)

type TaskEventRepository interface {
	Create(ctx context.Context, event *models.TaskEvent) (*models.TaskEvent, error)
	ListByProjectAfterID(ctx context.Context, projectID int, afterID int64, limit int) ([]models.TaskEvent, error)
	ListByProjectTaskAfterID(ctx context.Context, projectID int, taskID int, afterID int64, limit int) ([]models.TaskEvent, error)
	ListByProjectBeforeID(ctx context.Context, projectID int, beforeID int64, limit int) ([]models.TaskEvent, error)
	ListByProjectTaskBeforeID(ctx context.Context, projectID int, taskID int, beforeID int64, limit int) ([]models.TaskEvent, error)
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

func (r *taskEventRepo) ListByProjectTaskAfterID(ctx context.Context, projectID int, taskID int, afterID int64, limit int) ([]models.TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	var events []models.TaskEvent
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND task_id = ? AND id > ?", projectID, taskID, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *taskEventRepo) ListByProjectBeforeID(ctx context.Context, projectID int, beforeID int64, limit int) ([]models.TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.db.WithContext(ctx).
		Where("project_id = ?", projectID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}

	var events []models.TaskEvent
	err := query.
		Order("id DESC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (r *taskEventRepo) ListByProjectTaskBeforeID(ctx context.Context, projectID int, taskID int, beforeID int64, limit int) ([]models.TaskEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.db.WithContext(ctx).
		Where("project_id = ? AND task_id = ?", projectID, taskID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}

	var events []models.TaskEvent
	err := query.
		Order("id DESC").
		Limit(limit).
		Find(&events).Error
	return events, err
}
