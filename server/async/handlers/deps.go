package handlers

import "ToDoList/server/cache"

type Deps struct {
	Cache        cache.Cache
	UserCache    cache.UserCache
	ProjectCache cache.ProjectCache
}

var globalDeps Deps

func InitDeps(deps Deps) {
	globalDeps = deps
}
