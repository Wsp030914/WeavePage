package service

import (
	"ToDoList/server/cache"
	"ToDoList/server/models"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	apperrors "ToDoList/server/errors"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	// DocumentImportChunkSize is the default frontend/server chunk size for Markdown imports.
	DocumentImportChunkSize int64 = 1024 * 1024
	// DocumentImportMaxPartBytes caps a single uploaded Markdown chunk.
	DocumentImportMaxPartBytes int64 = 2 * 1024 * 1024
	// DocumentImportMaxMarkdownSize caps assembled Markdown stored in content_md.
	DocumentImportMaxMarkdownSize int64 = 16 * 1024 * 1024
	// DocumentImportMaxAssetBytes caps one referenced image asset.
	DocumentImportMaxAssetBytes int64 = 10 * 1024 * 1024

	documentImportSessionTTL = 24 * time.Hour
	maxDocumentImportParts   = 512
)

var (
	markdownImageRegexp = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)([^)]*)\)`)
	htmlImageSrcRegexp  = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc=["'])([^"']+)(["'])`)
)

// DocumentImportObjectStore is the object-storage contract used by Markdown import sessions.
type DocumentImportObjectStore interface {
	PutObject(ctx context.Context, key string, reader io.Reader, contentType string, contentLength int64) (string, error)
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, key string) error
}

// DocumentImportService coordinates resumable Markdown imports and final document creation.
type DocumentImportService struct {
	taskSvc *TaskService
	cache   cache.Cache
	store   DocumentImportObjectStore
}

// DocumentImportServiceDeps contains dependencies for DocumentImportService.
type DocumentImportServiceDeps struct {
	TaskService *TaskService
	Cache       cache.Cache
	Store       DocumentImportObjectStore
}

// CreateDocumentImportInput describes a new resumable Markdown import session.
type CreateDocumentImportInput struct {
	ProjectID         int
	FileName          string
	Title             string
	TotalSize         int64
	TotalParts        int
	ChunkSize         int64
	SHA256            string
	CollaborationMode string
}

