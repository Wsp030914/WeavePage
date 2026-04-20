package utils

// 文件说明：这个文件封装腾讯云 COS 对象存储能力。
// 实现方式：维护全局 COS client，并提供头像上传、通用对象存取、对象删除和 key/URL 规范化工具。
// 这样做的好处是上层服务可以只关心对象 key 和业务校验，不必重复处理底层 SDK 细节。

import (
	"ToDoList/server/config"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/tencentyun/cos-go-sdk-v5"

	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

var Client *cos.Client

// InitCos 初始化全局 COS client。
func InitCos(cfg *config.COSConfig) error {
	if cfg == nil {
		return fmt.Errorf("cos config is nil")
	}

	bucketURL := fmt.Sprintf("https://%s.cos.%s.myqcloud.com", cfg.Bucket, cfg.Region)
	u, err := url.Parse(bucketURL)
	if err != nil {
		return fmt.Errorf("parse bucket url failed: %w", err)
	}

	b := &cos.BaseURL{BucketURL: u}
	Client = cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
	})

	return nil
}

// PutObj 上传一个 multipart 文件到 COS。
// 这里自动探测 content-type 和生成随机对象 key，是为了减少同名覆盖和错误 MIME 类型带来的问题。
func PutObj(ctx context.Context, fh *multipart.FileHeader) (key string, url string, err error) {
	if Client == nil {
		return "", "", fmt.Errorf("cos client not initialized")
	}

	prefix := "images/"
	file, err := fh.Open()
	if err != nil {
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	rand8 := make([]byte, 4)
	_, _ = rand.Read(rand8)
	key = prefix + time.Now().Format("20060102_150405") + "_" + hex.EncodeToString(rand8) + ext

	ct := fh.Header.Get("Content-Type")
	var reader io.Reader = file
	if ct == "" {
		head := make([]byte, 512)
		n, _ := io.ReadFull(file, head)
		ct = http.DetectContentType(head[:n])
		reader = io.MultiReader(bytes.NewReader(head[:n]), file)
	}

	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: ct,
		},
	}
	if fh.Size > 0 {
		opt.ContentLength = fh.Size
	}

	resp, err := Client.Object.Put(ctx, key, reader, opt)
	if err != nil {
		return
	}

	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}

	objURL := Client.Object.GetObjectURL(key)
	if objURL.Scheme == "" {
		objURL.Scheme = "https"
	}
	url = objURL.String()
	return
}

// COSObjectStore exposes low-level COS object operations for services that
// already have their own validation and object-key strategy.
type COSObjectStore struct{}

// NewCOSObjectStore 创建一个可复用的 COS 对象存储适配器。
func NewCOSObjectStore() COSObjectStore {
	return COSObjectStore{}
}

// PutObject 按指定 key 写入对象并返回可访问 URL。
func (COSObjectStore) PutObject(ctx context.Context, key string, reader io.Reader, contentType string, contentLength int64) (string, error) {
	if Client == nil {
		return "", fmt.Errorf("cos client not initialized")
	}
	key = NormalizeObjectKey(key)
	if key == "" {
		return "", fmt.Errorf("empty object key")
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: contentType,
		},
	}
	if contentLength > 0 {
		opt.ContentLength = contentLength
	}

	resp, err := Client.Object.Put(ctx, key, reader, opt)
	if err != nil {
		return "", err
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	return ObjectURLFromKey(key), nil
}

// GetObject 读取对象内容流。
func (COSObjectStore) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if Client == nil {
		return nil, fmt.Errorf("cos client not initialized")
	}
	key = NormalizeObjectKey(key)
	if key == "" {
		return nil, fmt.Errorf("empty object key")
	}
	resp, err := Client.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, fmt.Errorf("empty object response body")
	}
	return resp.Body, nil
}

// DeleteObject 删除一个对象。
func (COSObjectStore) DeleteObject(ctx context.Context, key string) error {
	return DeleteObject(ctx, key)
}

// DeleteObject 删除一个对象 key 或 URL 指向的对象。
func DeleteObject(ctx context.Context, key string) error {
	if Client == nil {
		return fmt.Errorf("cos client not initialized")
	}
	key = NormalizeObjectKey(key)
	if key == "" {
		return fmt.Errorf("empty object key")
	}
	_, err := Client.Object.Delete(ctx, key)
	return err
}

// NormalizeObjectKey 把对象 URL 或原始 key 规范化成 COS key。
func NormalizeObjectKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return strings.TrimPrefix(parsed.Path, "/")
	}

	return strings.TrimPrefix(raw, "/")
}

// ObjectURLFromKey 根据 key 生成对象访问 URL。
func ObjectURLFromKey(key string) string {
	key = NormalizeObjectKey(key)
	if key == "" {
		return ""
	}
	if Client == nil {
		return key
	}

	objURL := Client.Object.GetObjectURL(key)
	if objURL.Scheme == "" {
		objURL.Scheme = "https"
	}
	return objURL.String()
}
