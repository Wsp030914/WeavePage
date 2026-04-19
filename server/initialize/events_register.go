package initialize

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
