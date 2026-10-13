/* ============================================================
 * BOT_MAX — pager.js
 * Шаг 6.42 · 2026-10-13 · B29.5 (унификация пагинации)
 *
 * Общий клиентский модуль пагинации для /api/encounters.
 * Экспортирует window.BotMaxPager.
 *
 * Зачем: до B29.5 каждый HTML-файл (patient_card.html,
 * doctor_cabinet.html) держал свою копию логики limit/offset/
 * total и свой renderPager. После регресса default limit
 * 100→20 (B29.1) doctor_cabinet отдавал неверный срез — этот
 * модуль делает логику единым источником правды.
 *
 * Контракт /api/encounters (ответ):
 *   { items: [...], limit: N, offset: N, count: N, total?: N }
 *
 * Контракт /api/encounters (query):
 *   patient_id | doctor_id | clinic_id  — хотя бы один (скоуп)
 *   kind, status, specialty, from, to   — фильтры
 *   limit  ∈ [1..200], default 20
 *   offset ∈ [0..10000], default 0
 *
 * Публичный API (window.BotMaxPager):
 *   create(opts)              -> { reload, reset, next, prev, fetch, getState, setOffset }
 *   buildQuery(params)        -> '?a=1&b=2'  (sorte keys, skip ''/null/undefined)
 *   computePagerMeta(state)   -> { from, to, total, canPrev, canNext, label }
 *   readFiltersFromURL(keys)  -> { key: value, ... }
 *   syncFiltersToURL(f, keys, extra)
 *
 * Зависимости: window.BotMaxAuth.authFetch (обязательно).
 * ============================================================ */

