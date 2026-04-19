package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ToDoList/server/models"

	"github.com/redis/go-redis/v9"
)

const (
	TaskDetailExpire = 5 * time.Minute
	TaskListExpire   = 30 * time.Minute
)

type TaskCache interface {
	GetDetail(ctx context.Context, uid, taskID int) (*models.Task, error)
	SetDetail(ctx context.Context, uid, taskID int, task *models.Task) error
	DelDetail(ctx context.Context, uid, taskID int) error
	SetTaskIDs(ctx context.Context, pid int, status string, items []models.TaskIDScore) error
	GetTaskIDs(ctx context.Context, pid int, status string, page, size int) ([]int, error)
	CountTaskIDs(ctx context.Context, pid int, status string) (int64, error)
	AddTaskID(ctx context.Context, pid int, status string, taskID int, score float64) error
	RemTaskID(ctx context.Context, pid int, status string, taskID int) error

	MGetDetail(ctx context.Context, uid int, taskIDs []int) (map[int]*models.Task, []int, error)
	MSetDetail(ctx context.Context, uid int, tasks []models.Task) error
}

type taskCache struct {
	cache Cache
}

func NewTaskCache(cache Cache) TaskCache {
	return &taskCache{cache: cache}
}

func (c *taskCache) detailKey(uid, taskID int) string {
	return fmt.Sprintf("task:detail:%d:%d", uid, taskID)
}

func (c *taskCache) GetDetail(ctx context.Context, uid, taskID int) (*models.Task, error) {
	key := c.detailKey(uid, taskID)
	val, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var task models.Task
	if err := json.Unmarshal([]byte(val), &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (c *taskCache) SetDetail(ctx context.Context, uid, taskID int, task *models.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	key := c.detailKey(uid, taskID)
	return c.cache.Set(ctx, key, string(data), TaskDetailExpire)
}

func (c *taskCache) DelDetail(ctx context.Context, uid, taskID int) error {
	key := c.detailKey(uid, taskID)
	return c.cache.Del(ctx, key)
}

func (c *taskCache) zsetKey(pid int, status string) string {
	if status == "" {
		return fmt.Sprintf("task:zset:%d:all", pid)
	}
	return fmt.Sprintf("task:zset:%d:%s", pid, status)
}

func (c *taskCache) SetTaskIDs(ctx context.Context, pid int, status string, items []models.TaskIDScore) error {
	key := c.zsetKey(pid, status)
	members := make([]redis.Z, len(items))
	for i, item := range items {

		score := float64(item.SortOrder)

		members[i] = redis.Z{
			Score:  score,
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
	return c.cache.Expire(ctx, key, TaskListExpire)
}

func (c *taskCache) GetTaskIDs(ctx context.Context, pid int, status string, page, size int) ([]int, error) {
	key := c.zsetKey(pid, status)
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

func (c *taskCache) CountTaskIDs(ctx context.Context, pid int, status string) (int64, error) {
	key := c.zsetKey(pid, status)
	exists, err := c.cache.Exists(ctx, key)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, ErrCacheMiss
	}
	return c.cache.ZCard(ctx, key)
}

func (c *taskCache) AddTaskID(ctx context.Context, pid int, status string, taskID int, score float64) error {
	key := c.zsetKey(pid, status)
	exists, err := c.cache.Exists(ctx, key)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return c.cache.ZAdd(ctx, key, redis.Z{Score: score, Member: taskID})
}

func (c *taskCache) RemTaskID(ctx context.Context, pid int, status string, taskID int) error {
	key := c.zsetKey(pid, status)
	return c.cache.ZRem(ctx, key, taskID)
}

func (c *taskCache) MGetDetail(ctx context.Context, uid int, taskIDs []int) (map[int]*models.Task, []int, error) {
	if len(taskIDs) == 0 {
		return map[int]*models.Task{}, []int{}, nil
	}

	keys := make([]string, len(taskIDs))
	for i, id := range taskIDs {
		keys[i] = c.detailKey(uid, id)
	}

	vals, err := c.cache.MGet(ctx, keys...)
	if err != nil {
		return nil, nil, err
	}

	result := make(map[int]*models.Task)
	missing := []int{}

	for i, val := range vals {
		if val == nil {
			missing = append(missing, taskIDs[i])
			continue
		}

		s, ok := val.(string)
		if !ok {
			missing = append(missing, taskIDs[i])
			continue
		}

		if s == "{}" {
			continue
		}

		var task models.Task
		if err := json.Unmarshal([]byte(s), &task); err != nil {
			missing = append(missing, taskIDs[i])
			continue
		}
		result[task.ID] = &task
	}

	return result, missing, nil
}

func (c *taskCache) MSetDetail(ctx context.Context, uid int, tasks []models.Task) error {
	if len(tasks) == 0 {
		return nil
	}

	for _, task := range tasks {
		// Clone task to avoid modifying original when passing pointer
		t := task
		if err := c.SetDetail(ctx, uid, t.ID, &t); err != nil {
			return err
		}
	}
	return nil
}