// DocumentImportSessionResult is returned when an import session is created.
type DocumentImportSessionResult struct {
	UploadID          string    `json:"upload_id"`
	ProjectID         int       `json:"project_id"`
	FileName          string    `json:"file_name"`
	Title             string    `json:"title"`
	TotalSize         int64     `json:"total_size"`
	TotalParts        int       `json:"total_parts"`
	ChunkSize         int64     `json:"chunk_size"`
	CollaborationMode string    `json:"collaboration_mode"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// DocumentImportPartResult describes one uploaded Markdown chunk.
type DocumentImportPartResult struct {
	UploadID string    `json:"upload_id"`
	PartNo   int       `json:"part_no"`
	Size     int64     `json:"size"`
	SHA256   string    `json:"sha256"`
	Uploaded time.Time `json:"uploaded_at"`
	Received int       `json:"received_parts"`
	Total    int       `json:"total_parts"`
}

// DocumentImportAssetResult describes one uploaded image reference.
type DocumentImportAssetResult struct {
	UploadID     string `json:"upload_id"`
	OriginalPath string `json:"original_path"`
	URL          string `json:"url"`
	Markdown     string `json:"markdown"`
}

// CompleteDocumentImportInput allows overriding final document metadata.
type CompleteDocumentImportInput struct {
	Title string
}

// DocumentImportCompleteResult is returned after a Markdown import creates a document.
type DocumentImportCompleteResult struct {
	Task   *models.Task                  `json:"task"`
	Assets []DocumentImportAssetResult   `json:"assets"`
	Stats  DocumentImportCompletionStats `json:"stats"`
}

// DocumentImportCompletionStats summarizes the import transformation.
type DocumentImportCompletionStats struct {
	SizeBytes      int64 `json:"size_bytes"`
	RewrittenLinks int   `json:"rewritten_links"`
}

type documentImportSession struct {
	UploadID          string                         `json:"upload_id"`
	UserID            int                            `json:"user_id"`
	ProjectID         int                            `json:"project_id"`
	FileName          string                         `json:"file_name"`
	Title             string                         `json:"title"`
	TotalSize         int64                          `json:"total_size"`
	TotalParts        int                            `json:"total_parts"`
	ChunkSize         int64                          `json:"chunk_size"`
	SHA256            string                         `json:"sha256,omitempty"`
	CollaborationMode string                         `json:"collaboration_mode"`
	Parts             map[int]documentImportPart     `json:"parts"`
	Assets            map[string]documentImportAsset `json:"assets"`
	CreatedAt         time.Time                      `json:"created_at"`
	ExpiresAt         time.Time                      `json:"expires_at"`
}

type documentImportPart struct {
	PartNo     int       `json:"part_no"`
	Key        string    `json:"key"`
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type documentImportAsset struct {
	OriginalPath string    `json:"original_path"`
	Key          string    `json:"key"`
	URL          string    `json:"url"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// NewDocumentImportService creates a Markdown import coordinator.
func NewDocumentImportService(deps DocumentImportServiceDeps) *DocumentImportService {
	return &DocumentImportService{
		taskSvc: deps.TaskService,
		cache:   deps.Cache,
		store:   deps.Store,
	}
}

// CreateSession validates metadata and creates a Redis-backed import session.
func (s *DocumentImportService) CreateSession(ctx context.Context, lg *zap.Logger, uid int, in CreateDocumentImportInput) (*DocumentImportSessionResult, error) {
	if s == nil || s.cache == nil || s.store == nil || s.taskSvc == nil {
		return nil, apperrors.NewInternalError("document import is not configured")
	}
	if in.ProjectID <= 0 {
		return nil, apperrors.NewParamError("project_id is required")
	}
	fileName := sanitizeImportFileName(in.FileName)
	if !isMarkdownFile(fileName) {
		return nil, apperrors.NewParamError("only .md or .markdown files can be imported")
	}
	if in.TotalSize <= 0 {
		return nil, apperrors.NewParamError("total_size is required")
	}
	if in.TotalSize > DocumentImportMaxMarkdownSize {
		return nil, apperrors.NewParamError(fmt.Sprintf("markdown file must be <= %d bytes", DocumentImportMaxMarkdownSize))
	}
	chunkSize := in.ChunkSize
	if chunkSize <= 0 {
		chunkSize = DocumentImportChunkSize
	}
	if chunkSize > DocumentImportMaxPartBytes {
		chunkSize = DocumentImportMaxPartBytes
	}
	totalParts := in.TotalParts
	if totalParts <= 0 {
		totalParts = int((in.TotalSize + chunkSize - 1) / chunkSize)
	}
	if totalParts <= 0 || totalParts > maxDocumentImportParts {
		return nil, apperrors.NewParamError("invalid total_parts")
	}
	expectedParts := int((in.TotalSize + chunkSize - 1) / chunkSize)
	if totalParts != expectedParts {
		return nil, apperrors.NewParamError("total_parts does not match total_size and chunk_size")
	}
	expectedSHA := strings.ToLower(strings.TrimSpace(in.SHA256))
	if expectedSHA != "" && !isHexSHA256(expectedSHA) {
		return nil, apperrors.NewParamError("sha256 must be a lowercase hex digest")
	}
	mode := normalizeCollaborationMode(in.CollaborationMode)
	if mode == "" {
		return nil, apperrors.NewParamError("invalid collaboration_mode")
	}

	project, err := s.taskSvc.projectRepo.GetByIDAndUserID(ctx, in.ProjectID, uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.NewNotFoundError("project not found")
		}
		lg.Error("document_import.create_session.get_project_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("failed to query project")
	}
	if project.UserID != uid {
		return nil, apperrors.NewForbiddenError("only project owner can import documents")
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = titleFromMarkdownFile(fileName)
	}
	if title == "" || len([]rune(title)) > 200 {
		return nil, apperrors.NewParamError("title must be 1-200 characters")
	}

	now := time.Now()
	session := &documentImportSession{
		UploadID:          uuid.NewString(),
		UserID:            uid,
		ProjectID:         in.ProjectID,
		FileName:          fileName,
		Title:             title,
		TotalSize:         in.TotalSize,
		TotalParts:        totalParts,
		ChunkSize:         chunkSize,
		SHA256:            expectedSHA,
		CollaborationMode: mode,
		Parts:             map[int]documentImportPart{},
		Assets:            map[string]documentImportAsset{},
		CreatedAt:         now,
		ExpiresAt:         now.Add(documentImportSessionTTL),
	}
	if err := s.saveSession(ctx, session); err != nil {
		lg.Error("document_import.create_session.save_failed", zap.Error(err))
		return nil, apperrors.NewInternalError("failed to create import session")
	}
	return session.result(), nil
}

// UploadPart stores one Markdown chunk for an import session.
func (s *DocumentImportService) UploadPart(ctx context.Context, lg *zap.Logger, uid int, uploadID string, partNo int, reader io.Reader, size int64) (*DocumentImportPartResult, error) {
	if size <= 0 {
		return nil, apperrors.NewParamError("empty part")
	}
	if size > DocumentImportMaxPartBytes {
		return nil, apperrors.NewParamError(fmt.Sprintf("part must be <= %d bytes", DocumentImportMaxPartBytes))
	}

	var result *DocumentImportPartResult
	err := s.withSessionLock(ctx, uploadID, func() error {
		session, err := s.loadSession(ctx, uid, uploadID)
		if err != nil {
			return err
		}
		if partNo <= 0 || partNo > session.TotalParts {
			return apperrors.NewParamError("invalid part_no")
		}
		if partNo < session.TotalParts && size != session.ChunkSize {
			return apperrors.NewParamError("non-final parts must match chunk_size")
		}
		if partNo == session.TotalParts {
			expectedLastSize := session.TotalSize - int64(session.TotalParts-1)*session.ChunkSize
			if expectedLastSize > 0 && size != expectedLastSize {
				return apperrors.NewParamError("final part size does not match total_size")
			}
		}

		hash := sha256.New()
		limited := io.LimitReader(reader, size)
		key := documentImportPartKey(uid, uploadID, partNo)
		if _, err := s.store.PutObject(ctx, key, io.TeeReader(limited, hash), "application/octet-stream", size); err != nil {
			lg.Error("document_import.upload_part.put_failed", zap.Int("part_no", partNo), zap.Error(err))
			return apperrors.NewInternalError("failed to store upload part")
		}

		part := documentImportPart{
			PartNo:     partNo,
			Key:        key,
			Size:       size,
			SHA256:     hex.EncodeToString(hash.Sum(nil)),
			UploadedAt: time.Now(),
		}
		session.Parts[partNo] = part
		if err := s.saveSession(ctx, session); err != nil {
			return err
		}
		result = &DocumentImportPartResult{
			UploadID: uploadID,
			PartNo:   partNo,
			Size:     size,
			SHA256:   part.SHA256,
			Uploaded: part.UploadedAt,
			Received: len(session.Parts),
			Total:    session.TotalParts,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// UploadAsset stores an image asset and records its original Markdown path.
func (s *DocumentImportService) UploadAsset(ctx context.Context, lg *zap.Logger, uid int, uploadID string, originalPath string, fh *multipart.FileHeader) (*DocumentImportAssetResult, error) {
	if fh == nil {
		return nil, apperrors.NewParamError("file is required")
	}
	if fh.Size <= 0 {
		return nil, apperrors.NewParamError("empty asset")
	}
	if fh.Size > DocumentImportMaxAssetBytes {
		return nil, apperrors.NewParamError(fmt.Sprintf("asset must be <= %d bytes", DocumentImportMaxAssetBytes))
	}
	fileName := sanitizeImportFileName(fh.Filename)
	if !isSupportedMarkdownAsset(fileName, fh.Header.Get("Content-Type")) {
		return nil, apperrors.NewParamError("unsupported image type")
	}
	normalizedPath := normalizeAssetPath(originalPath)
	if normalizedPath == "" {
		normalizedPath = fileName
	}

	var result *DocumentImportAssetResult
	err := s.withSessionLock(ctx, uploadID, func() error {
		session, err := s.loadSession(ctx, uid, uploadID)
		if err != nil {
			return err
		}

		file, err := fh.Open()
		if err != nil {
			return apperrors.NewParamError("failed to open asset")
		}
		defer file.Close()

		key := documentImportAssetKey(uid, uploadID, fileName)
		contentType := fh.Header.Get("Content-Type")
		if contentType == "" {
			contentType = mime.TypeByExtension(strings.ToLower(path.Ext(fileName)))
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		url, err := s.store.PutObject(ctx, key, file, contentType, fh.Size)
		if err != nil {
			lg.Error("document_import.upload_asset.put_failed", zap.String("path", normalizedPath), zap.Error(err))
			return apperrors.NewInternalError("failed to store asset")
		}

		asset := documentImportAsset{
			OriginalPath: normalizedPath,
			Key:          key,
			URL:          url,
			UploadedAt:   time.Now(),
		}
		session.Assets[normalizedPath] = asset
		if err := s.saveSession(ctx, session); err != nil {
			return err
		}

		result = &DocumentImportAssetResult{
			UploadID:     uploadID,
			OriginalPath: normalizedPath,
			URL:          url,
			Markdown:     fmt.Sprintf("![%s](%s)", strings.TrimSuffix(path.Base(normalizedPath), path.Ext(normalizedPath)), url),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Complete assembles uploaded chunks, rewrites image references, and creates the document.
func (s *DocumentImportService) Complete(ctx context.Context, lg *zap.Logger, uid int, uploadID string, in CompleteDocumentImportInput) (*DocumentImportCompleteResult, error) {
	var result *DocumentImportCompleteResult
	err := s.withSessionLock(ctx, uploadID, func() error {
		session, err := s.loadSession(ctx, uid, uploadID)
		if err != nil {
			return err
		}
		if len(session.Parts) != session.TotalParts {
			return apperrors.NewConflictError("not all parts have been uploaded")
		}

		content, digest, err := s.assembleMarkdown(ctx, session)
		if err != nil {
			lg.Error("document_import.complete.assemble_failed", zap.String("upload_id", uploadID), zap.Error(err))
			return err
		}
		if session.SHA256 != "" && session.SHA256 != digest {
			return apperrors.NewConflictError("sha256 mismatch")
		}
		rewritten, rewrittenCount := rewriteMarkdownImageRefs(content, session.Assets)

		title := strings.TrimSpace(in.Title)
		if title == "" {
			title = session.Title
		}
		task, err := s.taskSvc.Create(ctx, lg, uid, CreateTaskInput{
			Title:             title,
			ProjectID:         session.ProjectID,
			ContentMD:         &rewritten,
			DocType:           models.DocTypeDocument,
			CollaborationMode: session.CollaborationMode,
			Status:            stringPtr(models.TaskTodo),
		})
		if err != nil {
			return err
		}

		result = &DocumentImportCompleteResult{
			Task:   task,
			Assets: session.assetResults(uploadID),
			Stats: DocumentImportCompletionStats{
				SizeBytes:      int64(len(rewritten)),
				RewrittenLinks: rewrittenCount,
			},
		}

		s.cleanupSessionParts(ctx, lg, session)
		if err := s.cache.Del(ctx, documentImportSessionKey(uploadID)); err != nil {
			lg.Warn("document_import.complete.del_session_failed", zap.String("upload_id", uploadID), zap.Error(err))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// Abort removes temporary objects and deletes the import session.
func (s *DocumentImportService) Abort(ctx context.Context, lg *zap.Logger, uid int, uploadID string) error {
	return s.withSessionLock(ctx, uploadID, func() error {
		session, err := s.loadSession(ctx, uid, uploadID)
		if err != nil {
			return err
		}
		s.cleanupSessionParts(ctx, lg, session)
		s.cleanupSessionAssets(ctx, lg, session)
		if err := s.cache.Del(ctx, documentImportSessionKey(uploadID)); err != nil {
			return apperrors.NewInternalError("failed to abort import session")
		}
		return nil
	})
}

func (s *DocumentImportService) assembleMarkdown(ctx context.Context, session *documentImportSession) (string, string, error) {
	partNos := make([]int, 0, len(session.Parts))
	for partNo := range session.Parts {
		partNos = append(partNos, partNo)
	}
	sort.Ints(partNos)

	var buf bytes.Buffer
	hash := sha256.New()
	for _, partNo := range partNos {
		part := session.Parts[partNo]
		reader, err := s.store.GetObject(ctx, part.Key)
		if err != nil {
			return "", "", apperrors.NewInternalError("failed to read upload part")
		}

		remaining := DocumentImportMaxMarkdownSize - int64(buf.Len()) + 1
		if remaining <= 0 {
			_ = reader.Close()
			return "", "", apperrors.NewParamError("markdown file is too large")
		}
		_, copyErr := io.Copy(io.MultiWriter(&buf, hash), io.LimitReader(reader, remaining))
		closeErr := reader.Close()
		if copyErr != nil {
			return "", "", apperrors.NewInternalError("failed to assemble markdown")
		}
		if closeErr != nil {
			return "", "", apperrors.NewInternalError("failed to close upload part")
		}
		if int64(buf.Len()) > DocumentImportMaxMarkdownSize {
			return "", "", apperrors.NewParamError("markdown file is too large")
		}
	}

	contentBytes := buf.Bytes()
	contentBytes = bytes.TrimPrefix(contentBytes, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(contentBytes) {
		return "", "", apperrors.NewParamError("markdown file must be UTF-8 encoded")
	}
	return string(contentBytes), hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *DocumentImportService) withSessionLock(ctx context.Context, uploadID string, fn func() error) error {
	if s == nil || s.cache == nil {
		return apperrors.NewInternalError("document import is not configured")
	}
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return apperrors.NewParamError("upload_id is required")
	}
	lock := cache.NewDistributedLock(s.cache, "document_import:lock:"+uploadID, 10*time.Second)
	acquired, err := lock.Acquire(ctx)
	if err != nil {
		return apperrors.NewInternalError("failed to acquire import lock")
	}
	if !acquired {
		return apperrors.NewConflictError("import session is busy")
	}
	defer func() {
		_ = lock.Release(ctx)
	}()
	return fn()
}

func (s *DocumentImportService) saveSession(ctx context.Context, session *documentImportSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal import session: %w", err)
	}
	return s.cache.Set(ctx, documentImportSessionKey(session.UploadID), string(data), documentImportSessionTTL)
}

func (s *DocumentImportService) loadSession(ctx context.Context, uid int, uploadID string) (*documentImportSession, error) {
	raw, err := s.cache.Get(ctx, documentImportSessionKey(uploadID))
	if err != nil {
		return nil, apperrors.NewNotFoundError("import session not found or expired")
	}
	var session documentImportSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, apperrors.NewInternalError("failed to parse import session")
	}
	if session.UserID != uid {
		return nil, apperrors.NewForbiddenError("no permission to access import session")
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, apperrors.NewNotFoundError("import session expired")
	}
	if session.Parts == nil {
		session.Parts = map[int]documentImportPart{}
	}
	if session.Assets == nil {
		session.Assets = map[string]documentImportAsset{}
	}
	return &session, nil
}

func (s *DocumentImportService) cleanupSessionParts(ctx context.Context, lg *zap.Logger, session *documentImportSession) {
	for _, part := range session.Parts {
		if err := s.store.DeleteObject(ctx, part.Key); err != nil {
			lg.Warn("document_import.cleanup.part_failed", zap.String("key", part.Key), zap.Error(err))
		}
	}
}

func (s *DocumentImportService) cleanupSessionAssets(ctx context.Context, lg *zap.Logger, session *documentImportSession) {
	for _, asset := range session.Assets {
		if err := s.store.DeleteObject(ctx, asset.Key); err != nil {
			lg.Warn("document_import.cleanup.asset_failed", zap.String("key", asset.Key), zap.Error(err))
		}
	}
}

func (session *documentImportSession) result() *DocumentImportSessionResult {
	return &DocumentImportSessionResult{
		UploadID:          session.UploadID,
		ProjectID:         session.ProjectID,
		FileName:          session.FileName,
		Title:             session.Title,
		TotalSize:         session.TotalSize,
		TotalParts:        session.TotalParts,
		ChunkSize:         session.ChunkSize,
		CollaborationMode: session.CollaborationMode,
		ExpiresAt:         session.ExpiresAt,
	}
}

func (session *documentImportSession) assetResults(uploadID string) []DocumentImportAssetResult {
	if len(session.Assets) == 0 {
		return nil
	}
	keys := make([]string, 0, len(session.Assets))
	for key := range session.Assets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	results := make([]DocumentImportAssetResult, 0, len(keys))
	for _, key := range keys {
		asset := session.Assets[key]
		results = append(results, DocumentImportAssetResult{
			UploadID:     uploadID,
			OriginalPath: asset.OriginalPath,
			URL:          asset.URL,
			Markdown:     fmt.Sprintf("![%s](%s)", strings.TrimSuffix(path.Base(asset.OriginalPath), path.Ext(asset.OriginalPath)), asset.URL),
		})
	}
	return results
}

func rewriteMarkdownImageRefs(content string, assets map[string]documentImportAsset) (string, int) {
	if len(assets) == 0 || content == "" {
		return content, 0
	}
	lookup := buildAssetLookup(assets)
	rewrittenCount := 0

	content = markdownImageRegexp.ReplaceAllStringFunc(content, func(match string) string {
		parts := markdownImageRegexp.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		target := normalizeMarkdownImageTarget(parts[2])
		if shouldSkipMarkdownImageTarget(target) {
			return match
		}
		url, ok := lookup[normalizeAssetPath(target)]
		if !ok {
			return match
		}
		rewrittenCount++
		return fmt.Sprintf("![%s](%s%s)", parts[1], url, parts[3])
	})

	content = htmlImageSrcRegexp.ReplaceAllStringFunc(content, func(match string) string {
		parts := htmlImageSrcRegexp.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		target := normalizeMarkdownImageTarget(parts[2])
		if shouldSkipMarkdownImageTarget(target) {
			return match
		}
		url, ok := lookup[normalizeAssetPath(target)]
		if !ok {
			return match
		}
		rewrittenCount++
		return parts[1] + url + parts[3]
	})

	return content, rewrittenCount
}

func buildAssetLookup(assets map[string]documentImportAsset) map[string]string {
	lookup := make(map[string]string, len(assets)*2)
	for _, asset := range assets {
		normalized := normalizeAssetPath(asset.OriginalPath)
		if normalized == "" || asset.URL == "" {
			continue
		}
		lookup[normalized] = asset.URL
		base := path.Base(normalized)
		if _, exists := lookup[base]; !exists {
			lookup[base] = asset.URL
		}
	}
	return lookup
}

func normalizeMarkdownImageTarget(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "<>")
	return value
}

func shouldSkipMarkdownImageTarget(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return lower == "" ||
		strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(lower, "#")
}

func sanitizeImportFileName(value string) string {
	value = strings.TrimSpace(filepath.Base(value))
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Base(value)
	if value == "." || value == "/" {
		return ""
	}
	return value
}

func titleFromMarkdownFile(fileName string) string {
	ext := path.Ext(fileName)
	return strings.TrimSpace(strings.TrimSuffix(fileName, ext))
}

func isMarkdownFile(fileName string) bool {
	switch strings.ToLower(path.Ext(fileName)) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}

func isHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func normalizeAssetPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(value, "./")
	value = strings.TrimPrefix(value, "/")
	value = path.Clean(value)
	if value == "." || strings.HasPrefix(value, "../") || value == ".." {
		return ""
	}
	return value
}

func isSupportedMarkdownAsset(fileName string, contentType string) bool {
	ext := strings.ToLower(path.Ext(fileName))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
	default:
		return false
	}
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" || contentType == "application/octet-stream" {
		return true
	}
	return strings.HasPrefix(contentType, "image/")
}

func documentImportSessionKey(uploadID string) string {
	return "document_import:session:" + strings.TrimSpace(uploadID)
}

func documentImportPartKey(uid int, uploadID string, partNo int) string {
	return fmt.Sprintf("documents/imports/%d/%s/parts/%06d.part", uid, uploadID, partNo)
}

func documentImportAssetKey(uid int, uploadID string, fileName string) string {
	ext := strings.ToLower(path.Ext(fileName))
	name := strings.TrimSuffix(sanitizeImportFileName(fileName), ext)
	name = strings.NewReplacer(" ", "-", "_", "-").Replace(name)
	if name == "" {
		name = "asset"
	}
	return fmt.Sprintf("documents/assets/%d/%s/%s-%s%s", uid, uploadID, name, uuid.NewString(), ext)
}

func stringPtr(value string) *string {
	return &value
}
