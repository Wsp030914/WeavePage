package service

import (
	"ToDoList/server/cache"
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type taskRepoUpdateStub struct {
	getByIDFn               func(ctx context.Context, id int) (*models.Task, error)
	updateFn                func(ctx context.Context, id int, expectedVersion int, updates map[string]interface{}) (*models.Task, error, int64)
	updateContentSnapshotFn func(ctx context.Context, id int, content string) (int64, error)
	getByIDCall             int
	updateCall              int
	updateContentCall       int
}

func (s *taskRepoUpdateStub) Create(ctx context.Context, task *models.Task) (*models.Task, error) {
	panic("unexpected call to Create")
}

func (s *taskRepoUpdateStub) GetByID(ctx context.Context, id int) (*models.Task, error) {
	s.getByIDCall++
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	panic("unexpected call to GetByID")
}

func (s *taskRepoUpdateStub) GetByIDsAndProject(ctx context.Context, ids []int, projectID int, status string) ([]models.Task, error) {
	panic("unexpected call to GetByIDsAndProject")
}

func (s *taskRepoUpdateStub) GetByUserProjectTitle(ctx context.Context, userID, projectID int, title string) (*models.Task, error) {
	panic("unexpected call to GetByUserProjectTitle")
}

func (s *taskRepoUpdateStub) ListByProject(ctx context.Context, projectID int, status string, page, size int) ([]models.Task, int64, error) {
	panic("unexpected call to ListByProject")
}

func (s *taskRepoUpdateStub) ListByMember(ctx context.Context, userID, page, size int, status string, dueStart, dueEnd *time.Time) ([]models.Task, int64, error) {
	panic("unexpected call to ListByMember")
}

func (s *taskRepoUpdateStub) Update(ctx context.Context, id int, expectedVersion int, updates map[string]interface{}) (*models.Task, error, int64) {
	s.updateCall++
	if s.updateFn != nil {
		return s.updateFn(ctx, id, expectedVersion, updates)
	}
	panic("unexpected call to Update")
}

func (s *taskRepoUpdateStub) UpdateContentSnapshot(ctx context.Context, id int, content string) (int64, error) {
	s.updateContentCall++
	if s.updateContentSnapshotFn != nil {
		return s.updateContentSnapshotFn(ctx, id, content)
	}
	panic("unexpected call to UpdateContentSnapshot")
}

func (s *taskRepoUpdateStub) DeleteByID(ctx context.Context, id int) (int64, error) {
	panic("unexpected call to DeleteByID")
}

func (s *taskRepoUpdateStub) FindDueTasks(ctx context.Context, from, to time.Time, limit int) ([]models.Task, error) {
	panic("unexpected call to FindDueTasks")
}

func (s *taskRepoUpdateStub) MarkNotifiedDue(ctx context.Context, id int, triggeredAt time.Time) (int64, error) {
	panic("unexpected call to MarkNotifiedDue")
}

func (s *taskRepoUpdateStub) ResetNotifiedDue(ctx context.Context, id int) error {
	panic("unexpected call to ResetNotifiedDue")
}

func (s *taskRepoUpdateStub) GetAllIDs(ctx context.Context, projectID int, status string) ([]models.TaskIDScore, error) {
	panic("unexpected call to GetAllIDs")
}

type taskProjectRepoStub struct {
	getByIDFn          func(ctx context.Context, id int) (*models.Project, error)
	getByIDAndUserIDFn func(ctx context.Context, id, userID int) (*models.Project, error)
}

func (s *taskProjectRepoStub) Create(ctx context.Context, project *models.Project) (*models.Project, error) {
	panic("unexpected call to Create")
}

func (s *taskProjectRepoStub) GetByID(ctx context.Context, id int) (*models.Project, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	panic("unexpected call to GetByID")
}

func (s *taskProjectRepoStub) GetByIDAndUserID(ctx context.Context, id, userID int) (*models.Project, error) {
	if s.getByIDAndUserIDFn != nil {
		return s.getByIDAndUserIDFn(ctx, id, userID)
	}
	panic("unexpected call to GetByIDAndUserID")
}

func (s *taskProjectRepoStub) GetByIDsAndUserID(ctx context.Context, ids []int, userID int) ([]models.Project, error) {
	panic("unexpected call to GetByIDsAndUserID")
}

func (s *taskProjectRepoStub) List(ctx context.Context, userID int, page, size int) ([]models.Project, int64, error) {
	panic("unexpected call to List")
}

func (s *taskProjectRepoStub) Search(ctx context.Context, userID int, name string, page, size int) ([]models.Project, int64, error) {
	panic("unexpected call to Search")
}

func (s *taskProjectRepoStub) Update(ctx context.Context, id, userID int, updates map[string]interface{}) (*models.Project, error, int64) {
	panic("unexpected call to Update")
}

func (s *taskProjectRepoStub) DeleteWithTasks(ctx context.Context, id, userID int) (projAffected, taskAffected int64, err error) {
	panic("unexpected call to DeleteWithTasks")
}

func (s *taskProjectRepoStub) GetAllIDs(ctx context.Context, userID int) ([]models.ProjectIDScore, error) {
	panic("unexpected call to GetAllIDs")
}

type taskMemberRepoStub struct {
	getMemberRoleFn func(ctx context.Context, taskID, userID int) (string, error)
}

func (s *taskMemberRepoStub) AddMember(ctx context.Context, taskID, userID int, role string) error {
	panic("unexpected call to AddMember")
}

func (s *taskMemberRepoStub) RemoveMember(ctx context.Context, taskID, userID int) error {
	panic("unexpected call to RemoveMember")
}

func (s *taskMemberRepoStub) GetMemberRole(ctx context.Context, taskID, userID int) (string, error) {
	if s.getMemberRoleFn != nil {
		return s.getMemberRoleFn(ctx, taskID, userID)
	}
	return "", nil
}

func (s *taskMemberRepoStub) GetMembersByTaskID(ctx context.Context, taskID int) ([]models.TaskMemberInfo, error) {
	panic("unexpected call to GetMembersByTaskID")
}

type taskUserRepoStub struct{}

func (s *taskUserRepoStub) Create(ctx context.Context, user *models.User) (*models.User, error) {
	panic("unexpected call to Create")
}

func (s *taskUserRepoStub) GetByID(ctx context.Context, id int) (*models.User, error) {
	panic("unexpected call to GetByID")
}

func (s *taskUserRepoStub) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	panic("unexpected call to GetByUsername")
}

