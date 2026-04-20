package repo

// 文件说明：这个文件封装任务与文档的数据访问，包括软删除、恢复、分页、版本更新与到期扫描。
// 实现方式：把任务相关查询、事务删除和回收站读写集中到仓储层。
// 这样做的好处是数据访问规则统一，服务层可以专注业务编排。
import (
	"ToDoList/server/models"
	"context"
	"time"

	"gorm.io/gorm"
)

type TaskRepository interface {
	Create(ctx context.Context, task *models.Task) (*models.Task, error)
	GetByID(ctx context.Context, id int) (*models.Task, error)
	GetByIDsAndProject(ctx context.Context, ids []int, projectID int, status string) ([]models.Task, error)
	GetByUserProjectTitle(ctx context.Context, userID, projectID int, title string) (*models.Task, error)
	GetDeletedByIDAndUser(ctx context.Context, id, userID int) (*models.Task, error)
	ListByProject(ctx context.Context, projectID int, status string, page, size int) ([]models.Task, int64, error)
	ListByMember(ctx context.Context, userID, page, size int, status string, dueStart, dueEnd *time.Time) ([]models.Task, int64, error)
	SearchByUser(ctx context.Context, userID int, query string, limit int) ([]models.Task, error)
	ListDeletedByUser(ctx context.Context, userID, page, size int) ([]models.Task, int64, error)
	Update(ctx context.Context, id int, expectedVersion int, updates map[string]interface{}) (*models.Task, error, int64)
	UpdateContentSnapshot(ctx context.Context, id int, content string) (int64, error)
	SoftDeleteByID(ctx context.Context, id int, deletedBy int, deletedAt time.Time, trashedTitle, deletedTitle string) (int64, error)
	RestoreByID(ctx context.Context, id int, title string) (int64, error)
	DeleteByID(ctx context.Context, id int) (int64, error)
	FindDueTasks(ctx context.Context, from, to time.Time, limit int) ([]models.Task, error)
	MarkNotifiedDue(ctx context.Context, id int, triggeredAt time.Time) (int64, error)
	ResetNotifiedDue(ctx context.Context, id int) error
	GetAllIDs(ctx context.Context, projectID int, status string) ([]models.TaskIDScore, error)
}

type taskRepo struct {
	db *gorm.DB
}

// NewTaskRepository 用 GORM 连接创建任务仓储实现。
// 这里接收的是抽象 db 句柄，所以既可以传全局连接，也可以在事务里传 tx。
func NewTaskRepository(db *gorm.DB) TaskRepository {
	return &taskRepo{db: db}
}

func (r *taskRepo) Create(ctx context.Context, task *models.Task) (*models.Task, error) {
	err := r.db.WithContext(ctx).Create(task).Error
	return task, err
}

func (r *taskRepo) GetByID(ctx context.Context, id int) (*models.Task, error) {
	var task models.Task
	err := r.db.WithContext(ctx).First(&task, id).Error
	return &task, err
}

func (r *taskRepo) GetByUserProjectTitle(ctx context.Context, userID, projectID int, title string) (*models.Task, error) {
	var task models.Task
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND project_id = ? AND title = ?", userID, projectID, title).
		First(&task).Error
	return &task, err
}

func (r *taskRepo) GetDeletedByIDAndUser(ctx context.Context, id, userID int) (*models.Task, error) {
	var task models.Task
	err := r.db.WithContext(ctx).
		Unscoped().
		Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	restoreDeletedTaskTitle(&task)
	return &task, nil
}

func (r *taskRepo) GetByIDsAndProject(ctx context.Context, ids []int, projectID int, status string) ([]models.Task, error) {
	if len(ids) == 0 {
		return []models.Task{}, nil
	}

	var tasks []models.Task
	db := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Where("project_id = ? AND id IN ?", projectID, ids)

	if status != "" {
		db = db.Where("status = ?", status)
	}

	err := db.Find(&tasks).Error
	return tasks, err
}

func (r *taskRepo) ListByProject(ctx context.Context, projectID int, status string, page, size int) ([]models.Task, int64, error) {
	var tasks []models.Task
	var total int64

	tx := r.db.WithContext(ctx).Model(&models.Task{}).Where("project_id = ?", projectID)
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.Task{}, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	offset := (page - 1) * size
	err := tx.Order("sort_order DESC, priority DESC").
		Limit(size).
		Offset(offset).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

func (r *taskRepo) ListByMember(ctx context.Context, userID, page, size int, status string, dueStart, dueEnd *time.Time) ([]models.Task, int64, error) {
	var tasks []models.Task
	var total int64

	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}

	applyFilters := func(db *gorm.DB) *gorm.DB {
		if status != "" {
			db = db.Where("tasks.status = ?", status)
		}
		if dueStart != nil {
			db = db.Where("tasks.due_at IS NOT NULL AND tasks.due_at >= ?", *dueStart)
		}
		if dueEnd != nil {
			db = db.Where("tasks.due_at IS NOT NULL AND tasks.due_at <= ?", *dueEnd)
		}
		return db
	}

	countDB := applyFilters(
		r.db.WithContext(ctx).Model(&models.Task{}).
			Joins("LEFT JOIN task_members tm ON tm.task_id = tasks.id").
			Where("tasks.user_id = ? OR tm.user_id = ?", userID, userID),
	).Distinct("tasks.id")

	if err := countDB.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.Task{}, 0, nil
	}

	offset := (page - 1) * size
	queryDB := applyFilters(
		r.db.WithContext(ctx).Model(&models.Task{}).
			Select("tasks.*").
			Joins("LEFT JOIN task_members tm ON tm.task_id = tasks.id").
			Where("tasks.user_id = ? OR tm.user_id = ?", userID, userID),
	)

	err := queryDB.Distinct().
		Order("tasks.sort_order DESC, tasks.priority DESC").
		Limit(size).
		Offset(offset).
		Find(&tasks).Error
	if err != nil {
		return nil, 0, err
	}

	return tasks, total, nil
}

