package service

// 文件说明：这个文件为对应模块提供测试，重点保护关键边界、并发语义和容易回归的行为。
// 实现方式：通过 stub、最小集成场景或显式断言覆盖最脆弱的逻辑分支。
// 这样做的好处是后续重构、补注释或调整实现时，可以快速发现行为回归。

import (
	apperrors "ToDoList/server/errors"
	"ToDoList/server/models"
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type taskCommentRepoStub struct {
	createFn        func(ctx context.Context, comment *models.TaskComment) (*models.TaskComment, error)
	getByIDFn       func(ctx context.Context, id int) (*models.TaskComment, error)
	getDetailByIDFn func(ctx context.Context, id int) (*models.TaskCommentInfo, error)
	listByTaskIDFn  func(ctx context.Context, taskID int) ([]models.TaskCommentInfo, error)
	updateFn        func(ctx context.Context, id int, updates map[string]interface{}) (*models.TaskComment, error)
	deleteByIDFn    func(ctx context.Context, id int) (int64, error)
}

func (s *taskCommentRepoStub) Create(ctx context.Context, comment *models.TaskComment) (*models.TaskComment, error) {
	if s.createFn != nil {
		return s.createFn(ctx, comment)
	}
	panic("unexpected call to Create")
}

func (s *taskCommentRepoStub) GetByID(ctx context.Context, id int) (*models.TaskComment, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	panic("unexpected call to GetByID")
}

func (s *taskCommentRepoStub) GetDetailByID(ctx context.Context, id int) (*models.TaskCommentInfo, error) {
	if s.getDetailByIDFn != nil {
		return s.getDetailByIDFn(ctx, id)
	}
	panic("unexpected call to GetDetailByID")
}

func (s *taskCommentRepoStub) ListByTaskID(ctx context.Context, taskID int) ([]models.TaskCommentInfo, error) {
	if s.listByTaskIDFn != nil {
		return s.listByTaskIDFn(ctx, taskID)
	}
	panic("unexpected call to ListByTaskID")
}

func (s *taskCommentRepoStub) Update(ctx context.Context, id int, updates map[string]interface{}) (*models.TaskComment, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, id, updates)
	}
	panic("unexpected call to Update")
}

func (s *taskCommentRepoStub) DeleteByID(ctx context.Context, id int) (int64, error) {
	if s.deleteByIDFn != nil {
		return s.deleteByIDFn(ctx, id)
	}
	panic("unexpected call to DeleteByID")
}

type taskCommentSessionStub struct {
	openFn func(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*TaskContentSession, error)
}

func (s *taskCommentSessionStub) OpenTaskContentSession(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*TaskContentSession, error) {
	if s.openFn != nil {
		return s.openFn(ctx, lg, uid, taskID)
	}
	panic("unexpected call to OpenTaskContentSession")
}

// TestTaskCommentServiceCreate_Success 验证评论创建成功时会按会话权限补齐任务、项目和作者信息。
func TestTaskCommentServiceCreate_Success(t *testing.T) {
	repoStub := &taskCommentRepoStub{
		createFn: func(ctx context.Context, comment *models.TaskComment) (*models.TaskComment, error) {
			if comment.ProjectID != 9 || comment.TaskID != 3 || comment.UserID != 7 {
				t.Fatalf("unexpected comment create payload: %+v", comment)
			}
			if comment.ContentMD != "hello world" {
				t.Fatalf("expected trimmed content, got %q", comment.ContentMD)
			}
			comment.ID = 11
			return comment, nil
		},
		getDetailByIDFn: func(ctx context.Context, id int) (*models.TaskCommentInfo, error) {
			if id != 11 {
				t.Fatalf("unexpected comment id: %d", id)
			}
			return &models.TaskCommentInfo{
				TaskComment: models.TaskComment{
					ID:        11,
					ProjectID: 9,
					TaskID:    3,
					UserID:    7,
					ContentMD: "hello world",
				},
				User: models.UserInfo{Username: "alice"},
			}, nil
		},
	}

	svc := NewTaskCommentService(TaskCommentServiceDeps{
		Repo: repoStub,
		TaskSession: &taskCommentSessionStub{
			openFn: func(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*TaskContentSession, error) {
				return &TaskContentSession{
					UserID:    uid,
					ProjectID: 9,
					TaskID:    taskID,
					Role:      models.RoleOwner,
					CanEdit:   true,
				}, nil
			},
		},
	})

	comment, err := svc.Create(context.Background(), zap.NewNop(), 7, 3, CreateTaskCommentInput{ContentMD: "  hello world  "})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if comment.ID != 11 || comment.User.Username != "alice" {
		t.Fatalf("unexpected created comment: %+v", comment)
	}
}

