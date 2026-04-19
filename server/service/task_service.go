package service

import (
	"ToDoList/server/async"
	"ToDoList/server/cache"
	"ToDoList/server/config"
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"ToDoList/server/repo"
	"ToDoList/server/utils"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	dueScanInterval = time.Minute
	dueScanWindow   = 5 * time.Minute
	dueScanLimit    = 100

	listCacheRebuildLockTTL    = 30 * time.Second
	listCacheRebuildLockWait   = 200 * time.Millisecond
	listCacheRebuildQueryLimit = 20 * time.Second
)

type TaskService struct {
	repo                   repo.TaskRepository
	eventRepo              repo.TaskEventRepository
	contentRepo            repo.TaskContentRepository
	taskCache              cache.TaskCache
	projectRepo            repo.ProjectRepository
	projectCache           cache.ProjectCache
	taskMemberRepo         repo.TaskMemberRepository
	userRepo               repo.UserRepository
	db                     *gorm.DB
	dueScheduler           DueScheduler
	localDuePollingEnabled bool
	cacheClient            cache.Cache
	sf                     singleflight.Group
	bus                    async.IEventBus
	taskEventBroadcaster   TaskEventBroadcaster
}

type TaskServiceDeps struct {
	Repo                   repo.TaskRepository
	EventRepo              repo.TaskEventRepository
	ContentRepo            repo.TaskContentRepository
	TaskCache              cache.TaskCache
	ProjectRepo            repo.ProjectRepository
	ProjectCache           cache.ProjectCache
	TaskMemberRepo         repo.TaskMemberRepository
	UserRepo               repo.UserRepository
	DB                     *gorm.DB
	DueScheduler           DueScheduler
	LocalDuePollingEnabled bool
	CacheClient            cache.Cache
	Bus                    async.IEventBus
}

func NewTaskService(deps TaskServiceDeps) *TaskService {
	scheduler := deps.DueScheduler
	if scheduler == nil {
		scheduler = noopDueScheduler{}
	}
	return &TaskService{
		repo:                   deps.Repo,
		eventRepo:              deps.EventRepo,
		contentRepo:            deps.ContentRepo,
		taskCache:              deps.TaskCache,
		projectRepo:            deps.ProjectRepo,
		projectCache:           deps.ProjectCache,
		taskMemberRepo:         deps.TaskMemberRepo,
		userRepo:               deps.UserRepo,
		db:                     deps.DB,
		dueScheduler:           scheduler,
		localDuePollingEnabled: deps.LocalDuePollingEnabled,
		cacheClient:            deps.CacheClient,
		bus:                    deps.Bus,
	}
}

type CreateTaskInput struct {
	Title     string
	ProjectID int
	ContentMD *string
	Priority  *int
	Status    *string
	StartAt   *time.Time
	DueAt     *time.Time
}

func (s *TaskService) Create(ctx context.Context, lg *zap.Logger, uid int, in CreateTaskInput) (*models.Task, error) {
	lg.Info("task.create.begin", zap.Int("uid", uid), zap.Int("project_id", in.ProjectID))

	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, apperrors.NewParamError("任务标题不能为空")
	}

	if in.Priority != nil && (*in.Priority < 1 || *in.Priority > 5) {
		return nil, apperrors.NewParamError("优先级必须在 1~5 之间")
	}

	if in.StartAt != nil && in.DueAt != nil && in.DueAt.Before(*in.StartAt) {
		return nil, apperrors.NewParamError("截止时间必须晚于开始时间")
	}

	project, err := s.projectRepo.GetByIDAndUserID(ctx, in.ProjectID, uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("项目不存在")
		}
		lg.Error("task.create.get_project_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}

	if project.UserID != uid {
		return nil, apperrors.NewForbiddenError("只有项目拥有者可以创建任务")
	}

	ownerID := project.UserID

	exists, err := s.repo.GetByUserProjectTitle(ctx, ownerID, in.ProjectID, in.Title)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NewInternalError("系统错误")
	}
	if exists != nil && exists.ID != 0 {
		return nil, apperrors.NewConflictError("任务已存在")
	}

	status := models.TaskTodo
	if in.Status != nil {
		candidate := strings.TrimSpace(*in.Status)
		if candidate != models.TaskTodo && candidate != models.TaskDone {
			return nil, apperrors.NewParamError("无效的任务状态")
		}
		status = candidate
	}

	priority := 3
	if in.Priority != nil {
		priority = *in.Priority
	}

	contentMD := ""
	if in.ContentMD != nil {
		contentMD = *in.ContentMD
	}

	task := &models.Task{
		UserID:    uid,
		ProjectID: in.ProjectID,
		Title:     in.Title,
		ContentMD: contentMD,
		Status:    status,
		Priority:  priority,
		DueAt:     in.DueAt,
		SortOrder: time.Now().UnixNano(),
	}

	var created *models.Task
	var taskEvent *models.TaskEvent
	err = s.withTaskMutation(ctx, func(taskRepo repo.TaskRepository, eventRepo repo.TaskEventRepository) error {
		var createErr error
		created, createErr = taskRepo.Create(ctx, task)
		if createErr != nil {
			return createErr
		}
		var eventErr error
		taskEvent, eventErr = s.appendTaskEvent(ctx, eventRepo, models.TaskEventTypeCreated, uid, created)
		return eventErr
	})
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			return nil, apperrors.NewConflictError("任务已存在")
		}
		lg.Error("task.create.insert_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("创建任务失败")
	}

	s.scheduleDueIfNeeded(lg, created)
	s.publishTaskEvent(ctx, taskEvent)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		score := created.CalculateScore()
		if err := s.taskCache.AddTaskID(ctx, in.ProjectID, "", created.ID, score); err != nil {
			lg.Warn("task.create.cache_zset_failed", zap.Error(err))
		}
		if err := s.taskCache.AddTaskID(ctx, in.ProjectID, created.Status, created.ID, score); err != nil {
			lg.Warn("task.create.cache_zset_status_failed", zap.Error(err))
		}
	}()

	return created, nil
}

