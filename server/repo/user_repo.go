package repo

import (
	"ToDoList/server/models"
	"context"

	"gorm.io/gorm"
)

type UserRepository interface {
	Create(ctx context.Context, user *models.User) (*models.User, error)
	GetByID(ctx context.Context, id int) (*models.User, error)
	GetByUsername(ctx context.Context, username string) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByIdentity(ctx context.Context, identity string) (*models.User, error)
	Update(ctx context.Context, id int, updates map[string]interface{}) (*models.User, error, int64)
	GetVersion(ctx context.Context, id int) (*models.User, error)
}

type userRepo struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepo{db: db}
}

func (r *userRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	err := r.db.WithContext(ctx).Create(user).Error
	return user, err
}

func (r *userRepo) GetByID(ctx context.Context, id int) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) GetByIdentity(ctx context.Context, identity string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("username = ? OR email = ?", identity, identity).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepo) Update(ctx context.Context, id int, updates map[string]interface{}) (*models.User, error, int64) {
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return nil, res.Error, 0
	}

	var user models.User
	if err := r.db.WithContext(ctx).First(&user, id).Error; err != nil {
		return nil, err, 0
	}
	return &user, nil, res.RowsAffected
}

func (r *userRepo) GetVersion(ctx context.Context, id int) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Select("id, token_version").
		Where("id = ?", id).
		Take(&user).Error
	return &user, err
}
