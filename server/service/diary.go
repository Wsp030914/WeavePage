package service

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	diarySpaceName  = "日记"
	diarySpaceColor = "#7d8ca3"
)

// DiaryTodayResult is the response payload for opening the current user's daily note.
type DiaryTodayResult struct {
	Project *models.Project `json:"project"`
	Task    *models.Task    `json:"task"`
}

// OpenTodayDiary finds or creates the current user's daily-note space and today's diary document.
func (s *TaskService) OpenTodayDiary(ctx context.Context, lg *zap.Logger, uid int, now time.Time) (*DiaryTodayResult, error) {
	if uid <= 0 {
		return nil, apperrors.NewParamError("无效的用户")
	}
	if now.IsZero() {
		now = time.Now()
	}

	project, err := s.getOrCreateDiaryProject(ctx, lg, uid)
	if err != nil {
		return nil, err
	}

	title := todayDiaryTitle(now)
	task, err := s.repo.GetByUserProjectTitle(ctx, uid, project.ID, title)
	if err == nil {
		return &DiaryTodayResult{Project: project, Task: task}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		lg.Error("diary.today.get_task_failed", zap.Int("uid", uid), zap.Int("project_id", project.ID), zap.Error(err))
		return nil, apperrors.NewInternalError("打开日记失败")
	}

	content := fmt.Sprintf("# %s\n\n## Notes\n\n", strings.TrimSuffix(title, ".md"))
	task, err = s.Create(ctx, lg, uid, CreateTaskInput{
		Title:             title,
		ProjectID:         project.ID,
		ContentMD:         &content,
		DocType:           models.DocTypeDiary,
		CollaborationMode: models.CollaborationModePrivate,
		Status:            stringPtr(models.TaskTodo),
	})
	if err == nil {
		return &DiaryTodayResult{Project: project, Task: task}, nil
	}
	if isDuplicateDBError(err) || isConflictAppError(err) {
		task, getErr := s.repo.GetByUserProjectTitle(ctx, uid, project.ID, title)
		if getErr == nil {
			return &DiaryTodayResult{Project: project, Task: task}, nil
		}
		lg.Error("diary.today.get_duplicate_task_failed", zap.Int("uid", uid), zap.String("title", title), zap.Error(getErr))
	}
	return nil, err
}

func (s *TaskService) getOrCreateDiaryProject(ctx context.Context, lg *zap.Logger, uid int) (*models.Project, error) {
	project, err := s.projectRepo.GetByUserName(ctx, uid, diarySpaceName)
	if err == nil {
		return project, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		lg.Error("diary.today.get_space_failed", zap.Int("uid", uid), zap.Error(err))
		return nil, apperrors.NewInternalError("打开日记空间失败")
	}

	project = &models.Project{
		UserID:    uid,
		Name:      diarySpaceName,
		Color:     diarySpaceColor,
		SortOrder: time.Now().UnixNano(),
	}
	created, err := s.projectRepo.Create(ctx, project)
	if err == nil {
		s.cacheDiaryProjectAsync(lg, uid, created)
		return created, nil
	}
	if isDuplicateDBError(err) {
		existing, getErr := s.projectRepo.GetByUserName(ctx, uid, diarySpaceName)
		if getErr == nil {
			return existing, nil
		}
		lg.Error("diary.today.get_duplicate_space_failed", zap.Int("uid", uid), zap.Error(getErr))
		return nil, apperrors.NewInternalError("打开日记空间失败")
	}

	lg.Error("diary.today.create_space_failed", zap.Int("uid", uid), zap.Error(err))
	return nil, apperrors.NewInternalError("创建日记空间失败")
}

func (s *TaskService) cacheDiaryProjectAsync(lg *zap.Logger, uid int, project *models.Project) {
	cacheProjectAsync(s.projectCache, lg, uid, project, "diary.today")
}

func todayDiaryTitle(now time.Time) string {
	return now.In(shanghaiLocation()).Format("2006-01-02") + ".md"
}

func isConflictAppError(err error) bool {
	var appErr *apperrors.Error
	return apperrors.As(err, &appErr) && appErr.Code == apperrors.CodeConflict
}