type UpdateTaskInput struct {
	Title           *string
	ProjectID       *int
	ContentMD       *string
	Priority        *int
	Status          *string
	SortOrder       *int64
	ReDueAt         *time.Time
	ClearDue        *bool
	ExpectedVersion *int
}

func (s *TaskService) Update(ctx context.Context, lg *zap.Logger, uid, pid int, id int, in UpdateTaskInput) (*models.Task, error, int64) {
	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("任务不存在"), 0
		}
		return nil, apperrors.NewInternalError("系统错误"), 0
	}
	if _, err = s.projectRepo.GetByID(ctx, pid); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("项目不存在"), 0
		}
		return nil, apperrors.NewInternalError("系统错误"), 0
	}
	if task.ProjectID != pid {
		return nil, apperrors.NewNotFoundError("任务不存在"), 0
	}

	if in.ExpectedVersion == nil || *in.ExpectedVersion <= 0 {
		return nil, apperrors.NewParamError("expected_version is required"), 0
	}
	if task.UserID != uid {
		role, err := s.taskMemberRepo.GetMemberRole(ctx, id, uid)
		if err != nil {
			lg.Error("task.update.get_role_failed", zap.Int("uid", uid), zap.Int("task_id", id), zap.Error(err))
			return nil, apperrors.NewInternalError("failed to query member role"), 0
		}
		if role != models.RoleEditor {
			return nil, apperrors.NewForbiddenError("no permission to update task"), 0
		}
	}

	lockKey := fmt.Sprintf("task_lock:%d", id)

	lock := cache.NewDistributedLock(s.cacheClient, lockKey, 5*time.Second)

	acquired, err := lock.Acquire(ctx)
	if err != nil {
		lg.Error("task.update.lock_acquire_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统繁忙，请稍后重试"), 0
	}
	if !acquired {
		return nil, apperrors.NewConflictError("任务正在被修改，请稍后重试"), 0
	}
	defer func() {
		if err := lock.Release(ctx); err != nil {
			lg.Error("task.update.lock_release_failed", zap.Error(err))
		}
	}()

	if task.UserID != uid {
		role, err := s.taskMemberRepo.GetMemberRole(ctx, id, uid)
		if err != nil {
			lg.Error("task.update.get_role_failed", zap.Int("uid", uid), zap.Int("task_id", id), zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误"), 0
		}
		if role != models.RoleEditor {
			return nil, apperrors.NewForbiddenError("无权修改该任务"), 0
		}
	}

	task, err = s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("任务不存在"), 0
		}
		return nil, apperrors.NewInternalError("系统错误"), 0
	}
	if task.Version != *in.ExpectedVersion {
		return nil, apperrors.NewConflictError("task version conflict"), 0
	}
	before := *task

	if in.ProjectID != nil && *in.ProjectID != task.ProjectID {
		if *in.ProjectID <= 0 {
			return nil, apperrors.NewParamError("无效的项目ID"), 0
		}
		targetProject, err := s.projectRepo.GetByIDAndUserID(ctx, *in.ProjectID, uid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.NewNotFoundError("目标项目不存在或无权访问"), 0
			}
			return nil, apperrors.NewInternalError("系统错误"), 0
		}
		if targetProject.UserID != uid {
			return nil, apperrors.NewForbiddenError("只有目标项目拥有者可以移动任务"), 0
		}
	}

	update := map[string]interface{}{}

	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			return nil, apperrors.NewParamError("任务标题不能为空"), 0
		}
		update["title"] = title
	}

	if in.ContentMD != nil {
		contentMD := *in.ContentMD
		update["content_md"] = contentMD
	}

	if in.Priority != nil {
		if *in.Priority < 1 || *in.Priority > 5 {
			return nil, apperrors.NewParamError("优先级必须在 1~5 之间"), 0
		}
		update["priority"] = *in.Priority
	}

	if in.Status != nil {
		candidate := strings.TrimSpace(*in.Status)
		if candidate != models.TaskTodo && candidate != models.TaskDone {
			return nil, apperrors.NewParamError("无效的任务状态"), 0
		}
		update["status"] = candidate
		if candidate == models.TaskTodo {
			update["notified"] = false
		}
	}

	if in.SortOrder != nil && *in.SortOrder >= 0 {
		update["sort_order"] = *in.SortOrder
	}

	if in.ClearDue != nil && *in.ClearDue && in.ReDueAt != nil {
		return nil, apperrors.NewParamError("due_at and clear_due_at cannot both be set"), 0
	}

	if in.ReDueAt != nil {
		if in.ReDueAt.Before(time.Now()) {
			return nil, apperrors.NewParamError("截止时间必须晚于当前时间"), 0
		}
		update["due_at"] = *in.ReDueAt
		update["notified"] = false
	}

	if in.ClearDue != nil && *in.ClearDue {
		update["due_at"] = nil
		update["notified"] = false
	}

	if in.ProjectID != nil && *in.ProjectID != pid {
		if *in.ProjectID <= 0 {
			return nil, apperrors.NewParamError("无效的项目ID"), 0
		}
		targetProject, err := s.projectRepo.GetByIDAndUserID(ctx, *in.ProjectID, uid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.NewNotFoundError("目标项目不存在"), 0
			}
			return nil, apperrors.NewInternalError("系统错误"), 0
		}

		if targetProject.UserID != uid {
			return nil, apperrors.NewForbiddenError("只有目标项目拥有者可以接收移动的任务"), 0
		}
		update["project_id"] = *in.ProjectID
	}

	if len(update) == 0 {
		return nil, apperrors.NewParamError("没有需要更新的字段"), 0
	}

	var (
		updated   *models.Task
		affected  int64
		taskEvent *models.TaskEvent
	)
	err = s.withTaskMutation(ctx, func(taskRepo repo.TaskRepository, eventRepo repo.TaskEventRepository) error {
		var updateErr error
		updated, updateErr, affected = taskRepo.Update(ctx, id, *in.ExpectedVersion, update)
		if updateErr != nil {
			return updateErr
		}
		if affected == 0 {
			return nil
		}
		var eventErr error
		taskEvent, eventErr = s.appendTaskEvent(ctx, eventRepo, models.TaskEventTypeUpdated, uid, updated)
		return eventErr
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("任务不存在"), 0
		}
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			return nil, apperrors.NewConflictError("任务已存在"), 0
		}
		return nil, apperrors.NewInternalError("更新任务失败"), 0
	}
	if affected == 0 {
		return nil, apperrors.NewConflictError("task version conflict"), 0
	}

	s.taskCache.DelDetail(ctx, uid, id)
	s.publishTaskEvent(ctx, taskEvent)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.taskCache.RemTaskID(ctx, before.ProjectID, "", id)
		if before.Status != "" {
			_ = s.taskCache.RemTaskID(ctx, before.ProjectID, before.Status, id)
		}

		score := updated.CalculateScore()
		_ = s.taskCache.AddTaskID(ctx, updated.ProjectID, "", id, score)
		if updated.Status != "" {
			_ = s.taskCache.AddTaskID(ctx, updated.ProjectID, updated.Status, id, score)
		}
	}()

	s.syncDueSchedule(lg, &before, updated)
	return updated, nil, affected
}

