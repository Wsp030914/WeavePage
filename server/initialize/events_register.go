package initialize

// 文件说明：这个文件负责注册异步事件消费者与处理器依赖。
// 实现方式：先把缓存依赖注入 handlers，再集中把 topic 名称映射到具体处理函数。
// 这样做的好处是异步副作用装配路径集中，新增消费者时不需要到主启动文件里四处拼接注册逻辑。

import (
	"ToDoList/server/async"
	"ToDoList/server/async/handlers"
	"ToDoList/server/cache"
)

type AsyncHandlerDeps struct {
	Cache        cache.Cache
	UserCache    cache.UserCache
	ProjectCache cache.ProjectCache
}

// InitAsyncHandlers 初始化异步处理器依赖并注册 topic -> handler 映射。
// 这里集中注册所有主题，是为了让 Kafka 消费面有一个清晰的装配入口。
func InitAsyncHandlers(consumer *async.KafkaConsumer, deps AsyncHandlerDeps) {
	handlers.InitDeps(handlers.Deps{
		Cache:        deps.Cache,
		UserCache:    deps.UserCache,
		ProjectCache: deps.ProjectCache,
	})

	consumer.Register("DeleteCOS", handlers.DeleteCosObject)
	consumer.Register("UpdateAvatar", handlers.UpdateAvatarKey)
	consumer.Register("PutVersion", handlers.PutVersion)
	consumer.Register("PutAvatar", handlers.UpdateAvatarKey)
	consumer.Register("PutProjectsSummaryCache", handlers.PutProjectsSummary)
	consumer.Register("TaskDue", handlers.SendTaskDueNotification)
}
