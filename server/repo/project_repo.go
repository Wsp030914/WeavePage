package repo

// 文件说明：这个文件封装项目空间的数据库访问与项目级联删除逻辑。
// 实现方式：把项目查找、分页、搜索、更新、删除和排序读取统一放在仓储层处理。
// 这样做的好处是服务层只关心业务编排，不直接拼接 SQL 或 GORM 细节。

import (
	"ToDoList/server/models"
	"context"
	"time"

	"gorm.io/gorm"
)

type ProjectRepository interface {
	Create(ctx context.Context, project *models.Project) (*models.Project, error)
	GetByID(ctx context.Context, id int) (*models.Project, error)
	GetByIDAndUserID(ctx context.Context, id, userID int) (*models.Project, error)
	GetByUserName(ctx context.Context, userID int, name string) (*models.Project, error)
	GetByIDsAndUserID(ctx context.Context, ids []int, userID int) ([]models.Project, error)
	List(ctx context.Context, userID int, page, size int) ([]models.Project, int64, error)
	Search(ctx context.Context, userID int, name string, page, size int) ([]models.Project, int64, error)
	Update(ctx context.Context, id, userID int, updates map[string]interface{}) (*models.Project, error, int64)
	GetDeletedByIDAndUser(ctx context.Context, id, userID int) (*models.Project, error)
	ListDeletedByUser(ctx context.Context, userID, page, size int) ([]models.Project, int64, error)
	SoftDeleteByID(ctx context.Context, id, userID, deletedBy int, deletedAt time.Time, trashedName, deletedName string) (int64, error)
	RestoreByID(ctx context.Context, id, userID int, name string) (int64, error)
	DeleteWithTasks(ctx context.Context, id, userID int) (projAffected, taskAffected int64, err error)
	GetAllIDs(ctx context.Context, userID int) ([]models.ProjectIDScore, error)
}

type projectRepo struct {
	db *gorm.DB
}

// NewProjectRepository 创建项目仓储。
func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return &projectRepo{db: db}
}

// Create 写入一个项目空间。
func (r *projectRepo) Create(ctx context.Context, project *models.Project) (*models.Project, error) {
	err := r.db.WithContext(ctx).Create(project).Error
	return project, err
}

// GetByID 按项目 ID 读取项目。
func (r *projectRepo) GetByID(ctx context.Context, id int) (*models.Project, error) {
	var project models.Project
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&project).Error
	return &project, err
}

// GetByIDAndUserID 按项目 ID 和所属用户读取项目。
// 把 user_id 条件放进仓储查询，是为了让服务层权限判断尽量建立在最小结果集上。
func (r *projectRepo) GetByIDAndUserID(ctx context.Context, id, userID int) (*models.Project, error) {
	var project models.Project
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&project).Error
	return &project, err
}

// GetByUserName 查询某个用户名下的指定项目名称。
func (r *projectRepo) GetByUserName(ctx context.Context, userID int, name string) (*models.Project, error) {
	var project models.Project
	err := r.db.WithContext(ctx).Where("user_id = ? AND name = ?", userID, name).First(&project).Error
	return &project, err
}

func (r *projectRepo) GetDeletedByIDAndUser(ctx context.Context, id, userID int) (*models.Project, error) {
	var project models.Project
	err := r.db.WithContext(ctx).
		Unscoped().
		Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).
		First(&project).Error
	if err != nil {
		return nil, err
	}
	restoreDeletedProjectName(&project)
	return &project, nil
}

// GetByIDsAndUserID 批量查询项目详情。
// 这个批量接口主要服务于列表缓存 miss 后的按需回源。
func (r *projectRepo) GetByIDsAndUserID(ctx context.Context, ids []int, userID int) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.WithContext(ctx).Where("id IN ? AND user_id = ?", ids, userID).Find(&projects).Error
	return projects, err
}

// List 分页返回用户的项目列表。
// 这里先 count 再查分页，是为了给前端返回稳定总数并保留空列表的显式语义。
func (r *projectRepo) List(ctx context.Context, userID int, page, size int) ([]models.Project, int64, error) {
	var items []models.Project
	var total int64

	countDB := r.db.WithContext(ctx).Model(&models.Project{}).
		Where("user_id = ?", userID)

	if err := countDB.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []models.Project{}, 0, nil
	}

	err := r.db.WithContext(ctx).Model(&models.Project{}).
		Where("user_id = ?", userID).
		Order("sort_order DESC, id DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error

	return items, total, err
}