func (s *taskUserRepoStub) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	panic("unexpected call to GetByEmail")
}

func (s *taskUserRepoStub) GetByIdentity(ctx context.Context, identity string) (*models.User, error) {
	panic("unexpected call to GetByIdentity")
}

func (s *taskUserRepoStub) Update(ctx context.Context, id int, updates map[string]interface{}) (*models.User, error, int64) {
	panic("unexpected call to Update")
}

func (s *taskUserRepoStub) GetVersion(ctx context.Context, id int) (*models.User, error) {
	panic("unexpected call to GetVersion")
}

type taskCacheNoop struct{}

func (taskCacheNoop) GetDetail(ctx context.Context, uid, taskID int) (*models.Task, error) {
	return nil, cache.ErrCacheMiss
}

func (taskCacheNoop) SetDetail(ctx context.Context, uid, taskID int, task *models.Task) error {
	return nil
}

func (taskCacheNoop) DelDetail(ctx context.Context, uid, taskID int) error {
	return nil
}

func (taskCacheNoop) SetTaskIDs(ctx context.Context, pid int, status string, items []models.TaskIDScore) error {
	return nil
}

func (taskCacheNoop) GetTaskIDs(ctx context.Context, pid int, status string, page, size int) ([]int, error) {
	return nil, cache.ErrCacheMiss
}

func (taskCacheNoop) CountTaskIDs(ctx context.Context, pid int, status string) (int64, error) {
	return 0, cache.ErrCacheMiss
}

func (taskCacheNoop) AddTaskID(ctx context.Context, pid int, status string, taskID int, score float64) error {
	return nil
}

func (taskCacheNoop) RemTaskID(ctx context.Context, pid int, status string, taskID int) error {
	return nil
}

