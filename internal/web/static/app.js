// Client-side behaviour: the capture sheet, keyboard navigation, drag ordering
// and the small timers that make completing a task feel undoable.
//
// Deliberately thin. Nothing here parses shorthand -- that happens on the server
// so there is exactly one parser.

(() => {
  'use strict';

  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

  /* ── Capture sheet ─────────────────────────────────────────────────────── */

  const sheet = $('[data-capture]');
  const draft = $('[data-draft]');

  function openCapture() {
    if (!sheet) return;
    sheet.hidden = false;
    // Capture covers the whole page, so the list behind it must stop scrolling
    // or the wheel moves something invisible.
    document.body.style.overflow = 'hidden';
    restoreDraft();
    draft.focus();
    draft.setSelectionRange(draft.value.length, draft.value.length);
    if (draft.value.trim()) refreshPreview();
  }

  function closeCapture() {
    if (!sheet) return;
    sheet.hidden = true;
    document.body.style.overflow = '';
  }

  function refreshPreview() {
    if (window.htmx) window.htmx.trigger(draft, 'input');
  }

  // The draft is kept in the browser as you type. Closing the sheet by accident
  // mid-call, or reloading, must never cost you the notes you just took.
  const DRAFT = 'todo.draft';

  function saveDraft() {
    try {
      if (draft.value.trim()) localStorage.setItem(DRAFT, draft.value);
      else localStorage.removeItem(DRAFT);
    } catch {
      /* the draft simply will not survive a reload */
    }
  }

  function restoreDraft() {
    if (!draft || draft.value.trim()) return;
    try {
      const saved = localStorage.getItem(DRAFT);
      if (saved) draft.value = saved;
    } catch {
      /* nothing saved, or storage is unavailable */
    }
  }

  function clearDraft() {
    try {
      localStorage.removeItem(DRAFT);
    } catch {
      /* nothing to clear */
    }
  }

  draft?.addEventListener('input', saveDraft);
  restoreDraft();
  // Submitting is the one exit that means the notes are safely stored.
  $('[data-capture-form]')?.addEventListener('submit', clearDraft);

  $$('[data-capture-toggle]').forEach((el) =>
    el.addEventListener('click', () => (sheet.hidden ? openCapture() : closeCapture())));

  if (document.body.dataset.captureOpen) openCapture();

  // Clicking the veil, but not the sheet itself, dismisses.
  sheet?.addEventListener('mousedown', (e) => {
    if (e.target === sheet) closeCapture();
  });

  draft?.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      $('[data-capture-form]').requestSubmit();
    }
  });

  // "Make a task" on a skipped line: give it a topic and re-read the draft.
  document.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-promote]');
    if (!btn) return;
    const n = Number(btn.dataset.promote);
    const lines = draft.value.split('\n');
    if (!lines[n - 1]) return;
    lines[n - 1] = 'inbox | ' + lines[n - 1].trim();
    draft.value = lines.join('\n');
    refreshPreview();
    draft.focus();
  });

  /* ── Settings: appearance and colour ───────────────────────────────────── */

  const settings = $('[data-settings]');

  function toggleSettings(force) {
    if (!settings) return;
    settings.hidden = force !== undefined ? force : !settings.hidden;
    if (!settings.hidden) syncSettings();
  }

  function store(key, value) {
    try {
      if (value === null) localStorage.removeItem(key);
      else localStorage.setItem(key, value);
    } catch {
      /* the choice just will not survive a reload */
    }
  }

  function syncSettings() {
    const theme = document.documentElement.dataset.theme || 'system';
    $$('[data-theme-set]').forEach((b) =>
      b.setAttribute('aria-pressed', String(b.dataset.themeSet === theme)));
  }

  document.addEventListener('click', (e) => {
    const pal = e.target.closest('[data-palette-set]');
    if (pal) {
      document.documentElement.dataset.palette = pal.dataset.paletteSet;
      store('todo.palette', pal.dataset.paletteSet);
      return;
    }
    const theme = e.target.closest('[data-theme-set]');
    if (theme) {
      const val = theme.dataset.themeSet;
      if (val === 'system') delete document.documentElement.dataset.theme;
      else document.documentElement.dataset.theme = val;
      store('todo.theme', val === 'system' ? null : val);
      syncSettings();
      return;
    }
    if (e.target.closest('[data-settings-toggle]')) toggleSettings();
    else if (!settings?.hidden && !e.target.closest('.settings-card')) toggleSettings(true);
  });
  syncSettings();

  /* ── Sidebar ───────────────────────────────────────────────────────────── */

  function applyRail(on) {
    document.body.classList.toggle('is-rail', on);
    store('todo.rail', on ? '1' : null);
  }

  $$('[data-sidebar-toggle]').forEach((b) =>
    b.addEventListener('click', () => applyRail(!document.body.classList.contains('is-rail'))));

  try {
    if (localStorage.getItem('todo.rail')) document.body.classList.add('is-rail');
  } catch {
    /* nothing stored */
  }

  /* ── Keyboard reference ────────────────────────────────────────────────── */

  const keymap = $('[data-keymap]');

  function toggleKeymap(force) {
    if (!keymap) return;
    keymap.hidden = force !== undefined ? force : !keymap.hidden;
  }

  document.addEventListener('click', (e) => {
    if (e.target.closest('[data-keymap-toggle]')) toggleKeymap();
    else if (!keymap?.hidden && !e.target.closest('.keymap-card')) toggleKeymap(true);
  });

  /* ── Detail panel ──────────────────────────────────────────────────────── */

  const detail = $('#detail');

  function closeDetail() {
    if (detail) detail.innerHTML = '';
  }

  document.addEventListener('click', (e) => {
    if (e.target.closest('[data-close-detail]')) closeDetail();
    else if (detail?.firstElementChild && !e.target.closest('[data-detail-panel]') &&
             !e.target.closest('[data-open-detail]')) closeDetail();
  });

  // The panel closes once its request has actually landed, not before. htmx
  // raises the event from the form that issued the request, not the button that
  // was clicked, so match on the panel and on anything that is not a plain read.
  document.body.addEventListener('htmx:afterRequest', (e) => {
    if (!e.detail.successful) return;
    const verb = e.detail.requestConfig?.verb;
    if (verb && verb !== 'get' && e.target.closest('[data-detail-panel]')) closeDetail();
  });

  /* ── Completed rows ────────────────────────────────────────────────────── */

  // A task that no longer belongs in this view stays put, struck through, for a
  // moment before folding away -- long enough to click it again by mistake.
  const LINGER = 2200;

  function scheduleFold(root = document) {
    $$('[data-leaving]', root).forEach((row) => {
      if (row.dataset.folding) return;
      row.dataset.folding = '1';
      setTimeout(() => row.classList.add('is-leaving'), LINGER);
    });
  }

  document.body.addEventListener('htmx:afterSwap', (e) => {
    scheduleFold(e.detail.target.parentElement || document);
    hideToastsIn(e.detail.target);
    syncAddButton();
  });

  // "Add 4 tasks" beats "Add tasks" when you are about to commit a wall of text.
  function syncAddButton() {
    const btn = $('[data-add-button]');
    const n = $$('#preview .pv-task').length;
    if (!btn) return;
    btn.textContent = n === 0 ? 'Add tasks' : `Add ${n} task${n === 1 ? '' : 's'}`;
    btn.disabled = n === 0;
  }
  syncAddButton();

  /* ── Toasts ────────────────────────────────────────────────────────────── */

  function hideToastsIn(root) {
    $$('[data-autohide]', root === document ? document : root).forEach((t) => {
      if (t.dataset.timed) return;
      t.dataset.timed = '1';
      setTimeout(() => {
        t.classList.add('is-going');
        t.addEventListener('animationend', () => t.remove(), { once: true });
      }, 9000);
    });
  }
  hideToastsIn(document);

  /* ── Sort menu ─────────────────────────────────────────────────────────── */

  $('[data-sort-select]')?.addEventListener('change', (e) => e.target.form.requestSubmit());
  $$('[data-week-filter]').forEach((sel) =>
    sel.addEventListener('change', (e) => e.target.form.requestSubmit()));

  /* ── Search ────────────────────────────────────────────────────────────── */

  // Collapsed to an icon until wanted, and left open whenever a query is
  // actually in force so you can see what is filtering the list.
  const searchWrap = $('[data-search-wrap]');
  const searchInput = $('[data-search]');

  function openSearch() {
    searchWrap?.classList.add('is-open');
    searchInput?.focus();
    searchInput?.select();
  }

  function closeSearchIfEmpty() {
    if (searchWrap && !searchInput.value.trim()) searchWrap.classList.remove('is-open');
  }

  $('[data-search-open]')?.addEventListener('click', () => {
    if (searchWrap.classList.contains('is-open')) closeSearchIfEmpty();
    else openSearch();
  });
  searchInput?.addEventListener('blur', closeSearchIfEmpty);

  /* ── Collapsible sidebar groups ────────────────────────────────────────── */

  // Which groups you keep open is a per-browser preference, so it lives in
  // localStorage. Every access is guarded: private windows can throw outright.
  const COLLAPSED = 'todo.collapsed';

  function readCollapsed() {
    try {
      return new Set(JSON.parse(localStorage.getItem(COLLAPSED) || '[]'));
    } catch {
      return new Set();
    }
  }

  function writeCollapsed(set) {
    try {
      localStorage.setItem(COLLAPSED, JSON.stringify([...set]));
    } catch {
      /* nothing to do: the preference simply will not persist */
    }
  }

  function applyCollapsed() {
    const collapsed = readCollapsed();
    $$('[data-section]').forEach((nav) => {
      const off = collapsed.has(nav.dataset.section);
      nav.classList.toggle('is-collapsed', off);
      nav.querySelector('[data-collapse]')?.setAttribute('aria-expanded', String(!off));
    });
  }

  document.addEventListener('click', (e) => {
    const head = e.target.closest('[data-collapse]');
    if (!head) return;
    const nav = head.closest('[data-section]');
    const collapsed = readCollapsed();
    if (collapsed.has(nav.dataset.section)) collapsed.delete(nav.dataset.section);
    else collapsed.add(nav.dataset.section);
    writeCollapsed(collapsed);
    applyCollapsed();
  });

  applyCollapsed();
  // The sidebar is replaced wholesale when counts refresh out of band.
  document.body.addEventListener('htmx:afterSwap', applyCollapsed);

  /* ── Drag ordering ─────────────────────────────────────────────────────── */

  // Manual order is the only order you can drag: the other sorts are read-only
  // views onto it, so dragging in them would have nothing to mean.
  if (window.Sortable && $('[data-sortable="on"]') && !$('[data-week]')) {
    $$('[data-list]').forEach((list) => {
      window.Sortable.create(list, {
        animation: 140,
        handle: '[data-grip]',
        draggable: '.task',
        ghostClass: 'sortable-ghost',
        dragClass: 'sortable-drag',
        onEnd(evt) {
          const row = evt.item;
          const body = new URLSearchParams({
            above: row.previousElementSibling?.dataset.id ?? '0',
            below: row.nextElementSibling?.dataset.id ?? '0',
          });
          fetch(`/tasks/${row.dataset.id}/move`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body,
          }).catch(() => location.reload());
        },
      });
    });
  }

  /* ── Week planner ──────────────────────────────────────────────────────── */

  // Dragging a card between columns is the same act as setting a due date, so
  // planning a week and rescheduling are one gesture rather than two features.
  if (window.Sortable && $('[data-week]')) {
    $$('[data-drop]').forEach((list) => {
      window.Sortable.create(list, {
        group: 'week',
        animation: 140,
        draggable: '.card',
        ghostClass: 'sortable-ghost',
        onStart() {
          $$('[data-drop]').forEach((l) => l.classList.add('is-target'));
        },
        onEnd(evt) {
          $$('[data-drop]').forEach((l) => l.classList.remove('is-target', 'is-over'));
          const dest = evt.to;
          if (dest === evt.from && dest.dataset.date === evt.from.dataset.date) return;

          const body = new URLSearchParams({ date: dest.dataset.date ?? '' });
          fetch(`/tasks/${evt.item.dataset.id}/schedule`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body,
          }).then((r) => {
            if (!r.ok) throw new Error('schedule failed');
            // Counts and the empty-tray placeholders both go stale on a drop.
            location.reload();
          }).catch(() => location.reload());
        },
        onMove(evt) {
          $$('[data-drop]').forEach((l) => l.classList.remove('is-over'));
          evt.to.classList.add('is-over');
        },
      });
    });
  }

  /* ── Keyboard ──────────────────────────────────────────────────────────── */

  let cursor = -1;
  let chord = null;

  const rows = () => $$('.task:not(.is-leaving)');

  function moveCursor(delta) {
    const list = rows();
    if (!list.length) return;
    list.forEach((r) => r.classList.remove('is-cursor'));
    cursor = Math.max(0, Math.min(list.length - 1, cursor + delta));
    const row = list[cursor];
    row.classList.add('is-cursor');
    row.focus({ preventScroll: true });
    row.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }

  function current() {
    return rows()[cursor] ?? null;
  }

  const typing = (el) =>
    el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' ||
           el.isContentEditable);

  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      if (settings && !settings.hidden) return toggleSettings(true);
      if (keymap && !keymap.hidden) return toggleKeymap(true);
      if (sheet && !sheet.hidden) return closeCapture();
      if (detail?.firstElementChild) return closeDetail();
      if (typing(document.activeElement)) document.activeElement.blur();
      return;
    }
    if (typing(e.target) || e.metaKey || e.ctrlKey || e.altKey) return;

    // g is a prefix: g t, g u, g a, g l jump between views.
    if (chord === 'g') {
      chord = null;
      const view = { t: 'today', u: 'upcoming', a: 'anytime', d: 'delegated', l: 'logbook' }[e.key];
      if (view) {
        e.preventDefault();
        location.href = '/view/' + view;
      }
      return;
    }

    switch (e.key) {
      case '?':
        e.preventDefault();
        toggleKeymap();
        break;
      case 'a':
        e.preventDefault();
        location.href = '/view/all';
        break;
      case 'w':
        e.preventDefault();
        location.href = '/week';
        break;
      case '[':
        e.preventDefault();
        applyRail(!document.body.classList.contains('is-rail'));
        break;
      case 'g':
        chord = 'g';
        setTimeout(() => (chord = null), 900);
        break;
      case 'n':
        e.preventDefault();
        openCapture();
        break;
      case 'j':
        e.preventDefault();
        moveCursor(cursor < 0 ? 1 - cursor - 1 : 1);
        break;
      case 'k':
        e.preventDefault();
        moveCursor(-1);
        break;
      case 'x':
        e.preventDefault();
        current()?.querySelector('.check')?.click();
        break;
      case 'e':
        e.preventDefault();
        current()?.querySelector('[data-open-detail]')?.click();
        break;
      case '/':
        e.preventDefault();
        openSearch();
        break;
      case 'u':
        e.preventDefault();
        $('.toast-action')?.click();
        break;
    }
  });

  scheduleFold();
})();
