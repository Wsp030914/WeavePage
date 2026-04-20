package service

// 文件说明：这个文件封装任务到期调度器的客户端调用。
// 实现方式：通过一个抽象接口屏蔽 noop 和 HTTP 两种实现，再把调度、取消、健康检查统一成标准方法。
// 这样做的好处是任务服务只依赖调度能力本身，不需要感知调度器部署方式或具体协议细节。

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

// NewHTTPDueScheduler 创建基于 HTTP 回调的到期调度器客户端。
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

// Ping 检查调度器服务是否可用。
// 把健康检查单独抽出来，是为了让主服务启动时能尽早发现调度器不可达，而不是等第一次写入到期任务时才暴露。
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

// ScheduleTaskOnce 为任务注册一次性到期回调。
// 这里把 callback token 一起塞进调度负载和请求头，是为了同时兼容调度器转发链路和直连链路的鉴权。
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

// CancelTask 取消一个任务的到期调度。
func (s *httpDueScheduler) CancelTask(ctx context.Context, taskID int) error {
	if taskID <= 0 {
		return errors.New("invalid task id")
	}

	reqBody := cancelTaskRequest{
		JobID: dueTaskJobID(taskID),
	}
	return s.postJSON(ctx, s.cancelURL, reqBody)
}

// postJSON 发送调度器请求。
// 统一封装超时、JSON 编码和错误状态码判断，是为了让调度相关接口保持同一套失败语义。
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

// dueTaskJobID 生成任务到期作业 ID。
// 作业 ID 稳定绑定 taskID，能让重复调度直接覆盖同一作业，避免积累多条过期任务。
func dueTaskJobID(taskID int) string {
	return fmt.Sprintf("todo-task-due-%d", taskID)
}