func (r *taskRepo) SearchByUser(ctx context.Context, userID int, query string, limit int) ([]models.Task, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	like := "%" + query + "%"
	var tasks []models.Task
	err := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Select("tasks.*").
		Joins("JOIN projects p ON p.id = tasks.project_id AND p.deleted_at IS NULL").
		Where("tasks.user_id = ?", userID).
		Where("(tasks.title LIKE ? OR tasks.content_md LIKE ? OR tasks.doc_type LIKE ? OR p.name LIKE ?)", like, like, like, like).
		Order("tasks.updated_at DESC, tasks.id DESC").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

// ListDeletedByUser 读取当前用户回收站里的任务/文档。
// 使用 Unscoped 是为了显式包含软删除记录，而活动列表路径不会走这里。
func (r *taskRepo) ListDeletedByUser(ctx context.Context, userID, page, size int) ([]models.Task, int64, error) {
	var tasks []models.Task
	var total int64

	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}

	tx := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.Task{}).
		Where("user_id = ? AND deleted_at IS NOT NULL", userID)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.Task{}, 0, nil
	}

	offset := (page - 1) * size
	if err := tx.Order("deleted_at DESC, id DESC").Limit(size).Offset(offset).Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	for i := range tasks {
		restoreDeletedTaskTitle(&tasks[i])
	}
	return tasks, total, nil
}

func (r *taskRepo) Update(ctx context.Context, id int, expectedVersion int, updates map[string]interface{}) (*models.Task, error, int64) {
	updateDoc := make(map[string]interface{}, len(updates)+1)
	for key, value := range updates {
		updateDoc[key] = value
	}
	updateDoc["version"] = gorm.Expr("version + 1")

	res := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		Updates(updateDoc)
	if res.Error != nil {
		return nil, res.Error, 0
	}

	var task models.Task
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&task).Error; err != nil {
		return nil, err, 0
	}
	return &task, nil, res.RowsAffected
}

func (r *taskRepo) UpdateContentSnapshot(ctx context.Context, id int, content string) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Where("id = ?", id).
		Update("content_md", content)
	return res.RowsAffected, res.Error
}

// SoftDeleteByID 通过“改标题 + 标记删除元数据”的方式实现软删除。
// 改标题的原因是释放原有唯一索引占位，保证同一空间后续可以创建同名新文档。
func (r *taskRepo) SoftDeleteByID(ctx context.Context, id int, deletedBy int, deletedAt time.Time, trashedTitle, deletedTitle string) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"title":         trashedTitle,
			"deleted_title": deletedTitle,
			"deleted_by":    deletedBy,
			"deleted_at":    deletedAt,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return res.RowsAffected, nil
}

// RestoreByID 把软删除记录恢复回活动文档。
// 恢复时会把 title 还原，把 deleted_* 字段清空，重新回到正常查询路径。
func (r *taskRepo) RestoreByID(ctx context.Context, id int, title string) (int64, error) {
	res := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.Task{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).
		Updates(map[string]interface{}{
			"title":         title,
			"deleted_title": "",
			"deleted_by":    nil,
			"deleted_at":    nil,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return res.RowsAffected, nil
}

// DeleteByID 执行真正的硬删除。
// 这里显式删评论和成员，是为了避免任务被彻底删除后留下孤立关联记录。
func (r *taskRepo) DeleteByID(ctx context.Context, id int) (int64, error) {
	var affected int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", id).Delete(&models.TaskComment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", id).Delete(&models.TaskMember{}).Error; err != nil {
			return err
		}
		res := tx.Unscoped().Where("id = ?", id).Delete(&models.Task{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		affected = res.RowsAffected
		return nil
	})
	return affected, err
}

func (r *taskRepo) FindDueTasks(ctx context.Context, from, to time.Time, limit int) ([]models.Task, error) {
	var tasks []models.Task
	err := r.db.WithContext(ctx).
		Where("status = ? AND notified = ? AND due_at IS NOT NULL AND due_at >= ? AND due_at < ?",
			"todo", false, from, to).
		Order("due_at ASC").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

func (r *taskRepo) MarkNotifiedDue(ctx context.Context, id int, triggeredAt time.Time) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&models.Task{}).
		Where("id = ? AND status = ? AND notified = ? AND due_at IS NOT NULL AND due_at <= ?",
			id, models.TaskTodo, false, triggeredAt).
		Update("notified", true)
	return res.RowsAffected, res.Error
}

func (r *taskRepo) ResetNotifiedDue(ctx context.Context, id int) error {
	return r.db.WithContext(ctx).
		Model(&models.Task{}).
		Where("id = ? AND status = ?", id, models.TaskTodo).
		Update("notified", false).Error
}

func (r *taskRepo) GetAllIDs(ctx context.Context, projectID int, status string) ([]models.TaskIDScore, error) {
	var items []models.TaskIDScore
	db := r.db.WithContext(ctx).Model(&models.Task{}).
		Select("id, sort_order").
		Where("project_id = ?", projectID)

	if status != "" {
		db = db.Where("status = ?", status)
	}

	err := db.Order("sort_order DESC, priority DESC").Scan(&items).Error
	return items, err
}

func restoreDeletedTaskTitle(task *models.Task) {
	if task == nil {
		return
	}
	if task.DeletedTitle != "" {
		task.Title = task.DeletedTitle
	}
}
