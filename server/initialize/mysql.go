package initialize

// 文件说明：这个文件负责 MySQL 连接初始化和关键表结构补齐。
// 实现方式：启动时建立 GORM 连接、设置连接池参数、做 ping 校验，并执行必要的增量 schema 检查。
// 这样做的好处是线上升级时可以用最小侵入方式补齐新字段和新表，而不必依赖单独迁移步骤才能启动服务。

import (
	"ToDoList/server/config"
	"ToDoList/server/models"
	"context"
	"fmt"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InitMySQL 初始化 MySQL 连接并补齐运行所需的关键 schema。
// 把 schema ensure 放在启动阶段，是为了让新版本服务在缺少新字段时尽早自愈或快速失败。
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
	if err := ensureTaskTrashColumns(db); err != nil {
		return nil, err
	}
	if err := ensureProjectTrashColumns(db); err != nil {
		return nil, err
	}
	if err := ensureTaskEventTable(db); err != nil {
		return nil, err
	}
	if err := ensureTaskContentUpdateTable(db); err != nil {
		return nil, err
	}
	if err := ensureTaskCommentTable(db); err != nil {
		return nil, err
	}

	return db, nil
}

// ensureTaskDocumentColumns 确保 tasks 表具备文档类型和协作模式字段。
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

// ensureTaskVersionColumn 确保 tasks 表具备版本号字段。
// 版本字段是 CAS 写路径的基础，因此启动时必须优先保证存在。
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

// ensureTaskTrashColumns 确保 tasks 表具备回收站相关字段。
func ensureTaskTrashColumns(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if !db.Migrator().HasColumn(&models.Task{}, "DeletedTitle") {
		if err := db.Migrator().AddColumn(&models.Task{}, "DeletedTitle"); err != nil {
			return fmt.Errorf("add tasks.deleted_title column failed: %w", err)
		}
	}
	if !db.Migrator().HasColumn(&models.Task{}, "DeletedBy") {
		if err := db.Migrator().AddColumn(&models.Task{}, "DeletedBy"); err != nil {
			return fmt.Errorf("add tasks.deleted_by column failed: %w", err)
		}
	}
	if !db.Migrator().HasColumn(&models.Task{}, "DeletedAt") {
		if err := db.Migrator().AddColumn(&models.Task{}, "DeletedAt"); err != nil {
			return fmt.Errorf("add tasks.deleted_at column failed: %w", err)
		}
	}
	return nil
}

func ensureProjectTrashColumns(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if !db.Migrator().HasColumn(&models.Project{}, "DeletedName") {
		if err := db.Migrator().AddColumn(&models.Project{}, "DeletedName"); err != nil {
			return fmt.Errorf("add projects.deleted_name column failed: %w", err)
		}
	}
	if !db.Migrator().HasColumn(&models.Project{}, "DeletedBy") {
		if err := db.Migrator().AddColumn(&models.Project{}, "DeletedBy"); err != nil {
			return fmt.Errorf("add projects.deleted_by column failed: %w", err)
		}
	}
	if !db.Migrator().HasColumn(&models.Project{}, "DeletedAt") {
		if err := db.Migrator().AddColumn(&models.Project{}, "DeletedAt"); err != nil {
			return fmt.Errorf("add projects.deleted_at column failed: %w", err)
		}
	}
	return nil
}

// ensureTaskEventTable 确保项目事件表存在。
func ensureTaskEventTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if err := db.AutoMigrate(&models.TaskEvent{}); err != nil {
		return fmt.Errorf("migrate task_events table failed: %w", err)
	}

	return nil
}

// ensureTaskContentUpdateTable 确保正文增量更新表存在。
func ensureTaskContentUpdateTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if err := db.AutoMigrate(&models.TaskContentUpdate{}); err != nil {
		return fmt.Errorf("migrate task_content_updates table failed: %w", err)
	}

	return nil
}

// ensureTaskCommentTable 确保评论表存在。
func ensureTaskCommentTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("mysql schema check failed: db is nil")
	}

	if err := db.AutoMigrate(&models.TaskComment{}); err != nil {
		return fmt.Errorf("migrate task_comments table failed: %w", err)
	}

	return nil
}
