package repo

// 文件说明：这个文件负责用户数据的持久化访问。
// 实现方式：把用户基础查询、身份查询、资料更新和 token_version 读取统一收口到仓储层。
// 这样做的好处是服务层不需要直接拼 GORM 查询，账号规则和数据库细节可以保持清晰分层。

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

// NewUserRepository 创建用户仓储。
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepo{db: db}
}

// Create 写入一条用户记录。
func (r *userRepo) Create(ctx context.Context, user *models.User) (*models.User, error) {
	err := r.db.WithContext(ctx).Create(user).Error
	return user, err
}

// GetByID 按主键读取用户资料。
func (r *userRepo) GetByID(ctx context.Context, id int) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByUsername 按用户名读取用户。
func (r *userRepo) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail 按邮箱读取用户。
func (r *userRepo) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByIdentity 按“用户名或邮箱”查询用户。
// 把两种登录入口统一到 repo 层，是为了让上层登录逻辑不需要关心具体匹配字段。
func (r *userRepo) GetByIdentity(ctx context.Context, identity string) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Where("username = ? OR email = ?", identity, identity).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 更新用户字段并回读最新记录。
// 这里回读最新用户，是为了让服务层直接拿到 token_version、头像地址等更新后的最终状态。
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

// GetVersion 只读取用户 ID 和 token_version。
// 只选最小必要字段可以降低鉴权链路的查询成本。
func (r *userRepo) GetVersion(ctx context.Context, id int) (*models.User, error) {
	var user models.User
	err := r.db.WithContext(ctx).Select("id, token_version").
		Where("id = ?", id).
		Take(&user).Error
	return &user, err
}