func (s *TaskService) Delete(ctx context.Context, lg *zap.Logger, uid int, id int) (int64, error) {
	lg.Info("task.delete.begin", zap.Int("uid", uid), zap.Int("task_id", id))

	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("任务不存在")
		}
		return 0, apperrors.NewInternalError("删除任务失败")
	}

	lockKey := fmt.Sprintf("task_lock:%d", id)
	lock := cache.NewDistributedLock(s.cacheClient, lockKey, 5*time.Second)

	acquired, err := lock.Acquire(ctx)
	if err != nil {
		lg.Error("task.delete.lock_acquire_failed", zap.Error(err))
		return 0, apperrors.NewInternalError("系统繁忙，请稍后重试")
	}
	if !acquired {
		return 0, apperrors.NewConflictError("任务正在被修改，请稍后重试")
	}
	defer func() {
		if err := lock.Release(ctx); err != nil {
			lg.Error("task.delete.lock_release_failed", zap.Error(err))
		}
	}()

	if task.UserID != uid {
		return 0, apperrors.NewForbiddenError("无权删除该任务")
	}

	task, err = s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("任务不存在")
		}
		return 0, apperrors.NewInternalError("删除任务失败")
	}

	var affected int64
	var taskEvent *models.TaskEvent
	err = s.withTaskMutation(ctx, func(taskRepo repo.TaskRepository, eventRepo repo.TaskEventRepository) error {
		var deleteErr error
		affected, deleteErr = taskRepo.DeleteByID(ctx, id)
		if deleteErr != nil {
			return deleteErr
		}
		var eventErr error
		taskEvent, eventErr = s.appendTaskEvent(ctx, eventRepo, models.TaskEventTypeDeleted, uid, task)
		return eventErr
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("任务不存在")
		}
		return 0, apperrors.NewInternalError("删除任务失败")
	}

	s.taskCache.DelDetail(ctx, uid, id)
	s.publishTaskEvent(ctx, taskEvent)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.taskCache.RemTaskID(ctx, task.ProjectID, "", id)
		if task.Status != "" {
			_ = s.taskCache.RemTaskID(ctx, task.ProjectID, task.Status, id)
		}
	}()

	s.cancelDue(lg, task.ID)
	return affected, nil
}

