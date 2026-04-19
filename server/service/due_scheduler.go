package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const SchedulerTokenHeader = "X-Scheduler-Token"

type DueScheduler interface {
	ScheduleTaskOnce(ctx context.Context, taskID int, dueAt time.Time) error
	CancelTask(ctx context.Context, taskID int) error
	Ping(ctx context.Context) error
}

type noopDueScheduler struct{}

func (s noopDueScheduler) ScheduleTaskOnce(ctx context.Context, taskID int, dueAt time.Time) error {
	return nil
}
func (s noopDueScheduler) CancelTask(ctx context.Context, taskID int) error {
	return nil
}
func (s noopDueScheduler) Ping(ctx context.Context) error {
	return nil
}

type HTTPDueSchedulerConfig struct {
	ScheduleURL    string
	CancelURL      string
	CallbackURL    string
	CallbackToken  string
	RequestTimeout time.Duration
	PingURL        string
}

type httpDueScheduler struct {
	scheduleURL    string
	cancelURL      string
	callbackURL    string
	callbackToken  string
	requestTimeout time.Duration
	pingURL        string
	client         *http.Client
}

type scheduleTaskRequest struct {
	JobID    string           `json:"job_id"`
	RunAt    time.Time        `json:"run_at"`
	Callback scheduleCallback `json:"callback"`
}

type scheduleCallback struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    callbackBody      `json:"body"`
}

type callbackBody struct {
	TaskID int `json:"task_id"`
}

type cancelTaskRequest struct {
	JobID string `json:"job_id"`
}

func NewHTTPDueScheduler(cfg HTTPDueSchedulerConfig) DueScheduler {
	return &httpDueScheduler{
		scheduleURL:    cfg.ScheduleURL,
		cancelURL:      cfg.CancelURL,
		callbackURL:    cfg.CallbackURL,
		callbackToken:  strings.TrimSpace(cfg.CallbackToken),
		requestTimeout: cfg.RequestTimeout,
		pingURL:        cfg.PingURL,
		client: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

func (s *httpDueScheduler) Ping(ctx context.Context) error {
	if s.pingURL == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.pingURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scheduler health check failed: status %d", resp.StatusCode)
	}
	return nil
}

func (s *httpDueScheduler) ScheduleTaskOnce(ctx context.Context, taskID int, dueAt time.Time) error {
	if taskID <= 0 {
		return errors.New("invalid task id")
	}

	reqBody := scheduleTaskRequest{
		JobID: dueTaskJobID(taskID),
		RunAt: dueAt.In(time.Local),
		Callback: scheduleCallback{
			Method: http.MethodPost,
			URL:    s.callbackURL,
			Headers: map[string]string{
				SchedulerTokenHeader: s.callbackToken,
			},
			Body: callbackBody{TaskID: taskID},
		},
	}
	return s.postJSON(ctx, s.scheduleURL, reqBody)
}

func (s *httpDueScheduler) CancelTask(ctx context.Context, taskID int) error {
	if taskID <= 0 {
		return errors.New("invalid task id")
	}

	reqBody := cancelTaskRequest{
		JobID: dueTaskJobID(taskID),
	}
	return s.postJSON(ctx, s.cancelURL, reqBody)
}

func (s *httpDueScheduler) postJSON(ctx context.Context, endpoint string, body any) error {
	if strings.TrimSpace(endpoint) == "" {
		return errors.New("scheduler endpoint is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, s.requestTimeout)
	defer cancel()

	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("build scheduler request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.callbackToken != "" {
		req.Header.Set(SchedulerTokenHeader, s.callbackToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send scheduler request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("scheduler request failed: status=%d", resp.StatusCode)
	}
	return nil
}

func dueTaskJobID(taskID int) string {
	return fmt.Sprintf("todo-task-due-%d", taskID)
}
