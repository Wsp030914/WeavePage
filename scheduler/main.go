package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	schedulerJobsKey       = "scheduler:jobs"
	schedulerJobKeyPrefix  = "scheduler:job:"
	schedulerLockKeyPrefix = "scheduler:lock:"

	schedulerTokenHeader = "X-Scheduler-Token"

	maxPollBatchSize    = 10
	processingLockTTL   = 30 * time.Second
	defaultRetryDelay   = 10 * time.Second
	defaultListenAddr   = ":9090"
	defaultRedisAddr    = "127.0.0.1:6379"
	defaultAllowPrefix  = "http://127.0.0.1:8080/api/internal/"
	maxRequestBodyBytes = 1 << 20
)

type ScheduleTaskRequest struct {
	JobID    string           `json:"job_id"`
	RunAt    time.Time        `json:"run_at"`
	Callback ScheduleCallback `json:"callback"`
}

type ScheduleCallback struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body"`
}

type CancelTaskRequest struct {
	JobID string `json:"job_id"`
}

type Scheduler struct {
	client                  *redis.Client
	ctx                     context.Context
	callbackToken           string
	allowedCallbackPrefixes []string
	retryDelay              time.Duration
}

func NewScheduler(redisAddr, redisPassword, callbackToken string, allowedCallbackPrefixes []string) *Scheduler {
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
	})

	ctx := context.Background()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	return &Scheduler{
		client:                  rdb,
		ctx:                     ctx,
		callbackToken:           strings.TrimSpace(callbackToken),
		allowedCallbackPrefixes: normalizePrefixes(allowedCallbackPrefixes),
		retryDelay:              defaultRetryDelay,
	}
}

func (s *Scheduler) Schedule(req ScheduleTaskRequest) {
	data, err := json.Marshal(req)
	if err != nil {
		log.Printf("[Scheduler] Failed to marshal task request: %v", err)
		return
	}

	pipe := s.client.Pipeline()
	pipe.Set(s.ctx, s.jobKey(req.JobID), data, 0)
	pipe.ZAdd(s.ctx, schedulerJobsKey, redis.Z{
		Score:  float64(req.RunAt.Unix()),
		Member: req.JobID,
	})

	if _, err := pipe.Exec(s.ctx); err != nil {
		log.Printf("[Scheduler] Failed to schedule job %s: %v", req.JobID, err)
		return
	}

	log.Printf("[Scheduler] Scheduled job %s to run at %v", req.JobID, req.RunAt)
}

func (s *Scheduler) Cancel(jobID string) {
	pipe := s.client.Pipeline()
	pipe.ZRem(s.ctx, schedulerJobsKey, jobID)
	pipe.Del(s.ctx, s.jobKey(jobID))

	if _, err := pipe.Exec(s.ctx); err != nil {
		log.Printf("[Scheduler] Failed to cancel job %s: %v", jobID, err)
		return
	}
	log.Printf("[Scheduler] Canceled job %s", jobID)
}

func (s *Scheduler) Run() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	script := redis.NewScript(`
		local jobs = redis.call('ZRANGEBYSCORE', KEYS[1], 0, ARGV[1], 'LIMIT', 0, ARGV[2])
		return jobs
	`)

	for range ticker.C {
		now := time.Now().Unix()
		result, err := script.Run(s.ctx, s.client, []string{schedulerJobsKey}, now, maxPollBatchSize).Result()
		if err != nil {
			log.Printf("[Scheduler] Failed to poll jobs: %v", err)
			continue
		}

		jobIDs, ok := result.([]interface{})
		if !ok || len(jobIDs) == 0 {
			continue
		}

		var wg sync.WaitGroup
		for _, idValue := range jobIDs {
			jobID, ok := parseRedisString(idValue)
			if !ok || strings.TrimSpace(jobID) == "" {
				log.Printf("[Scheduler] Skip invalid job id payload: %#v", idValue)
				continue
			}

			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				s.processJob(id)
			}(jobID)
		}
		wg.Wait()
	}
}

func (s *Scheduler) processJob(jobID string) {
	lockKey := s.processingKey(jobID)
	acquired, err := s.client.SetNX(s.ctx, lockKey, "1", processingLockTTL).Result()
	if err != nil {
		log.Printf("[Scheduler] Failed to acquire lock for %s: %v", jobID, err)
		return
	}
	if !acquired {
		return
	}
	defer func() {
		if err := s.client.Del(s.ctx, lockKey).Err(); err != nil {
			log.Printf("[Scheduler] Failed to release lock for %s: %v", jobID, err)
		}
	}()

	key := s.jobKey(jobID)
	val, err := s.client.Get(s.ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			s.cleanupStaleJob(jobID)
			return
		}
		log.Printf("[Scheduler] Failed to get job details for %s: %v", jobID, err)
		return
	}

	var req ScheduleTaskRequest
	if err := json.Unmarshal(val, &req); err != nil {
		log.Printf("[Scheduler] Failed to unmarshal job %s: %v", jobID, err)
		s.reschedule(jobID)
		return
	}

	if err := s.executeCallback(req.Callback); err != nil {
		log.Printf("[Scheduler] Callback failed for %s: %v", jobID, err)
		s.reschedule(jobID)
		return
	}

	pipe := s.client.Pipeline()
	pipe.ZRem(s.ctx, schedulerJobsKey, jobID)
	pipe.Del(s.ctx, key)
	if _, err := pipe.Exec(s.ctx); err != nil {
		log.Printf("[Scheduler] Failed to finalize job %s: %v", jobID, err)
		return
	}

	log.Printf("[Scheduler] Job %s completed", jobID)
}

