package utils

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
