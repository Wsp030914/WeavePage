package service

import (
	"ToDoList/server/models"
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

type projectRepoStub struct {
	listFn              func(ctx context.Context, userID int, page, size int) ([]models.Project, int64, error)
	getAllIDsFn         func(ctx context.Context, userID int) ([]models.ProjectIDScore, error)
	getByIDsAndUserIDFn func(ctx context.Context, ids []int, userID int) ([]models.Project, error)
}

func (s *projectRepoStub) Create(ctx context.Context, project *models.Project) (*models.Project, error) {
	panic("unexpected call to Create")
}

func (s *projectRepoStub) GetByID(ctx context.Context, id int) (*models.Project, error) {
	panic("unexpected call to GetByID")
}

func (s *projectRepoStub) GetByIDAndUserID(ctx context.Context, id, userID int) (*models.Project, error) {
	panic("unexpected call to GetByIDAndUserID")
}

func (s *projectRepoStub) GetByUserName(ctx context.Context, userID int, name string) (*models.Project, error) {
	panic("unexpected call to GetByUserName")
}

func (s *projectRepoStub) GetByIDsAndUserID(ctx context.Context, ids []int, userID int) ([]models.Project, error) {
	if s.getByIDsAndUserIDFn != nil {
		return s.getByIDsAndUserIDFn(ctx, ids, userID)
	}
	panic("unexpected call to GetByIDsAndUserID")
}

func (s *projectRepoStub) List(ctx context.Context, userID int, page, size int) ([]models.Project, int64, error) {
	if s.listFn != nil {
		return s.listFn(ctx, userID, page, size)
	}
	panic("unexpected call to List")
}

func (s *projectRepoStub) Search(ctx context.Context, userID int, name string, page, size int) ([]models.Project, int64, error) {
	panic("unexpected call to Search")
}

func (s *projectRepoStub) Update(ctx context.Context, id, userID int, updates map[string]interface{}) (*models.Project, error, int64) {
	panic("unexpected call to Update")
}

func (s *projectRepoStub) DeleteWithTasks(ctx context.Context, id, userID int) (projAffected, taskAffected int64, err error) {
	panic("unexpected call to DeleteWithTasks")
}

func (s *projectRepoStub) GetAllIDs(ctx context.Context, userID int) ([]models.ProjectIDScore, error) {
	if s.getAllIDsFn != nil {
		return s.getAllIDsFn(ctx, userID)
	}
	panic("unexpected call to GetAllIDs")
}

type projectCacheStub struct {
	getProjectIDsFn func(ctx context.Context, uid int, page, size int) ([]int, error)
	msetFn          func(ctx context.Context, uid int, projects []models.Project) error
	setProjectIDsFn func(ctx context.Context, uid int, items []models.ProjectIDScore) error
}

func (s *projectCacheStub) Get(ctx context.Context, uid, pid int) (*models.Project, error) {
	panic("unexpected call to Get")
}

func (s *projectCacheStub) MGet(ctx context.Context, uid int, pids []int) (map[int]*models.Project, []int, error) {
	panic("unexpected call to MGet")
}

func (s *projectCacheStub) Set(ctx context.Context, uid, pid int, project *models.Project) error {
	panic("unexpected call to Set")
}

func (s *projectCacheStub) MSet(ctx context.Context, uid int, projects []models.Project) error {
	if s.msetFn != nil {
		return s.msetFn(ctx, uid, projects)
	}
	return nil
}

func (s *projectCacheStub) Del(ctx context.Context, uid, pid int) {}

func (s *projectCacheStub) SetProjectIDs(ctx context.Context, uid int, items []models.ProjectIDScore) error {
	if s.setProjectIDsFn != nil {
		return s.setProjectIDsFn(ctx, uid, items)
	}
	return nil
}

func (s *projectCacheStub) GetProjectIDs(ctx context.Context, uid int, page, size int) ([]int, error) {
	if s.getProjectIDsFn != nil {
		return s.getProjectIDsFn(ctx, uid, page, size)
	}
	panic("unexpected call to GetProjectIDs")
}

func (s *projectCacheStub) CountProjectIDs(ctx context.Context, uid int) (int64, error) {
	panic("unexpected call to CountProjectIDs")
}

func (s *projectCacheStub) AddProjectID(ctx context.Context, uid int, pid int, score float64) error {
	panic("unexpected call to AddProjectID")
}

func (s *projectCacheStub) RemProjectID(ctx context.Context, uid, pid int) error {
	panic("unexpected call to RemProjectID")
}

func TestProjectServiceList_DBFallbackRepairsCacheAsync(t *testing.T) {
	t.Parallel()

	const userID = 42
	expectedProjects := []models.Project{
		{ID: 1001, UserID: userID, Name: "Alpha", SortOrder: 20},
		{ID: 1002, UserID: userID, Name: "Beta", SortOrder: 10},
	}
	expectedIDs := []models.ProjectIDScore{
		{ID: 1001, SortOrder: 20},
		{ID: 1002, SortOrder: 10},
	}

	warmed := make(chan []models.Project, 1)
	rebuilt := make(chan []models.ProjectIDScore, 1)

	svc := NewProjectService(ProjectServiceDeps{
		Repo: &projectRepoStub{
			listFn: func(ctx context.Context, uid int, page, size int) ([]models.Project, int64, error) {
				if uid != userID {
					t.Fatalf("unexpected uid: got %d want %d", uid, userID)
				}
				return expectedProjects, int64(len(expectedProjects)), nil
			},
			getAllIDsFn: func(ctx context.Context, uid int) ([]models.ProjectIDScore, error) {
				if uid != userID {
					t.Fatalf("unexpected uid: got %d want %d", uid, userID)
				}
				return expectedIDs, nil
			},
		},
		ProjectCache: &projectCacheStub{
			getProjectIDsFn: func(ctx context.Context, uid int, page, size int) ([]int, error) {
				return nil, errors.New("redis unavailable")
			},
			msetFn: func(ctx context.Context, uid int, projects []models.Project) error {
				select {
				case warmed <- append([]models.Project(nil), projects...):
				default:
				}
				return nil
			},
			setProjectIDsFn: func(ctx context.Context, uid int, items []models.ProjectIDScore) error {
				select {
				case rebuilt <- append([]models.ProjectIDScore(nil), items...):
				default:
				}
				return nil
			},
		},
	})

	result, err := svc.List(context.Background(), zap.NewNop(), userID, ProjectListInput{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if result.Total != int64(len(expectedProjects)) {
		t.Fatalf("unexpected total: got %d want %d", result.Total, len(expectedProjects))
	}
	if len(result.Projects) != len(expectedProjects) {
		t.Fatalf("unexpected project count: got %d want %d", len(result.Projects), len(expectedProjects))
	}

	select {
	case got := <-warmed:
		if len(got) != len(expectedProjects) {
			t.Fatalf("unexpected warmed detail count: got %d want %d", len(got), len(expectedProjects))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for detail cache warmup")
	}

	select {
	case got := <-rebuilt:
		if len(got) != len(expectedIDs) {
			t.Fatalf("unexpected rebuilt id count: got %d want %d", len(got), len(expectedIDs))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for list cache rebuild")
	}
}
