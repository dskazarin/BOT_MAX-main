package main

// ratelimit.go — подпатч 6.12 (B3.1): in-memory rate-limiter для login.
//
// Назначение: защита POST /api/auth/login от brute-force.
// Простая модель — скользящее окно: для каждого ключа храним метки
// времени попыток; при попытке:
//   1) чистим метки старше окна,
//   2) если len(attempts) >= limit → отказ + Retry-After,
//   3) иначе — добавляем метку и разрешаем.
//
// In-memory, sync.Mutex + map[string][]time.Time.
// Без внешних зависимостей. Для масштабирования — вынести в Redis.

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limiter — потокобезопасный лимитер скользящего окна.
type Limiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time

	limit  int
	window time.Duration

	// now — инъекция времени для тестов. По умолчанию time.Now.
	now func() time.Time
}

// NewLimiter создаёт лимитер: не более limit попыток за window.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		now:      time.Now,
	}
}

// Allow — «взять попытку» для ключа.
//   - ok=true  → попытка разрешена и учтена;
//   - ok=false → лимит превышен; retryAfter — сколько ждать до следующей.
func (l *Limiter) Allow(key string) (ok bool, retryAfter time.Duration) {
	now := l.now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Чистим старые метки.
	old := l.attempts[key]
	kept := old[:0]
	for _, t := range old {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	// edge: если всё выкинули — освобождаем map-слот.
	if len(kept) == 0 && len(old) > 0 {
		delete(l.attempts, key)
		kept = nil
	}

	// Лимит превышен?
	if len(kept) >= l.limit {
		// retryAfter = когда самая старая из оставшихся меток выйдет за окно.
		oldest := kept[0]
		retry := oldest.Add(l.window).Sub(now)
		if retry < time.Second {
			retry = time.Second
		}
		l.attempts[key] = kept
		return false, retry
	}

	// Разрешаем и записываем.
	l.attempts[key] = append(kept, now)
	return true, 0
}

// Reset — сбросить счётчик ключа (например, после успешного логина).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

// ---------------------------------------------------------------
// Пакетные лимитеры для /api/auth/login
// ---------------------------------------------------------------

const (
	loginLimitPerIP    = 10
	loginLimitPerEmail = 5
	loginWindow        = 15 * time.Minute
)

var (
	loginLimiterPerIP    = NewLimiter(loginLimitPerIP, loginWindow)
	loginLimiterPerEmail = NewLimiter(loginLimitPerEmail, loginWindow)
)

// ---------------------------------------------------------------
// HTTP-хелперы
// ---------------------------------------------------------------

// clientIP — приблизительный IP клиента.
//
// Приоритет: X-Forwarded-For (первый в списке), иначе RemoteAddr.
// ВНИМАНИЕ: XFF легко подделать. Использовать только за доверенным
// прокси/балансером. Прямое подключение к интернету → XFF игнорировать.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeTooManyRequests — 429 + Retry-After + JSON-тело.
func writeTooManyRequests(w http.ResponseWriter, retryAfter time.Duration) {
	secs := int(retryAfter.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      "too many requests",
		"retryAfter": secs,
	})
}
