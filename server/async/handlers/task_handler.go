package handlers

import (
	"ToDoList/server/async"
	"ToDoList/server/config"
	"ToDoList/server/utils"
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

type TaskDuePayload struct {
	TaskID int    `json:"task_id"`
	UserID int    `json:"user_id"`
	Title  string `json:"title"`
	Email  string `json:"email"`
}

func SendTaskDueNotification(ctx context.Context, job async.KafkaJob, lg *zap.Logger) error {
	var p TaskDuePayload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		lg.Error(job.Type+" payload unmarshal error", zap.String("trace_id", job.TraceID), zap.Error(err))
		return nil
	}

	if p.TaskID <= 0 || p.UserID <= 0 {
		lg.Warn(job.Type+" skipped: invalid task/user id", zap.Int("task_id", p.TaskID), zap.Int("user_id", p.UserID))
		return nil
	}
	if p.Email == "" {
		lg.Warn(job.Type+" skipped: email is empty", zap.Int("task_id", p.TaskID))
		return nil
	}

	title := p.Title
	if title == "" {
		title = fmt.Sprintf("task-%d", p.TaskID)
	}
	subject := fmt.Sprintf("Task due reminder: %s", title)
	body := fmt.Sprintf("Your task '%s' is due. Please check and process it in time.", title)

	if err := utils.SendEmail(config.GlobalConfig.Email, p.Email, subject, body); err != nil {
		lg.Error(job.Type+" send email failed",
			zap.String("email", p.Email),
			zap.Int("task_id", p.TaskID),
			zap.Error(err))
		return err
	}

	lg.Info(job.Type+" email sent success", zap.String("email", p.Email), zap.Int("task_id", p.TaskID))
	return nil
}
