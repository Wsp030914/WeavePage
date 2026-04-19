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
	meetingSpaceName  = "会议"
	meetingSpaceColor = "#d7a85b"
	maxMeetingRetries = 5
)

// CreateMeetingInput describes how to create a meeting note.
type CreateMeetingInput struct {
	ProjectID *int
	Title     string
	Now       time.Time
}

// MeetingCreateResult contains the created meeting note and its space.
type MeetingCreateResult struct {
	Project *models.Project `json:"project"`
	Task    *models.Task    `json:"task"`
}

// CreateMeetingNote creates a collaborative meeting note with a default template.
func (s *TaskService) CreateMeetingNote(ctx context.Context, lg *zap.Logger, uid int, in CreateMeetingInput) (*MeetingCreateResult, error) {
	if uid <= 0 {
		return nil, apperrors.NewParamError("无效的用户")
	}
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	project, err := s.resolveMeetingProject(ctx, lg, uid, in.ProjectID)
	if err != nil {
		return nil, err
	}

	baseTitle := strings.TrimSpace(in.Title)
	if baseTitle == "" {
		baseTitle = defaultMeetingTitle(now)
	}
	content := defaultMeetingTemplate(now)

	for attempt := 0; attempt < maxMeetingRetries; attempt++ {
		title := meetingTitleByAttempt(baseTitle, attempt)
		task, createErr := s.Create(ctx, lg, uid, CreateTaskInput{
			Title:             title,
			ProjectID:         project.ID,
			ContentMD:         &content,
			DocType:           models.DocTypeMeeting,
			CollaborationMode: models.CollaborationModeCollaborative,
			Status:            stringPtr(models.TaskTodo),
		})
		if createErr == nil {
			return &MeetingCreateResult{Project: project, Task: task}, nil
		}
		if !isConflictAppError(createErr) && !isDuplicateDBError(createErr) {
			return nil, createErr
		}
	}

	return nil, apperrors.NewConflictError("会议纪要标题冲突，请重试")
}

func (s *TaskService) resolveMeetingProject(ctx context.Context, lg *zap.Logger, uid int, projectID *int) (*models.Project, error) {
	if projectID != nil {
		if *projectID <= 0 {
			return nil, apperrors.NewParamError("无效的空间")
		}
		project, err := s.projectRepo.GetByIDAndUserID(ctx, *projectID, uid)
		if err == nil {
			return project, nil
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("空间不存在")
		}
		lg.Error("meeting.create.get_project_failed", zap.Int("uid", uid), zap.Int("project_id", *projectID), zap.Error(err))
		return nil, apperrors.NewInternalError("查询空间失败")
	}

	return s.getOrCreateMeetingProject(ctx, lg, uid)
}

func (s *TaskService) getOrCreateMeetingProject(ctx context.Context, lg *zap.Logger, uid int) (*models.Project, error) {
	project, err := s.projectRepo.GetByUserName(ctx, uid, meetingSpaceName)
	if err == nil {
		return project, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		lg.Error("meeting.create.get_space_failed", zap.Int("uid", uid), zap.Error(err))
		return nil, apperrors.NewInternalError("查询会议空间失败")
	}

	project = &models.Project{
		UserID:    uid,
		Name:      meetingSpaceName,
		Color:     meetingSpaceColor,
		SortOrder: time.Now().UnixNano(),
	}
	created, err := s.projectRepo.Create(ctx, project)
	if err == nil {
		s.cacheMeetingProjectAsync(lg, uid, created)
		return created, nil
	}
	if isDuplicateDBError(err) {
		existing, getErr := s.projectRepo.GetByUserName(ctx, uid, meetingSpaceName)
		if getErr == nil {
			return existing, nil
		}
		lg.Error("meeting.create.get_duplicate_space_failed", zap.Int("uid", uid), zap.Error(getErr))
		return nil, apperrors.NewInternalError("查询会议空间失败")
	}

	lg.Error("meeting.create.create_space_failed", zap.Int("uid", uid), zap.Error(err))
	return nil, apperrors.NewInternalError("创建会议空间失败")
}

func cacheProjectAsync(projectCacheSetter interface {
	Set(ctx context.Context, uid, pid int, project *models.Project) error
	AddProjectID(ctx context.Context, uid int, pid int, score float64) error
}, lg *zap.Logger, uid int, project *models.Project, logKey string) {
	if projectCacheSetter == nil || project == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := projectCacheSetter.Set(ctx, uid, project.ID, project); err != nil {
			lg.Warn(logKey+".cache_project_failed", zap.Int("uid", uid), zap.Int("project_id", project.ID), zap.Error(err))
		}
		if err := projectCacheSetter.AddProjectID(ctx, uid, project.ID, float64(project.SortOrder)); err != nil {
			lg.Warn(logKey+".cache_project_id_failed", zap.Int("uid", uid), zap.Int("project_id", project.ID), zap.Error(err))
		}
	}()
}

func (s *TaskService) cacheMeetingProjectAsync(lg *zap.Logger, uid int, project *models.Project) {
	cacheProjectAsync(s.projectCache, lg, uid, project, "meeting.create")
}

func meetingTitleByAttempt(baseTitle string, attempt int) string {
	if attempt == 0 {
		return baseTitle
	}
	return fmt.Sprintf("%s (%d)", baseTitle, attempt+1)
}

func defaultMeetingTitle(now time.Time) string {
	return "会议纪要 " + now.In(shanghaiLocation()).Format("2006-01-02 15:04")
}

func defaultMeetingTemplate(now time.Time) string {
	timestamp := now.In(shanghaiLocation()).Format("2006-01-02 15:04")
	return fmt.Sprintf(
		"# 会议纪要\n\n## 时间\n\n- %s (Asia/Shanghai)\n\n## 参会人\n\n- \n\n## 议题\n\n-\n\n## 结论\n\n-\n\n## 行动项\n\n- [ ] Owner - 截止日期\n",
		timestamp,
	)
}

func shanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}
