package service

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"ToDoList/server/repo"
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const maxTaskContentSyncLimit = 200

type TaskContentSession struct {
	UserID    int
	ProjectID int
	TaskID    int
	Role      string
	CanEdit   bool
}

type TaskContentSyncInput struct {
	Cursor int64
	Limit  int
}

type TaskContentSyncResult struct {
	Updates    []models.TaskContentUpdate `json:"updates"`
	NextCursor int64                      `json:"next_cursor"`
	HasMore    bool                       `json:"has_more"`
}

type AppendTaskContentUpdateInput struct {
	MessageID       string
	Update          []byte
	ContentSnapshot *string
}

type AppendTaskContentUpdateResult struct {
	Update    *models.TaskContentUpdate
	Duplicate bool
}

func (s *TaskService) OpenTaskContentSession(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*TaskContentSession, error) {
	if taskID <= 0 {
		return nil, apperrors.NewParamError("invalid task id")
	}

	task, err := s.repo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("task not found")
		}
		lg.Error("task.content.get_task_failed", zap.Int("uid", uid), zap.Int("task_id", taskID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to query task")
	}

	role := models.RoleOwner
	if task.UserID != uid {
		role, err = s.taskMemberRepo.GetMemberRole(ctx, taskID, uid)
		if err != nil {
			lg.Error("task.content.get_role_failed", zap.Int("uid", uid), zap.Int("task_id", taskID), zap.Error(err))
			return nil, apperrors.NewInternalError("failed to query task permission")
		}
	}

	canEdit := role == models.RoleOwner || role == models.RoleEditor
	if !canEdit && role != models.RoleViewer {
		return nil, apperrors.NewForbiddenError("no permission to access task content")
	}

	return &TaskContentSession{
		UserID:    uid,
		ProjectID: task.ProjectID,
		TaskID:    task.ID,
		Role:      role,
		CanEdit:   canEdit,
	}, nil
}

func (s *TaskService) SyncTaskContentUpdates(ctx context.Context, lg *zap.Logger, session TaskContentSession, in TaskContentSyncInput) (*TaskContentSyncResult, error) {
	if s.contentRepo == nil {
		return nil, apperrors.NewInternalError("task content sync is not configured")
	}
	if session.TaskID <= 0 || session.ProjectID <= 0 {
		return nil, apperrors.NewParamError("invalid task content session")
	}

	limit := normalizeTaskContentSyncLimit(in.Limit)
	updates, err := s.contentRepo.ListByTaskAfterID(ctx, session.ProjectID, session.TaskID, in.Cursor, limit+1)
	if err != nil {
		lg.Error("task.content.list_updates_failed", zap.Int("task_id", session.TaskID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to load task content updates")
	}

	hasMore := len(updates) > limit
	if hasMore {
		updates = updates[:limit]
	}

	nextCursor := in.Cursor
	if len(updates) > 0 {
		nextCursor = updates[len(updates)-1].ID
	}

	return &TaskContentSyncResult{
		Updates:    updates,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *TaskService) AppendTaskContentUpdate(ctx context.Context, lg *zap.Logger, session TaskContentSession, in AppendTaskContentUpdateInput) (*AppendTaskContentUpdateResult, error) {
	if s.contentRepo == nil {
		return nil, apperrors.NewInternalError("task content sync is not configured")
	}
	if !session.CanEdit {
		return nil, apperrors.NewForbiddenError("no permission to edit task content")
	}

	in.MessageID = strings.TrimSpace(in.MessageID)
	if in.MessageID == "" {
		return nil, apperrors.NewParamError("message_id is required")
	}
	if len(in.Update) == 0 {
		return nil, apperrors.NewParamError("content update is required")
	}

	contentUpdate := &models.TaskContentUpdate{
		MessageID:       in.MessageID,
		ProjectID:       session.ProjectID,
		TaskID:          session.TaskID,
		ActorID:         session.UserID,
		Update:          in.Update,
		ContentSnapshot: in.ContentSnapshot,
	}

	created, err := s.createTaskContentUpdate(ctx, contentUpdate)
	if err != nil {
		if isDuplicateDBError(err) {
			existing, getErr := s.contentRepo.GetByMessageID(ctx, in.MessageID)
			if getErr != nil {
				lg.Error("task.content.get_duplicate_failed", zap.String("message_id", in.MessageID), zap.Error(getErr))
				return nil, apperrors.NewInternalError("failed to load duplicate content update")
			}
			return &AppendTaskContentUpdateResult{Update: existing, Duplicate: true}, nil
		}
		lg.Error("task.content.create_update_failed", zap.Int("task_id", session.TaskID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to save task content update")
	}

	return &AppendTaskContentUpdateResult{Update: created}, nil
}

func (s *TaskService) createTaskContentUpdate(ctx context.Context, update *models.TaskContentUpdate) (*models.TaskContentUpdate, error) {
	if s.db == nil {
		created, err := s.contentRepo.Create(ctx, update)
		if err != nil {
			return nil, err
		}
		if update.ContentSnapshot != nil {
			if _, err := s.repo.UpdateContentSnapshot(ctx, update.TaskID, *update.ContentSnapshot); err != nil {
				return nil, fmt.Errorf("update task content snapshot: %w", err)
			}
		}
		return created, nil
	}

	var created *models.TaskContentUpdate
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		contentRepo := repo.NewTaskContentRepository(tx)
		taskRepo := repo.NewTaskRepository(tx)

		var createErr error
		created, createErr = contentRepo.Create(ctx, update)
		if createErr != nil {
			return createErr
		}
		if update.ContentSnapshot != nil {
			if _, err := taskRepo.UpdateContentSnapshot(ctx, update.TaskID, *update.ContentSnapshot); err != nil {
				return fmt.Errorf("update task content snapshot: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func normalizeTaskContentSyncLimit(limit int) int {
	switch {
	case limit <= 0:
		return 100
	case limit > maxTaskContentSyncLimit:
		return maxTaskContentSyncLimit
	default:
		return limit
	}
}

func isDuplicateDBError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Duplicate") || strings.Contains(msg, "1062") || strings.Contains(strings.ToLower(msg), "unique")
}
