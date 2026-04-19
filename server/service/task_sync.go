package service

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"ToDoList/server/repo"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const maxProjectSyncLimit = 200

type ProjectSyncInput struct {
	Cursor int64
	Limit  int
}

type ProjectSyncResult struct {
	Events     []models.TaskEvent `json:"events"`
	NextCursor int64              `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

type ProjectRealtimeSession struct {
	UserID    int
	Username  string
	ProjectID int
}

type TaskEventBroadcaster interface {
	BroadcastTaskEvent(ctx context.Context, event models.TaskEvent)
}

func (s *TaskService) SetTaskEventBroadcaster(broadcaster TaskEventBroadcaster) {
	s.taskEventBroadcaster = broadcaster
}

func (s *TaskService) withTaskMutation(ctx context.Context, fn func(taskRepo repo.TaskRepository, eventRepo repo.TaskEventRepository) error) error {
	if s.db == nil {
		return fn(s.repo, s.eventRepo)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(repo.NewTaskRepository(tx), repo.NewTaskEventRepository(tx))
	})
}

func (s *TaskService) appendTaskEvent(ctx context.Context, eventRepo repo.TaskEventRepository, eventType string, actorID int, task *models.Task) (*models.TaskEvent, error) {
	if eventRepo == nil || task == nil {
		return nil, nil
	}

	payload, err := marshalTaskEventPayload(task)
	if err != nil {
		return nil, fmt.Errorf("marshal task event payload: %w", err)
	}

	event, err := eventRepo.Create(ctx, &models.TaskEvent{
		EventID:     uuid.NewString(),
		ProjectID:   task.ProjectID,
		TaskID:      task.ID,
		ActorID:     actorID,
		EventType:   eventType,
		TaskVersion: task.Version,
		Payload:     payload,
	})
	if err != nil {
		return nil, fmt.Errorf("create task event: %w", err)
	}

	return event, nil
}

func (s *TaskService) publishTaskEvent(ctx context.Context, event *models.TaskEvent) {
	if s.taskEventBroadcaster == nil || event == nil {
		return
	}
	s.taskEventBroadcaster.BroadcastTaskEvent(ctx, *event)
}

func marshalTaskEventPayload(task *models.Task) (json.RawMessage, error) {
	if task == nil {
		return nil, nil
	}

	snapshot := *task
	snapshot.Members = nil

	payload, err := json.Marshal(models.TaskEventPayload{
		Task: &snapshot,
	})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(payload), nil
}

func (s *TaskService) OpenProjectRealtimeSession(ctx context.Context, lg *zap.Logger, uid, projectID int) (*ProjectRealtimeSession, error) {
	if projectID <= 0 {
		return nil, apperrors.NewParamError("invalid project id")
	}

	if _, err := s.projectRepo.GetByIDAndUserID(ctx, projectID, uid); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("project not found")
		}
		lg.Error("project.realtime.get_project_failed", zap.Int("uid", uid), zap.Int("project_id", projectID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to query project")
	}

	return &ProjectRealtimeSession{
		UserID:    uid,
		ProjectID: projectID,
	}, nil
}

func (s *TaskService) SyncProjectEvents(ctx context.Context, lg *zap.Logger, uid, projectID int, in ProjectSyncInput) (*ProjectSyncResult, error) {
	if s.eventRepo == nil {
		return nil, apperrors.NewInternalError("task sync is not configured")
	}

	if projectID <= 0 {
		return nil, apperrors.NewParamError("invalid project id")
	}

	if _, err := s.projectRepo.GetByIDAndUserID(ctx, projectID, uid); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("project not found")
		}
		lg.Error("task.sync.get_project_failed", zap.Int("uid", uid), zap.Int("project_id", projectID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to query project")
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxProjectSyncLimit {
		limit = maxProjectSyncLimit
	}

	events, err := s.eventRepo.ListByProjectAfterID(ctx, projectID, in.Cursor, limit+1)
	if err != nil {
		lg.Error("task.sync.list_events_failed", zap.Int("uid", uid), zap.Int("project_id", projectID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to load sync events")
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	nextCursor := in.Cursor
	if len(events) > 0 {
		nextCursor = events[len(events)-1].ID
	}

	return &ProjectSyncResult{
		Events:     events,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