func (s *TaskService) GetDetail(ctx context.Context, lg *zap.Logger, uid, id int) (*models.Task, error) {

	role, err := s.taskMemberRepo.GetMemberRole(ctx, id, uid)
	if err != nil {
		lg.Error("task.check_permission.failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}
	if role != models.RoleEditor && role != models.RoleOwner && role != models.RoleViewer {
		return nil, apperrors.NewForbiddenError("无权访问该任务")
	}

	cached, err := s.taskCache.GetDetail(ctx, uid, id)
	if err == nil && cached != nil {
		return cached, nil
	}

	task, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("任务不存在")
		}
		lg.Error("task.get_detail.failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}

	members, err := s.taskMemberRepo.GetMembersByTaskID(ctx, id)
	if err != nil {
		lg.Error("task.get_detail.get_members_failed", zap.Error(err))
	} else {
		task.Members = members
	}

	if err := s.taskCache.SetDetail(ctx, uid, id, task); err != nil {
		lg.Warn("task.get_detail.cache_failed", zap.Error(err))
	}

	return task, nil
}

type TaskListInput struct {
	Page   int
	Size   int
	Status string
	Pid    int
}

type TaskListResult struct {
	Tasks []models.Task
	Total int64
}

func (s *TaskService) rebuildTaskListCacheAsync(lg *zap.Logger, pid int, status string) {
	go func() {
		statusKey := status
		if statusKey == "" {
			statusKey = "all"
		}
		sfKey := fmt.Sprintf("task:list:rebuild:%d:%s", pid, statusKey)

		_, _, _ = s.sf.Do(sfKey, func() (interface{}, error) {
			if s.cacheClient == nil {
				return nil, nil
			}

			lock := cache.NewDistributedLock(s.cacheClient, sfKey, listCacheRebuildLockTTL)
			lockCtx, lockCancel := context.WithTimeout(context.Background(), listCacheRebuildLockWait)
			acquired, lockErr := lock.Acquire(lockCtx)
			lockCancel()
			if lockErr != nil {
				lg.Warn("task.list.rebuild_cache.lock_acquire_failed", zap.Int("project_id", pid), zap.String("status", status), zap.Error(lockErr))
				return nil, nil
			}
			if !acquired {
				return nil, nil
			}

			defer func() {
				releaseCtx, releaseCancel := context.WithTimeout(context.Background(), time.Second)
				defer releaseCancel()
				if err := lock.Release(releaseCtx); err != nil {
					lg.Warn("task.list.rebuild_cache.lock_release_failed", zap.Int("project_id", pid), zap.String("status", status), zap.Error(err))
				}
			}()

			cacheCtx, cancel := context.WithTimeout(context.Background(), listCacheRebuildQueryLimit)
			defer cancel()

			items, err := s.repo.GetAllIDs(cacheCtx, pid, status)
			if err != nil {
				lg.Warn("task.list.rebuild_cache.get_all_ids_failed", zap.Int("project_id", pid), zap.String("status", status), zap.Error(err))
				return nil, nil
			}
			if err := s.taskCache.SetTaskIDs(cacheCtx, pid, status, items); err != nil {
				lg.Warn("task.list.rebuild_cache.set_failed", zap.Int("project_id", pid), zap.String("status", status), zap.Error(err))
			}
			return nil, nil
		})
	}()
}

func (s *TaskService) List(ctx context.Context, lg *zap.Logger, uid int, in TaskListInput) (*TaskListResult, error) {
	lg = lg.With(zap.Int("user_id", uid), zap.Int("project_id", in.Pid))

	if in.Status != models.TaskTodo && in.Status != models.TaskDone && in.Status != "" {
		return nil, apperrors.NewParamError("无效的任务状态")
	}

	page, size := in.Page, in.Size
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	var (
		err         error
		ids         []int
		tasks       []models.Task
		total       int64
		repairCache bool
	)

	project, err := s.projectCache.Get(ctx, uid, in.Pid)
	if err != nil || project == nil {
		project, err = s.projectRepo.GetByID(ctx, in.Pid)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, apperrors.NewNotFoundError("项目不存在")
			}
			return nil, apperrors.NewInternalError("系统错误")
		}
		_ = s.projectCache.Set(ctx, uid, in.Pid, project)
	}

	if project.UserID != uid {
		return nil, apperrors.NewForbiddenError("无权查看该项目的任务列表")
	}

	ids, err = s.taskCache.GetTaskIDs(ctx, in.Pid, in.Status, page, size)
	if err != nil {
		if errors.Is(err, cache.ErrCacheMiss) {
			// Avoid full-table ID rebuild in request path under high concurrency.
			// Rebuild asynchronously and serve this request from paged DB fallback.
			repairCache = true
			goto DB_FALLBACK
		} else {
			lg.Error("task.list.zset_failed", zap.Error(err))
			repairCache = true
			goto DB_FALLBACK
		}
	}

	if len(ids) > 0 {
		// MGet from Redis first
		cachedTasks, missingIDs, err := s.taskCache.MGetDetail(ctx, uid, ids)
		if err != nil {
			lg.Warn("task.list.mget_failed", zap.Error(err))
			// Fallback to fetch all from DB if MGet fails entirely
			missingIDs = ids
			cachedTasks = make(map[int]*models.Task)
		} else if cachedTasks == nil {
			cachedTasks = make(map[int]*models.Task)
		}

		// If there are missing tasks, fetch them from DB
		if len(missingIDs) > 0 {
			fetchIDs := append([]int(nil), ids...)
			statusKey := in.Status
			if statusKey == "" {
				statusKey = "all"
			}
			sfKey := fmt.Sprintf("task:list:hydrate:%d:%d:%s:%d:%d", in.Pid, uid, statusKey, page, size)

			shared, sharedErr, _ := s.sf.Do(sfKey, func() (interface{}, error) {
				dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				dbTasks, dbErr := s.repo.GetByIDsAndProject(dbCtx, fetchIDs, in.Pid, in.Status)
				if dbErr != nil {
					return nil, dbErr
				}
				if len(dbTasks) > 0 {
					cacheCtx, cacheCancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cacheCancel()
					if cacheErr := s.taskCache.MSetDetail(cacheCtx, uid, dbTasks); cacheErr != nil {
						lg.Warn("task.list.mset_detail_failed", zap.Error(cacheErr), zap.Int("project_id", in.Pid), zap.Int("uid", uid))
					}
				}
				return dbTasks, nil
			})
			if sharedErr != nil {
				lg.Error("task.list.get_by_ids_failed", zap.Error(sharedErr))
				repairCache = true
				goto DB_FALLBACK
			}
			dbTasks, ok := shared.([]models.Task)
			if !ok {
				lg.Error("task.list.get_by_ids_failed", zap.String("reason", "invalid shared result type"))
				repairCache = true
				goto DB_FALLBACK
			}

			// Fill DB tasks into result map and cache them back
			for _, t := range dbTasks {
				// Use local variable to avoid pointer reuse issue in loop
				task := t
				cachedTasks[task.ID] = &task
			}
		}

		// Validate if we retrieved all requested tasks (handle stale ZSet entries)
		tasks = make([]models.Task, 0, len(ids))
		for _, id := range ids {
			t, ok := cachedTasks[id]
			if !ok {
				lg.Warn("task.list.stale_task_id", zap.Int("task_id", id))
				repairCache = true
				goto DB_FALLBACK
			}
			tasks = append(tasks, *t)
		}

		total, err = s.taskCache.CountTaskIDs(ctx, in.Pid, in.Status)
		if err != nil {
			lg.Error("task.list.count_failed", zap.Error(err))
			repairCache = true
			goto DB_FALLBACK
		}

		return &TaskListResult{Tasks: tasks, Total: total}, nil
	}

	total, err = s.taskCache.CountTaskIDs(ctx, in.Pid, in.Status)
	if err != nil {
		lg.Error("task.list.count_empty_failed", zap.Error(err))
		repairCache = true
		goto DB_FALLBACK
	}
	return &TaskListResult{Tasks: []models.Task{}, Total: total}, nil

DB_FALLBACK:
	lg.Info("task.list.fallback_db")
	if repairCache {
		s.rebuildTaskListCacheAsync(lg, in.Pid, in.Status)
	}
	statusKey := in.Status
	if statusKey == "" {
		statusKey = "all"
	}
	sfKey := fmt.Sprintf("task:list:fallback:%d:%s:%d:%d", in.Pid, statusKey, page, size)

	shared, sharedErr, _ := s.sf.Do(sfKey, func() (interface{}, error) {
		// Decouple from caller cancellation so one canceled request does not
		// abort the shared fallback query for all concurrent callers.
		dbCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		dbTasks, dbTotal, dbErr := s.repo.ListByProject(dbCtx, in.Pid, in.Status, page, size)
		if dbErr != nil {
			return nil, dbErr
		}
		return &TaskListResult{Tasks: dbTasks, Total: dbTotal}, nil
	})
	if sharedErr != nil {
		lg.Error("task.list.failed", zap.Error(sharedErr))
		return nil, apperrors.NewInternalError("获取任务列表失败")
	}

	result, ok := shared.(*TaskListResult)
	if !ok || result == nil {
		lg.Error("task.list.failed", zap.String("reason", "invalid shared result type"))
		return nil, apperrors.NewInternalError("获取任务列表失败")
	}
	return result, nil
}

