package service

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"ToDoList/server/repo"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	maxProjectActivityLimit = 200
	maxProjectSyncLimit     = 200
	maxTaskContentSyncLimit = 200
)

type ProjectActivityInput struct {
	Cursor int64
	Limit  int
	TaskID int
}

type TaskActivityEntry struct {
	ID           int64        `json:"id"`
	EventID      string       `json:"event_id"`
	ProjectID    int          `json:"project_id"`
	TaskID       int          `json:"task_id"`
	ActorID      int          `json:"actor_id"`
	EventType    string       `json:"event_type"`
	ActivityType string       `json:"activity_type"`
	Summary      string       `json:"summary"`
	TaskVersion  int          `json:"task_version"`
	Task         *models.Task `json:"task,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type ProjectActivityResult struct {
	Activities []TaskActivityEntry `json:"activities"`
	NextCursor int64               `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
}

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

type SavePlainDocumentContentInput struct {
	ContentMD       string
	ExpectedVersion *int
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

func (s *TaskService) ListProjectActivities(ctx context.Context, lg *zap.Logger, uid, projectID int, in ProjectActivityInput) (*ProjectActivityResult, error) {
	if s.eventRepo == nil {
		return nil, apperrors.NewInternalError("task activity is not configured")
	}
	if projectID <= 0 {
		return nil, apperrors.NewParamError("invalid project id")
	}

	if _, err := s.projectRepo.GetByIDAndUserID(ctx, projectID, uid); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("project not found")
		}
		lg.Error("task.activity.get_project_failed", zap.Int("uid", uid), zap.Int("project_id", projectID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to query project")
	}

	if in.TaskID > 0 {
		if s.repo == nil {
			return nil, apperrors.NewInternalError("task activity is not configured")
		}
		task, err := s.repo.GetByID(ctx, in.TaskID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.NewNotFoundError("document not found")
			}
			lg.Error("task.activity.get_task_failed", zap.Int("uid", uid), zap.Int("project_id", projectID), zap.Int("task_id", in.TaskID), zap.Error(err))
			return nil, apperrors.NewInternalError("failed to query document")
		}
		if task.ProjectID != projectID {
			return nil, apperrors.NewNotFoundError("document not found")
		}
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > maxProjectActivityLimit {
		limit = maxProjectActivityLimit
	}

	var (
		events []models.TaskEvent
		err    error
	)
	if in.TaskID > 0 {
		events, err = s.eventRepo.ListByProjectTaskBeforeID(ctx, projectID, in.TaskID, in.Cursor, limit+1)
	} else {
		events, err = s.eventRepo.ListByProjectBeforeID(ctx, projectID, in.Cursor, limit+1)
	}
	if err != nil {
		lg.Error("task.activity.list_events_failed", zap.Int("uid", uid), zap.Int("project_id", projectID), zap.Int("task_id", in.TaskID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to load document activities")
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	nextCursor := int64(0)
	activities := make([]TaskActivityEntry, 0, len(events))
	for _, event := range events {
		activities = append(activities, buildTaskActivityEntry(event))
		nextCursor = event.ID
	}

	return &ProjectActivityResult{
		Activities: activities,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
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
	if task.DocType == models.DocTypeDiary {
		return nil, apperrors.NewForbiddenError("diary content uses the plain Markdown save API")
	}

	role := models.RoleOwner
	if task.UserID != uid {
		if task.CollaborationMode == models.CollaborationModePrivate {
			return nil, apperrors.NewForbiddenError("no permission to access private document")
		}
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

func (s *TaskService) SavePlainDocumentContent(ctx context.Context, lg *zap.Logger, uid, taskID int, in SavePlainDocumentContentInput) (*models.Task, error, int64) {
	if taskID <= 0 {
		return nil, apperrors.NewParamError("invalid task id"), 0
	}
	if in.ExpectedVersion == nil || *in.ExpectedVersion <= 0 {
		return nil, apperrors.NewParamError("expected_version is required"), 0
	}
	if int64(len(in.ContentMD)) > DocumentImportMaxMarkdownSize {
		return nil, apperrors.NewParamError(fmt.Sprintf("content_md must be <= %d bytes", DocumentImportMaxMarkdownSize)), 0
	}

	task, err := s.repo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("document not found"), 0
		}
		lg.Error("document.content.get_task_failed", zap.Int("uid", uid), zap.Int("task_id", taskID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to query document"), 0
	}
	if task.DocType != models.DocTypeDiary {
		return nil, apperrors.NewForbiddenError("plain Markdown content save is only available for diary documents"), 0
	}
	if task.UserID != uid {
		return nil, apperrors.NewForbiddenError("only the diary owner can save content"), 0
	}

	contentMD := in.ContentMD
	return s.Update(ctx, lg, uid, task.ProjectID, task.ID, UpdateTaskInput{
		ContentMD:       &contentMD,
		ExpectedVersion: in.ExpectedVersion,
	})
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

func buildTaskActivityEntry(event models.TaskEvent) TaskActivityEntry {
	task := extractTaskEventTask(event.Payload)
	activityType, summary := describeTaskActivity(event.EventType, task)

	return TaskActivityEntry{
		ID:           event.ID,
		EventID:      event.EventID,
		ProjectID:    event.ProjectID,
		TaskID:       event.TaskID,
		ActorID:      event.ActorID,
		EventType:    event.EventType,
		ActivityType: activityType,
		Summary:      summary,
		TaskVersion:  event.TaskVersion,
		Task:         task,
		CreatedAt:    event.CreatedAt,
	}
}

func extractTaskEventTask(raw json.RawMessage) *models.Task {
	if len(raw) == 0 {
		return nil
	}

	var payload models.TaskEventPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload.Task
}

func describeTaskActivity(eventType string, task *models.Task) (string, string) {
	kindKey, kindLabel := taskActivityKind(task)

	switch eventType {
	case models.TaskEventTypeCreated:
		return fmt.Sprintf("%s.created", kindKey), fmt.Sprintf("Created %s", kindLabel)
	case models.TaskEventTypeDeleted:
		return fmt.Sprintf("%s.deleted", kindKey), fmt.Sprintf("Moved %s to trash", kindLabel)
	case models.TaskEventTypeUpdated:
		return fmt.Sprintf("%s.updated", kindKey), fmt.Sprintf("Updated %s metadata", kindLabel)
	default:
		return fmt.Sprintf("%s.event", kindKey), fmt.Sprintf("Updated %s activity", kindLabel)
	}
}

func taskActivityKind(task *models.Task) (string, string) {
	switch taskDocType(task) {
	case models.DocTypeMeeting:
		return "meeting", "meeting note"
	case models.DocTypeDiary:
		return "diary", "daily note"
	case models.DocTypeTodo:
		return "todo", "todo"
	default:
		return "document", "document"
	}
}

func taskDocType(task *models.Task) string {
	if task == nil {
		return models.DocTypeDocument
	}
	if normalized := normalizeDocType(task.DocType); normalized != "" {
		return normalized
	}
	return models.DocTypeDocument
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