func (taskCacheNoop) MGetDetail(ctx context.Context, uid int, taskIDs []int) (map[int]*models.Task, []int, error) {
	return map[int]*models.Task{}, taskIDs, nil
}

func (taskCacheNoop) MSetDetail(ctx context.Context, uid int, tasks []models.Task) error {
	return nil
}

type taskLockCacheStub struct {
	mu           sync.Mutex
	values       map[string]string
	setNXCalls   int
	releaseCalls int
	refreshCalls int
}

func newTaskLockCacheStub() *taskLockCacheStub {
	return &taskLockCacheStub{values: make(map[string]string)}
}

func (s *taskLockCacheStub) Get(ctx context.Context, key string) (string, error) {
	panic("unexpected call to Get")
}

func (s *taskLockCacheStub) MGet(ctx context.Context, keys ...string) ([]interface{}, error) {
	panic("unexpected call to MGet")
}

func (s *taskLockCacheStub) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	panic("unexpected call to Set")
}

func (s *taskLockCacheStub) Del(ctx context.Context, keys ...string) error {
	panic("unexpected call to Del")
}

func (s *taskLockCacheStub) Exists(ctx context.Context, key string) (bool, error) {
	panic("unexpected call to Exists")
}

func (s *taskLockCacheStub) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setNXCalls++
	if _, ok := s.values[key]; ok {
		return false, nil
	}
	s.values[key] = value.(string)
	return true, nil
}

func (s *taskLockCacheStub) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := keys[0]
	owner, _ := args[0].(string)
	current, ok := s.values[key]
	if !ok || current != owner {
		return int64(0), nil
	}

	if strings.Contains(script, "pexpire") {
		s.refreshCalls++
		return int64(1), nil
	}

	delete(s.values, key)
	s.releaseCalls++
	return int64(1), nil
}

func (s *taskLockCacheStub) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	panic("unexpected call to ZAdd")
}

func (s *taskLockCacheStub) ZRem(ctx context.Context, key string, members ...interface{}) error {
	panic("unexpected call to ZRem")
}

func (s *taskLockCacheStub) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	panic("unexpected call to ZRevRange")
}

func (s *taskLockCacheStub) ZCard(ctx context.Context, key string) (int64, error) {
	panic("unexpected call to ZCard")
}

func (s *taskLockCacheStub) Expire(ctx context.Context, key string, ttl time.Duration) error {
	panic("unexpected call to Expire")
}

type taskEventRepoStub struct {
	createFn               func(ctx context.Context, event *models.TaskEvent) (*models.TaskEvent, error)
	listByProjectAfterIDFn func(ctx context.Context, projectID int, afterID int64, limit int) ([]models.TaskEvent, error)
}

func (s *taskEventRepoStub) Create(ctx context.Context, event *models.TaskEvent) (*models.TaskEvent, error) {
	if s.createFn != nil {
		return s.createFn(ctx, event)
	}
	panic("unexpected call to Create")
}

func (s *taskEventRepoStub) ListByProjectAfterID(ctx context.Context, projectID int, afterID int64, limit int) ([]models.TaskEvent, error) {
	if s.listByProjectAfterIDFn != nil {
		return s.listByProjectAfterIDFn(ctx, projectID, afterID, limit)
	}
	panic("unexpected call to ListByProjectAfterID")
}

type taskContentRepoStub struct {
	createFn            func(ctx context.Context, update *models.TaskContentUpdate) (*models.TaskContentUpdate, error)
	getByMessageIDFn    func(ctx context.Context, messageID string) (*models.TaskContentUpdate, error)
	listByTaskAfterIDFn func(ctx context.Context, projectID int, taskID int, afterID int64, limit int) ([]models.TaskContentUpdate, error)
	getLatestSnapshotFn func(ctx context.Context, projectID int, taskID int) (*models.TaskContentUpdate, error)
}

func (s *taskContentRepoStub) Create(ctx context.Context, update *models.TaskContentUpdate) (*models.TaskContentUpdate, error) {
	if s.createFn != nil {
		return s.createFn(ctx, update)
	}
	panic("unexpected call to Create")
}

