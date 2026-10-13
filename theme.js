/**
 * theme.js — общий модуль тёмной/светлой темы.
 * Экспортирует window.BotMaxTheme (паттерн auth.js / pager.js).
 *
 * B29.6.5 — https://github.com/... (SNAPSHOTS.md §6.44)
 *
 * Ключ localStorage: 'theme'.
 * Значения: 'dark' | 'light'. Default: 'dark'.
 *
 * Использование:
 *   <script src="theme.js"></script>
 *   <script>BotMaxTheme.init({ button: '.header-top' });</script>
 *
 * API:
 *   init(opts)              — применить тему из localStorage; opts.button — куда
 *                             вставить кнопку переключения (селектор или Node);
 *                             opts.default — 'dark'|'light' для страниц, где
 *                             другая тема по умолчанию (например, light).
 *   toggle()                — dark ⇄ light.
 *   set(theme)              — явно 'dark' | 'light'.
 *   current()               — 'dark' | 'light'.
 *   mountButton(selector)   — вставить кнопку #themeToggle в контейнер.
 *   onChange(fn)            — подписка на смену темы; fn(theme, prev).
 */
(function () {
    'use strict';

    var KEY     = 'theme';
    var DEFAULT = 'dark';
    var VALID   = { dark: true, light: true };
    var BTN_ID  = 'themeToggle';

    var _subs = [];

    function _read() {
        try {
            var t = localStorage.getItem(KEY);
            return VALID[t] ? t : DEFAULT;
        } catch (e) {
            return DEFAULT;
        }
    }

    function _write(theme) {
        try { localStorage.setItem(KEY, theme); } catch (e) { /* ignore */ }
    }

    function _apply(theme) {
        var t = VALID[theme] ? theme : DEFAULT;
        document.documentElement.setAttribute('data-theme', t);
        return t;
    }

    function current() {
        return document.documentElement.getAttribute('data-theme') || _read();
    }

    function _notify(next, prev) {
        for (var i = 0; i < _subs.length; i++) {
            try { _subs[i](next, prev); } catch (e) { /* swallow */ }
        }
    }

    function _updateButton() {
        var b = document.getElementById(BTN_ID);
        if (!b) return;
        b.textContent = (current() === 'dark') ? '🌙' : '☀️';
        b.title = (current() === 'dark') ? 'Светлая тема' : 'Тёмная тема';
    }

    function set(theme) {
        var prev = current();
        var next = _apply(theme);
        _write(next);
        _updateButton();
        if (next !== prev) _notify(next, prev);
        return next;
    }

    function toggle() {
        return set(current() === 'dark' ? 'light' : 'dark');
    }

    function mountButton(selector) {
        var host = (typeof selector === 'string')
            ? document.querySelector(selector)
            : selector;
        if (!host) return null;

        var b = document.getElementById(BTN_ID);
        if (!b) {
            b = document.createElement('button');
            b.id = BTN_ID;
            b.type = 'button';
            b.style.cssText =
                'background:rgba(159,122,234,0.15);' +
                'border:1px solid rgba(159,122,234,0.4);' +
                'color:#9f7aea;' +
                'padding:10px 14px;' +
                'border-radius:10px;' +
                'cursor:pointer;' +
                'font-size:1em;' +
                'font-weight:600;' +
                'line-height:1;';
            b.addEventListener('click', toggle);
            host.appendChild(b);
        }
        _updateButton();
        return b;
    }

    function onChange(fn) {
        if (typeof fn === 'function') _subs.push(fn);
    }

    function init(opts) {
        opts = opts || {};
        // B29.6.5b: если в localStorage нет сохранённой темы,
        // используем opts.default (per-page). Иначе — глобальный DEFAULT.
        var saved = null;
        try { saved = localStorage.getItem(KEY); } catch (e) { /* ignore */ }
        if (!VALID[saved]) {
            saved = VALID[opts.default] ? opts.default : DEFAULT;
            // B29.6.12: зафиксировать дефолт в localStorage,
            // чтобы тема пережила закрытие браузера.
            _write(saved);
        }
        _apply(saved);
        if (opts.button) {
            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', function () {
                    mountButton(opts.button);
                });
            } else {
                mountButton(opts.button);
            }
        }
        return saved;
    }

    window.BotMaxTheme = {
        init:         init,
        toggle:       toggle,
        set:          set,
        current:      current,
        mountButton:  mountButton,
        onChange:     onChange
    };
})();
