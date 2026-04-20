package cache

// 文件说明：这个文件负责某类缓存或锁能力封装。
// 实现方式：统一封装缓存键与读写语义。
// 这样做的好处是缓存行为更一致，也更方便测试。
import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"ToDoList/server/models"

	"github.com/redis/go-redis/v9"
)

const (
	ProjectCacheExpire   = 10 * time.Minute
	ProjectListExpire    = 30 * time.Minute
	ProjectSummaryExpire = 15 * time.Minute
)

type ProjectCache interface {
	Get(ctx context.Context, uid, pid int) (*models.Project, error)
	MGet(ctx context.Context, uid int, pids []int) (map[int]*models.Project, []int, error)
	Set(ctx context.Context, uid, pid int, project *models.Project) error
	MSet(ctx context.Context, uid int, projects []models.Project) error
	Del(ctx context.Context, uid, pid int)
	SetProjectIDs(ctx context.Context, uid int, items []models.ProjectIDScore) error
	GetProjectIDs(ctx context.Context, uid int, page, size int) ([]int, error)
	CountProjectIDs(ctx context.Context, uid int) (int64, error)
	AddProjectID(ctx context.Context, uid int, pid int, score float64) error
	RemProjectID(ctx context.Context, uid, pid int) error
	GetSummaryVersion(ctx context.Context, uid int) (int64, error)
	BumpSummaryVersion(ctx context.Context, uid int) (int64, error)
	GetSummary(ctx context.Context, uid int, ver int64, name string, page, size int) (*models.ProjectSummaryCache, error)
	SetSummary(ctx context.Context, summary models.ProjectSummaryCache) error
}

type projectCache struct {
	cache Cache
	ttl   time.Duration
}

func NewProjectCache(cache Cache) ProjectCache {
	return &projectCache{cache: cache, ttl: ProjectCacheExpire}
}

func (c *projectCache) detailKey(uid, pid int) string {
	return fmt.Sprintf("project:detail:%d:%d", uid, pid)
}

func (c *projectCache) Get(ctx context.Context, uid, pid int) (*models.Project, error) {
	key := c.detailKey(uid, pid)
	val, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	if val == "{}" {
		return nil, ErrCacheNull
	}

	var project models.Project
	if err := json.Unmarshal([]byte(val), &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *projectCache) MGet(ctx context.Context, uid int, pids []int) (map[int]*models.Project, []int, error) {
	if len(pids) == 0 {
		return map[int]*models.Project{}, []int{}, nil
	}

	keys := make([]string, len(pids))
	for i, pid := range pids {
		keys[i] = c.detailKey(uid, pid)
	}

	vals, err := c.cache.MGet(ctx, keys...)
	if err != nil {
		return nil, nil, err
	}

	result := make(map[int]*models.Project)
	missing := []int{}

	for i, val := range vals {
		if val == nil {
			missing = append(missing, pids[i])
			continue
		}

		s, ok := val.(string)
		if !ok {
			missing = append(missing, pids[i])
			continue
		}

		if s == "{}" {
			continue
		}

		var project models.Project
		if err := json.Unmarshal([]byte(s), &project); err != nil {
			missing = append(missing, pids[i])
			continue
		}
		result[pids[i]] = &project
	}

	return result, missing, nil
}

func (c *projectCache) Set(ctx context.Context, uid, pid int, project *models.Project) error {
	var data []byte
	var err error

	if project == nil {
		data = []byte("{}")
	} else {
		data, err = json.Marshal(project)
		if err != nil {
			return err
		}
	}

	key := c.detailKey(uid, pid)

	ttl := c.ttl
	if project == nil {
		ttl = time.Minute
	}
	return c.cache.Set(ctx, key, string(data), ttl)
}

func (c *projectCache) MSet(ctx context.Context, uid int, projects []models.Project) error {
	for _, p := range projects {
		if err := c.Set(ctx, uid, p.ID, &p); err != nil {
			return err
		}
	}
	return nil
}

func (c *projectCache) Del(ctx context.Context, uid, pid int) {
	key := c.detailKey(uid, pid)
	_ = c.cache.Del(ctx, key)
}

func (c *projectCache) zsetKey(uid int) string {
	return fmt.Sprintf("project:zset:%d", uid)
}

func (c *projectCache) summaryVersionKey(uid int) string {
	return fmt.Sprintf("project:summary:version:%d", uid)
}

func (c *projectCache) summaryKey(uid int, ver int64, name string, page, size int) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(name)))
	return fmt.Sprintf("project:summary:%d:%d:%08x:%d:%d", uid, ver, h.Sum32(), page, size)
}

