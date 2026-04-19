package repo

import (
	"ToDoList/server/models"
	"context"

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
	DeleteWithTasks(ctx context.Context, id, userID int) (projAffected, taskAffected int64, err error)
	GetAllIDs(ctx context.Context, userID int) ([]models.ProjectIDScore, error)
}

type projectRepo struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return &projectRepo{db: db}
}

func (r *projectRepo) Create(ctx context.Context, project *models.Project) (*models.Project, error) {
	err := r.db.WithContext(ctx).Create(project).Error
	return project, err
}

func (r *projectRepo) GetByID(ctx context.Context, id int) (*models.Project, error) {
	var project models.Project
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&project).Error
	return &project, err
}

func (r *projectRepo) GetByIDAndUserID(ctx context.Context, id, userID int) (*models.Project, error) {
	var project models.Project
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&project).Error
	return &project, err
}

func (r *projectRepo) GetByUserName(ctx context.Context, userID int, name string) (*models.Project, error) {
	var project models.Project
	err := r.db.WithContext(ctx).Where("user_id = ? AND name = ?", userID, name).First(&project).Error
	return &project, err
}

func (r *projectRepo) GetByIDsAndUserID(ctx context.Context, ids []int, userID int) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.WithContext(ctx).Where("id IN ? AND user_id = ?", ids, userID).Find(&projects).Error
	return projects, err
}

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

func (r *projectRepo) DeleteWithTasks(ctx context.Context, id, userID int) (projAffected, taskAffected int64, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		taskRes := tx.Where("project_id = ?", id).Delete(&models.Task{})
		if taskRes.Error != nil {
			return taskRes.Error
		}
		taskAffected = taskRes.RowsAffected

		projRes := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&models.Project{})
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

func (r *projectRepo) GetAllIDs(ctx context.Context, userID int) ([]models.ProjectIDScore, error) {
	var items []models.ProjectIDScore
	err := r.db.WithContext(ctx).Model(&models.Project{}).
		Where("user_id = ?", userID).
		Select("id, sort_order").
		Scan(&items).Error
	return items, err
}
