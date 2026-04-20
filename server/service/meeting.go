package service

// 文件说明：这个文件实现会议纪要创建相关业务。
// 实现方式：围绕“找到或创建会议空间、生成标题模板、创建协作文档”这条链路做编排。
// 这样做的好处是会议入口可以保持一键创建，同时把空间创建、标题冲突处理和默认模板注入集中管理。

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

type MeetingActionTodoInput struct {
	Title string
	DueAt *time.Time
}

// CreateMeetingNote 创建一份协作型会议纪要。
// 标题冲突时按次数重试生成备选标题，是为了让“快速创建会议”尽量无感成功，不把重命名压力推给前端。
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

func (s *TaskService) CreateTodoFromMeetingAction(ctx context.Context, lg *zap.Logger, uid, meetingID int, in MeetingActionTodoInput) (*models.Task, error) {
	meeting, err := s.GetDetail(ctx, lg, uid, meetingID)
	if err != nil {
		return nil, err
	}
	if meeting.DocType != models.DocTypeMeeting {
		return nil, apperrors.NewParamError("document is not a meeting note")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, apperrors.NewParamError("todo title is required")
	}
	content := fmt.Sprintf("Created from meeting: %s", meeting.Title)
	return s.Create(ctx, lg, uid, CreateTaskInput{
		Title:             title,
		ProjectID:         meeting.ProjectID,
		ContentMD:         &content,
		DocType:           models.DocTypeTodo,
		CollaborationMode: models.CollaborationModeCollaborative,
		Status:            stringPtr(models.TaskTodo),
		DueAt:             in.DueAt,
	})
}

// resolveMeetingProject 决定会议纪要落在哪个空间。
// 调用方显式传了 project_id 时优先复用该空间，否则自动落到“会议”空间。
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

// getOrCreateMeetingProject 获取或创建用户的“会议”空间。
// 这里对并发创建场景做了 duplicate 回查，是为了避免用户快速重复点击时创建出多个同名会议空间。
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

// cacheProjectAsync 异步把自动创建的空间回填到项目缓存。
// 会议空间和日记空间都复用这段逻辑，是为了避免重复维护同一套缓存热身代码。
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

// cacheMeetingProjectAsync 回填会议空间缓存。
func (s *TaskService) cacheMeetingProjectAsync(lg *zap.Logger, uid int, project *models.Project) {
	cacheProjectAsync(s.projectCache, lg, uid, project, "meeting.create")
}

// meetingTitleByAttempt 根据重试次数生成会议标题。
func meetingTitleByAttempt(baseTitle string, attempt int) string {
	if attempt == 0 {
		return baseTitle
	}
	return fmt.Sprintf("%s (%d)", baseTitle, attempt+1)
}

// defaultMeetingTitle 生成默认会议标题。
func defaultMeetingTitle(now time.Time) string {
	return "会议纪要 " + now.In(shanghaiLocation()).Format("2006-01-02 15:04")
}

// defaultMeetingTemplate 生成会议纪要默认 Markdown 模板。
// 默认模板把时间、参会人、议题、结论和行动项一次性铺开，是为了让会议记录从创建瞬间就进入可填写状态。
func defaultMeetingTemplate(now time.Time) string {
	timestamp := now.In(shanghaiLocation()).Format("2006-01-02 15:04")
	return fmt.Sprintf(
		"# 会议纪要\n\n## 时间\n\n- %s (Asia/Shanghai)\n\n## 参会人\n\n- \n\n## 议题\n\n-\n\n## 结论\n\n-\n\n## 行动项\n\n- [ ] Owner - 截止日期\n",
		timestamp,
	)
}

// shanghaiLocation 返回 Asia/Shanghai 时区。
// 这里对时区加载失败做 fixed zone 兜底，是为了让标题和模板时间在运行环境不完整时仍然稳定。
func shanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("Asia/Shanghai", 8*60*60)
	}
	return loc
}
