package repo

import (
	"ToDoList/server/models"
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type TaskMemberRepository interface {
	AddMember(ctx context.Context, taskID, userID int, role string) error
	RemoveMember(ctx context.Context, taskID, userID int) error
	GetMemberRole(ctx context.Context, taskID, userID int) (string, error)
	GetMembersByTaskID(ctx context.Context, taskID int) ([]models.TaskMemberInfo, error)
}

type taskMemberRepo struct {
	db *gorm.DB
}

func NewTaskMemberRepository(db *gorm.DB) TaskMemberRepository {
	return &taskMemberRepo{db: db}
}

func (r *taskMemberRepo) AddMember(ctx context.Context, taskID, userID int, role string) error {
	member := models.TaskMember{
		TaskID:   taskID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
	}
	return r.db.WithContext(ctx).Create(&member).Error
}

func (r *taskMemberRepo) RemoveMember(ctx context.Context, taskID, userID int) error {
	return r.db.WithContext(ctx).
		Where("task_id = ? AND user_id = ?", taskID, userID).
		Delete(&models.TaskMember{}).Error
}

func (r *taskMemberRepo) GetMemberRole(ctx context.Context, taskID, userID int) (string, error) {
	var member models.TaskMember
	err := r.db.WithContext(ctx).
		Where("task_id = ? AND user_id = ?", taskID, userID).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return member.Role, nil
}

func (r *taskMemberRepo) GetMembersByTaskID(ctx context.Context, taskID int) ([]models.TaskMemberInfo, error) {
	var members []models.TaskMemberInfo
	err := r.db.WithContext(ctx).
		Table("task_members tm").
		Select("tm.user_id, tm.role, tm.joined_at, u.username as user_username, u.avatar_url as user_avatar_url").
		Joins("JOIN users u ON u.id = tm.user_id").
		Where("tm.task_id = ?", taskID).
		Scan(&members).Error
	return members, err
}
