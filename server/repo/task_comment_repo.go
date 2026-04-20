package repo

// 文件说明：这个文件负责评论数据的持久化访问。
// 实现方式：把评论基础表读写和带用户信息的详情查询都收口到 repo 层。
// 这样做的好处是 service 不需要感知 SQL join 细节，权限和业务逻辑可以专注在上层。
import (
	"ToDoList/server/models"
	"context"

	"gorm.io/gorm"
)

type TaskCommentRepository interface {
	Create(ctx context.Context, comment *models.TaskComment) (*models.TaskComment, error)
	GetByID(ctx context.Context, id int) (*models.TaskComment, error)
	GetDetailByID(ctx context.Context, id int) (*models.TaskCommentInfo, error)
	ListByTaskID(ctx context.Context, taskID int) ([]models.TaskCommentInfo, error)
	Update(ctx context.Context, id int, updates map[string]interface{}) (*models.TaskComment, error)
	DeleteByID(ctx context.Context, id int) (int64, error)
}

type taskCommentRepo struct {
	db *gorm.DB
}

// NewTaskCommentRepository 创建评论仓储。
func NewTaskCommentRepository(db *gorm.DB) TaskCommentRepository {
	return &taskCommentRepo{db: db}
}

// Create 写入一条评论记录。
func (r *taskCommentRepo) Create(ctx context.Context, comment *models.TaskComment) (*models.TaskComment, error) {
	err := r.db.WithContext(ctx).Create(comment).Error
	return comment, err
}

// GetByID 读取评论基础记录。
func (r *taskCommentRepo) GetByID(ctx context.Context, id int) (*models.TaskComment, error) {
	var comment models.TaskComment
	err := r.db.WithContext(ctx).First(&comment, id).Error
	return &comment, err
}

// GetDetailByID 查询一条带作者展示信息的评论详情。
func (r *taskCommentRepo) GetDetailByID(ctx context.Context, id int) (*models.TaskCommentInfo, error) {
	var comment models.TaskCommentInfo
	err := r.baseDetailQuery(ctx).
		Where("tc.id = ?", id).
		Take(&comment).Error
	return &comment, err
}

// ListByTaskID 按任务列出评论详情。
// 这里把未解决评论排在前面，能让前端默认展示更贴近“待处理讨论”视角。
func (r *taskCommentRepo) ListByTaskID(ctx context.Context, taskID int) ([]models.TaskCommentInfo, error) {
	var comments []models.TaskCommentInfo
	err := r.baseDetailQuery(ctx).
		Where("tc.task_id = ?", taskID).
		Order("tc.resolved ASC, tc.created_at ASC, tc.id ASC").
		Scan(&comments).Error
	return comments, err
}

// Update 更新评论状态并返回最新记录。
func (r *taskCommentRepo) Update(ctx context.Context, id int, updates map[string]interface{}) (*models.TaskComment, error) {
	res := r.db.WithContext(ctx).
		Model(&models.TaskComment{}).
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}

	var comment models.TaskComment
	if err := r.db.WithContext(ctx).First(&comment, id).Error; err != nil {
		return nil, err
	}
	return &comment, nil
}

// DeleteByID 删除评论。
func (r *taskCommentRepo) DeleteByID(ctx context.Context, id int) (int64, error) {
	res := r.db.WithContext(ctx).Delete(&models.TaskComment{}, id)
	return res.RowsAffected, res.Error
}

// baseDetailQuery 统一封装评论详情查询所需的用户 join。
// 这样做的好处是详情读取口径保持一致，后续扩字段时只需要改一个地方。
func (r *taskCommentRepo) baseDetailQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("task_comments tc").
		Select("tc.*, u.username as user_username, u.avatar_url as user_avatar_url").
		Joins("JOIN users u ON u.id = tc.user_id")
}