type DueCallbackInput struct {
	TaskID      int
	TriggeredAt *time.Time
}

type MyTaskListInput struct {
	Page     int
	Size     int
	Status   string
	DueStart *time.Time
	DueEnd   *time.Time
}

func (s *TaskService) ListMyTasks(ctx context.Context, lg *zap.Logger, uid int, in MyTaskListInput) (*TaskListResult, error) {
	page := in.Page
	size := in.Size
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	if in.Status != "" && in.Status != models.TaskTodo && in.Status != models.TaskDone {
		return nil, apperrors.NewParamError("invalid status")
	}
	if in.DueStart != nil && in.DueEnd != nil && in.DueStart.After(*in.DueEnd) {
		return nil, apperrors.NewParamError("due_start must be <= due_end")
	}

	tasks, total, err := s.repo.ListByMember(ctx, uid, page, size, in.Status, in.DueStart, in.DueEnd)
	if err != nil {
		lg.Error("task.list_my_tasks.failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}

	return &TaskListResult{Tasks: tasks, Total: total}, nil
}

func (s *TaskService) AddMember(ctx context.Context, lg *zap.Logger, uid, taskID int, targetEmail, role string) error {
	lg.Info("task.add_member.begin", zap.Int("uid", uid), zap.Int("task_id", taskID), zap.String("target", targetEmail))

	task, err := s.repo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFoundError("任务不存在")
		}
		return apperrors.NewInternalError("系统错误")
	}

	project, err := s.projectRepo.GetByID(ctx, task.ProjectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFoundError("项目不存在")
		}
		return apperrors.NewInternalError("系统错误")
	}

	if project.UserID != uid {
		return apperrors.NewForbiddenError("只有项目拥有者可以添加任务成员")
	}

	targetUser, err := s.userRepo.GetByEmail(ctx, targetEmail)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFoundError("用户不存在")
		}
		return apperrors.NewInternalError("查找用户失败")
	}

	if targetUser.ID == project.UserID {
		return apperrors.NewParamError("项目拥有者默认拥有所有权限，无需添加")
	}

	if role != models.RoleEditor && role != models.RoleViewer {
		role = models.RoleViewer
	}

	if err := s.taskMemberRepo.AddMember(ctx, taskID, targetUser.ID, role); err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			return apperrors.NewConflictError("用户已是成员")
		}
		lg.Error("task.add_member.failed", zap.Error(err))
		return apperrors.NewInternalError("添加成员失败")
	}

	return nil
}

