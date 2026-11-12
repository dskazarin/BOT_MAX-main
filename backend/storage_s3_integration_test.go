//go:build integration

package main

// storage_s3_integration_test.go — B30.15 шаг 2.
// Гоняет реальный s3Storage против S3/minio.
//
// Запуск:
//   BOTMAX_STORAGE_S3_ENDPOINT=127.0.0.1:9000 \
//   BOTMAX_STORAGE_S3_BUCKET=botmax-smoke \
//   BOTMAX_STORAGE_S3_ACCESS_KEY=minioadmin \
//   BOTMAX_STORAGE_S3_SECRET_KEY=minioadmin \
//   go test -tags=integration ./backend/ -run TestS3Storage -v
//
// Без BOTMAX_STORAGE_S3_ENDPOINT тест скипается (инфра недоступна).

import (
	"bytes"
	"os"
	"testing"
	"time"
)

func s3EnvOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func s3IntegrationCfg(t *testing.T) S3Cfg {
	t.Helper()
	ep := os.Getenv("BOTMAX_STORAGE_S3_ENDPOINT")
	if ep == "" {
		t.Skip("BOTMAX_STORAGE_S3_ENDPOINT не задан — S3 integration skip")
	}
	return S3Cfg{
		Endpoint:   ep,
		Bucket:     s3EnvOr("BOTMAX_STORAGE_S3_BUCKET", "botmax-smoke"),
		AccessKey:  s3EnvOr("BOTMAX_STORAGE_S3_ACCESS_KEY", "minioadmin"),
		SecretKey:  s3EnvOr("BOTMAX_STORAGE_S3_SECRET_KEY", "minioadmin"),
		Region:     s3EnvOr("BOTMAX_STORAGE_S3_REGION", "us-east-1"),
		UseSSL:     false,
		Presign:    true,
		PresignTTL: 2 * time.Minute,
	}
}

func TestS3Storage_Integration(t *testing.T) {
	cfg := s3IntegrationCfg(t)
	st, err := newS3Storage(cfg)
	if err != nil {
		t.Fatalf("newS3Storage: %v", err)
	}

	pid := "PAT-INTEGRATION"
	id := "it-" + time.Now().Format("20060102-150405.000")
	t.Cleanup(func() { _ = st.Delete(pid, id) })

	payload := []byte("s3-integration " + time.Now().Format(time.RFC3339Nano))
	filename := "it.txt"
	mime := "text/plain"

	if err := st.Put(pid, id, filename, mime, payload); err != nil {
		t.Fatalf("Put: %v", err)
	}

	u, ttl, err := st.PresignURL(pid, id)
	if err != nil {
		t.Fatalf("PresignURL: %v", err)
	}
	if u == "" {
		t.Fatal("PresignURL: пустой URL")
	}
	if ttl != cfg.PresignTTL {
		t.Fatalf("PresignURL TTL=%v, ждали %v", ttl, cfg.PresignTTL)
	}

	fn, gmime, data, ok, err := st.Get(pid, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get: ok=false, объект должен существовать")
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("Get data mismatch: got %q, want %q", data, payload)
	}
	if fn != filename {
		t.Fatalf("Get filename=%q, want %q", fn, filename)
	}
	if gmime != mime {
		t.Fatalf("Get mime=%q, want %q", gmime, mime)
	}

	if err := st.Delete(pid, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, _, _, ok, err = st.Get(pid, id)
	if err != nil {
		t.Fatalf("Get after Delete: err=%v, ждали nil", err)
	}
	if ok {
		t.Fatal("Get after Delete: ok=true, объект должен отсутствовать")
	}
}

func TestS3Storage_PresignDisabled_Integration(t *testing.T) {
	cfg := s3IntegrationCfg(t)
	cfg.Presign = false
	st, err := newS3Storage(cfg)
	if err != nil {
		t.Fatalf("newS3Storage: %v", err)
	}
	if _, _, err := st.PresignURL("p", "i"); err == nil {
		t.Fatal("PresignURL при Presign=false: ждали ошибку")
	}
}
