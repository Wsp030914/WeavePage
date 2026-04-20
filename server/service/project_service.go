package service

// 文件说明：这个文件实现空间项目业务，包括创建、查询、列表、更新、删除和缓存维护。
// 实现方式：服务层组合仓储、项目缓存和用户数据，统一处理项目读写、权限约束以及列表缓存回填。
// 这样做的好处是空间模型有独立编排层，后续扩展 Spaces 信息架构时不需要把规则散落到 handler 或 repo 中。

import (
	"ToDoList/server/async"
	"ToDoList/server/cache"
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"ToDoList/server/repo"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

type ProjectService struct {
	repo         repo.ProjectRepository
	projectCache cache.ProjectCache
	userRepo     repo.UserRepository
	cacheClient  cache.Cache
	bus          async.IEventBus
	sf           singleflight.Group
}

type ProjectServiceDeps struct {
	Repo         repo.ProjectRepository
	ProjectCache cache.ProjectCache
	UserRepo     repo.UserRepository
	CacheClient  cache.Cache
	Bus          async.IEventBus
}

// NewProjectService 创建项目服务。
func NewProjectService(deps ProjectServiceDeps) *ProjectService {
	return &ProjectService{
		repo:         deps.Repo,
		projectCache: deps.ProjectCache,
		userRepo:     deps.UserRepo,
		cacheClient:  deps.CacheClient,
		bus:          deps.Bus,
	}
}

func (s *ProjectService) currentProjectSummaryVersion(ctx context.Context, lg *zap.Logger, userID int) int64 {
	ver, err := s.projectCache.GetSummaryVersion(ctx, userID)
	if err != nil {
		lg.Warn("project.summary.get_version_failed", zap.Int("user_id", userID), zap.Error(err))
		return 0
	}
	return ver
}

func (s *ProjectService) getCachedProjectSummary(ctx context.Context, lg *zap.Logger, userID int, ver int64, name string, page, size int) (*ProjectListResult, bool) {
	summary, err := s.projectCache.GetSummary(ctx, userID, ver, name, page, size)
	if err == nil && summary != nil {
		lg.Info("project.summary.hit_cache", zap.Int("user_id", userID), zap.String("name", name), zap.Int("page", page), zap.Int("size", size))
		return &ProjectListResult{Projects: summary.Projects, Total: summary.Total}, true
	}
	if err != nil && !errors.Is(err, cache.ErrCacheMiss) {
		lg.Warn("project.summary.get_failed", zap.Int("user_id", userID), zap.String("name", name), zap.Error(err))
	}
	return nil, false
}

func (s *ProjectService) publishProjectSummaryCacheAsync(lg *zap.Logger, userID int, ver int64, name string, page, size int, projects []models.Project, total int64) {
	if s.bus == nil {
		return
	}
	payload := models.ProjectSummaryCache{
		Projects: append([]models.Project(nil), projects...),
		Total:    total,
		UID:      userID,
		Ver:      ver,
		Name:     strings.TrimSpace(name),
		Page:     page,
		Size:     size,
	}
	async.PublishWithTimeout(s.bus, lg, "PutProjectsSummaryCache", payload, time.Second,
		zap.Int("user_id", userID),
		zap.Int64("summary_version", ver),
		zap.String("name", payload.Name),
		zap.Int("page", page),
		zap.Int("size", size),
	)
}

func (s *ProjectService) bumpProjectSummaryVersionAsync(lg *zap.Logger, userID int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := s.projectCache.BumpSummaryVersion(ctx, userID); err != nil {
			lg.Warn("project.summary.bump_version_failed", zap.Int("user_id", userID), zap.Error(err))
		}
	}()
}