func (s *TaskService) RemoveMember(ctx context.Context, lg *zap.Logger, uid, taskID, targetUserID int) error {
	lg.Info("task.remove_member.begin", zap.Int("uid", uid), zap.Int("task_id", taskID), zap.Int("target_uid", targetUserID))

	task, err := s.repo.GetByID(ctx, taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFoundError("任务不存在")
		}
		return apperrors.NewInternalError("系统错误")
	}

	project, err := s.projectRepo.GetByID(ctx, task.ProjectID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFoundError("项目不存在")
		}
		return apperrors.NewInternalError("系统错误")
	}

	if project.UserID != uid {
		return apperrors.NewForbiddenError("只有项目拥有者可以移除任务成员")
	}

	if targetUserID == project.UserID {
		return apperrors.NewParamError("不能移除项目拥有者")
	}

	if err := s.taskMemberRepo.RemoveMember(ctx, taskID, targetUserID); err != nil {
		lg.Error("task.remove_member.failed", zap.Error(err))
		return apperrors.NewInternalError("移除成员失败")
	}

	return nil
}

func (s *TaskService) HandleDueCallback(ctx context.Context, lg *zap.Logger, in DueCallbackInput) (bool, error) {
	if in.TaskID <= 0 {
		return false, apperrors.NewParamError("无效的任务ID")
	}

	triggeredAt := time.Now()
	if in.TriggeredAt != nil {
		triggeredAt = *in.TriggeredAt
	}

	task, err := s.repo.GetByID(ctx, in.TaskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			lg.Warn("task.due_callback.not_found", zap.Int("task_id", in.TaskID))
			return false, nil
		}
		lg.Error("task.due_callback.get_task_failed", zap.Error(err))
		return false, apperrors.NewInternalError("系统错误")
	}

	if task.DueAt == nil || task.DueAt.After(triggeredAt.Add(time.Minute)) {
		lg.Info("task.due_callback.ignored_future_due",
			zap.Int("task_id", in.TaskID),
			zap.Time("db_due_at", *task.DueAt),
			zap.Time("triggered_at", triggeredAt))
		return false, nil
	}

	if task.Notified {
		lg.Info("task.due_callback.ignored_already_notified", zap.Int("task_id", in.TaskID))
		return false, nil
	}

	if task.Status == models.TaskDone {
		lg.Info("task.due_callback.ignored_task_done", zap.Int("task_id", in.TaskID))
		return false, nil
	}

	affected, err := s.repo.MarkNotifiedDue(ctx, in.TaskID, triggeredAt)
	if err != nil {
		lg.Error("task.due_callback.mark_notified_failed", zap.Int("task_id", in.TaskID), zap.Error(err))
		return false, apperrors.NewInternalError("标记任务通知失败")
	}
	if affected == 0 {
		lg.Info("task.due_callback.ignored_cas_failed", zap.Int("task_id", in.TaskID))
		return false, nil
	}

	s.taskCache.DelDetail(ctx, task.UserID, task.ID)

	user, err := s.userRepo.GetByID(ctx, task.UserID)
	if err != nil {
		lg.Error("task.due_callback.get_user_failed", zap.Int("user_id", task.UserID), zap.Error(err))
		if resetErr := s.repo.ResetNotifiedDue(ctx, task.ID); resetErr != nil {
			lg.Error("task.due_callback.reset_notified_failed", zap.Int("task_id", task.ID), zap.Error(resetErr))
		}
		return false, apperrors.NewInternalError("failed to send due reminder")
	}
	if strings.TrimSpace(user.Email) != "" {
		if notifyErr := s.notifyTaskDue(lg, task.ID, task.UserID, task.Title, user.Email); notifyErr != nil {
			if resetErr := s.repo.ResetNotifiedDue(ctx, task.ID); resetErr != nil {
				lg.Error("task.due_callback.reset_notified_failed", zap.Int("task_id", task.ID), zap.Error(resetErr))
			}
			return false, apperrors.NewInternalError("failed to send due reminder")
		}
	} else {
		lg.Warn("task.due_callback.email_empty", zap.Int("task_id", task.ID), zap.Int("user_id", task.UserID))
	}

	lg.Info("task.due_callback.marked", zap.Int("task_id", in.TaskID), zap.Time("triggered_at", triggeredAt))
	return true, nil
}

