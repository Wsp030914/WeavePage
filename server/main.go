package main

// 文件说明：这个文件是后端服务入口，负责组装配置、数据库、缓存、异步总线、实时协同与 HTTP 服务。
// 实现方式：在启动阶段一次性完成依赖初始化与服务装配，再把运行时职责交给各层组件协作。
// 这样做的好处是启动路径集中、依赖关系清晰，便于排查环境问题和扩展新组件。

import (
	"ToDoList/server/async"
	"ToDoList/server/cache"
	"ToDoList/server/config"
	"ToDoList/server/initialize"
	"ToDoList/server/realtime"
	"ToDoList/server/repo"
	"ToDoList/server/service"
	"ToDoList/server/utils"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	_ "ToDoList/docs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	config.InitJWT(&cfg.JWT)
	if err := utils.InitCos(&cfg.COS); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := initialize.InitMySQL(cfg)
	if err != nil {
		panic(err)
	}

	//  if err := db.AutoMigrate(&models.User{}, &models.Task{}, &models.Project{}, &models.TaskMember{}); err != nil {
	//  	panic(err)
	// }

	rdb, err := initialize.InitRedis(cfg)
	if err != nil {
		panic(err)
	}

	userRepo := repo.NewUserRepository(db)
	projectRepo := repo.NewProjectRepository(db)
	taskRepo := repo.NewTaskRepository(db)
	taskEventRepo := repo.NewTaskEventRepository(db)
	taskContentRepo := repo.NewTaskContentRepository(db)
	taskCommentRepo := repo.NewTaskCommentRepository(db)
	taskMemberRepo := repo.NewTaskMemberRepository(db)

	redisCache := cache.NewRedisCache(rdb)
	userCache := cache.NewUserCache(redisCache)
	projectCache := cache.NewProjectCache(redisCache)
	taskCache := cache.NewTaskCache(redisCache)

	// ── EventBus hot-switch ──────────────────────────────────────────────────
	// Kafka is the only async event bus backend.
	if !cfg.Kafka.Enable {
		log.Fatalf("config error: kafka.enable must be true")
	}

	var bus async.IEventBus
	kafkaProducer := async.NewKafkaProducer(cfg.Kafka.Brokers, cfg.Kafka.Topic)
	defer kafkaProducer.Close()

	kafkaConsumer := async.NewKafkaConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.Topic,
		cfg.Kafka.GroupID,
		async.WithWorkerCount(cfg.Kafka.Workers),
		async.WithBackoff(async.ConsumerBaseBackoff, async.ConsumerMaxBackoff),
		async.WithDeadLetterQueue(kafkaProducer, cfg.Kafka.DLQTopic),
	)
	initialize.InitAsyncHandlers(kafkaConsumer, initialize.AsyncHandlerDeps{
		Cache:        redisCache,
		UserCache:    userCache,
		ProjectCache: projectCache,
	})
	kafkaConsumer.Start()
	defer kafkaConsumer.Stop()

	bus = async.NewEventBus(kafkaProducer)

	dueScheduler := service.NewHTTPDueScheduler(service.HTTPDueSchedulerConfig{
		ScheduleURL:    cfg.DueScheduler.ScheduleURL,
		CancelURL:      cfg.DueScheduler.CancelURL,
		CallbackURL:    cfg.DueScheduler.CallbackURL,
		CallbackToken:  cfg.DueScheduler.CallbackToken,
		RequestTimeout: cfg.DueScheduler.RequestTimeout,
		PingURL:        cfg.DueScheduler.PingURL,
	})

	userSvc := service.NewUserService(service.UserServiceDeps{
		Repo:         userRepo,
		UserCache:    userCache,
		CacheClient:  redisCache,
		Bus:          bus,
		HashPassword: utils.HashPassword,
		PutAvatar:    utils.PutObj,
	})

	projectSvc := service.NewProjectService(service.ProjectServiceDeps{
		Repo:         projectRepo,
		ProjectCache: projectCache,
		UserRepo:     userRepo,
		CacheClient:  redisCache,
		Bus:          bus,
	})

	taskSvc := service.NewTaskService(service.TaskServiceDeps{
		Repo:                   taskRepo,
		EventRepo:              taskEventRepo,
		ContentRepo:            taskContentRepo,
		TaskCache:              taskCache,
		ProjectRepo:            projectRepo,
		ProjectCache:           projectCache,
		TaskMemberRepo:         taskMemberRepo,
		UserRepo:               userRepo,
		DB:                     db,
		DueScheduler:           dueScheduler,
		LocalDuePollingEnabled: cfg.DueScheduler.LocalPollingEnabled,
		CacheClient:            redisCache,
		Bus:                    bus,
	})
	taskCommentSvc := service.NewTaskCommentService(service.TaskCommentServiceDeps{
		Repo:        taskCommentRepo,
		TaskSession: taskSvc,
	})

	authSvc := service.NewAuthService(service.AuthServiceDeps{
		Repo:      userRepo,
		UserCache: userCache,
		Bus:       bus,
	})

	nodeID := uuid.NewString()
	projectHub := realtime.NewProjectHub(taskSvc, rdb, nodeID, redisCache)
	contentHub := realtime.NewContentHub(taskSvc, rdb, nodeID)
	taskSvc.SetTaskEventBroadcaster(projectHub)
	documentImportSvc := service.NewDocumentImportService(service.DocumentImportServiceDeps{
		TaskService: taskSvc,
		Cache:       redisCache,
		Store:       utils.NewCOSObjectStore(),
	})
	aiSvc, err := service.NewAIService(ctx, cfg.AI)
	if err != nil {
		log.Fatalf("Failed to initialize AI service: %v", err)
	}

	app := &App{
		Bus:               bus,
		Rdb:               rdb,
		Db:                db,
		ProjectHub:        projectHub,
		ContentHub:        contentHub,
		DocumentImportSvc: documentImportSvc,
		AIService:         aiSvc,
	}

	logger := config.InitZap(&cfg.Zap)
	zap.ReplaceGlobals(logger)

	taskSvc.StartDueWatcher(ctx, logger)
	projectHub.Start(ctx, logger)
	contentHub.Start(ctx, logger)

	r := NewRouter(ctx, app, userSvc, projectSvc, taskSvc, taskCommentSvc, authSvc)
	defer func() {
		_ = zap.L().Sync()
	}()

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn(err.Error())
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v, force close", err)
		_ = srv.Close()
	}
}