// rebuildProjectListCacheAsync 异步回填项目详情缓存和项目 ID 列表缓存。
// 这里把详情缓存和 ZSet 列表分开重建，是为了让列表降级后能尽快恢复到增量读取路径。
func (s *ProjectService) rebuildProjectListCacheAsync(lg *zap.Logger, userID int, projects []models.Project) {
	warmProjects := append([]models.Project(nil), projects...)

	go func() {
		if len(warmProjects) > 0 {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := s.projectCache.MSet(cacheCtx, userID, warmProjects); err != nil {
				lg.Warn("project.list.rebuild_cache.mset_failed", zap.Int("user_id", userID), zap.Error(err))
			}
			cancel()
		}

		sfKey := fmt.Sprintf("project:list:rebuild:%d", userID)
		_, _, _ = s.sf.Do(sfKey, func() (interface{}, error) {
			if s.cacheClient != nil {
				lock := cache.NewDistributedLock(s.cacheClient, sfKey, listCacheRebuildLockTTL)
				lockCtx, lockCancel := context.WithTimeout(context.Background(), listCacheRebuildLockWait)
				acquired, lockErr := lock.Acquire(lockCtx)
				lockCancel()
				if lockErr != nil {
					lg.Warn("project.list.rebuild_cache.lock_acquire_failed", zap.Int("user_id", userID), zap.Error(lockErr))
					return nil, nil
				}
				if !acquired {
					return nil, nil
				}

				defer func() {
					releaseCtx, releaseCancel := context.WithTimeout(context.Background(), time.Second)
					defer releaseCancel()
					if err := lock.Release(releaseCtx); err != nil {
						lg.Warn("project.list.rebuild_cache.lock_release_failed", zap.Int("user_id", userID), zap.Error(err))
					}
				}()
			}

			cacheCtx, cancel := context.WithTimeout(context.Background(), listCacheRebuildQueryLimit)
			defer cancel()

			items, err := s.repo.GetAllIDs(cacheCtx, userID)
			if err != nil {
				lg.Warn("project.list.rebuild_cache.get_all_ids_failed", zap.Int("user_id", userID), zap.Error(err))
				return nil, nil
			}
			if len(items) == 0 {
				return nil, nil
			}
			if err := s.projectCache.SetProjectIDs(cacheCtx, userID, items); err != nil {
				lg.Warn("project.list.rebuild_cache.set_failed", zap.Int("user_id", userID), zap.Error(err))
			}
			return nil, nil
		})
	}()
}

// Create 创建一个新的空间项目。
// 创建成功后异步写入项目排序集合，是为了让主写路径只关心数据库成功，再把缓存热身放到副作用阶段。
func (s *ProjectService) Create(ctx context.Context, lg *zap.Logger, userID int, name, color string) (*models.Project, error) {
	lg = lg.With(zap.Int("user_id", userID), zap.String("name", name))
	lg.Info("project.create.begin")

	if strings.TrimSpace(name) == "" {
		return nil, apperrors.NewParamError("项目名称不能为空")
	}

	if color == "" {
		color = "#808080"
	}

	project := &models.Project{
		UserID:    userID,
		Name:      strings.TrimSpace(name),
		Color:     color,
		SortOrder: time.Now().UnixNano(),
	}

	created, err := s.repo.Create(ctx, project)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			lg.Info("project.create.duplicate")
			return nil, apperrors.NewConflictError("项目已存在")
		}
		lg.Error("project.create.failed", zap.Error(err))
		return nil, apperrors.NewInternalError("创建失败")
	}

	lg.Info("project.create.success", zap.Int("project_id", created.ID))
	s.bumpProjectSummaryVersionAsync(lg, userID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.projectCache.AddProjectID(ctx, userID, created.ID, float64(created.SortOrder)); err != nil {
			lg.Warn("project.create.cache_zset_failed", zap.Error(err))
		}
	}()

	return created, nil
}