func (s *taskContentRepoStub) GetByMessageID(ctx context.Context, messageID string) (*models.TaskContentUpdate, error) {
	if s.getByMessageIDFn != nil {
		return s.getByMessageIDFn(ctx, messageID)
	}
	panic("unexpected call to GetByMessageID")
}

func (s *taskContentRepoStub) ListByTaskAfterID(ctx context.Context, projectID int, taskID int, afterID int64, limit int) ([]models.TaskContentUpdate, error) {
	if s.listByTaskAfterIDFn != nil {
		return s.listByTaskAfterIDFn(ctx, projectID, taskID, afterID, limit)
	}
	panic("unexpected call to ListByTaskAfterID")
}

func (s *taskContentRepoStub) GetLatestSnapshot(ctx context.Context, projectID int, taskID int) (*models.TaskContentUpdate, error) {
	if s.getLatestSnapshotFn != nil {
		return s.getLatestSnapshotFn(ctx, projectID, taskID)
	}
	panic("unexpected call to GetLatestSnapshot")
}

type taskEventBroadcasterStub struct {
	events []models.TaskEvent
}

func (s *taskEventBroadcasterStub) BroadcastTaskEvent(ctx context.Context, event models.TaskEvent) {
	s.events = append(s.events, event)
}

func TestTaskServiceUpdate_RequiresExpectedVersion(t *testing.T) {
	t.Parallel()

	lockCache := newTaskLockCacheStub()
	repo := &taskRepoUpdateStub{
		getByIDFn: func(ctx context.Context, id int) (*models.Task, error) {
			return &models.Task{ID: id, ProjectID: 9, UserID: 7, Version: 2, Status: models.TaskTodo}, nil
		},
	}
	svc := NewTaskService(TaskServiceDeps{
		Repo:      repo,
		TaskCache: taskCacheNoop{},
		ProjectRepo: &taskProjectRepoStub{getByIDFn: func(ctx context.Context, id int) (*models.Project, error) {
			return &models.Project{ID: id, UserID: 7}, nil
		}},
		TaskMemberRepo: &taskMemberRepoStub{},
		UserRepo:       &taskUserRepoStub{},
		CacheClient:    lockCache,
	})

	title := "updated"
	updated, err, affected := svc.Update(context.Background(), zap.NewNop(), 7, 9, 1, UpdateTaskInput{
		Title: &title,
	})
	if updated != nil {
		t.Fatalf("expected no updated task, got %+v", updated)
	}
	if affected != 0 {
		t.Fatalf("expected affected rows to be 0, got %d", affected)
	}
	assertTaskServiceErrorCode(t, err, apperrors.CodeParamInvalid)
	if repo.updateCall != 0 {
		t.Fatalf("expected repository Update not to be called, got %d", repo.updateCall)
	}
	if lockCache.setNXCalls != 0 {
		t.Fatalf("expected lock acquisition not to run, got %d calls", lockCache.setNXCalls)
	}
}