(function (global) {
    'use strict';

    // ---------- Константы ----------
    var DEFAULT_LIMIT = 20;
    var PAGER_ID      = 'encountersPager';

    var BTN_STYLE_ON  = 'padding:6px 14px;border-radius:20px;border:none;cursor:pointer;font-weight:600;'
                      + 'background:linear-gradient(135deg,#667eea,#764ba2);color:white;';
    var BTN_STYLE_OFF = 'padding:6px 14px;border-radius:20px;border:none;cursor:not-allowed;font-weight:600;'
                      + 'background:rgba(255,255,255,0.08);color:#718096;';

    // ---------- Утилиты ----------

    // buildQuery — сериализация в query-string.
    // Пропускает: '' , null, undefined. Сортирует ключи (детерминизм).
    // Значения — через encodeURIComponent.
    function buildQuery(params) {
        if (!params) return '';
        var keys = Object.keys(params).filter(function (k) {
            var v = params[k];
            return v !== '' && v !== null && v !== undefined;
        }).sort();
        if (!keys.length) return '';
        var parts = [];
        for (var i = 0; i < keys.length; i++) {
            var k = keys[i];
            parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(String(params[k])));
        }
        return '?' + parts.join('&');
    }

    // computePagerMeta — из {offset, limit, total, count} строит
    // метаданные для рендера и текст-подпись.
    // total === null  -> keyset-режим (без COUNT), пишем «стр. K».
    function computePagerMeta(s) {
        s = s || {};
        var offset = s.offset || 0;
        var limit  = s.limit  || DEFAULT_LIMIT;
        var total  = (typeof s.total === 'number') ? s.total : null;

        var canPrev = offset > 0;
        var canNext;
        if (total !== null) {
            canNext = (offset + limit) < total;
        } else {
            // keyset/неизвестный total: кнопка «Вперёд» доступна,
            // пока текущая страница непустая.
            canNext = (s.count || 0) >= limit;
        }

        var label;
        if (total !== null) {
            var from = total === 0 ? 0 : (offset + 1);
            var to   = Math.min(offset + limit, total);
            label = from + '–' + to + ' из ' + total;
        } else if (offset === 0 && (s.count || 0) === 0) {
            label = '';
        } else {
            label = 'стр. ' + (Math.floor(offset / limit) + 1);
        }

        return {
            from:    offset + 1,
            to:      offset + limit,
            total:   total,
            canPrev: canPrev,
            canNext: canNext,
            label:   label
        };
    }

    // readFiltersFromURL — читает только указанные ключи из location.search.
    // Возвращает объект; отсутствующие ключи не попадают.
    function readFiltersFromURL(keys) {
        var out = {};
        if (typeof location === 'undefined' || !location.search) return out;
        var params;
        try {
            params = new URLSearchParams(location.search);
        } catch (e) {
            return out;
        }
        keys = keys || [];
        for (var i = 0; i < keys.length; i++) {
            var k = keys[i];
            var v = params.get(k);
            if (v !== null && v !== '') out[k] = v;
        }
        return out;
    }

    // syncFiltersToURL — обновляет URL (replaceState) набором фильтров.
    // extra — дополнительные пары (например {offset: 20}).
    // offset === 0 не пишется в URL (чтобы не мусорить).
    function syncFiltersToURL(filters, keys, extra) {
        if (typeof history === 'undefined' || !history.replaceState) return;
        if (typeof location === 'undefined') return;

        var params = new URLSearchParams(location.search);
        keys = keys || [];

        for (var i = 0; i < keys.length; i++) {
            var k = keys[i];
            var v = filters ? filters[k] : undefined;
            if (v === '' || v === null || v === undefined) {
                params.delete(k);
            } else {
                params.set(k, String(v));
            }
        }

        if (extra) {
            Object.keys(extra).forEach(function (ek) {
                var ev = extra[ek];
                if (ek === 'offset' && (ev === 0 || ev === '0')) {
                    params.delete(ek);
                } else if (ev === '' || ev === null || ev === undefined) {
                    params.delete(ek);
                } else {
                    params.set(ek, String(ev));
                }
            });
        }

        var qs = params.toString();
        var url = location.pathname + (qs ? ('?' + qs) : '') + (location.hash || '');
        try {
            history.replaceState(null, '', url);
        } catch (e) { /* ignore (file://, Safari old) */ }
    }

    // ---------- Фабрика ----------

    // create(opts) — создаёт контроллер пагинации.
    //
    // opts:
    //   endpoint    — '/api/encounters' (default)
    //   fixedParams — { doctor_id } | { patient_id } | ... (неизменные)
    //   getFilters  — function() -> { kind, status, specialty, from, to, limit }
    //   onItems     — function(items, meta) -> void   meta = { total, offset, limit, count }
    //   onError     — function(err) -> void
    //   pagerEl     — HTMLElement | string-id  (default: 'encountersPager')
    //   urlSync     — bool  (default: true)
    //   urlKeys     — array строк для URL-синхронизации (default: общий набор)
    //   onAfterFetch— function() -> void   (для обновления счётчиков вне пагинатора)
    function create(opts) {
        opts = opts || {};
        var endpoint    = opts.endpoint || '/api/encounters';
        var fixedParams = opts.fixedParams || {};
        var getFilters  = opts.getFilters  || function () { return {}; };
        var onItems     = opts.onItems     || function () {};
        var onError     = opts.onError     || function () {};
        var onAfterFetch= opts.onAfterFetch|| function () {};
        var urlSync     = opts.urlSync !== false;
        var urlKeys     = opts.urlKeys || ['kind', 'status', 'specialty', 'from', 'to', 'limit'];

        var pagerEl = (typeof opts.pagerEl === 'string')
            ? document.getElementById(opts.pagerEl)
            : (opts.pagerEl || document.getElementById(PAGER_ID));

        var state = {
            offset:  0,
            limit:   DEFAULT_LIMIT,
            total:   null,
            count:   0,
            loading: false
        };

        function currentLimit() {
            var f = getFilters() || {};
            var n = Number(f.limit);
            if (!n || n < 1) n = DEFAULT_LIMIT;
            return n;
        }

        function fetchPage() {
            if (state.loading) return Promise.resolve();
            if (!global.BotMaxAuth || typeof global.BotMaxAuth.authFetch !== 'function') {
                onError(new Error('BotMaxAuth.authFetch недоступен (pager.js должен подключаться после auth.js)'));
                return Promise.resolve();
            }

            state.loading = true;
            var filters = getFilters() || {};
            var limit   = currentLimit();
            state.limit = limit;

            var merged = {};
            Object.keys(fixedParams).forEach(function (k) { merged[k] = fixedParams[k]; });
            Object.keys(filters).forEach(function (k) { merged[k] = filters[k]; });
            merged.limit  = limit;
            merged.offset = state.offset;

            var url = endpoint + buildQuery(merged);

            return global.BotMaxAuth.authFetch('GET', url)
                .then(function (res) {
                    if (!res.ok) {
                        return res.text().then(function (t) {
                            var msg = 'HTTP ' + res.status;
                            try { var j = JSON.parse(t); if (j && j.error) msg = j.error; } catch (e) {}
                            throw new Error(msg);
                        });
                    }
                    return res.json();
                })
                .then(function (data) {
                    var items = (data && data.items) || [];
                    state.total = (typeof data.total === 'number') ? data.total : null;
                    state.count = items.length;

                    if (urlSync) syncFiltersToURL(filters, urlKeys, { offset: state.offset });

                    var meta = {
                        total:  state.total,
                        offset: state.offset,
                        limit:  state.limit,
                        count:  state.count
                    };
                    onItems(items, meta);
                    renderPager();
                    onAfterFetch(meta);
                })
                .catch(function (err) {
                    onError(err);
                })
                .then(function () {
                    state.loading = false;
                });
        }

        function renderPager() {
            if (!pagerEl) return;

            var meta = computePagerMeta(state);
            var hasAnyNav = meta.canPrev || meta.canNext;

            // Скрываем пагинатор, если нет навигации и нечего показывать.
            if (!hasAnyNav && state.offset === 0) {
                pagerEl.innerHTML = '';
                return;
            }

            var html = '';
            html += '<button id="encPagePrev" type="button" '
                 +  (meta.canPrev ? 'style="' + BTN_STYLE_ON + '"' : 'disabled style="' + BTN_STYLE_OFF + '"')
                 +  '>← Назад</button>';
            if (meta.label) {
                html += '<span style="color:#a0aec0;font-size:0.9em;">' + meta.label + '</span>';
            }
            html += '<button id="encPageNext" type="button" '
                 +  (meta.canNext ? 'style="' + BTN_STYLE_ON + '"' : 'disabled style="' + BTN_STYLE_OFF + '"')
                 +  '>Вперёд →</button>';

            pagerEl.innerHTML = html;

            var prev = document.getElementById('encPagePrev');
            var next = document.getElementById('encPageNext');
            if (prev && meta.canPrev) {
                prev.addEventListener('click', function () {
                    state.offset = Math.max(0, state.offset - state.limit);
                    fetchPage();
                });
            }
            if (next && meta.canNext) {
                next.addEventListener('click', function () {
                    state.offset = state.offset + state.limit;
                    fetchPage();
                });
            }
        }

        function reload()   { state.offset = 0; return fetchPage(); }
        function reset()    { state.offset = 0; return fetchPage(); }
        function next()     { state.offset = state.offset + state.limit; return fetchPage(); }
        function prev()     { state.offset = Math.max(0, state.offset - state.limit); return fetchPage(); }
        function setOffset(o) { state.offset = Math.max(0, Number(o) || 0); return fetchPage(); }
        function getState() { return { offset: state.offset, limit: state.limit, total: state.total, count: state.count }; }
        function setLimit(n) {
            var v = Number(n) || DEFAULT_LIMIT;
            if (v < 1) v = DEFAULT_LIMIT;
            state.limit = v;
            state.offset = 0;
            return fetchPage();
        }

        return {
            reload:    reload,
            reset:     reset,
            fetch:     fetchPage,
            next:      next,
            prev:      prev,
            setOffset: setOffset,
            setLimit:  setLimit,
            getState:  getState,
            render:    renderPager
        };
    }

    // ---------- export ----------
    global.BotMaxPager = {
        create:             create,
        buildQuery:         buildQuery,
        computePagerMeta:   computePagerMeta,
        readFiltersFromURL: readFiltersFromURL,
        syncFiltersToURL:   syncFiltersToURL,
        DEFAULT_LIMIT:      DEFAULT_LIMIT
    };

})(window);
