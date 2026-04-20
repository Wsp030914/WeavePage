package service

// 文件说明：这个文件为对应模块提供测试，重点保护关键边界、并发语义和容易回归的行为。
// 实现方式：通过 stub、最小集成场景或显式断言覆盖最脆弱的逻辑分支。
// 这样做的好处是后续重构、补注释或调整实现时，可以快速发现行为回归。

import (
	"ToDoList/server/async"
	"ToDoList/server/models"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

type projectRepoStub struct {
	listFn              func(ctx context.Context, userID int, page, size int) ([]models.Project, int64, error)
	searchFn            func(ctx context.Context, userID int, name string, page, size int) ([]models.Project, int64, error)
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
	if s.searchFn != nil {
		return s.searchFn(ctx, userID, name, page, size)
	}
	panic("unexpected call to Search")
}

func (s *projectRepoStub) Update(ctx context.Context, id, userID int, updates map[string]interface{}) (*models.Project, error, int64) {
	panic("unexpected call to Update")
}

func (s *projectRepoStub) GetDeletedByIDAndUser(ctx context.Context, id, userID int) (*models.Project, error) {
	panic("unexpected call to GetDeletedByIDAndUser")
}

func (s *projectRepoStub) ListDeletedByUser(ctx context.Context, userID, page, size int) ([]models.Project, int64, error) {
	panic("unexpected call to ListDeletedByUser")
}

func (s *projectRepoStub) SoftDeleteByID(ctx context.Context, id, userID, deletedBy int, deletedAt time.Time, trashedName, deletedName string) (int64, error) {
	panic("unexpected call to SoftDeleteByID")
}

func (s *projectRepoStub) RestoreByID(ctx context.Context, id, userID int, name string) (int64, error) {
	panic("unexpected call to RestoreByID")
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
	getProjectIDsFn      func(ctx context.Context, uid int, page, size int) ([]int, error)
	msetFn               func(ctx context.Context, uid int, projects []models.Project) error
	setProjectIDsFn      func(ctx context.Context, uid int, items []models.ProjectIDScore) error
	getSummaryVersionFn  func(ctx context.Context, uid int) (int64, error)
	bumpSummaryVersionFn func(ctx context.Context, uid int) (int64, error)
	getSummaryFn         func(ctx context.Context, uid int, ver int64, name string, page, size int) (*models.ProjectSummaryCache, error)
	setSummaryFn         func(ctx context.Context, summary models.ProjectSummaryCache) error
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

func (s *projectCacheStub) GetSummaryVersion(ctx context.Context, uid int) (int64, error) {
	if s.getSummaryVersionFn != nil {
		return s.getSummaryVersionFn(ctx, uid)
	}
	return 0, nil
}

func (s *projectCacheStub) BumpSummaryVersion(ctx context.Context, uid int) (int64, error) {
	if s.bumpSummaryVersionFn != nil {
		return s.bumpSummaryVersionFn(ctx, uid)
	}
	return 0, nil
}

func (s *projectCacheStub) GetSummary(ctx context.Context, uid int, ver int64, name string, page, size int) (*models.ProjectSummaryCache, error) {
	if s.getSummaryFn != nil {
		return s.getSummaryFn(ctx, uid, ver, name, page, size)
	}
	return nil, errors.New("summary cache miss")
}

func (s *projectCacheStub) SetSummary(ctx context.Context, summary models.ProjectSummaryCache) error {
	if s.setSummaryFn != nil {
		return s.setSummaryFn(ctx, summary)
	}
	return nil
}

type projectEventBusStub struct {
	mu      sync.Mutex
	topics  []string
	payload []any
}

func (s *projectEventBusStub) Publish(ctx context.Context, topic string, payload any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topics = append(s.topics, topic)
	s.payload = append(s.payload, payload)
	return true
}

func (s *projectEventBusStub) Close() error { return nil }

func (s *projectEventBusStub) lastPayload() (string, any, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.topics) == 0 {
		return "", nil, false
	}
	idx := len(s.topics) - 1
	return s.topics[idx], s.payload[idx], true
}

var _ async.IEventBus = (*projectEventBusStub)(nil)

// TestProjectServiceList_DBFallbackRepairsCacheAsync 验证项目列表走数据库降级后会异步修复详情缓存和 ID 列表缓存。
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

func TestProjectServiceList_SearchUsesProjectSummaryCache(t *testing.T) {
	t.Parallel()

	const userID = 42
	expected := []models.Project{{ID: 1001, UserID: userID, Name: "Alpha"}}
	svc := NewProjectService(ProjectServiceDeps{
		Repo: &projectRepoStub{},
		ProjectCache: &projectCacheStub{
			getSummaryVersionFn: func(ctx context.Context, uid int) (int64, error) {
				if uid != userID {
					t.Fatalf("unexpected uid: got %d want %d", uid, userID)
				}
				return 99, nil
			},
			getSummaryFn: func(ctx context.Context, uid int, ver int64, name string, page, size int) (*models.ProjectSummaryCache, error) {
				if uid != userID || ver != 99 || name != "Alpha" || page != 1 || size != 20 {
					t.Fatalf("unexpected summary lookup: uid=%d ver=%d name=%q page=%d size=%d", uid, ver, name, page, size)
				}
				return &models.ProjectSummaryCache{
					Projects: expected,
					Total:    int64(len(expected)),
				}, nil
			},
		},
	})

	result, err := svc.List(context.Background(), zap.NewNop(), userID, ProjectListInput{Page: 1, Size: 20, Name: " Alpha "})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if result.Total != int64(len(expected)) || len(result.Projects) != len(expected) || result.Projects[0].ID != expected[0].ID {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestProjectServiceList_SearchPublishesProjectSummaryCacheMiss(t *testing.T) {
	t.Parallel()

	const userID = 42
	expected := []models.Project{{ID: 1001, UserID: userID, Name: "Alpha"}}
	bus := &projectEventBusStub{}
	svc := NewProjectService(ProjectServiceDeps{
		Repo: &projectRepoStub{
			searchFn: func(ctx context.Context, uid int, name string, page, size int) ([]models.Project, int64, error) {
				if uid != userID || name != "Alpha" || page != 1 || size != 20 {
					t.Fatalf("unexpected search call: uid=%d name=%q page=%d size=%d", uid, name, page, size)
				}
				return expected, int64(len(expected)), nil
			},
		},
		ProjectCache: &projectCacheStub{
			getSummaryVersionFn: func(ctx context.Context, uid int) (int64, error) {
				return 123, nil
			},
			getSummaryFn: func(ctx context.Context, uid int, ver int64, name string, page, size int) (*models.ProjectSummaryCache, error) {
				return nil, errors.New("summary cache miss")
			},
		},
		Bus: bus,
	})

	result, err := svc.List(context.Background(), zap.NewNop(), userID, ProjectListInput{Page: 1, Size: 20, Name: "Alpha"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if result.Total != int64(len(expected)) {
		t.Fatalf("unexpected total: got %d want %d", result.Total, len(expected))
	}

	topic, payload, ok := bus.lastPayload()
	if !ok {
		t.Fatal("expected summary cache publish")
	}
	if topic != "PutProjectsSummaryCache" {
		t.Fatalf("unexpected topic: %s", topic)
	}
	summary, ok := payload.(models.ProjectSummaryCache)
	if !ok {
		t.Fatalf("unexpected payload type: %T", payload)
	}
	if summary.UID != userID || summary.Ver != 123 || summary.Name != "Alpha" || summary.Page != 1 || summary.Size != 20 {
		t.Fatalf("unexpected summary metadata: %+v", summary)
	}
	if summary.Total != int64(len(expected)) || len(summary.Projects) != len(expected) {
		t.Fatalf("unexpected summary payload: %+v", summary)
	}
}