// GetByID 读取单个项目详情。
// 这里对不存在和无权限都统一返回 not found，是为了避免通过接口探测他人项目是否存在。
func (s *ProjectService) GetByID(ctx context.Context, lg *zap.Logger, userID, projectID int) (*models.Project, error) {
	lg = lg.With(zap.Int("user_id", userID), zap.Int("project_id", projectID))

	cached, err := s.projectCache.Get(ctx, userID, projectID)
	if err == nil && cached != nil {
		lg.Info("project.get.hit_cache")
		return cached, nil
	}

	if errors.Is(err, cache.ErrCacheNull) {
		lg.Info("project.get.hit_null_cache")
		return nil, apperrors.NewNotFoundError("项目不存在")
	}

	key := fmt.Sprintf("project:%d:%d", userID, projectID)
	val, err := loadWithCacheProtection(ctx, lg, &s.sf, s.cacheClient, key, func(loadCtx context.Context) (interface{}, error) {
		project, err := s.repo.GetByIDAndUserID(loadCtx, projectID, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				lg.Info("project.get.not_found")
				cacheCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = s.projectCache.Set(cacheCtx, userID, projectID, nil)
				return nil, apperrors.NewNotFoundError("项目不存在")
			}
			lg.Error("project.get.failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}

		if project.UserID != userID {
			lg.Info("project.get.forbidden")
			cacheCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.projectCache.Set(cacheCtx, userID, projectID, nil)
			return nil, apperrors.NewNotFoundError("项目不存在")
		}

		cacheCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.projectCache.Set(cacheCtx, userID, projectID, project); err != nil {
			lg.Warn("project.get.cache_set_failed", zap.Error(err))
		}

		return project, nil
	}, func(readCtx context.Context) (interface{}, bool, error) {
		project, err := s.projectCache.Get(readCtx, userID, projectID)
		if err == nil && project != nil {
			return project, true, nil
		}
		if errors.Is(err, cache.ErrCacheNull) {
			return nil, false, apperrors.NewNotFoundError("项目不存在")
		}
		return nil, false, nil
	})

	if err != nil {
		return nil, err
	}

	return val.(*models.Project), nil
}

type ProjectListInput struct {
	Page int
	Size int
	Name string
}

type ProjectListResult struct {
	Projects []models.Project
	Total    int64
}

type SpaceTrashListInput struct {
	Page int
	Size int
}

// List 返回项目列表或搜索结果。
// 无搜索词时优先走缓存列表链路，有搜索词时直接查库，是因为模糊搜索结果难以稳定缓存，而普通列表读取频次更高。
func (s *ProjectService) List(ctx context.Context, lg *zap.Logger, userID int, in ProjectListInput) (*ProjectListResult, error) {
	lg = lg.With(zap.Int("user_id", userID))

	page, size := in.Page, in.Size
	repairCache := false
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	name := strings.TrimSpace(in.Name)
	summaryVersion := s.currentProjectSummaryVersion(ctx, lg, userID)

	if name == "" {
		ids, err := s.projectCache.GetProjectIDs(ctx, userID, page, size)
		if err != nil {
			if errors.Is(err, cache.ErrCacheMiss) {
				key := fmt.Sprintf("project:list:ids:%d:%d:%d", userID, page, size)
				shared, err := loadWithCacheProtection(ctx, lg, &s.sf, s.cacheClient, key, func(loadCtx context.Context) (interface{}, error) {
					allProjects, err := s.repo.GetAllIDs(loadCtx, userID)
					if err != nil {
						return nil, err
					}
					if len(allProjects) == 0 {
						return []int{}, nil
					}
					if err := s.projectCache.SetProjectIDs(loadCtx, userID, allProjects); err != nil {
						return nil, err
					}
					return s.projectCache.GetProjectIDs(loadCtx, userID, page, size)
				}, func(readCtx context.Context) (interface{}, bool, error) {
					ids, err := s.projectCache.GetProjectIDs(readCtx, userID, page, size)
					if err == nil {
						return ids, true, nil
					}
					return nil, false, nil
				})
				if err != nil {
					lg.Error("project.list.rebuild_ids_failed", zap.Error(err))
					repairCache = true
					goto DB_FALLBACK
				}
				typedIDs, ok := shared.([]int)
				if !ok {
					lg.Error("project.list.rebuild_ids_invalid_result")
					repairCache = true
					goto DB_FALLBACK
				}
				ids = typedIDs
			} else {
				lg.Error("project.list.zset_failed", zap.Error(err))
				repairCache = true
				goto DB_FALLBACK
			}
		}

		if len(ids) > 0 {
			projectsMap, missingIDs, err := s.projectCache.MGet(ctx, userID, ids)
			if err != nil {
				lg.Warn("project.list.mget_failed", zap.Error(err))
				repairCache = true
				goto DB_FALLBACK
			}

			if len(missingIDs) > 0 {
				dbProjects, err := s.repo.GetByIDsAndUserID(ctx, missingIDs, userID)
				if err != nil {
					lg.Error("project.list.get_by_ids_failed", zap.Error(err))
					repairCache = true
					goto DB_FALLBACK
				}

				if len(dbProjects) != len(missingIDs) {
					lg.Warn("project.list.missing_ids_in_db", zap.Ints("missing", missingIDs), zap.Int("found", len(dbProjects)))
					repairCache = true
					goto DB_FALLBACK
				}

				for _, p := range dbProjects {
					p := p
					projectsMap[p.ID] = &p
				}

				go func(projs []models.Project) {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					if err := s.projectCache.MSet(ctx, userID, projs); err != nil {
						lg.Warn("project.list.mset_failed", zap.Error(err))
					}
				}(dbProjects)
			}

			projects := make([]models.Project, 0, len(ids))
			for _, id := range ids {
				if p, ok := projectsMap[id]; ok {
					projects = append(projects, *p)
				} else {
					lg.Warn("project.list.missing_in_map", zap.Int("id", id))
					repairCache = true
					goto DB_FALLBACK
				}
			}

			total, err := s.projectCache.CountProjectIDs(ctx, userID)
			if err != nil {
				lg.Error("project.list.count_failed", zap.Error(err))
				repairCache = true
				goto DB_FALLBACK
			}

			return &ProjectListResult{Projects: projects, Total: total}, nil
		} else {
			total, err := s.projectCache.CountProjectIDs(ctx, userID)
			if err != nil {
				lg.Error("project.list.count_empty_failed", zap.Error(err))
				repairCache = true
				goto DB_FALLBACK
			}
			return &ProjectListResult{Projects: []models.Project{}, Total: total}, nil
		}
	} else {
		if result, ok := s.getCachedProjectSummary(ctx, lg, userID, summaryVersion, name, page, size); ok {
			return result, nil
		}
		projects, total, err := s.repo.Search(ctx, userID, name, page, size)
		if err != nil {
			lg.Error("project.search.failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}
		s.publishProjectSummaryCacheAsync(lg, userID, summaryVersion, name, page, size, projects, total)
		return &ProjectListResult{Projects: projects, Total: total}, nil
	}

DB_FALLBACK:
	if result, ok := s.getCachedProjectSummary(ctx, lg, userID, summaryVersion, name, page, size); ok {
		return result, nil
	}
	lg.Info("project.list.fallback_db")
	projects, total, err := s.repo.List(ctx, userID, page, size)
	if err != nil {
		lg.Error("project.list.failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}
	if repairCache {
		s.rebuildProjectListCacheAsync(lg, userID, projects)
	}
	s.publishProjectSummaryCacheAsync(lg, userID, summaryVersion, name, page, size, projects, total)
	return &ProjectListResult{Projects: projects, Total: total}, nil
}

type UpdateProjectInput struct {
	Name      *string
	Color     *string
	SortOrder *int64
}

// Update 更新项目名称、颜色和排序。
// 排序字段单独支持增量更新，是为了兼容前端拖拽排序，不必每次都重写整条项目记录。
func (s *ProjectService) Update(ctx context.Context, lg *zap.Logger, userID, projectID int, in UpdateProjectInput) (*models.Project, error, int64) {
	lg = lg.With(zap.Int("user_id", userID), zap.Int("project_id", projectID))

	update := map[string]interface{}{}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		update["name"] = strings.TrimSpace(*in.Name)
	}
	if in.Color != nil {
		update["color"] = *in.Color
	}
	if in.SortOrder != nil {
		update["sort_order"] = *in.SortOrder
	}

	if len(update) == 0 {
		return nil, apperrors.NewParamError("没有需要更新的字段"), 0
	}

	project, err := s.repo.GetByIDAndUserID(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("项目不存在"), 0
		}
		return nil, apperrors.NewInternalError("系统错误"), 0
	}

	if project.UserID != userID {
		return nil, apperrors.NewForbiddenError("只有项目所有者可以修改项目设置"), 0
	}

	updatedProject, err, affected := s.repo.Update(ctx, projectID, userID, update)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("项目不存在"), 0
		}
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			return nil, apperrors.NewConflictError("项目已存在"), 0
		}
		lg.Error("project.update.failed", zap.Error(err))
		return nil, apperrors.NewInternalError("更新失败"), 0
	}

	s.projectCache.Del(ctx, userID, projectID)
	s.bumpProjectSummaryVersionAsync(lg, userID)

	if in.SortOrder != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := s.projectCache.AddProjectID(ctx, userID, projectID, float64(*in.SortOrder)); err != nil {
				lg.Warn("project.update.cache_zset_failed", zap.Error(err))
			}
		}()
	}

	lg.Info("project.update.success", zap.Int64("affected", affected))
	return updatedProject, nil, affected
}

