package main

// ratelimit_test.go — подпатч 6.12 (B3.3): тесты Limiter, clientIP.
//
// Все тесты чистые: без БД, без HTTP-сервера, без сети.
// Инъекция времени — через поле limiter.now (мы в том же пакете).

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestLimiter — Limiter с управляемым временем.
// Возвращает (l, advance) — advance(d) двигает фейковое «сейчас».
func newTestLimiter(limit int, window time.Duration) (*Limiter, func(time.Duration)) {
	fakeNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := NewLimiter(limit, window)
	l.now = func() time.Time { return fakeNow }
	return l, func(d time.Duration) { fakeNow = fakeNow.Add(d) }
}

// ---------------------------------------------------------------
// Allow: базовая семантика
// ---------------------------------------------------------------

func TestLimiterAllow(t *testing.T) {
	l, _ := newTestLimiter(3, time.Minute)

	for i := 1; i <= 3; i++ {
		ok, _ := l.Allow("k")
		if !ok {
			t.Fatalf("попытка %d: ожидали ok=true", i)
		}
	}

	ok, retry := l.Allow("k")
	if ok {
		t.Fatalf("попытка 4: ожидали ok=false")
	}
	if retry <= 0 {
		t.Fatalf("ожидали retryAfter > 0, получили %v", retry)
	}
}

// ---------------------------------------------------------------
// Reset: сбрасывает счётчик
// ---------------------------------------------------------------

func TestLimiterReset(t *testing.T) {
	l, _ := newTestLimiter(2, time.Minute)

	l.Allow("k")
	l.Allow("k")
	if ok, _ := l.Allow("k"); ok {
		t.Fatalf("ожидали блокировку после 2 попыток")
	}

	l.Reset("k")
	if ok, _ := l.Allow("k"); !ok {
		t.Fatalf("после Reset ожидали ok=true")
	}
}

// ---------------------------------------------------------------
// Скользящее окно: старые метки уходят
// ---------------------------------------------------------------

func TestLimiterWindowExpiry(t *testing.T) {
	l, advance := newTestLimiter(2, time.Minute)

	l.Allow("k")
	l.Allow("k")
	if ok, _ := l.Allow("k"); ok {
		t.Fatalf("ожидали блокировку")
	}

	// прокручиваем время за окно → счётчик должен очиститься
	advance(61 * time.Second)

	if ok, _ := l.Allow("k"); !ok {
		t.Fatalf("после истечения окна ожидали ok=true")
	}
}

// ---------------------------------------------------------------
// Разные ключи не влияют друг на друга
// ---------------------------------------------------------------

func TestLimiterIndependentKeys(t *testing.T) {
	l, _ := newTestLimiter(1, time.Minute)

	if ok, _ := l.Allow("a"); !ok {
		t.Fatalf("key=a: первый Allow должен быть ok")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatalf("key=a: второй Allow должен быть !ok")
	}

	// key=b — свежий, должен быть ok
	if ok, _ := l.Allow("b"); !ok {
		t.Fatalf("key=b: первый Allow должен быть ok")
	}
}

// ---------------------------------------------------------------
// clientIP: XFF и RemoteAddr
// ---------------------------------------------------------------

func TestClientIP(t *testing.T) {
	cases := []struct {
		name   string
		xff    string
		remote string
		wantIP string
	}{
		{
			name:   "no-xff-remote-with-port",
			remote: "1.2.3.4:55555",
			wantIP: "1.2.3.4",
		},
		{
			name:   "no-xff-remote-no-port",
			remote: "1.2.3.4",
			wantIP: "1.2.3.4",
		},
		{
			name:   "xff-single",
			xff:    "9.9.9.9",
			remote: "1.2.3.4:55555",
			wantIP: "9.9.9.9",
		},
		{
			name:   "xff-multi",
			xff:    "9.9.9.9, 8.8.8.8, 7.7.7.7",
			remote: "1.2.3.4:55555",
			wantIP: "9.9.9.9",
		},
		{
			name:   "xff-with-spaces",
			xff:    "  9.9.9.9  ",
			remote: "1.2.3.4:55555",
			wantIP: "9.9.9.9",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
			r.RemoteAddr = c.remote
			if c.xff != "" {
				r.Header.Set("X-Forwarded-For", c.xff)
			}
			got := clientIP(r)
			if got != c.wantIP {
				t.Fatalf("clientIP() = %q, ожидали %q", got, c.wantIP)
			}
		})
	}
}
