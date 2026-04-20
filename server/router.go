package main

// 文件说明：这个文件负责注册 Gin 路由，把公开接口、鉴权接口、内部接口与实时接口串到统一入口。
// 实现方式：按访问级别拆分路由组，并统一挂载中间件、处理器与 Swagger。
// 这样做的好处是接口边界清楚，权限控制与限流策略更容易统一维护。
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
	Bus               async.IEventBus
	Rdb               *redis.Client
	Db                *gorm.DB
	ProjectHub        *realtime.ProjectHub
	ContentHub        *realtime.ContentHub
	DocumentImportSvc *service.DocumentImportService
	AIService         *service.AIService
}

func NewRouter(ctx interface{}, app *App, userSvc *service.UserService, projectSvc *service.ProjectService, taskSvc *service.TaskService, taskCommentSvc *service.TaskCommentService, authSvc *service.AuthService) *gin.Engine {
	r := gin.New()
	r.Use(middlewares.AccessLogMiddleware(), middlewares.RecoveryWithZap())
	r.Use(middlewares.CORSMiddleware())
	rateLimitEnabled := os.Getenv("DISABLE_RATE_LIMIT") != "1"

	userCtl := handler.NewUserHandler(userSvc)
	projectCtl := handler.NewProjectHandler(projectSvc)
	taskCtl := handler.NewTaskHandler(taskSvc)
	activityCtl := handler.NewTaskActivityHandler(taskSvc)
	commentCtl := handler.NewTaskCommentHandler(taskCommentSvc)
	diaryCtl := handler.NewDiaryHandler(taskSvc)
	meetingCtl := handler.NewMeetingHandler(taskSvc)
	searchCtl := handler.NewSearchHandler(taskSvc)
	syncCtl := handler.NewSyncHandler(taskSvc)
	projectWSCtl := handler.NewProjectWSHandler(taskSvc, authSvc, app.ProjectHub)
	contentWSCtl := handler.NewContentWSHandler(taskSvc, authSvc, app.ContentHub)
	realtimeMetricsCtl := handler.NewRealtimeMetricsHandler(app.ProjectHub, app.ContentHub)
	documentImportCtl := handler.NewDocumentImportHandler(app.DocumentImportSvc)
	aiCtl := handler.NewAIHandler(app.AIService, taskSvc)

	public := r.Group("/api/v1")
	if rateLimitEnabled {
		public.Use(middlewares.RedisRateLimitMiddleware(app.Rdb, 400, 800))
	}
	{
		public.POST("/login", userCtl.Login)
		public.POST("/register", userCtl.Register)
		public.GET("/projects/:id/ws", projectWSCtl.ProjectEvents)
		public.GET("/tasks/:id/content/ws", contentWSCtl.TaskContent)
	}

	internal := r.Group("/api/internal")
	if rateLimitEnabled {
		internal.Use(middlewares.RedisRateLimitMiddleware(app.Rdb, 400, 800))
	}
	{
		internal.POST("/scheduler/task-due", taskCtl.DueCallback)
	}

	protected := r.Group("/api/v1")
	protected.Use(middlewares.AuthMiddleware(authSvc))
	if rateLimitEnabled {
		protected.Use(middlewares.RedisRateLimitMiddleware(app.Rdb, 400, 800))
	}
	{
		protected.GET("/users/me", userCtl.GetProfile)
		protected.PATCH("/users/me", userCtl.Update)
		protected.POST("/logout", userCtl.Logout)
		protected.GET("/projects/:id", projectCtl.GetProjectByID)
		protected.GET("/projects/:id/activities", activityCtl.ProjectActivities)
		protected.GET("/projects/:id/sync", syncCtl.ProjectEvents)
		protected.GET("/realtime/metrics", realtimeMetricsCtl.Snapshot)
		protected.GET("/search", searchCtl.Workspace)
		protected.GET("/projects", projectCtl.Search)
		protected.POST("/projects", projectCtl.Create)
		protected.PATCH("/projects/:id", projectCtl.Update)
		protected.DELETE("/projects/:id", projectCtl.Delete)

		protected.POST("/tasks", taskCtl.Create)
		protected.GET("/tasks/me", taskCtl.ListMyTasks)
		protected.GET("/trash/tasks", taskCtl.ListTrash)
		protected.POST("/trash/tasks/:id/restore", taskCtl.RestoreFromTrash)
		protected.DELETE("/trash/tasks/:id", taskCtl.DeleteFromTrash)
		protected.GET("/trash/spaces", projectCtl.ListTrash)
		protected.POST("/trash/spaces/:id/restore", projectCtl.RestoreFromTrash)
		protected.DELETE("/trash/spaces/:id", projectCtl.DeleteFromTrash)
		protected.PATCH("/projects/:id/tasks/:task_id", taskCtl.Update)
		protected.PATCH("/documents/:id/content", taskCtl.SaveDocumentContent)
		protected.GET("/documents/:id/comments", commentCtl.List)
		protected.POST("/documents/:id/comments", commentCtl.Create)
		protected.PATCH("/comments/:id", commentCtl.Update)
		protected.DELETE("/comments/:id", commentCtl.Delete)
		protected.DELETE("/tasks/:id", taskCtl.Delete)
		protected.GET("/tasks/:id", taskCtl.GetDetail)
		protected.GET("/tasks", taskCtl.List)
		protected.POST("/projects/:id/tasks/:task_id/members", taskCtl.AddMember)
		protected.DELETE("/projects/:id/tasks/:task_id/members", taskCtl.RemoveMember)
		protected.POST("/diary/today", diaryCtl.Today)
		protected.POST("/diary/:date", diaryCtl.OpenDate)
		protected.POST("/meetings", meetingCtl.Create)
		protected.POST("/meetings/:id/actions", meetingCtl.CreateActionTodo)
		protected.POST("/ai/draft/stream", aiCtl.DraftPreview)
		protected.POST("/ai/continue/stream", aiCtl.ContinuePreview)
		protected.POST("/ai/meetings/generate", aiCtl.MeetingPreview)
		protected.POST("/documents/imports", documentImportCtl.CreateSession)
		protected.PUT("/documents/imports/:upload_id/parts/:part_no", documentImportCtl.UploadPart)
		protected.POST("/documents/imports/:upload_id/assets", documentImportCtl.UploadAsset)
		protected.POST("/documents/imports/:upload_id/complete", documentImportCtl.Complete)
		protected.DELETE("/documents/imports/:upload_id", documentImportCtl.Abort)
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	return r
}