// TestTaskCommentServiceCreate_RejectsBlankContent 验证空白评论内容会被拒绝。
func TestTaskCommentServiceCreate_RejectsBlankContent(t *testing.T) {
	svc := NewTaskCommentService(TaskCommentServiceDeps{
		Repo:        &taskCommentRepoStub{},
		TaskSession: &taskCommentSessionStub{},
	})

	_, err := svc.Create(context.Background(), zap.NewNop(), 7, 3, CreateTaskCommentInput{ContentMD: "   "})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *apperrors.Error
	if !apperrors.As(err, &appErr) || appErr.Code != apperrors.CodeParamInvalid {
		t.Fatalf("expected param error, got %v", err)
	}
}

// TestTaskCommentServiceUpdate_RejectsViewerResolvingOthersComment 验证 viewer 不能关闭他人的评论。
func TestTaskCommentServiceUpdate_RejectsViewerResolvingOthersComment(t *testing.T) {
	repoStub := &taskCommentRepoStub{
		getByIDFn: func(ctx context.Context, id int) (*models.TaskComment, error) {
			return &models.TaskComment{ID: id, TaskID: 3, UserID: 99}, nil
		},
	}

	svc := NewTaskCommentService(TaskCommentServiceDeps{
		Repo: repoStub,
		TaskSession: &taskCommentSessionStub{
			openFn: func(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*TaskContentSession, error) {
				return &TaskContentSession{
					UserID:    uid,
					ProjectID: 9,
					TaskID:    taskID,
					Role:      models.RoleViewer,
					CanEdit:   false,
				}, nil
			},
		},
	})

	resolved := true
	_, err := svc.Update(context.Background(), zap.NewNop(), 7, 11, UpdateTaskCommentInput{
		Resolved: &resolved,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *apperrors.Error
	if !apperrors.As(err, &appErr) || appErr.Code != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

// TestTaskCommentServiceDelete_AllowsEditorModerator 验证 editor 角色可以执行评论治理删除。
func TestTaskCommentServiceDelete_AllowsEditorModerator(t *testing.T) {
	deleted := false
	repoStub := &taskCommentRepoStub{
		getByIDFn: func(ctx context.Context, id int) (*models.TaskComment, error) {
			return &models.TaskComment{ID: id, TaskID: 3, UserID: 99}, nil
		},
		deleteByIDFn: func(ctx context.Context, id int) (int64, error) {
			deleted = true
			return 1, nil
		},
	}

	svc := NewTaskCommentService(TaskCommentServiceDeps{
		Repo: repoStub,
		TaskSession: &taskCommentSessionStub{
			openFn: func(ctx context.Context, lg *zap.Logger, uid int, taskID int) (*TaskContentSession, error) {
				return &TaskContentSession{
					UserID:    uid,
					ProjectID: 9,
					TaskID:    taskID,
					Role:      models.RoleEditor,
					CanEdit:   true,
				}, nil
			},
		},
	})

	if err := svc.Delete(context.Background(), zap.NewNop(), 7, 11); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if !deleted {
		t.Fatal("expected delete to be called")
	}
}

// TestTaskCommentServiceDelete_CommentNotFound 验证删除不存在评论时会返回 not found。
func TestTaskCommentServiceDelete_CommentNotFound(t *testing.T) {
	svc := NewTaskCommentService(TaskCommentServiceDeps{
		Repo: &taskCommentRepoStub{
			getByIDFn: func(ctx context.Context, id int) (*models.TaskComment, error) {
				return nil, gorm.ErrRecordNotFound
			},
		},
		TaskSession: &taskCommentSessionStub{},
	})

	err := svc.Delete(context.Background(), zap.NewNop(), 7, 11)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, apperrors.NewNotFoundError("comment not found")) {
		var appErr *apperrors.Error
		if !apperrors.As(err, &appErr) || appErr.Code != apperrors.CodeNotFound {
			t.Fatalf("expected not found error, got %v", err)
		}
	}
}
