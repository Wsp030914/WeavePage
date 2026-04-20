package repo

// 文件说明：这个文件负责某类持久化访问逻辑。
// 实现方式：把数据库查询、更新与事务细节收口在仓储层。
// 这样做的好处是业务层不直接依赖 ORM 细节。
import (
	"ToDoList/server/models"
	"context"

	"gorm.io/gorm"
)

const (
	defaultTaskContentUpdateLimit = 100
	maxTaskContentUpdateLimit     = 500
)

// TaskContentRepository persists and reads Yjs task body updates.
type TaskContentRepository interface {
	Create(ctx context.Context, update *models.TaskContentUpdate) (*models.TaskContentUpdate, error)
	GetByMessageID(ctx context.Context, messageID string) (*models.TaskContentUpdate, error)
	ListByTaskAfterID(ctx context.Context, projectID int, taskID int, afterID int64, limit int) ([]models.TaskContentUpdate, error)
	GetLatestSnapshot(ctx context.Context, projectID int, taskID int) (*models.TaskContentUpdate, error)
}

type taskContentRepo struct {
	db *gorm.DB
}

// NewTaskContentRepository creates a task content update repository.
func NewTaskContentRepository(db *gorm.DB) TaskContentRepository {
	return &taskContentRepo{db: db}
}

func (r *taskContentRepo) Create(ctx context.Context, update *models.TaskContentUpdate) (*models.TaskContentUpdate, error) {
	if err := r.db.WithContext(ctx).Create(update).Error; err != nil {
		return nil, err
	}
	return update, nil
}

func (r *taskContentRepo) GetByMessageID(ctx context.Context, messageID string) (*models.TaskContentUpdate, error) {
	var update models.TaskContentUpdate
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&update).Error
	return &update, err
}

func (r *taskContentRepo) ListByTaskAfterID(ctx context.Context, projectID int, taskID int, afterID int64, limit int) ([]models.TaskContentUpdate, error) {
	limit = normalizeTaskContentUpdateLimit(limit)

	var updates []models.TaskContentUpdate
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND task_id = ? AND id > ?", projectID, taskID, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&updates).Error
	return updates, err
}

func (r *taskContentRepo) GetLatestSnapshot(ctx context.Context, projectID int, taskID int) (*models.TaskContentUpdate, error) {
	var update models.TaskContentUpdate
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND task_id = ? AND content_snapshot IS NOT NULL", projectID, taskID).
		Order("id DESC").
		First(&update).Error
	return &update, err
}

func normalizeTaskContentUpdateLimit(limit int) int {
	switch {
	case limit <= 0:
		return defaultTaskContentUpdateLimit
	case limit > maxTaskContentUpdateLimit:
		return maxTaskContentUpdateLimit
	default:
		return limit
	}
}