// Delete 把空间移入回收站。
// 删除完成后同时清理详情缓存和项目 ID 集合，是为了避免前端列表短时间内看到已经消失的空间。
func (s *ProjectService) Delete(ctx context.Context, lg *zap.Logger, userID, projectID int) (int64, error) {
	lg = lg.With(zap.Int("user_id", userID), zap.Int("project_id", projectID))

	project, err := s.repo.GetByIDAndUserID(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("项目不存在")
		}
		return 0, apperrors.NewInternalError("系统错误")
	}

	if project.UserID != userID {
		return 0, apperrors.NewForbiddenError("只有项目所有者可以删除项目")
	}

	deletedAt := time.Now()
	affected, err := s.repo.SoftDeleteByID(ctx, projectID, userID, userID, deletedAt, buildTrashedProjectName(project.Name, projectID, deletedAt), project.Name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("项目不存在")
		}
		lg.Error("project.delete.failed", zap.Error(err))
		return 0, apperrors.NewInternalError("删除失败")
	}

	s.projectCache.Del(ctx, userID, projectID)
	s.bumpProjectSummaryVersionAsync(lg, userID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.projectCache.RemProjectID(ctx, userID, projectID); err != nil {
			lg.Warn("project.delete.cache_zset_failed", zap.Error(err))
		}
	}()

	lg.Info("project.delete.success", zap.Int64("project_affected", affected))
	return affected, nil
}

