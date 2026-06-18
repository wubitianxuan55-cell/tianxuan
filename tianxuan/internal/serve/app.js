// tianxuan HTTP client — zero-dependency browser chat frontend
(() => {
  const log = document.getElementById('log');
  const input = document.getElementById('in');
  const approval = document.getElementById('approval');
  const statusEl = document.getElementById('status');
  let asst = null;
  let thinkingEl = null;  // current <details> thinking block
  let thinkingText = '';
  let toolStart = 0;      // performance.now of the last ToolDispatch

  // ── helpers ──────────────────────────────────────────────
  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }
  function line(cls, text) {
    const e = el('div', cls, text);
    log.appendChild(e);
    log.scrollTop = log.scrollHeight;
    return e;
  }
  function post(path, body) {
    return fetch(path, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  // ── status line ──────────────────────────────────────────
  function ctxColor(pct) {
    if (pct < 50) return '#16a34a';
    if (pct < 80) return '#ca8a04';
    return '#dc2626';
  }
  function formatBytes(b) { return b >= 1000 ? (b / 1000).toFixed(1) + 'k' : b; }

  async function refreshContext() {
    try {
      const r = await fetch('/context');
      if (!r.ok) return;
      const ctx = await r.json();
      const pct = ctx.percent || 0;
      const fill = `<span class="ctx-fill" style="width:${Math.min(pct,100)}%;background:${ctxColor(pct)}"></span>`;
      if (statusEl.children.length === 0) {
        statusEl.innerHTML =
          `<span class="metric" id="s-ctx">ctx <span class="ctx-gauge">${fill}</span> ${pct}%</span>` +
          `<span class="metric" id="s-cost"></span>` +
          `<span class="metric" id="s-cache"></span>`;
      } else {
        const g = document.getElementById('s-ctx');
        if (g) g.innerHTML = `ctx <span class="ctx-gauge">${fill}</span> ${pct}%`;
      }
    } catch (_) {}
  }

  // ── send ─────────────────────────────────────────────────
  function send() {
    const v = input.value.trim();
    if (!v) return;
    line('user', '› ' + v);
    input.value = '';
    post('/submit', { input: v });
  }

  input.addEventListener('keydown', e => {
    if (e.key === 'Enter') { e.preventDefault(); send(); }
  });

  // ── approval ─────────────────────────────────────────────
  function showApproval(a) {
    approval.style.display = 'block';
    approval.innerHTML =
      `approve <b>${a.tool}</b> — ${a.subject || ''}?  ` +
      `<kbd>y</kbd> allow once  <kbd>a</kbd> allow session  <kbd>n</kbd> deny`;
    const answer = (allow, session) => {
      post('/approve', { id: a.id, allow, session });
      approval.style.display = 'none';
      document.removeEventListener('keydown', onkey);
    };
    const onkey = e => {
      if (e.key === '1' || e.key === 'y') answer(true, false);
      else if (e.key === '2' || e.key === 'a') answer(true, true);
      else if (e.key === '3' || e.key === 'n') answer(false, false);
    };
    document.addEventListener('keydown', onkey);
  }

  // ── flush thinking block ─────────────────────────────────
  function flushThinking() {
    if (!thinkingEl && thinkingText) {
      const d = el('details', 'thinking');
      d.innerHTML = `<summary>💭 thought for …</summary>${thinkingText}`;
      log.appendChild(d);
      thinkingEl = d;
    }
    if (thinkingEl && thinkingText) {
      thinkingEl.innerHTML = `<summary>💭 thought for …</summary>${thinkingText}`;
    }
    log.scrollTop = log.scrollHeight;
  }

  // ── SSE connection with exponential backoff ──────────────
  let retryMs = 1000;
  const MAX_RETRY = 30000;

  function connectSSE() {
    const es = new EventSource('/events');
    let connected = false;

    es.onopen = () => {
      if (!connected) connected = true;
      retryMs = 1000; // reset on successful connect
      refreshContext();
    };

    es.onmessage = ev => {
      const e = JSON.parse(ev.data);
      switch (e.kind) {
        case 'reasoning': {
          if (!thinkingEl) { thinkingText = ''; thinkingEl = null; }
          thinkingText += e.text;
          flushThinking();
          break;
        }
        case 'text': {
          flushThinking();
          thinkingEl = null; thinkingText = '';
          if (!asst || asst.dataset.r) { asst = line('', ''); }
          asst.textContent += e.text;
          break;
        }
        case 'message': {
          asst = null;
          flushThinking();
          break;
        }
        case 'tool_dispatch': {
          asst = null;
          flushThinking();
          toolStart = performance.now();
          line('tool', '→ ' + e.tool.name + ' ' + (e.tool.args || ''));
          break;
        }
        case 'tool_result': {
          const ms = toolStart ? Math.round(performance.now() - toolStart) : 0;
          const dur = ms ? ` <span class="tool-sep">${ms}ms</span>` : '';
          if (e.tool && e.tool.err) {
            line('tool-err', '⊘ ' + e.tool.name + ' ' + e.tool.err + dur);
          } else {
            line('tool-ok', '✓ ' + e.tool.name + dur);
          }
          break;
        }
        case 'usage': {
          if (e.usage) {
            const cost = e.usage.costUsd ? ` · ¥${(e.usage.costUsd * 7.25).toFixed(4)}` : '';
            line('usage', `<span>${e.usage.totalTokens} tok</span><span>in ${e.usage.promptTokens} (${e.usage.cacheHitTokens} cached)</span><span>out ${e.usage.completionTokens}</span>${cost}`);
            const sc = document.getElementById('s-cost');
            if (sc && e.usage.costUsd) sc.textContent = `¥${(e.usage.costUsd * 7.25).toFixed(4)}`;
            const sh = e.usage.cacheHitTokens + e.usage.cacheMissTokens;
            const sr = document.getElementById('s-cache');
            if (sr && sh > 0) {
              const pct = Math.round(e.usage.cacheHitTokens / sh * 100);
              sr.textContent = `⫸ ${pct}%`;
            }
          }
          refreshContext();
          break;
        }
        case 'notice': {
          line(e.level === 'warn' ? 'err' : 'notice', (e.level === 'warn' ? '! ' : '· ') + e.text);
          break;
        }
        case 'phase': {
          asst = null;
          line('notice', '[' + e.text + ']');
          break;
        }
        case 'approval_request': {
          showApproval(e.approval);
          break;
        }
        case 'turn_done': {
          asst = null;
          flushThinking();
          if (e.err) line('err', '! ' + e.err);
          refreshContext();
          break;
        }
      }
    };

    es.onerror = () => {
      es.close();
      if (connected) line('err', '· disconnected — reconnecting…');
      setTimeout(connectSSE, retryMs);
      retryMs = Math.min(retryMs * 2, MAX_RETRY);
    };
  }

  // ── history load ─────────────────────────────────────────
  fetch('/history').then(r => r.json()).then(ms => {
    (ms || []).forEach(m => {
      if (m.content) line(m.role === 'user' ? 'user' : '',
        (m.role === 'user' ? '› ' : '') + m.content);
    });
  });

  // ── go ───────────────────────────────────────────────────
  connectSSE();
  refreshContext();
})();
