/* ============================================================
 * BOT_MAX — auth.js
 * Шаг 6.7 · 2026-09-24 · обновлено 6.17 (B8.2) 2026-09-25
 *
 * Клиентский модуль авторизации. Экспортирует window.BotMaxAuth.
 *
 * Ключи localStorage:
 *   botmax_doctor — JSON { id, name, role }  (единственный ключ)
 *
 * Токены (access + refresh) хранятся ТОЛЬКО в HttpOnly-cookie
 * (устанавливаются сервером на /api/auth/login и /api/auth/refresh).
 * JS их не видит и не может прочитать.
 *
 * Контракт:
 *   authFetch(method, url, rawBody, extraHeaders) -> Promise<Response>
 *     - ставит Content-Type: application/json
 *     - ставит credentials: 'include' (для cookie)
 *     - добавляет extraHeaders (напр. X-Idempotency-Key)
 *     - при 401 и НЕ-auth-пути: single-flight refresh + 1 retry
 *     - если refresh провалился: clearDoctor() + redirect login.html
 *
 * Важно: authFetch возвращает Response (не JSON), чтобы вызывающий
 * код (apiRequest) сам решал, как обрабатывать res.ok / 204 / json.
 * ============================================================ */

(function (global) {
    'use strict';

    // ---------- Константы ----------
    var K_DOCTOR  = 'botmax_doctor';
    var LOGIN_URL = 'login.html';

    // ---------- Storage (только doctor) ----------
    function getDoctor() {
        try {
            return JSON.parse(localStorage.getItem(K_DOCTOR) || 'null');
        } catch (e) {
            return null;
        }
    }

    function setDoctor(doctor) {
        if (doctor) {
            localStorage.setItem(K_DOCTOR, JSON.stringify(doctor));
        }
    }

    function clearDoctor() {
        localStorage.removeItem(K_DOCTOR);
    }

    // isLoggedIn — есть ли данные о враче (признак залогиненности).
    // Реальная проверка токена — на сервере: если access-cookie истёк,
    // authFetch словит 401 и попробует refresh.
    function isLoggedIn() {
        return !!getDoctor();
    }

    // clearTokens — обратная совместимость (старые HTML вызывают).
    // Теперь чистит только doctor.
    function clearTokens() {
        clearDoctor();
    }

    // ---------- Single-flight refresh ----------
    // Если несколько запросов одновременно словили 401 —
    // все ждут один и тот же промис, refresh делается один раз.
    var refreshPromise = null;

    function refreshAccess() {
        if (refreshPromise) return refreshPromise;

        // Не пытаемся refresh-ить, если вообще никто не залогинен.
        if (!isLoggedIn()) return Promise.resolve(false);

        refreshPromise = fetch('/api/auth/refresh', {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: '{}'
        }).then(function (res) {
            if (!res.ok) return false;
            return res.json().then(function (data) {
                // Сервер возвращает doctor в JSON — обновим localStorage.
                if (data && data.doctor) {
                    setDoctor(data.doctor);
                }
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
            extraHeaders || {}
        );
        var opts = {
            method: method,
            headers: headers,
            credentials: 'include'
        };
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
                        clearDoctor();
                        if (location.pathname.indexOf(LOGIN_URL) === -1) {
                            location.replace(LOGIN_URL);
                        }
                        return res; // отдаём оригинальный 401
                    }
                    // retry (тело не изменилось)
                    return fetch(url, buildOpts(method, rawBody, extraHeaders));
                });
            });
    }

    // ---------- logout ----------
    function logout() {
        var done = function () {
            clearDoctor();
            location.replace(LOGIN_URL);
        };

        // Fire-and-forget: не блокируем UI.
        // refresh-токен сервер возьмёт из cookie.
        try {
            fetch('/api/auth/logout', {
                method: 'POST',
                credentials: 'include',
                headers: { 'Content-Type': 'application/json' },
                body: '{}'
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
        if (!isLoggedIn()) {
            location.replace(LOGIN_URL);
        }
    }

    // ---------- export ----------
    global.BotMaxAuth = {
        // storage (только doctor)
        getDoctor:   getDoctor,
        setDoctor:   setDoctor,
        clearDoctor: clearDoctor,
        isLoggedIn:  isLoggedIn,
        clearTokens: clearTokens, // обратная совместимость

        // auth-flow
        refreshAccess: refreshAccess,
        authFetch:     authFetch,
        logout:        logout,
        requireAuth:   requireAuth,

        // роли
        ROLE:                ROLE,
        hasRole:             hasRole,
        isSuperadmin:        isSuperadmin,
        isAdmin:             isAdmin,
        isDoctor:            isDoctor,
        applyRoleVisibility: applyRoleVisibility
    };

})(window);
