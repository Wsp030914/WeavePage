package handlers

import "ToDoList/server/cache"

// Deps holds runtime dependencies used by async handlers.
// It is initialized once during server startup.
type Deps struct {
	Cache        cache.Cache
	UserCache    cache.UserCache
	ProjectCache cache.ProjectCache
}

var globalDeps Deps

func InitDeps(deps Deps) {
	globalDeps = deps
}
