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

    // B29.7.1: Chart.js — цвета осей/легенды/сетки под тему.
    // Canvas не понимает CSS var(), поэтому дублируем палитру в JS.
    var CHART_THEME = {
        dark: {
            text:      '#a0aec0',
            legend:    '#e2e8f0',
            grid:      'rgba(255,255,255,0.1)',
            tooltipBg: 'rgba(22,27,34,0.95)',
            tooltipFg: '#e2e8f0'
        },
        light: {
            text:      '#4a5568',
            legend:    '#1f2328',
            grid:      'rgba(31,35,40,0.12)',
            tooltipBg: 'rgba(255,255,255,0.98)',
            tooltipFg: '#1f2328'
        }
    };

    function getChartColors(theme) {
        var t = VALID[theme] ? theme : current();
        return CHART_THEME[t] || CHART_THEME.dark;
    }

    function applyChartTheme(chart, theme) {
        if (!chart || !chart.options) return;
        var c = getChartColors(theme);

        var scales = chart.options.scales || {};
        for (var k in scales) {
            if (!Object.prototype.hasOwnProperty.call(scales, k)) continue;
            var s = scales[k];
            if (s.ticks) s.ticks.color = c.text;
            if (s.grid)  s.grid.color  = c.grid;
            if (s.title) s.title.color = c.text;
        }

        var plugins = chart.options.plugins || {};
        if (plugins.legend && plugins.legend.labels) {
            plugins.legend.labels.color = c.legend;
        }
        if (plugins.tooltip) {
            plugins.tooltip.backgroundColor = c.tooltipBg;
            plugins.tooltip.titleColor      = c.tooltipFg;
            plugins.tooltip.bodyColor       = c.tooltipFg;
        }

        chart.update('none');
    }

    window.BotMaxTheme = {
        init:         init,
        toggle:       toggle,
        set:          set,
        current:      current,
        mountButton:  mountButton,
        onChange:     onChange,
        // B29.7.1: Chart.js под тему
        CHART_THEME:     CHART_THEME,
        getChartColors:  getChartColors,
        applyChartTheme: applyChartTheme
    };
})();
