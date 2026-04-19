package main

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
		Bus:          bus,
		HashPassword: utils.HashPassword,
		PutAvatar:    utils.PutObj,
	})

	projectSvc := service.NewProjectService(service.ProjectServiceDeps{
		Repo:         projectRepo,
		ProjectCache: projectCache,
		UserRepo:     userRepo,
		CacheClient:  redisCache,
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

	authSvc := service.NewAuthService(service.AuthServiceDeps{
		Repo:      userRepo,
		UserCache: userCache,
		Bus:       bus,
	})

	nodeID := uuid.NewString()
	projectHub := realtime.NewProjectHub(taskSvc, rdb, nodeID, redisCache)
	contentHub := realtime.NewContentHub(taskSvc, rdb, nodeID)
	taskSvc.SetTaskEventBroadcaster(projectHub)

	app := &App{
		Bus:        bus,
		Rdb:        rdb,
		Db:         db,
		ProjectHub: projectHub,
		ContentHub: contentHub,
	}

	logger := config.InitZap(&cfg.Zap)
	zap.ReplaceGlobals(logger)

	taskSvc.StartDueWatcher(ctx, logger)
	projectHub.Start(ctx, logger)
	contentHub.Start(ctx, logger)

	r := NewRouter(ctx, app, userSvc, projectSvc, taskSvc, authSvc)
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