func TestTaskServiceUpdate_ReturnsConflictOnVersionMismatchAfterLock(t *testing.T) {
	t.Parallel()

	lockCache := newTaskLockCacheStub()
	repo := &taskRepoUpdateStub{}
	repo.getByIDFn = func(ctx context.Context, id int) (*models.Task, error) {
		switch repo.getByIDCall {
		case 1:
			return &models.Task{ID: id, ProjectID: 9, UserID: 7, Version: 2, Status: models.TaskTodo}, nil
		case 2:
			return &models.Task{ID: id, ProjectID: 9, UserID: 7, Version: 3, Status: models.TaskTodo}, nil
		default:
			return nil, gorm.ErrRecordNotFound
		}
	}
	repo.updateFn = func(ctx context.Context, id int, expectedVersion int, updates map[string]interface{}) (*models.Task, error, int64) {
		t.Fatalf("expected repository Update not to be called")
		return nil, nil, 0
	}

	svc := NewTaskService(TaskServiceDeps{
		Repo:      repo,
		TaskCache: taskCacheNoop{},
		ProjectRepo: &taskProjectRepoStub{getByIDFn: func(ctx context.Context, id int) (*models.Project, error) {
			return &models.Project{ID: id, UserID: 7}, nil
		}},
		TaskMemberRepo: &taskMemberRepoStub{},
		UserRepo:       &taskUserRepoStub{},
		CacheClient:    lockCache,
	})

	title := "updated"
	expectedVersion := 2
	updated, err, affected := svc.Update(context.Background(), zap.NewNop(), 7, 9, 1, UpdateTaskInput{
		Title:           &title,
		ExpectedVersion: &expectedVersion,
	})
	if updated != nil {
		t.Fatalf("expected no updated task, got %+v", updated)
	}
	if affected != 0 {
		t.Fatalf("expected affected rows to be 0, got %d", affected)
	}
	assertTaskServiceErrorCode(t, err, apperrors.CodeConflict)
	if repo.updateCall != 0 {
		t.Fatalf("expected repository Update not to be called, got %d", repo.updateCall)
	}

	lockCache.mu.Lock()
	defer lockCache.mu.Unlock()
	if lockCache.setNXCalls != 1 {
		t.Fatalf("expected one lock acquisition, got %d", lockCache.setNXCalls)
	}
	if lockCache.releaseCalls != 1 {
		t.Fatalf("expected one lock release, got %d", lockCache.releaseCalls)
	}
}

func TestTaskServiceUpdate_BroadcastsPersistedEventAfterMutation(t *testing.T) {
	t.Parallel()

	lockCache := newTaskLockCacheStub()
	repo := &taskRepoUpdateStub{
		getByIDFn: func(ctx context.Context, id int) (*models.Task, error) {
			return &models.Task{ID: id, ProjectID: 9, UserID: 7, Version: 2, Status: models.TaskTodo}, nil
		},
		updateFn: func(ctx context.Context, id int, expectedVersion int, updates map[string]interface{}) (*models.Task, error, int64) {
			if expectedVersion != 2 {
				t.Fatalf("unexpected expected version: got %d", expectedVersion)
			}
			return &models.Task{ID: id, ProjectID: 9, UserID: 7, Version: 3, Status: models.TaskDone}, nil, 1
		},
	}
	eventRepo := &taskEventRepoStub{createFn: func(ctx context.Context, event *models.TaskEvent) (*models.TaskEvent, error) {
		event.ID = 21
		return event, nil
	}}
	broadcaster := &taskEventBroadcasterStub{}
	svc := NewTaskService(TaskServiceDeps{
		Repo:      repo,
		EventRepo: eventRepo,
		TaskCache: taskCacheNoop{},
		ProjectRepo: &taskProjectRepoStub{getByIDFn: func(ctx context.Context, id int) (*models.Project, error) {
			return &models.Project{ID: id, UserID: 7}, nil
		}},
		TaskMemberRepo: &taskMemberRepoStub{},
		UserRepo:       &taskUserRepoStub{},
		CacheClient:    lockCache,
	})
	svc.SetTaskEventBroadcaster(broadcaster)

	status := models.TaskDone
	expectedVersion := 2
	updated, err, affected := svc.Update(context.Background(), zap.NewNop(), 7, 9, 1, UpdateTaskInput{
		Status:          &status,
		ExpectedVersion: &expectedVersion,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected affected rows 1, got %d", affected)
	}
	if updated == nil || updated.Version != 3 {
		t.Fatalf("expected updated task version 3, got %+v", updated)
	}
	if len(broadcaster.events) != 1 {
		t.Fatalf("expected one broadcast event, got %d", len(broadcaster.events))
	}
	event := broadcaster.events[0]
	if event.ID != 21 {
		t.Fatalf("expected event cursor 21, got %d", event.ID)
	}
	if event.EventType != models.TaskEventTypeUpdated {
		t.Fatalf("expected updated event, got %s", event.EventType)
	}
	if event.TaskVersion != 3 {
		t.Fatalf("expected task version 3 in event, got %d", event.TaskVersion)
	}
}

func TestTaskServiceSyncProjectEvents_ReturnsCursorPage(t *testing.T) {
	t.Parallel()

	var gotLimit int
	svc := NewTaskService(TaskServiceDeps{
		Repo: &taskRepoUpdateStub{},
		EventRepo: &taskEventRepoStub{listByProjectAfterIDFn: func(ctx context.Context, projectID int, afterID int64, limit int) ([]models.TaskEvent, error) {
			gotLimit = limit
			if projectID != 9 {
				t.Fatalf("unexpected project id: got %d", projectID)
			}
			if afterID != 10 {
				t.Fatalf("unexpected cursor: got %d", afterID)
			}
			return []models.TaskEvent{
				{ID: 11, ProjectID: projectID, EventType: models.TaskEventTypeCreated},
				{ID: 12, ProjectID: projectID, EventType: models.TaskEventTypeUpdated},
				{ID: 13, ProjectID: projectID, EventType: models.TaskEventTypeDeleted},
			}, nil
		}},
		TaskCache: taskCacheNoop{},
		ProjectRepo: &taskProjectRepoStub{getByIDAndUserIDFn: func(ctx context.Context, id, userID int) (*models.Project, error) {
			return &models.Project{ID: id, UserID: userID}, nil
		}},
		TaskMemberRepo: &taskMemberRepoStub{},
		UserRepo:       &taskUserRepoStub{},
	})

	result, err := svc.SyncProjectEvents(context.Background(), zap.NewNop(), 7, 9, ProjectSyncInput{
		Cursor: 10,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("SyncProjectEvents returned error: %v", err)
	}
	if gotLimit != 3 {
		t.Fatalf("expected repository limit 3, got %d", gotLimit)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result.Events))
	}
	if !result.HasMore {
		t.Fatal("expected HasMore to be true")
	}
	if result.NextCursor != 12 {
		t.Fatalf("expected next cursor 12, got %d", result.NextCursor)
	}
}

