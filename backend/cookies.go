package main

// cookies.go — подпатч 6.16 (B7.2): установка/снятие auth-cookie.
//
// Схема (SameSite=Strict, HttpOnly, Secure=currentEnv.CookieSecure):
//   botmax_access  — JWT access,  TTL = AccessTTL,  Path=/api
//   botmax_refresh — JWT refresh, TTL = RefreshTTL, Path=/api/auth
//
// Обратная совместимость: сервер и ставит cookie, и возвращает токены
// в JSON — старый фронт (localStorage) продолжает работать.
//
// CSRF: SameSite=Strict достаточно для same-origin SPA.

import (
	"net/http"
	"time"
)

const (
	cookieNameAccess  = "botmax_access"
	cookieNameRefresh = "botmax_refresh"

	cookiePathAccess  = "/api"
	cookiePathRefresh = "/api/auth"

	// B30.2: cookies пациента. Отдельные имена — чтобы врач и пациент
	// могли быть залогинены в одном браузере без коллизии.
	cookieNamePatientAccess  = "botmax_pat_access"
	cookieNamePatientRefresh = "botmax_pat_refresh"

	cookiePathPatientAccess  = "/api"
	cookiePathPatientRefresh = "/api/patient-auth"
)

// setAuthCookies ставит access+refresh cookie.
// Secure берётся из currentEnv.CookieSecure (prod → true, dev → false).
func setAuthCookies(w http.ResponseWriter, access, refresh string) {
	if currentEnv == nil {
		// без currentEnv не знаем Secure — пропускаем, но это значит,
		// что main не вызвал validateEnv, т.е. баг сборки.
		return
	}
	secure := currentEnv.CookieSecure

	if access != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieNameAccess,
			Value:    access,
			Path:     cookiePathAccess,
			MaxAge:   int(AccessTTL / time.Second),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		})
	}
	if refresh != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieNameRefresh,
			Value:    refresh,
			Path:     cookiePathRefresh,
			MaxAge:   int(RefreshTTL / time.Second),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		})
	}
}

// clearAuthCookies снимает обе cookie (MaxAge<0 → удалить сейчас же).
func clearAuthCookies(w http.ResponseWriter) {
	secure := false
	if currentEnv != nil {
		secure = currentEnv.CookieSecure
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieNameAccess,
		Value:    "",
		Path:     cookiePathAccess,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     cookieNameRefresh,
		Value:    "",
		Path:     cookiePathRefresh,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

// setPatientCookies — аналог setAuthCookies для пациента (B30.2).
func setPatientCookies(w http.ResponseWriter, access, refresh string) {
	if currentEnv == nil {
		return
	}
	secure := currentEnv.CookieSecure

	if access != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieNamePatientAccess,
			Value:    access,
			Path:     cookiePathPatientAccess,
			MaxAge:   int(AccessTTL / time.Second),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		})
	}
	if refresh != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieNamePatientRefresh,
			Value:    refresh,
			Path:     cookiePathPatientRefresh,
			MaxAge:   int(RefreshTTL / time.Second),
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		})
	}
}

// clearPatientCookies снимает patient-cookies.
func clearPatientCookies(w http.ResponseWriter) {
	for _, c := range []struct{ name, path string }{
		{cookieNamePatientAccess, cookiePathPatientAccess},
		{cookieNamePatientRefresh, cookiePathPatientRefresh},
	} {
		http.SetCookie(w, &http.Cookie{
			Name:     c.name,
			Value:    "",
			Path:     c.path,
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
	}
}