func (s *TaskService) checkAndNotifyDue(ctx context.Context, lg *zap.Logger) {
	now := time.Now()
	from := now
	to := now.Add(dueScanWindow)

	tasks, err := s.repo.FindDueTasks(ctx, from, to, dueScanLimit)
	if err != nil {
		lg.Error("due_watcher.find_due_tasks_failed", zap.Error(err))
		return
	}
	if len(tasks) == 0 {
		return
	}

	for _, t := range tasks {
		affected, err := s.repo.MarkNotifiedDue(ctx, t.ID, now)
		if err != nil {
			lg.Error("due_watcher.mark_notified_failed", zap.Int("task_id", t.ID), zap.Error(err))
			continue
		}
		if affected == 0 {
			continue
		}
		lg.Info("due_watcher.notify", zap.Int("task_id", t.ID), zap.Int("uid", t.UserID))

		user, err := s.userRepo.GetByID(ctx, t.UserID)
		if err != nil {
			lg.Error("due_watcher.get_user_failed", zap.Int("user_id", t.UserID), zap.Error(err))
			if resetErr := s.repo.ResetNotifiedDue(ctx, t.ID); resetErr != nil {
				lg.Error("due_watcher.reset_notified_failed", zap.Int("task_id", t.ID), zap.Error(resetErr))
			}
			continue
		}

		if strings.TrimSpace(user.Email) == "" {
			lg.Warn("due_watcher.email_empty", zap.Int("task_id", t.ID), zap.Int("user_id", t.UserID))
			continue
		}

		if notifyErr := s.notifyTaskDue(lg, t.ID, t.UserID, t.Title, user.Email); notifyErr != nil {
			lg.Error("due_watcher.notify_failed", zap.Int("task_id", t.ID), zap.Int("user_id", t.UserID), zap.Error(notifyErr))
			if resetErr := s.repo.ResetNotifiedDue(ctx, t.ID); resetErr != nil {
				lg.Error("due_watcher.reset_notified_failed", zap.Int("task_id", t.ID), zap.Error(resetErr))
			}
		}
	}
}