func TestTaskServiceOpenProjectRealtimeSession_ChecksProjectAccess(t *testing.T) {
	t.Parallel()

	svc := NewTaskService(TaskServiceDeps{
		ProjectRepo: &taskProjectRepoStub{getByIDAndUserIDFn: func(ctx context.Context, id, userID int) (*models.Project, error) {
			if id != 9 || userID != 7 {
				t.Fatalf("unexpected access scope: project=%d uid=%d", id, userID)
			}
			return &models.Project{ID: id, UserID: userID}, nil
		}},
	})

	session, err := svc.OpenProjectRealtimeSession(context.Background(), zap.NewNop(), 7, 9)
	if err != nil {
		t.Fatalf("OpenProjectRealtimeSession returned error: %v", err)
	}
	if session.UserID != 7 || session.ProjectID != 9 {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestTaskServiceOpenTaskContentSession_OwnerCanEdit(t *testing.T) {
	t.Parallel()

	svc := NewTaskService(TaskServiceDeps{
		Repo: &taskRepoUpdateStub{getByIDFn: func(ctx context.Context, id int) (*models.Task, error) {
			return &models.Task{ID: id, ProjectID: 9, UserID: 7}, nil
		}},
		TaskMemberRepo: &taskMemberRepoStub{},
	})

	session, err := svc.OpenTaskContentSession(context.Background(), zap.NewNop(), 7, 1)
	if err != nil {
		t.Fatalf("OpenTaskContentSession returned error: %v", err)
	}
	if session.Role != models.RoleOwner {
		t.Fatalf("expected owner role, got %s", session.Role)
	}
	if !session.CanEdit {
		t.Fatal("expected owner to edit content")
	}
}

func TestTaskServiceAppendTaskContentUpdate_RejectsViewer(t *testing.T) {
	t.Parallel()

	svc := NewTaskService(TaskServiceDeps{
		Repo:        &taskRepoUpdateStub{},
		ContentRepo: &taskContentRepoStub{},
	})

	_, err := svc.AppendTaskContentUpdate(context.Background(), zap.NewNop(), TaskContentSession{
		UserID:    8,
		ProjectID: 9,
		TaskID:    1,
		Role:      models.RoleViewer,
		CanEdit:   false,
	}, AppendTaskContentUpdateInput{
		MessageID: "msg-1",
		Update:    []byte("update"),
	})
	assertTaskServiceErrorCode(t, err, apperrors.CodeForbidden)
}

func TestTaskServiceAppendTaskContentUpdate_PersistsAndRefreshesSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := "hello"
	repoStub := &taskRepoUpdateStub{
		updateContentSnapshotFn: func(ctx context.Context, id int, content string) (int64, error) {
			if id != 1 {
				t.Fatalf("unexpected task id: got %d", id)
			}
			if content != snapshot {
				t.Fatalf("unexpected snapshot: got %q", content)
			}
			return 1, nil
		},
	}
	contentRepo := &taskContentRepoStub{
		createFn: func(ctx context.Context, update *models.TaskContentUpdate) (*models.TaskContentUpdate, error) {
			if update.MessageID != "msg-1" {
				t.Fatalf("unexpected message id: got %s", update.MessageID)
			}
			if string(update.Update) != "update" {
				t.Fatalf("unexpected update payload: got %q", string(update.Update))
			}
			update.ID = 11
			return update, nil
		},
	}
	svc := NewTaskService(TaskServiceDeps{
		Repo:        repoStub,
		ContentRepo: contentRepo,
	})

	result, err := svc.AppendTaskContentUpdate(context.Background(), zap.NewNop(), TaskContentSession{
		UserID:    7,
		ProjectID: 9,
		TaskID:    1,
		Role:      models.RoleEditor,
		CanEdit:   true,
	}, AppendTaskContentUpdateInput{
		MessageID:       "msg-1",
		Update:          []byte("update"),
		ContentSnapshot: &snapshot,
	})
	if err != nil {
		t.Fatalf("AppendTaskContentUpdate returned error: %v", err)
	}
	if result == nil || result.Update == nil {
		t.Fatal("expected persisted update")
	}
	if result.Update.ID != 11 {
		t.Fatalf("expected update id 11, got %d", result.Update.ID)
	}
	if repoStub.updateContentCall != 1 {
		t.Fatalf("expected one content snapshot refresh, got %d", repoStub.updateContentCall)
	}
}

func TestTaskServiceSyncTaskContentUpdates_ReturnsCursorPage(t *testing.T) {
	t.Parallel()

	var gotLimit int
	svc := NewTaskService(TaskServiceDeps{
		ContentRepo: &taskContentRepoStub{listByTaskAfterIDFn: func(ctx context.Context, projectID int, taskID int, afterID int64, limit int) ([]models.TaskContentUpdate, error) {
			gotLimit = limit
			if projectID != 9 || taskID != 1 {
				t.Fatalf("unexpected scope: project=%d task=%d", projectID, taskID)
			}
			if afterID != 10 {
				t.Fatalf("unexpected cursor: got %d", afterID)
			}
			return []models.TaskContentUpdate{
				{ID: 11, ProjectID: projectID, TaskID: taskID, MessageID: "msg-11"},
				{ID: 12, ProjectID: projectID, TaskID: taskID, MessageID: "msg-12"},
				{ID: 13, ProjectID: projectID, TaskID: taskID, MessageID: "msg-13"},
			}, nil
		}},
	})

	result, err := svc.SyncTaskContentUpdates(context.Background(), zap.NewNop(), TaskContentSession{
		UserID:    7,
		ProjectID: 9,
		TaskID:    1,
		CanEdit:   true,
	}, TaskContentSyncInput{
		Cursor: 10,
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("SyncTaskContentUpdates returned error: %v", err)
	}
	if gotLimit != 3 {
		t.Fatalf("expected repository limit 3, got %d", gotLimit)
	}
	if len(result.Updates) != 2 {
		t.Fatalf("expected 2 updates, got %d", len(result.Updates))
	}
	if !result.HasMore {
		t.Fatal("expected HasMore to be true")
	}
	if result.NextCursor != 12 {
		t.Fatalf("expected next cursor 12, got %d", result.NextCursor)
	}
}

func assertTaskServiceErrorCode(t *testing.T, err error, code apperrors.Code) {
	t.Helper()

	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *errors.Error, got %T (%v)", err, err)
	}
	if appErr.Code != code {
		t.Fatalf("unexpected error code: got %d want %d", appErr.Code, code)
	}
}
