package initialize

import (
	"ToDoList/server/config"
	"ToDoList/server/models"
	"context"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitMySQL(cfg *config.Config) (*gorm.DB, error) {
	mysqlCfg := &cfg.Database
	db, err := gorm.Open(mysql.Open(mysqlCfg.DSN()), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("mysql connect error: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	if mysqlCfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(mysqlCfg.MaxOpenConns)
	}
	if mysqlCfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(mysqlCfg.MaxIdleConns)
	}
	if mysqlCfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(mysqlCfg.ConnMaxLifetime)
	}
	if mysqlCfg.ConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(mysqlCfg.ConnMaxIdleTime)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mysql ping failed: %w", err)
	}

	if err := ensureTaskVersionColumn(db); err != nil {
		return nil, err
	}
	if err := ensureTaskDocumentColumns(db); err != nil {
		return nil, err
	}
	if err := ensureTaskEventTable(db); err != nil {
		return nil, err
	}
	if err := ensureTaskContentUpdateTable(db); err != nil {
		return nil, err
	}

	return db, nil
}

func ensureTaskDocumentColumns(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if !db.Migrator().HasColumn(&models.Task{}, "DocType") {
		if err := db.Migrator().AddColumn(&models.Task{}, "DocType"); err != nil {
			return fmt.Errorf("add tasks.doc_type column failed: %w", err)
		}
	}
	if !db.Migrator().HasColumn(&models.Task{}, "CollaborationMode") {
		if err := db.Migrator().AddColumn(&models.Task{}, "CollaborationMode"); err != nil {
			return fmt.Errorf("add tasks.collaboration_mode column failed: %w", err)
		}
	}
	return nil
}

func ensureTaskVersionColumn(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if db.Migrator().HasColumn(&models.Task{}, "Version") {
		return nil
	}

	if err := db.Migrator().AddColumn(&models.Task{}, "Version"); err != nil {
		return fmt.Errorf("add tasks.version column failed: %w", err)
	}

	return nil
}

func ensureTaskEventTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if err := db.AutoMigrate(&models.TaskEvent{}); err != nil {
		return fmt.Errorf("migrate task_events table failed: %w", err)
	}

	return nil
}

func ensureTaskContentUpdateTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if err := db.AutoMigrate(&models.TaskContentUpdate{}); err != nil {
		return fmt.Errorf("migrate task_content_updates table failed: %w", err)
	}

	return nil
}
