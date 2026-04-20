package initialize

// 文件说明：这个文件负责 Redis 客户端初始化。
// 实现方式：按配置创建 redis client，并在启动阶段立即做 ping 校验。
// 这样做的好处是缓存、锁和实时 fan-out 依赖能在服务启动时尽早暴露可用性问题，而不是运行中随机失败。

import (
	"ToDoList/server/config"
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// InitRedis 初始化 Redis 客户端并验证连接。
func InitRedis(cfg *config.Config) (*redis.Client, error) {
	redisCfg := cfg.Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	return rdb, nil
}