func (s *TaskService) notifyTaskDue(lg *zap.Logger, taskID, userID int, title, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}

	payload := struct {
		TaskID int    `json:"task_id"`
		UserID int    `json:"user_id"`
		Title  string `json:"title"`
		Email  string `json:"email"`
	}{
		TaskID: taskID,
		UserID: userID,
		Title:  title,
		Email:  email,
	}

	published := async.PublishWithTimeout(
		s.bus,
		lg,
		"TaskDue",
		payload,
		500*time.Millisecond,
		zap.Int("task_id", taskID),
		zap.Int("user_id", userID),
		zap.String("email", email),
	)
	if published {
		return nil
	}

	if config.GlobalConfig == nil {
		cfgErr := errors.New("global config not loaded for email fallback")
		lg.Warn("task.due_email_fallback.skipped", zap.Int("task_id", taskID), zap.String("reason", "global_config_nil"), zap.Error(cfgErr))
		return cfgErr
	}

	safeTitle := strings.TrimSpace(title)
	if safeTitle == "" {
		safeTitle = fmt.Sprintf("task-%d", taskID)
	}

	subject := fmt.Sprintf("【待办系统】任务到期提醒：%s", safeTitle)
	body := fmt.Sprintf("你在待办系统中的任务「%s」已到期，请及时处理。", safeTitle)

	if err := utils.SendEmail(config.GlobalConfig.Email, email, subject, body); err != nil {
		lg.Error("task.due_email_fallback.failed", zap.Int("task_id", taskID), zap.String("email", email), zap.Error(err))
		return err
	}

	lg.Info("task.due_email_fallback.sent", zap.Int("task_id", taskID), zap.String("email", email))
	return nil
}

func (s *TaskService) scheduleDueIfNeeded(lg *zap.Logger, task *models.Task) {
	if task == nil || !shouldScheduleTask(task) {
		return
	}
	if err := s.dueScheduler.ScheduleTaskOnce(context.Background(), task.ID, *task.DueAt); err != nil {
		lg.Warn("task.due_schedule_failed", zap.Int("task_id", task.ID), zap.Error(err))
	}
}

func (s *TaskService) cancelDue(lg *zap.Logger, taskID int) {
	if taskID <= 0 {
		return
	}
	if err := s.dueScheduler.CancelTask(context.Background(), taskID); err != nil {
		lg.Warn("task.due_cancel_failed", zap.Int("task_id", taskID), zap.Error(err))
	}
}

func (s *TaskService) syncDueSchedule(lg *zap.Logger, before *models.Task, after *models.Task) {
	beforeSched := shouldScheduleTask(before)
	afterSched := shouldScheduleTask(after)

	switch {
	case !beforeSched && afterSched:
		s.scheduleDueIfNeeded(lg, after)
	case beforeSched && !afterSched:
		s.cancelDue(lg, before.ID)
	case beforeSched && afterSched:
		beforeDue := before.DueAt
		afterDue := after.DueAt
		if beforeDue == nil || afterDue == nil || !beforeDue.Equal(*afterDue) {
			s.scheduleDueIfNeeded(lg, after)
		}
	}
}

func shouldScheduleTask(task *models.Task) bool {
	return task != nil &&
		task.Status == models.TaskTodo &&
		task.DueAt != nil &&
		!task.Notified
}

func (s *TaskService) StartDueWatcher(ctx context.Context, lg *zap.Logger) {
	if !s.localDuePollingEnabled {
		lg.Info("due_watcher.disabled", zap.String("reason", "local_polling_disabled"))
		return
	}

	ticker := time.NewTicker(dueScanInterval)
	healthTicker := time.NewTicker(30 * time.Second)

	isWatcherActive := true

	go func() {
		defer ticker.Stop()
		defer healthTicker.Stop()

		lg.Info("due_watcher.started", zap.String("reason", "local_polling_enabled"))
		for {
			select {
			case <-ctx.Done():
				return
			case <-healthTicker.C:
				pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				err := s.dueScheduler.Ping(pingCtx)
				cancel()

				if err != nil {
					if !isWatcherActive {
						lg.Warn("due_watcher.activated", zap.Error(err), zap.String("reason", "scheduler_unhealthy"))
						isWatcherActive = true
					}
				} else {
					if isWatcherActive {
						lg.Info("due_watcher.deactivated", zap.String("reason", "scheduler_healthy"))
						isWatcherActive = false
					}
				}

			case <-ticker.C:
				if isWatcherActive {
					s.checkAndNotifyDue(ctx, lg)
				}
			}
		}
	}()
}