func (s *ProjectService) ListTrash(ctx context.Context, lg *zap.Logger, userID int, in SpaceTrashListInput) (*ProjectListResult, error) {
	page, size := in.Page, in.Size
	if page < 1 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}

	projects, total, err := s.repo.ListDeletedByUser(ctx, userID, page, size)
	if err != nil {
		lg.Error("project.list_trash.failed", zap.Int("user_id", userID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to load trashed spaces")
	}
	return &ProjectListResult{Projects: projects, Total: total}, nil
}

func (s *ProjectService) RestoreFromTrash(ctx context.Context, lg *zap.Logger, userID, projectID int) (*models.Project, error) {
	project, err := s.repo.GetDeletedByIDAndUser(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("space not found in trash")
		}
		return nil, apperrors.NewInternalError("failed to query trashed space")
	}

	restoreName := strings.TrimSpace(project.DeletedName)
	if restoreName == "" {
		restoreName = strings.TrimSpace(project.Name)
	}
	if restoreName == "" {
		return nil, apperrors.NewConflictError("space name is missing")
	}

	existing, err := s.repo.GetByUserName(ctx, userID, restoreName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NewInternalError("failed to restore space")
	}
	if existing != nil && existing.ID != 0 {
		return nil, apperrors.NewConflictError("a space with the same name already exists")
	}

	affected, err := s.repo.RestoreByID(ctx, projectID, userID, restoreName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("space not found in trash")
		}
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "1062") {
			return nil, apperrors.NewConflictError("a space with the same name already exists")
		}
		return nil, apperrors.NewInternalError("failed to restore space")
	}
	if affected == 0 {
		return nil, apperrors.NewConflictError("space restore conflict")
	}

	restored, err := s.repo.GetByIDAndUserID(ctx, projectID, userID)
	if err != nil {
		return nil, apperrors.NewInternalError("failed to load restored space")
	}

	s.projectCache.Del(ctx, userID, projectID)
	s.bumpProjectSummaryVersionAsync(lg, userID)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.projectCache.AddProjectID(ctx, userID, restored.ID, float64(restored.SortOrder))
	}()

	return restored, nil
}

func (s *ProjectService) DeleteFromTrash(ctx context.Context, lg *zap.Logger, userID, projectID int) (int64, error) {
	if _, err := s.repo.GetDeletedByIDAndUser(ctx, projectID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("space not found in trash")
		}
		return 0, apperrors.NewInternalError("failed to query trashed space")
	}

	affected, _, err := s.repo.DeleteWithTasks(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("space not found in trash")
		}
		return 0, apperrors.NewInternalError("failed to permanently delete space")
	}
	s.projectCache.Del(ctx, userID, projectID)
	s.bumpProjectSummaryVersionAsync(lg, userID)
	return affected, nil
}

func buildTrashedProjectName(name string, id int, deletedAt time.Time) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = fmt.Sprintf("space-%d", id)
	}
	return fmt.Sprintf("%s [trashed-%d-%d]", base, id, deletedAt.Unix())
}