func (s *Scheduler) cleanupStaleJob(jobID string) {
	if err := s.client.ZRem(s.ctx, schedulerJobsKey, jobID).Err(); err != nil {
		log.Printf("[Scheduler] Failed to cleanup stale job %s: %v", jobID, err)
		return
	}
	log.Printf("[Scheduler] Removed stale job %s from schedule set", jobID)
}

func (s *Scheduler) reschedule(jobID string) {
	nextRunAt := time.Now().Add(s.retryDelay).Unix()
	if err := s.client.ZAdd(s.ctx, schedulerJobsKey, redis.Z{
		Score:  float64(nextRunAt),
		Member: jobID,
	}).Err(); err != nil {
		log.Printf("[Scheduler] Failed to reschedule job %s: %v", jobID, err)
		return
	}
	log.Printf("[Scheduler] Rescheduled job %s at %v", jobID, time.Unix(nextRunAt, 0))
}

func (s *Scheduler) jobKey(jobID string) string {
	return schedulerJobKeyPrefix + jobID
}

func (s *Scheduler) processingKey(jobID string) string {
	return schedulerLockKeyPrefix + jobID
}

func (s *Scheduler) executeCallback(cb ScheduleCallback) error {
	log.Printf("[Scheduler] Executing callback for %s", cb.URL)

	var bodyMap map[string]interface{}
	bodyBytes, err := json.Marshal(cb.Body)
	if err != nil {
		return fmt.Errorf("marshal callback body: %w", err)
	}
	if err := json.Unmarshal(bodyBytes, &bodyMap); err == nil {
		bodyMap["triggered_at"] = time.Now().UTC()
		modifiedBytes, marshalErr := json.Marshal(bodyMap)
		if marshalErr == nil {
			bodyBytes = modifiedBytes
		}
	}

	method := strings.ToUpper(strings.TrimSpace(cb.Method))
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequest(method, cb.URL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("build callback request: %w", err)
	}

	for k, v := range cb.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("execute callback request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("callback returned non-2xx status: %s", resp.Status)
	}

	log.Printf("[Scheduler] Callback executed, status: %s", resp.Status)
	return nil
}

func (s *Scheduler) isAuthorized(r *http.Request) bool {
	token := strings.TrimSpace(r.Header.Get(schedulerTokenHeader))
	if token == "" || s.callbackToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.callbackToken)) == 1
}

func (s *Scheduler) validateScheduleRequest(req ScheduleTaskRequest) error {
	if strings.TrimSpace(req.JobID) == "" {
		return fmt.Errorf("job_id is required")
	}
	if req.RunAt.IsZero() {
		return fmt.Errorf("run_at is required")
	}
	method := strings.ToUpper(strings.TrimSpace(req.Callback.Method))
	if method == "" {
		method = http.MethodPost
	}
	if method != http.MethodPost {
		return fmt.Errorf("callback method must be POST")
	}
	if !s.isCallbackAllowed(req.Callback.URL) {
		return fmt.Errorf("callback url is not allowed")
	}
	return nil
}

func (s *Scheduler) isCallbackAllowed(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" {
		return false
	}

	for _, prefix := range s.allowedCallbackPrefixes {
		if strings.HasPrefix(rawURL, prefix) {
			return true
		}
	}
	return false
}

func parseRedisString(v interface{}) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case []byte:
		return string(t), true
	default:
		return "", false
	}
}

func normalizePrefixes(prefixes []string) []string {
	out := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		trimmed := strings.TrimSpace(prefix)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func parseAllowedCallbackPrefixes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{defaultAllowPrefix}
	}
	parts := strings.Split(raw, ",")
	return normalizePrefixes(parts)
}

func main() {
	redisAddr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if redisAddr == "" {
		redisAddr = defaultRedisAddr
	}
	redisPassword := os.Getenv("REDIS_PASSWORD")
	callbackToken := strings.TrimSpace(os.Getenv("SCHEDULER_CALLBACK_TOKEN"))
	if callbackToken == "" {
		log.Fatal("SCHEDULER_CALLBACK_TOKEN is required")
	}

	allowedCallbackPrefixes := parseAllowedCallbackPrefixes(os.Getenv("SCHEDULER_ALLOWED_CALLBACK_PREFIXES"))
	scheduler := NewScheduler(redisAddr, redisPassword, callbackToken, allowedCallbackPrefixes)

	go scheduler.Run()

	http.HandleFunc("/api/v1/jobs/once", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !scheduler.isAuthorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		var req ScheduleTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := scheduler.validateScheduleRequest(req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		scheduler.Schedule(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, `{"status":"scheduled"}`); err != nil {
			log.Printf("[Scheduler] Write response failed: %v", err)
		}
	})

	http.HandleFunc("/api/v1/jobs/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !scheduler.isAuthorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		var req CancelTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.JobID) == "" {
			http.Error(w, "job_id is required", http.StatusBadRequest)
			return
		}

		scheduler.Cancel(req.JobID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, `{"status":"canceled"}`); err != nil {
			log.Printf("[Scheduler] Write response failed: %v", err)
		}
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, `{"status":"ok"}`); err != nil {
			log.Printf("[Scheduler] Write response failed: %v", err)
		}
	})

	log.Println("Scheduler service running on", defaultListenAddr)
	if err := http.ListenAndServe(defaultListenAddr, nil); err != nil {
		log.Fatal(err)
	}
}