// Search 按名称搜索项目。
// 搜索和普通列表复用同一个仓储层，但不复用缓存，是因为模糊查询结果更适合实时从库里取。
func (r *projectRepo) Search(ctx context.Context, userID int, name string, page, size int) ([]models.Project, int64, error) {
	var items []models.Project
	var total int64

	db := r.db.WithContext(ctx).Model(&models.Project{}).
		Where("user_id = ?", userID)

	if name != "" {
		db = db.Where("name LIKE ?", "%"+name+"%")
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []models.Project{}, 0, nil
	}

	err := db.Order("sort_order DESC, id DESC").
		Offset((page - 1) * size).
		Limit(size).
		Find(&items).Error

	return items, total, err
}

func (r *projectRepo) ListDeletedByUser(ctx context.Context, userID, page, size int) ([]models.Project, int64, error) {
	var items []models.Project
	var total int64
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}

	tx := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.Project{}).
		Where("user_id = ? AND deleted_at IS NOT NULL", userID)
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []models.Project{}, 0, nil
	}

	if err := tx.Order("deleted_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	for i := range items {
		restoreDeletedProjectName(&items[i])
	}
	return items, total, nil
}

// Update 更新项目并回读最新记录。
func (r *projectRepo) Update(ctx context.Context, id, userID int, updates map[string]interface{}) (*models.Project, error, int64) {
	res := r.db.WithContext(ctx).Model(&models.Project{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates)
	if res.Error != nil {
		return nil, res.Error, 0
	}

	var project models.Project
	if err := r.db.WithContext(ctx).First(&project, "id = ?", id).Error; err != nil {
		return nil, err, 0
	}
	return &project, nil, res.RowsAffected
}

func (r *projectRepo) SoftDeleteByID(ctx context.Context, id, userID, deletedBy int, deletedAt time.Time, trashedName, deletedName string) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&models.Project{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{
			"name":         trashedName,
			"deleted_name": deletedName,
			"deleted_by":   deletedBy,
			"deleted_at":   deletedAt,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return res.RowsAffected, nil
}

func (r *projectRepo) RestoreByID(ctx context.Context, id, userID int, name string) (int64, error) {
	res := r.db.WithContext(ctx).
		Unscoped().
		Model(&models.Project{}).
		Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", id, userID).
		Updates(map[string]interface{}{
			"name":         name,
			"deleted_name": "",
			"deleted_by":   nil,
			"deleted_at":   nil,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return res.RowsAffected, nil
}

// DeleteWithTasks 在事务里删除项目及其关联任务数据。
// 这里显式先删评论和成员，再删任务和项目，是为了避免数据库外键或关联残留导致项目删除不完整。
func (r *projectRepo) DeleteWithTasks(ctx context.Context, id, userID int) (projAffected, taskAffected int64, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		taskScope := tx.Unscoped().Model(&models.Task{}).Select("id").Where("project_id = ?", id)

		if err := tx.Where("task_id IN (?)", taskScope).Delete(&models.TaskComment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id IN (?)", taskScope).Delete(&models.TaskMember{}).Error; err != nil {
			return err
		}

		taskRes := tx.Unscoped().Where("project_id = ?", id).Delete(&models.Task{})
		if taskRes.Error != nil {
			return taskRes.Error
		}
		taskAffected = taskRes.RowsAffected

		projRes := tx.Unscoped().Where("id = ? AND user_id = ?", id, userID).Delete(&models.Project{})
		if projRes.Error != nil {
			return projRes.Error
		}
		projAffected = projRes.RowsAffected

		if projAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return nil
	})
	return
}

// GetAllIDs 返回构建项目排序缓存所需的最小字段集合。
func (r *projectRepo) GetAllIDs(ctx context.Context, userID int) ([]models.ProjectIDScore, error) {
	var items []models.ProjectIDScore
	err := r.db.WithContext(ctx).Model(&models.Project{}).
		Where("user_id = ?", userID).
		Select("id, sort_order").
		Scan(&items).Error
	return items, err
}

func restoreDeletedProjectName(project *models.Project) {
	if project == nil {
		return
	}
	if project.DeletedName != "" {
		project.Name = project.DeletedName
	}
}
