package main

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

// stubS3 — минимальная in-memory реализация s3Client.
type stubS3 struct {
	objects map[string][]byte
	meta    map[string]map[string]string
	force   error
}

func newStubS3() *stubS3 {
	return &stubS3{objects: map[string][]byte{}, meta: map[string]map[string]string{}}
}

func (s *stubS3) PutObject(_ context.Context, _, object string, r io.Reader, _ int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	if s.force != nil {
		return minio.UploadInfo{}, s.force
	}
	b, _ := io.ReadAll(r)
	s.objects[object] = b
	s.meta[object] = map[string]string{"filename": opts.UserMetadata["filename"], "mime": opts.ContentType}
	return minio.UploadInfo{}, nil
}

func (s *stubS3) GetObject(_ context.Context, _, object string, _ minio.GetObjectOptions) (*minio.Object, error) {
	if s.force != nil {
		return nil, s.force
	}
	if _, ok := s.objects[object]; !ok {
		return nil, minio.ErrorResponse{Code: "NoSuchKey"}
	}
	return nil, nil
}

func (s *stubS3) RemoveObject(_ context.Context, _, object string, _ minio.RemoveObjectOptions) error {
	if s.force != nil {
		return s.force
	}
	delete(s.objects, object)
	delete(s.meta, object)
	return nil
}

func (s *stubS3) StatObject(_ context.Context, _, object string, _ minio.StatObjectOptions) (minio.ObjectInfo, error) {
	if s.force != nil {
		return minio.ObjectInfo{}, s.force
	}
	b, ok := s.objects[object]
	if !ok {
		return minio.ObjectInfo{}, minio.ErrorResponse{Code: "NoSuchKey"}
	}
	m := s.meta[object]
	return minio.ObjectInfo{
		Size:         int64(len(b)),
		ContentType:  m["mime"],
		UserMetadata: map[string]string{"filename": m["filename"]},
	}, nil
}


func (s *stubS3) PresignedGetObject(_ context.Context, _, object string, _ time.Duration, _ url.Values) (*url.URL, error) {
	return url.Parse("https://stub/" + object)
}

func TestS3Storage_Put(t *testing.T) {
	st := newStubS3()
	s := &s3Storage{client: st, bucket: "b"}
	data := []byte("hello")
	if err := s.Put("p1", "f1", "a.txt", "text/plain", data); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := st.objects["patients/p1/f1"]; !ok {
		t.Fatal("объект не сохранился в стабе")
	}
	info, err := st.StatObject(context.Background(), "b", "patients/p1/f1", minio.StatObjectOptions{})
	if err != nil {
		t.Fatalf("StatObject: %v", err)
	}
	if info.ContentType != "text/plain" || info.UserMetadata["filename"] != "a.txt" {
		t.Fatalf("метаданные не совпадают: %+v", info)
	}
}

func TestS3Storage_Delete(t *testing.T) {
	st := newStubS3()
	s := &s3Storage{client: st, bucket: "b"}
	_ = s.Put("p1", "f1", "a.txt", "text/plain", []byte("x"))
	if err := s.Delete("p1", "f1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, _, _, ok, err := s.Get("p1", "f1")
	if err != nil || ok {
		t.Fatalf("после Delete ожидали ok=false err=nil, получили ok=%v err=%v", ok, err)
	}
}

func TestS3Storage_GetMissing(t *testing.T) {
	s := &s3Storage{client: newStubS3(), bucket: "b"}
	_, _, _, ok, err := s.Get("p1", "nope")
	if err != nil || ok {
		t.Fatalf("ожидали ok=false err=nil, получили ok=%v err=%v", ok, err)
	}
}

func TestS3Storage_PutEmptyIDs(t *testing.T) {
	s := &s3Storage{client: newStubS3(), bucket: "b"}
	if err := s.Put("", "f1", "a", "text/plain", nil); err == nil {
		t.Fatal("ожидали ошибку при пустом pid")
	}
	if err := s.Put("p1", "", "a", "text/plain", nil); err == nil {
		t.Fatal("ожидали ошибку при пустом id")
	}
}

func TestS3Storage_DeleteEmptyIDs(t *testing.T) {
	s := &s3Storage{client: newStubS3(), bucket: "b"}
	if err := s.Delete("", "f1"); err == nil {
		t.Fatal("ожидали ошибку при пустом pid")
	}
}

func TestS3Storage_PutError(t *testing.T) {
	st := newStubS3()
	st.force = errors.New("boom")
	s := &s3Storage{client: st, bucket: "b"}
	if err := s.Put("p1", "f1", "a", "text/plain", []byte("x")); err == nil {
		t.Fatal("ожидали ошибку из клиента")
	}
}

func TestS3Storage_PresignURL(t *testing.T) {
	st := newStubS3()
	s := &s3Storage{client: st, bucket: "b", presign: true, presignTTL: 15 * time.Minute}
	u, ttl, err := s.PresignURL("p1", "f1")
	if err != nil {
		t.Fatalf("PresignURL: %v", err)
	}
	if !strings.Contains(u, "patients/p1/f1") {
		t.Fatalf("URL не содержит ключ: %q", u)
	}
	if ttl != 15*time.Minute {
		t.Fatalf("ttl=%v, want 15m", ttl)
	}
}

func TestS3Storage_PresignDisabled(t *testing.T) {
	s := &s3Storage{client: newStubS3(), bucket: "b", presign: false}
	if _, _, err := s.PresignURL("p1", "f1"); err == nil {
		t.Fatal("ожидали ошибку при presign=false")
	}
}