func (c *projectCache) SetProjectIDs(ctx context.Context, uid int, items []models.ProjectIDScore) error {
	key := c.zsetKey(uid)
	members := make([]redis.Z, len(items))
	for i, item := range items {
		members[i] = redis.Z{
			Score:  float64(item.SortOrder),
			Member: item.ID,
		}
	}

	if err := c.cache.Del(ctx, key); err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	if err := c.cache.ZAdd(ctx, key, members...); err != nil {
		return err
	}
	return c.cache.Expire(ctx, key, ProjectListExpire)
}

func (c *projectCache) GetProjectIDs(ctx context.Context, uid int, page, size int) ([]int, error) {
	key := c.zsetKey(uid)
	start := int64((page - 1) * size)
	stop := start + int64(size) - 1

	vals, err := c.cache.ZRevRange(ctx, key, start, stop)
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		exists, err := c.cache.Exists(ctx, key)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrCacheMiss
		}
		return []int{}, nil
	}

	ids := make([]int, len(vals))
	for i, v := range vals {
		var id int
		if _, err := fmt.Sscanf(v, "%d", &id); err != nil {
			continue
		}
		ids[i] = id
	}
	return ids, nil
}

func (c *projectCache) CountProjectIDs(ctx context.Context, uid int) (int64, error) {
	key := c.zsetKey(uid)
	exists, err := c.cache.Exists(ctx, key)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrCacheMiss
	}
	return c.cache.ZCard(ctx, key)
}

func (c *projectCache) AddProjectID(ctx context.Context, uid int, pid int, score float64) error {
	key := c.zsetKey(uid)
	exists, err := c.cache.Exists(ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return c.cache.ZAdd(ctx, key, redis.Z{Score: score, Member: pid})
}

func (c *projectCache) RemProjectID(ctx context.Context, uid, pid int) error {
	key := c.zsetKey(uid)
	return c.cache.ZRem(ctx, key, pid)
}

func (c *projectCache) GetSummaryVersion(ctx context.Context, uid int) (int64, error) {
	raw, err := c.cache.Get(ctx, c.summaryVersionKey(uid))
	if err != nil {
		if err == ErrCacheMiss {
			return 0, nil
		}
		return 0, err
	}
	ver, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return ver, nil
}

func (c *projectCache) BumpSummaryVersion(ctx context.Context, uid int) (int64, error) {
	ver := time.Now().UnixNano()
	return ver, c.cache.Set(ctx, c.summaryVersionKey(uid), strconv.FormatInt(ver, 10), ProjectListExpire)
}

func (c *projectCache) GetSummary(ctx context.Context, uid int, ver int64, name string, page, size int) (*models.ProjectSummaryCache, error) {
	raw, err := c.cache.Get(ctx, c.summaryKey(uid, ver, name, page, size))
	if err != nil {
		return nil, err
	}
	var summary models.ProjectSummaryCache
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *projectCache) SetSummary(ctx context.Context, summary models.ProjectSummaryCache) error {
	raw, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	return c.cache.Set(ctx, c.summaryKey(summary.UID, summary.Ver, summary.Name, summary.Page, summary.Size), string(raw), ProjectSummaryExpire)
}
