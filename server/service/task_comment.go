package service

// 文件说明：这个文件实现文档评论业务。
// 实现方式：先复用任务正文会话做权限校验，再通过 repo 完成评论的增删改查。
// 这样做的好处是评论能力不需要再维护一套独立 ACL，和正文协作权限保持一致。
import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"ToDoList/server/repo"
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const maxTaskCommentLength = 8000

type TaskCommentSessionOpener interface {
	OpenTaskContentSession(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*TaskContentSession, error)
}

type TaskCommentServiceDeps struct {
	Repo        repo.TaskCommentRepository
	TaskSession TaskCommentSessionOpener
}

type TaskCommentService struct {
	repo        repo.TaskCommentRepository
	taskSession TaskCommentSessionOpener
}

type UpdateTaskCommentInput struct {
	Resolved *bool
}

type CreateTaskCommentInput struct {
	ContentMD  string
	AnchorType string
	AnchorText string
}

// NewTaskCommentService 创建评论服务。
func NewTaskCommentService(deps TaskCommentServiceDeps) *TaskCommentService {
	return &TaskCommentService{
		repo:        deps.Repo,
		taskSession: deps.TaskSession,
	}
}

// ListByTask 返回文档下的评论列表。
// 先打开正文会话再查评论，是为了复用同一套文档访问控制，不让 diary/private 文档绕过限制。
func (s *TaskCommentService) ListByTask(ctx context.Context, lg *zap.Logger, uid, taskID int) ([]models.TaskCommentInfo, error) {
	if s.repo == nil || s.taskSession == nil {
		return nil, apperrors.NewInternalError("task comments are not configured")
	}
	if _, err := s.taskSession.OpenTaskContentSession(ctx, lg, uid, taskID); err != nil {
		return nil, err
	}

	comments, err := s.repo.ListByTaskID(ctx, taskID)
	if err != nil {
		lg.Error("task_comment.list.failed", zap.Int("uid", uid), zap.Int("task_id", taskID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to load comments")
	}
	return comments, nil
}

// Create 创建评论并回查详情。
// 这里写入后立刻回查详情，而不是直接返回基础表记录，是为了把用户名、头像等展示字段一次性补齐给前端。
func (s *TaskCommentService) Create(ctx context.Context, lg *zap.Logger, uid, taskID int, in CreateTaskCommentInput) (*models.TaskCommentInfo, error) {
	if s.repo == nil || s.taskSession == nil {
		return nil, apperrors.NewInternalError("task comments are not configured")
	}

	contentMD := strings.TrimSpace(in.ContentMD)
	if contentMD == "" {
		return nil, apperrors.NewParamError("content_md is required")
	}
	if len(contentMD) > maxTaskCommentLength {
		return nil, apperrors.NewParamError("content_md is too long")
	}

	session, err := s.taskSession.OpenTaskContentSession(ctx, lg, uid, taskID)
	if err != nil {
		return nil, err
	}
	anchorType := strings.TrimSpace(in.AnchorType)
	if anchorType == "" {
		anchorType = "document"
	}
	if anchorType != "document" && anchorType != "selection" {
		return nil, apperrors.NewParamError("invalid anchor_type")
	}

	created, err := s.repo.Create(ctx, &models.TaskComment{
		ProjectID:  session.ProjectID,
		TaskID:     session.TaskID,
		UserID:     uid,
		ContentMD:  contentMD,
		AnchorType: anchorType,
		AnchorText: strings.TrimSpace(in.AnchorText),
	})
	if err != nil {
		lg.Error("task_comment.create.failed", zap.Int("uid", uid), zap.Int("task_id", taskID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to create comment")
	}

	comment, err := s.repo.GetDetailByID(ctx, created.ID)
	if err != nil {
		lg.Error("task_comment.create.reload_failed", zap.Int("comment_id", created.ID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to load created comment")
	}
	return comment, nil
}

// Update 更新评论状态。
// resolved 的权限规则区分“评论作者”和“文档 owner/editor”，是为了支持讨论治理但避免 viewer 随意关闭他人评论。
func (s *TaskCommentService) Update(ctx context.Context, lg *zap.Logger, uid, commentID int, in UpdateTaskCommentInput) (*models.TaskCommentInfo, error) {
	if s.repo == nil || s.taskSession == nil {
		return nil, apperrors.NewInternalError("task comments are not configured")
	}
	if commentID <= 0 {
		return nil, apperrors.NewParamError("invalid comment id")
	}

	comment, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("comment not found")
		}
		lg.Error("task_comment.update.get_failed", zap.Int("comment_id", commentID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to query comment")
	}

	session, err := s.taskSession.OpenTaskContentSession(ctx, lg, uid, comment.TaskID)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})
	if in.Resolved != nil {
		if uid != comment.UserID && !canModerateTaskComments(session.Role) {
			return nil, apperrors.NewForbiddenError("no permission to update comment state")
		}
		updates["resolved"] = *in.Resolved
		if *in.Resolved {
			now := time.Now()
			updates["resolved_by"] = uid
			updates["resolved_at"] = &now
		} else {
			updates["resolved_by"] = nil
			updates["resolved_at"] = nil
		}
	}

	if len(updates) == 0 {
		return nil, apperrors.NewParamError("no comment fields to update")
	}

	if _, err := s.repo.Update(ctx, commentID, updates); err != nil {
		lg.Error("task_comment.update.failed", zap.Int("comment_id", commentID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to update comment")
	}

	detail, err := s.repo.GetDetailByID(ctx, commentID)
	if err != nil {
		lg.Error("task_comment.update.reload_failed", zap.Int("comment_id", commentID), zap.Error(err))
		return nil, apperrors.NewInternalError("failed to load updated comment")
	}
	return detail, nil
}

// Delete 删除评论。
// 删除前再次打开正文会话，是为了在评论所属文档权限变化后仍然按最新权限判断，而不是信任旧快照。
func (s *TaskCommentService) Delete(ctx context.Context, lg *zap.Logger, uid, commentID int) error {
	if s.repo == nil || s.taskSession == nil {
		return apperrors.NewInternalError("task comments are not configured")
	}
	if commentID <= 0 {
		return apperrors.NewParamError("invalid comment id")
	}

	comment, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.NewNotFoundError("comment not found")
		}
		lg.Error("task_comment.delete.get_failed", zap.Int("comment_id", commentID), zap.Error(err))
		return apperrors.NewInternalError("failed to query comment")
	}

	session, err := s.taskSession.OpenTaskContentSession(ctx, lg, uid, comment.TaskID)
	if err != nil {
		return err
	}
	if uid != comment.UserID && !canModerateTaskComments(session.Role) {
		return apperrors.NewForbiddenError("no permission to delete comment")
	}

	affected, err := s.repo.DeleteByID(ctx, commentID)
	if err != nil {
		lg.Error("task_comment.delete.failed", zap.Int("comment_id", commentID), zap.Error(err))
		return apperrors.NewInternalError("failed to delete comment")
	}
	if affected == 0 {
		return apperrors.NewNotFoundError("comment not found")
	}
	return nil
}

// canModerateTaskComments 判断当前文档角色是否拥有评论治理能力。
func canModerateTaskComments(role string) bool {
	return role == models.RoleOwner || role == models.RoleEditor
}
