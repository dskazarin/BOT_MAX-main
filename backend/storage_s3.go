package main

// storage_s3.go — B30.14 §6.59 шаг 1: s3Storage (реализация FileStorage).
//
// Семантика совпадает с blobStorage/fsStorage:
//   Put(pid, id, filename, mime, data) — upsert по ключу patients/{pid}/{id}
//   Get(pid, id)                       — ok=false при отсутствии объекта (не ошибка)
//   Delete(pid, id)                    — идемпотентно (отсутствие не ошибка)
//
// Метаданные filename/mime кладутся в UserMetadata объекта; тело — в data.
// Presigned URL — вне FileStorage, добавляется в Шаге 2 отдельным интерфейсом.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3Storage — реализация FileStorage поверх S3-совместимого хранилища.
type s3Storage struct {
	client s3Client // интерфейс для stub в тестах
	bucket string
}

var _ FileStorage = (*s3Storage)(nil)

// s3Client — минимальный интерфейс над minio.Client для подмены в тестах.
type s3Client interface {
	PutObject(ctx context.Context, bucket, object string, r io.Reader, size int64, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	GetObject(ctx context.Context, bucket, object string, opts minio.GetObjectOptions) (*minio.Object, error)
	RemoveObject(ctx context.Context, bucket, object string, opts minio.RemoveObjectOptions) error
	StatObject(ctx context.Context, bucket, object string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
	PresignedGetObject(ctx context.Context, bucket, object string, expires time.Duration, params url.Values) (*url.URL, error)
}

func newS3Storage(cfg S3Cfg) (*s3Storage, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, errors.New("s3: endpoint и bucket обязательны")
	}
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("s3: init client: %w", err)
	}
	return &s3Storage{client: cli, bucket: cfg.Bucket}, nil
}

// s3Key — единый формат ключа объекта.
func s3Key(pid, id string) string {
	return fmt.Sprintf("patients/%s/%s", pid, id)
}

// Put — загрузка тела с метаданными filename/mime.
func (s *s3Storage) Put(pid, id, filename, mime string, data []byte) error {
	if pid == "" || id == "" {
		return fmt.Errorf("s3Storage.Put: пустой pid/id")
	}
	_, err := s.client.PutObject(
		context.Background(), s.bucket, s3Key(pid, id),
		bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{
			ContentType: mime,
			UserMetadata: map[string]string{
				"filename": filename,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("s3Storage.Put: %w", err)
	}
	return nil
}

// Get — ok=false, err=nil при отсутствии объекта (как blobStorage).
func (s *s3Storage) Get(pid, id string) (string, string, []byte, bool, error) {
	if pid == "" || id == "" {
		return "", "", nil, false, fmt.Errorf("s3Storage.Get: пустой pid/id")
	}
	key := s3Key(pid, id)

	info, statErr := s.client.StatObject(context.Background(), s.bucket, key, minio.StatObjectOptions{})
	if statErr != nil {
		var er minio.ErrorResponse
		if errors.As(statErr, &er) && er.Code == "NoSuchKey" {
			return "", "", nil, false, nil
		}
		if strings.Contains(statErr.Error(), "NoSuchKey") {
			return "", "", nil, false, nil
		}
		return "", "", nil, false, fmt.Errorf("s3Storage.Get: stat: %w", statErr)
	}
	filename := info.UserMetadata["filename"]
	mime := info.ContentType

	obj, getErr := s.client.GetObject(context.Background(), s.bucket, key, minio.GetObjectOptions{})
	if getErr != nil {
		return "", "", nil, false, fmt.Errorf("s3Storage.Get: get: %w", getErr)
	}
	if obj == nil {
		return "", "", nil, false, fmt.Errorf("s3Storage.Get: объект пуст (nil)")
	}
	defer obj.Close()

	body, readErr := io.ReadAll(obj)
	if readErr != nil {
		return "", "", nil, false, fmt.Errorf("s3Storage.Get: read: %w", readErr)
	}
	return filename, mime, body, true, nil
}

// Delete — идемпотентно (отсутствие объекта не ошибка).
func (s *s3Storage) Delete(pid, id string) error {
	if pid == "" || id == "" {
		return fmt.Errorf("s3Storage.Delete: пустой pid/id")
	}
	if err := s.client.RemoveObject(context.Background(), s.bucket, s3Key(pid, id), minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("s3Storage.Delete: %w", err)
	}
	return nil
}
