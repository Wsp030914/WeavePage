package service

import (
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
	sf           singleflight.Group
}

type ProjectServiceDeps struct {
	Repo         repo.ProjectRepository
	ProjectCache cache.ProjectCache
	UserRepo     repo.UserRepository
	CacheClient  cache.Cache
}

func NewProjectService(deps ProjectServiceDeps) *ProjectService {
	return &ProjectService{
		repo:         deps.Repo,
		projectCache: deps.ProjectCache,
		userRepo:     deps.UserRepo,
		cacheClient:  deps.CacheClient,
	}
}

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

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.projectCache.AddProjectID(ctx, userID, created.ID, float64(created.SortOrder)); err != nil {
			lg.Warn("project.create.cache_zset_failed", zap.Error(err))
		}
	}()

	return created, nil
}

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
	val, err, _ := s.sf.Do(key, func() (interface{}, error) {
		project, err := s.repo.GetByIDAndUserID(ctx, projectID, userID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				lg.Info("project.get.not_found")
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					_ = s.projectCache.Set(ctx, userID, projectID, nil)
				}()
				return nil, apperrors.NewNotFoundError("项目不存在")
			}
			lg.Error("project.get.failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}

		if project.UserID != userID {
			lg.Info("project.get.forbidden")
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = s.projectCache.Set(ctx, userID, projectID, nil)
			}()
			return nil, apperrors.NewNotFoundError("项目不存在")
		}

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			s.projectCache.Set(ctx, userID, projectID, project)
		}()

		return project, nil
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

	if in.Name == "" {
		ids, err := s.projectCache.GetProjectIDs(ctx, userID, page, size)
		if err != nil {
			if errors.Is(err, cache.ErrCacheMiss) {
				allProjects, err := s.repo.GetAllIDs(ctx, userID)
				if err != nil {
					lg.Error("project.list.get_all_ids_failed", zap.Error(err))
					repairCache = true
					goto DB_FALLBACK
				}

				if len(allProjects) == 0 {
					return &ProjectListResult{Projects: []models.Project{}, Total: 0}, nil
				}

				if err := s.projectCache.SetProjectIDs(ctx, userID, allProjects); err != nil {
					lg.Error("project.list.set_zset_failed", zap.Error(err))
					repairCache = true
					goto DB_FALLBACK
				}

				ids, err = s.projectCache.GetProjectIDs(ctx, userID, page, size)
				if err != nil {
					lg.Error("project.list.get_ids_after_rebuild_failed", zap.Error(err))
					repairCache = true
					goto DB_FALLBACK
				}
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
		projects, total, err := s.repo.Search(ctx, userID, strings.TrimSpace(in.Name), page, size)
		if err != nil {
			lg.Error("project.search.failed", zap.Error(err))
			return nil, apperrors.NewInternalError("系统错误")
		}
		return &ProjectListResult{Projects: projects, Total: total}, nil
	}

DB_FALLBACK:
	lg.Info("project.list.fallback_db")
	projects, total, err := s.repo.List(ctx, userID, page, size)
	if err != nil {
		lg.Error("project.list.failed", zap.Error(err))
		return nil, apperrors.NewInternalError("系统错误")
	}
	if repairCache {
		s.rebuildProjectListCacheAsync(lg, userID, projects)
	}
	return &ProjectListResult{Projects: projects, Total: total}, nil
}

type UpdateProjectInput struct {
	Name      *string
	Color     *string
	SortOrder *int64
}

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

	projAffected, taskAffected, err := s.repo.DeleteWithTasks(ctx, projectID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.NewNotFoundError("项目不存在")
		}
		lg.Error("project.delete.failed", zap.Error(err))
		return 0, apperrors.NewInternalError("删除失败")
	}

	s.projectCache.Del(ctx, userID, projectID)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.projectCache.RemProjectID(ctx, userID, projectID); err != nil {
			lg.Warn("project.delete.cache_zset_failed", zap.Error(err))
		}
	}()

	lg.Info("project.delete.success", zap.Int64("project_affected", projAffected), zap.Int64("task_affected", taskAffected))
	return projAffected, nil
}
