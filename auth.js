/* ============================================================
 * BOT_MAX — auth.js
 * Шаг 6.7 · 2026-09-24
 *
 * Клиентский модуль авторизации. Экспортирует window.BotMaxAuth.
 *
 * Ключи localStorage:
 *   botmax_access   — JWT access (15m)
 *   botmax_refresh  — JWT refresh (7d)
 *   botmax_doctor   — JSON { id, name, role }
 *
 * Контракт:
 *   authFetch(method, url, rawBody, extraHeaders) -> Promise<Response>
 *     - ставит Content-Type: application/json
 *     - ставит Authorization: Bearer <access> (если токен есть)
 *     - добавляет extraHeaders (напр. X-Idempotency-Key)
 *     - при 401 и НЕ-auth-пути: single-flight refresh + 1 retry
 *     - если refresh провалился: clearTokens() + redirect login.html
 *
 * Важно: authFetch возвращает Response (не JSON), чтобы вызывающий
 * код (apiRequest) сам решал, как обрабатывать res.ok / 204 / json.
 * ============================================================ */

(function (global) {
    'use strict';

    // ---------- Константы ----------
    var K_ACCESS  = 'botmax_access';
    var K_REFRESH = 'botmax_refresh';
    var K_DOCTOR  = 'botmax_doctor';
    var LOGIN_URL = 'login.html';

    // ---------- Storage ----------
    function getToken() {
        return localStorage.getItem(K_ACCESS) || '';
    }

    function getRefreshToken() {
        return localStorage.getItem(K_REFRESH) || '';
    }

    function getDoctor() {
        try {
            return JSON.parse(localStorage.getItem(K_DOCTOR) || 'null');
        } catch (e) {
            return null;
        }
    }

    function setTokens(access, refresh, doctor) {
        if (access)  localStorage.setItem(K_ACCESS,  access);
        if (refresh) localStorage.setItem(K_REFRESH, refresh);
        if (doctor)  localStorage.setItem(K_DOCTOR,  JSON.stringify(doctor));
    }

    function clearTokens() {
        localStorage.removeItem(K_ACCESS);
        localStorage.removeItem(K_REFRESH);
        localStorage.removeItem(K_DOCTOR);
    }

    // ---------- Headers ----------
    function getAuthHeaders() {
        var t = getToken();
        return t ? { Authorization: 'Bearer ' + t } : {};
    }

    // ---------- Single-flight refresh ----------
    // Если несколько запросов одновременно словили 401 —
    // все ждут один и тот же промис, refresh делается один раз.
    var refreshPromise = null;

    function refreshAccess() {
        if (refreshPromise) return refreshPromise;

        var rt = getRefreshToken();
        if (!rt) return Promise.resolve(false);

        refreshPromise = fetch('/api/auth/refresh', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ refreshToken: rt })
        }).then(function (res) {
            if (!res.ok) return false;
            return res.json().then(function (data) {
                if (!data || !data.accessToken) return false;
                setTokens(data.accessToken, data.refreshToken, data.doctor);
                return true;
            });
        }).catch(function () {
            return false;
        }).then(function (ok) {
            refreshPromise = null;
            return ok;
        });

        return refreshPromise;
    }

    // ---------- authFetch ----------
    function isAuthPath(url) {
        // login / refresh / logout не ретраим — иначе рекурсия.
        return url.indexOf('/api/auth/') !== -1;
    }

    function buildOpts(method, rawBody, extraHeaders) {
        var headers = Object.assign(
            { 'Content-Type': 'application/json' },
            extraHeaders || {},
            getAuthHeaders()
        );
        var opts = { method: method, headers: headers };
        if (rawBody !== undefined) {
            opts.body = JSON.stringify(rawBody);
        }
        return opts;
    }

    function authFetch(method, url, rawBody, extraHeaders) {
        return fetch(url, buildOpts(method, rawBody, extraHeaders))
            .then(function (res) {
                if (res.status !== 401) return res;
                if (isAuthPath(url)) return res;

                // 401 на обычном API-пути → пробуем refresh один раз.
                return refreshAccess().then(function (ok) {
                    if (!ok) {
                        clearTokens();
                        if (location.pathname.indexOf(LOGIN_URL) === -1) {
                            location.replace(LOGIN_URL);
                        }
                        return res; // отдаём оригинальный 401
                    }
                    // retry с новым токеном (тело не изменилось)
                    return fetch(url, buildOpts(method, rawBody, extraHeaders));
                });
            });
    }

    // ---------- logout ----------
    function logout() {
        var rt = getRefreshToken();
        var done = function () {
            clearTokens();
            location.replace(LOGIN_URL);
        };

        // Fire-and-forget: не блокируем UI, не ждём долго.
        try {
            fetch('/api/auth/logout', {
                method: 'POST',
                headers: Object.assign(
                    { 'Content-Type': 'application/json' },
                    getAuthHeaders()
                ),
                body: JSON.stringify({ refreshToken: rt })
            }).catch(function () {}).then(done);
        } catch (e) {
            done();
        }
    }

    // ---------- роли (Шаг 6.8) ----------
    // Совпадают со значениями Role в backend/auth.go.
    var ROLE = {
        SUPERADMIN: 'superadmin',
        ADMIN:      'admin',
        DOCTOR:     'doctor'
    };

    /**
     * hasRole(...roles) — true, если текущий врач имеет одну
     * из перечисленных ролей. Без аргументов — false.
     * Без залогиненного врача — false.
     */
    function hasRole() {
        var roles = Array.prototype.slice.call(arguments);
        if (roles.length === 0) return false;
        var d = getDoctor();
        if (!d || !d.role) return false;
        return roles.indexOf(d.role) !== -1;
    }

    function isSuperadmin() { return hasRole(ROLE.SUPERADMIN); }
    function isAdmin()      { return hasRole(ROLE.ADMIN, ROLE.SUPERADMIN); }
    function isDoctor()     { return hasRole(ROLE.DOCTOR, ROLE.ADMIN, ROLE.SUPERADMIN); }

    // data-requires-role="role1 role2" — показать элемент, если роль
    // входит в список. Элемент ДОЛЖЕН быть hidden в HTML изначально.
    // ВАЖНО: только UX. Сервер (requireRole) — источник правды.
    function applyRoleVisibility() {
        var els = document.querySelectorAll('[data-requires-role]');
        for (var i = 0; i < els.length; i++) {
            var el = els[i];
            var raw = el.getAttribute('data-requires-role') || '';
            var roles = raw.split(/\s+/).filter(Boolean);
            if (roles.length > 0 && hasRole.apply(null, roles)) {
                el.hidden = false;
            } else {
                el.hidden = true;
            }
        }
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', applyRoleVisibility);
    } else {
        applyRoleVisibility();
    }

    // ---------- guard ----------
    function requireAuth() {
        if (!getToken()) {
            location.replace(LOGIN_URL);
        }
    }

    // ---------- export ----------
    global.BotMaxAuth = {
        getToken:        getToken,
        getRefreshToken: getRefreshToken,
        getDoctor:       getDoctor,
        setTokens:       setTokens,
        clearTokens:     clearTokens,
        getAuthHeaders:  getAuthHeaders,
        refreshAccess:   refreshAccess,
        authFetch:       authFetch,
        logout:          logout,
        requireAuth:     requireAuth,
        // Шаг 6.8:
        ROLE:                ROLE,
        hasRole:             hasRole,
        isSuperadmin:        isSuperadmin,
        isAdmin:             isAdmin,
        isDoctor:            isDoctor,
        applyRoleVisibility: applyRoleVisibility
    };

})(window);
