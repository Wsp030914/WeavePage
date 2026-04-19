package main

import (
	"ToDoList/server/async"
	"ToDoList/server/handler"
	"ToDoList/server/middlewares"
	"ToDoList/server/realtime"
	"ToDoList/server/service"
	"os"

	"github.com/redis/go-redis/v9"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

type App struct {
	Bus        async.IEventBus
	Rdb        *redis.Client
	Db         *gorm.DB
	ProjectHub *realtime.ProjectHub
	ContentHub *realtime.ContentHub
}

func NewRouter(ctx interface{}, app *App, userSvc *service.UserService, projectSvc *service.ProjectService, taskSvc *service.TaskService, authSvc *service.AuthService) *gin.Engine {
	r := gin.New()
	r.Use(middlewares.AccessLogMiddleware(), middlewares.RecoveryWithZap())
	if os.Getenv("DISABLE_RATE_LIMIT") != "1" {
		r.Use(middlewares.RateLimitMiddleware(400, 800))
	}
	r.Use(middlewares.CORSMiddleware())

	userCtl := handler.NewUserHandler(userSvc)
	projectCtl := handler.NewProjectHandler(projectSvc)
	taskCtl := handler.NewTaskHandler(taskSvc)
	syncCtl := handler.NewSyncHandler(taskSvc)
	projectWSCtl := handler.NewProjectWSHandler(taskSvc, authSvc, app.ProjectHub)
	contentWSCtl := handler.NewContentWSHandler(taskSvc, authSvc, app.ContentHub)

	public := r.Group("/api/v1")
	{
		public.POST("/login", userCtl.Login)
		public.POST("/register", userCtl.Register)
		public.GET("/projects/:id/ws", projectWSCtl.ProjectEvents)
		public.GET("/tasks/:id/content/ws", contentWSCtl.TaskContent)
	}

	internal := r.Group("/api/internal")
	{
		internal.POST("/scheduler/task-due", taskCtl.DueCallback)
	}

	protected := r.Group("/api/v1")
	protected.Use(middlewares.AuthMiddleware(authSvc))
	{
		protected.GET("/users/me", userCtl.GetProfile)
		protected.PATCH("/users/me", userCtl.Update)
		protected.POST("/logout", userCtl.Logout)
		protected.GET("/projects/:id", projectCtl.GetProjectByID)
		protected.GET("/projects/:id/sync", syncCtl.ProjectEvents)
		protected.GET("/projects", projectCtl.Search)
		protected.POST("/projects", projectCtl.Create)
		protected.PATCH("/projects/:id", projectCtl.Update)
		protected.DELETE("/projects/:id", projectCtl.Delete)

		protected.POST("/tasks", taskCtl.Create)
		protected.GET("/tasks/me", taskCtl.ListMyTasks)
		protected.PATCH("/projects/:id/tasks/:task_id", taskCtl.Update)
		protected.DELETE("/tasks/:id", taskCtl.Delete)
		protected.GET("/tasks/:id", taskCtl.GetDetail)
		protected.GET("/tasks", taskCtl.List)
		protected.POST("/projects/:id/tasks/:task_id/members", taskCtl.AddMember)
		protected.DELETE("/projects/:id/tasks/:task_id/members", taskCtl.RemoveMember)
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	return r
}
