(function (global) {
    let state;
    let DOM;
    let fetchData;

    function init(context) {
        state = context.state;
        DOM = context.DOM;
        fetchData = context.fetchData;
    }

    function showLogsModal() {
  if (!state.currentRunData) return;
  const runId = state.currentRunData.run_info.run_id;
  const isComplete = state.currentRunData.run_info.is_complete;

  document.getElementById('logs-modal-title').textContent = `Agent Logs for run ${runId.slice(0, 8)}`;
  DOM.logsContainer.innerHTML = `<p class="text-[var(--text-secondary)]">Loading logs...</p>`;

  // Open modal animation
  DOM.logsModal.classList.remove('hidden');
  setTimeout(() => {
DOM.logsModal.classList.add('opacity-100');
DOM.logsModalContent.classList.remove('scale-95');
  }, 10);

  // Build filters + bind controls
  try { buildLogsFilters(); } catch {}
  try { initLogsUIControls(); } catch {}

  fetchAndRenderLogs(runId);
  if (!isComplete) {
if (state.logPollingInterval) clearInterval(state.logPollingInterval);
state.logPollingInterval = setInterval(() => fetchAndRenderLogs(runId), 3000);
  }
}

function initLogsUIControls() {
  const wrap = document.getElementById('logs-toggle-wrap');
  const ts = document.getElementById('logs-toggle-ts');
  const structured = document.getElementById('logs-toggle-structured');

  if (wrap) {
wrap.checked = !!state.logsWrap;
wrap.addEventListener('change', () => {
  state.logsWrap = !!wrap.checked;
  renderLogsWithFilters();
});
  }
  if (ts) {
ts.checked = !!state.logsShowTimestamps;
ts.addEventListener('change', () => {
  state.logsShowTimestamps = !!ts.checked;
  renderLogsWithFilters();
});
  }
  if (structured) {
structured.checked = !!state.logsStructured;
structured.addEventListener('change', () => {
  state.logsStructured = !!structured.checked;
  renderLogsWithFilters();
});
  }

  // Level chips
  document.querySelectorAll('[data-level-chip]').forEach(btn => {
const lvl = btn.getAttribute('data-level-chip');
const activate = () => btn.classList.add('ring-1','ring-[var(--border-accent)]','text-[var(--text-primary)]');
const deactivate = () => btn.classList.remove('ring-1','ring-[var(--border-accent)]','text-[var(--text-primary)]');

if (state.logsLevelFilter.has(lvl)) activate(); else deactivate();

btn.addEventListener('click', () => {
  if (state.logsLevelFilter.has(lvl)) state.logsLevelFilter.delete(lvl);
  else state.logsLevelFilter.add(lvl);
  // visual state
  if (state.logsLevelFilter.has(lvl)) activate(); else deactivate();
  state._logsFocusFirstMatch = true;
  renderLogsWithFilters();
});
  });
}

    function closeLogsModal() {
        if (state.logPollingInterval) clearInterval(state.logPollingInterval);
        state.logPollingInterval = null;

        DOM.logsModal.classList.remove('opacity-100');
        DOM.logsModalContent.classList.add('scale-95');
        setTimeout(() => DOM.logsModal.classList.add('hidden'), 300);
    }

    async function fetchAndRenderLogs(runId) {
        const logs = await fetchData(`/v1/runs/${runId}/logs`);
        if (logs && logs.length > 0) {
            state._logsRaw = logs;
            renderLogsWithFilters();
        } else if (DOM.logsContainer.innerHTML.includes('Loading')) {
            DOM.logsContainer.innerHTML = `<p class="text-[var(--text-secondary)]">No logs yet...</p>`;
        }
    }

    function getLogStepName(log) {
        return log.step || log.step_name || log.stepName || (log.meta && (log.meta.step || log.meta.step_name)) || null;
    }

    function buildLogsFilters() {
        state.logsAllSteps = (state.currentRunData?.steps || []).map(s => s.name);
        updateLogsStepList();
    }

    function updateLogsStepList() {
        const list = DOM.logsStepList; if (!list) return;
        const q = (DOM.logsStepSearch?.value || '').toLowerCase();
        const steps = (state.logsAllSteps || []).filter(n => !q || n.toLowerCase().includes(q));
        const selectedOrder = Array.from(state.logsSelectedSteps || []);
        list.innerHTML = steps.map(name => {
          const isSelected = state.logsSelectedSteps.has(name);
          const idx = isSelected ? (selectedOrder.indexOf(name) % 8) : -1;
          const selClass = isSelected ? (` is-active c${idx}`) : '';
          const dot = isSelected ? (`<span class="step-dot c${idx}"></span>`) : '';
          return `
            <button type="button" class="logs-step-item w-full text-left px-3 py-1.5 rounded-md border border-[var(--border-primary)] bg-[var(--bg-primary)] hover:bg-[var(--bg-tertiary)] text-xs${selClass}" data-step="${name}">
              <span class="inline-flex items-center gap-2 truncate"><span>${dot}</span><span class="truncate">${name}</span></span>
            </button>`;
        }).join('');
    }

function parseKVLine(raw) {
  // Robust key=value parser.
  // - Keys: [A-Za-z0-9_-]+
  // - Values: either unquoted (no whitespace) or double-quoted strings
  //   that may span multiple lines and include escaped quotes \" and escapes \\ etc.
  // Returns { hasKV: boolean, kv: object }
  const kv = {};
  let i = 0;
  const n = raw.length;

  const isSpace = ch => /\s/.test(ch);
  const isKeyChar = ch => /[A-Za-z0-9_-]/.test(ch);

  while (i < n) {
// skip leading whitespace
while (i < n && isSpace(raw[i])) i++;
if (i >= n) break;

// read key
const kStart = i;
while (i < n && isKeyChar(raw[i])) i++;
const key = raw.slice(kStart, i);
if (!key) { i++; continue; }

// find separator (= or :) after optional whitespace
let sepIndex = i;
while (sepIndex < n && isSpace(raw[sepIndex])) sepIndex++;
if (sepIndex >= n) { kv[key] = ''; break; }

const sep = raw[sepIndex];
if (sep !== '=' && sep !== ':') {
  // not a key/value pair; continue scanning from the first non-space char we found
  i = sepIndex;
  continue;
}

// move i to the first character of the value
i = sepIndex + 1;
while (i < n && isSpace(raw[i])) i++;
if (i >= n) { kv[key] = ''; break; }

// read value
let val = '';
if (raw[i] === '"') {
  i++; // skip opening quote
  let esc = false;
  while (i < n) {
    const ch = raw[i];
    if (esc) { val += ch; esc = false; i++; continue; }
    if (ch === '\\') { esc = true; i++; continue; }
    if (ch === '"') { i++; break; }
    val += ch; i++;
  }
} else {
  // unquoted value ends at whitespace
  const vStart = i;
  while (i < n && !isSpace(raw[i])) i++;
  val = raw.slice(vStart, i);
}

kv[key] = val;
  }

  return { hasKV: Object.keys(kv).length > 0, kv };
}
    function escapeRegExp(str){ return str.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); }
    function renderLogsWithFilters() {
  const logs = state._logsRaw || [];
  const selected = state.logsSelectedSteps || new Set();
  const query = (state.logsSearchText || '').toLowerCase();
  const showTs = !!state.logsShowTimestamps;
  const structuredOn = !!state.logsStructured;
  const wrap = !!state.logsWrap;
  const levelFilter = state.logsLevelFilter || new Set(['info','warn','error','debug']);

  // container presentation
  DOM.logsContainer.classList.toggle('whitespace-pre-wrap', wrap);
  DOM.logsContainer.classList.toggle('whitespace-pre', !wrap);

  const ansiRegex = /[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g;

  const colorConfig = {
info:   { levelBg: 'bg-indigo-100 dark:bg-indigo-900', levelText: 'text-indigo-600 dark:text-indigo-300', borderColor: 'border-indigo-500' },
warn:   { levelBg: 'bg-amber-100  dark:bg-amber-900',  levelText: 'text-amber-600  dark:text-amber-300', borderColor: 'border-amber-500'  },
error:  { levelBg: 'bg-rose-100   dark:bg-rose-900',   levelText: 'text-rose-600   dark:text-rose-300',   borderColor: 'border-rose-500'   },
debug:  { levelBg: 'bg-slate-200  dark:bg-slate-800',  levelText: 'text-slate-700  dark:text-slate-300',  borderColor: 'border-slate-500'  },
default:{ levelBg: 'bg-slate-100  dark:bg-slate-900',  levelText: 'text-slate-600  dark:text-slate-300',  borderColor: 'border-slate-500'  },
success:{ levelBg: 'bg-emerald-100 dark:bg-emerald-900', levelText: 'text-emerald-600 dark:text-emerald-300', borderColor: 'border-emerald-500' },
  };

  const preferredDetailOrder = new Map([
['runid', 0],
['pipeline', 1],
['step', 2],
['task', 3],
['status', 4],
['stage', 5],
['job', 6],
['task_id', 7],
['attempt', 8]
  ]);

  const multilineKeys = new Set(['action','script','cmd','command','stderr','stdout','details']);
  const messageAllowedKeys = new Set(['status','action','output']);
  const tagKeys = ['runid','pipeline','step','task','status','component','session','container_id','child_run_id','image'];
  const tagLabels = {
runid: 'Run ID',
pipeline: 'Pipeline',
step: 'Step',
task: 'Task',
status: 'Status',
component: 'Component',
session: 'Session',
container_id: 'Container ID',
child_run_id: 'Child Run',
image: 'Image'
  };

  // assign stable color index to selected steps for background tint
  const nameToIdx = new Map(Array.from(selected).map((n, i) => [n, i % 8]));

  const escapeRegExp = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const rx = query ? new RegExp(`(${escapeRegExp(query)})`, 'ig') : null;
  let matches = 0;

  const html = logs.map((log, i) => {
const rawLine = (log.line || '').replace(ansiRegex, '');
// timestamp (fallback to log.timestamp if present)
const tsMatch = rawLine.match(/^(\d{1,2}:\d{2}:\d{2}\s[AP]M)/);
const ts = tsMatch ? tsMatch[0] : (log.timestamp ? new Date(log.timestamp).toLocaleTimeString() : '');

// If "structured" toggle is on, try JSON first
const jsonStart = rawLine.indexOf('{');
if (structuredOn && jsonStart !== -1) {
  try {
    const json = JSON.parse(rawLine.substring(jsonStart));
    return renderStructured(json, {rawLine, i, ts});
  } catch { /* fall through to KV and then to raw */ }
}

// Try key=value parsing (supports multi-line quoted values)
if (structuredOn) {
  const { hasKV, kv } = parseKVLine(rawLine);
  if (hasKV) {
    if (!kv.level) {
      const prefixMatch = rawLine.match(/^\s*(INFO|WARN|WARNING|ERROR|DEBUG|TRACE|SUCCESS|FAIL|FAILED)\b/i);
      if (prefixMatch) {
        const candidate = prefixMatch[1].toLowerCase();
        let normalized = candidate;
        if (candidate === 'warning') normalized = 'warn';
        else if (candidate === 'success') normalized = 'info';
        else if (candidate === 'fail' || candidate === 'failed') normalized = 'error';
        else if (candidate === 'trace') normalized = 'debug';
        kv.level = normalized;
      }
    }

    // normalize level from level/status
    const derivedLevel = deriveLevelFromStatus(kv.status);
    const level = (kv.level || derivedLevel || 'info').toLowerCase();
    if (!levelFilter.has(level)) return '';

    // step selection tinting
    let isSelectedLine = false;
    let lineColorIdx = -1;
    if (selected.size > 0) {
      const stepName = (typeof getLogStepName === 'function') ? getLogStepName({ ...log, ...kv }) : null;
      if (stepName && selected.has(stepName)) {
        isSelectedLine = true;
        lineColorIdx = nameToIdx.get(stepName) ?? 0;
      }
    }
    const dimClass = (selected.size > 0 && !isSelectedLine) ? ' log-line--dim' : '';
    const selClass = (isSelectedLine && lineColorIdx >= 0) ? ` log-line--sel c${lineColorIdx}` : '';

    // message: prefer 'message'; else fallback to status/raw line
    let message = kv.message || '';
    if (!message) {
      const parts = [];
      if (!parts.length && kv.status) parts.push(`status: ${kv.status}`);
      message = parts.join(' • ') || rawLine;
    }

    // Build details rows (pretty-print multi-line values like action/script/command)
    const tagValues = new Map();
    const detailRows = [];
    let fallbackOrder = preferredDetailOrder.size;
    for (const key in kv) {
      const keyLower = key.toLowerCase();
      if (['message','level','output','action'].includes(keyLower)) continue;
      const v = String(kv[key] ?? '');
      const trimmed = v.trim();
      const isMultiline = v.includes('\n');
      const vDisplay = rx && v.toLowerCase().includes(query) ? v.replace(rx, '<span class="log-highlight">$1</span>') : v;
      if (rx && v.toLowerCase().includes(query)) matches++;
      if (tagKeys.includes(keyLower)) {
        if (!tagValues.has(keyLower) && trimmed) tagValues.set(keyLower, { raw: trimmed, display: vDisplay });
        continue;
      }
      const markup = (isMultiline || multilineKeys.has(keyLower))
        ? `<div class="grid grid-cols-[120px,1fr] gap-x-2 items-start">
          <strong class="text-right text-gray-500 dark:text-gray-400 font-normal mt-1">${key}:</strong>
          <pre class="bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${vDisplay}</pre>
        </div>`
        : rowKV(key, vDisplay);
      const order = preferredDetailOrder.has(key) ? preferredDetailOrder.get(key) : fallbackOrder++;
      detailRows.push({ order, markup });
    }
    detailRows.sort((a, b) => a.order - b.order);
    const detailsHTML = detailRows.map(r => r.markup).join('');

    // action block (full-width)
    let actionBlock = '';
    if (kv.action !== undefined) {
      const actionRaw = String(kv.action ?? '');
      const actionLower = actionRaw.toLowerCase();
      const actionHit = rx && actionLower.includes(query);
      const actionDisplay = actionHit ? actionRaw.replace(rx, '<span class="log-highlight">$1</span>') : actionRaw;
      if (actionHit) matches++;
      const actionPretty = actionDisplay.replace(/\r/g, '');
      actionBlock = `<div class="mt-1 flex items-start gap-2">
      <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-indigo-200 dark:text-indigo-100 bg-indigo-950/70 dark:bg-indigo-700/50 px-2 py-0.5 rounded">Action</span>
      <pre class="flex-1 bg-indigo-950/90 dark:bg-indigo-950 text-indigo-100 dark:text-indigo-50 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${actionPretty}</pre>
    </div>`;
    }

    if (actionBlock && message) {
      message = message.replace(/action:\s?[^•]+(?:•\s*)?/i, '').trim();
      if (message.endsWith('•')) message = message.slice(0, -1).trim();
    }

    // output block (empty allowed)
    let outputBlock = '';
    if ('output' in kv) {
      const out = kv.output ?? '';
      const outStr = String(out);
      const outLower = outStr.toLowerCase();
      const outHit = rx && outLower.includes(query);
      const outDisplay = outHit ? outStr.replace(rx, '<span class="log-highlight">$1</span>') : outStr;
      if (outHit) matches++;
      if (outStr === '') {
        outputBlock = `<div class="mt-1 flex items-start gap-2">
          <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-slate-600 dark:text-slate-300 bg-slate-500/10 dark:bg-slate-500/20 px-2 py-0.5 rounded">Output</span>
          <pre class="flex-1 bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto italic opacity-70">(empty output)</pre>
        </div>`;
      } else {
        const pretty = outDisplay.replace(/\r/g, '');
        outputBlock = `<div class="mt-1 flex items-start gap-2">
          <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-slate-600 dark:text-slate-300 bg-slate-500/10 dark:bg-slate-500/20 px-2 py-0.5 rounded">Output</span>
          <pre class="flex-1 bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${pretty}</pre>
        </div>`;
      }
    }

    // highlight message
    if (rx && message.toLowerCase().includes(query)) {
      message = message.replace(rx, '<span class="log-highlight">$1</span>');
      matches++;
    }

    if (!outputBlock && message) {
      const label = 'Output';
      outputBlock = `<div class="mt-1 flex items-start gap-2">
          <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-slate-600 dark:text-slate-300 bg-slate-500/10 dark:bg-slate-500/20 px-2 py-0.5 rounded">${label}</span>
          <pre class="flex-1 bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${message.replace(/\r/g, '')}</pre>
        </div>`;
      message = '';
    }

    const isSuccess = (kv.status && kv.status.toLowerCase() === 'success');
    const colors = isSuccess ? colorConfig.success : (colorConfig[level] || colorConfig.default);

    const tagsBlock = renderTagPills(tagValues, { inline: true });
    const bodyPieces = [];
    if (actionBlock) bodyPieces.push(actionBlock);
    if (outputBlock) bodyPieces.push(outputBlock);
    if (!outputBlock && message) {
      bodyPieces.push(actionBlock ? `<div class="mt-0.5">${message}</div>` : message);
    }
    const bodyHtml = bodyPieces.length > 0 ? bodyPieces.join('') : '<span class="opacity-50">—</span>';

    return `
      <div class="log-line-structured ${dimClass} ${selClass}">
        <div class="border-l-4 ${colors.borderColor} pl-4">
          <div class="flex flex-col">
            <div class="flex flex-wrap items-center gap-2 text-xs">
              ${showTs ? `<span class="text-[var(--text-secondary)] select-none">${ts}</span>` : ''}
              <span class="font-semibold px-2 py-0.5 rounded-full ${colors.levelBg} ${colors.levelText}">${(kv.status ? kv.status : level).toString().toUpperCase()}</span>
            </div>
            <div class="flex flex-wrap items-center gap-0.5 text-xs mt-1">
              ${tagsBlock ? `<div class="flex flex-wrap items-center gap-0.5">${tagsBlock}</div>` : ''}
            </div>
            <div class="pl-0 text-sm text-[var(--text-primary)] leading-6 mt-1">${bodyHtml}</div>
          </div>
        </div>
      </div>`;
  }
}

// Fallback: plain/raw rendering
let line = rawLine.replace(/</g, '&lt;').replace(/>/g, '&gt;');
if (rx && rawLine.toLowerCase().includes(query)) {
  matches++;
  line = line.replace(rx, '<span class="log-highlight">$1</span>');
}
const left = [
  showTs ? `<span class="text-[var(--text-secondary)] select-none pr-3">${ts}</span>` : ''
].join('');
return `<div class="log-line py-0 grid grid-cols-[auto,1fr] gap-1">
          <span class="inline-flex items-baseline">${left}</span>
          <span>${line}</span>
        </div>`;

// ---- helpers (scoped) ----
function renderStructured(json, ctx) {
  const baseMessage = json.message || '';
  const parsedMessage = baseMessage ? parseKVLine(baseMessage) : { hasKV: false, kv: {} };
  const messagePairsAll = parsedMessage.hasKV ? parsedMessage.kv : {};
  const messagePairs = {};
  for (const key in messagePairsAll) {
    if (messageAllowedKeys.has(key)) {
      messagePairs[key] = messagePairsAll[key];
    }
  }
  const messageKeys = Object.keys(messagePairs);
  const kvOnlyRegex = /^(?:\s*[A-Za-z0-9_-]+(?:=|:)(?:"(?:[^"\\]|\\.)*"|[^\s"]+)\s*)+$/;
  const statusTextRaw = (json.status !== undefined && json.status !== null && json.status !== '')
    ? json.status
    : (Object.prototype.hasOwnProperty.call(messagePairs, 'status') ? messagePairs.status : undefined);
  const levelCandidateRaw = json.level || (statusTextRaw ? deriveLevelFromStatus(statusTextRaw) : 'info');
  const level = String(levelCandidateRaw || 'info').toLowerCase();
  if (!levelFilter.has(level)) return '';

  // step selection tinting
  let isSelectedLine = false;
  let lineColorIdx = -1;
  if (selected.size > 0) {
    const stepName = (typeof getLogStepName === 'function') ? getLogStepName({ ...log, ...json }) : null;
    if (stepName && selected.has(stepName)) {
      isSelectedLine = true;
      lineColorIdx = nameToIdx.get(stepName) ?? 0;
    }
  }
  const dimClass = (selected.size > 0 && !isSelectedLine) ? ' log-line--dim' : '';
  const selClass = (isSelectedLine && lineColorIdx >= 0) ? ` log-line--sel c${lineColorIdx}` : '';

  // detail rows
  const tagValues = new Map();
  const detailRows = [];
  let fallbackOrder = preferredDetailOrder.size;
  const seenKeys = new Set(['message','level','time']);
  let message = baseMessage;

  for (const key in json) {
    const keyLower = key.toLowerCase();
    if (['message','level','time','output','action'].includes(keyLower)) continue;
    const vStr = String(json[key]);
    const trimmed = vStr.trim();
    const vHtml = rx && vStr.toLowerCase().includes(query) ? vStr.replace(rx, '<span class="log-highlight">$1</span>') : vStr;
    if (rx && vStr.toLowerCase().includes(query)) matches++;
    if (tagKeys.includes(keyLower)) {
      if (!tagValues.has(keyLower) && trimmed) tagValues.set(keyLower, { raw: trimmed, display: vHtml });
      seenKeys.add(keyLower);
      continue;
    }
    const order = preferredDetailOrder.has(key) ? preferredDetailOrder.get(key) : fallbackOrder++;
    detailRows.push({ order, markup: rowKV(key, vHtml) });
    seenKeys.add(keyLower);
  }

  if (messageKeys.length > 0) {
    for (const key of messageKeys) {
      const keyLower = key.toLowerCase();
      if (seenKeys.has(keyLower) || keyLower === 'output' || keyLower === 'action') continue;
      const rawVal = messagePairs[key] ?? '';
      const vStr = String(rawVal);
      const trimmed = vStr.trim();
      const isMultiline = vStr.includes('\n');
      const vHtml = rx && vStr.toLowerCase().includes(query) ? vStr.replace(rx, '<span class="log-highlight">$1</span>') : vStr;
      if (rx && vStr.toLowerCase().includes(query)) matches++;
      if (tagKeys.includes(keyLower)) {
        if (!tagValues.has(keyLower) && trimmed) tagValues.set(keyLower, { raw: trimmed, display: vHtml });
        seenKeys.add(keyLower);
        continue;
      }
      const markup = (isMultiline || multilineKeys.has(keyLower))
        ? `<div class="grid grid-cols-[120px,1fr] gap-x-2 items-start">
          <strong class="text-right text-gray-500 dark:text-gray-400 font-normal mt-1">${key}:</strong>
          <pre class="bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${vHtml}</pre>
        </div>`
        : rowKV(key, vHtml);
      const order = preferredDetailOrder.has(key) ? preferredDetailOrder.get(key) : fallbackOrder++;
      detailRows.push({ order, markup });
      seenKeys.add(keyLower);
    }
  }

  detailRows.sort((a, b) => a.order - b.order);
  const detailsHTML = detailRows.map(r => r.markup).join('');

  // action block (prefers message-derived value)
  let actionBlock = '';
  let actionSource = undefined;
  if (Object.prototype.hasOwnProperty.call(messagePairs, 'action')) {
    actionSource = messagePairs.action;
  } else if (Object.prototype.hasOwnProperty.call(json, 'action')) {
    actionSource = json.action;
  }
  if (actionSource !== undefined) {
    const actionRaw = String(actionSource ?? '');
    const actionLower = actionRaw.toLowerCase();
    const actionHit = rx && actionLower.includes(query);
    const actionDisplay = actionHit ? actionRaw.replace(rx, '<span class="log-highlight">$1</span>') : actionRaw;
    if (actionHit) matches++;
    const actionPretty = actionDisplay.replace(/\r/g, '');
    actionBlock = `<div class="mt-1 flex items-start gap-2">
      <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-indigo-200 dark:text-indigo-100 bg-indigo-950/70 dark:bg-indigo-700/50 px-2 py-0.5 rounded">Action</span>
      <pre class="flex-1 bg-indigo-950/90 dark:bg-indigo-950 text-indigo-100 dark:text-indigo-50 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${actionPretty}</pre>
    </div>`;
    seenKeys.add('action');
  }

  if (actionBlock && message) {
    message = message.replace(/action:\s?[^•]+(?:•\s*)?/i, '').trim();
    if (message.endsWith('•')) message = message.slice(0, -1).trim();
  }

  // output block (prefers message-derived value)
  let outputBlock = '';
  const hasMessageOutput = Object.prototype.hasOwnProperty.call(messagePairs, 'output');
  if (hasMessageOutput || Object.prototype.hasOwnProperty.call(json, 'output')) {
    const outVal = hasMessageOutput ? (messagePairs.output ?? '') : (json.output ?? '');
    const outStr = String(outVal);
    const outLower = outStr.toLowerCase();
    const hit = rx && outLower.includes(query);
    const outDisplay = hit ? outStr.replace(rx, '<span class="log-highlight">$1</span>') : outStr;
    if (hit) matches++;
    if (outStr === '') {
      outputBlock = `<div class="mt-1 flex items-start gap-2">
        <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-slate-600 dark:text-slate-300 bg-slate-500/10 dark:bg-slate-500/20 px-2 py-0.5 rounded">Output</span>
        <pre class="flex-1 bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto italic opacity-70">(empty output)</pre>
      </div>`;
    } else {
      const pretty = outDisplay.replace(/\r/g, '');
      outputBlock = `<div class="mt-1 flex items-start gap-2">
        <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-slate-600 dark:text-slate-300 bg-slate-500/10 dark:bg-slate-500/20 px-2 py-0.5 rounded">Output</span>
        <pre class="flex-1 bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${pretty}</pre>
      </div>`;
    }
  }

  if (messageKeys.length > 0 && kvOnlyRegex.test(baseMessage.trim())) {
    message = '';
  }

  if (message && rx && message.toLowerCase().includes(query)) {
    message = message.replace(rx, '<span class="log-highlight">$1</span>');
    matches++;
  }

  if (!outputBlock && message) {
    const label = 'Output';
    outputBlock = `<div class="mt-1 flex items-start gap-2">
        <span class="shrink-0 text-[11px] uppercase tracking-wide font-semibold text-slate-600 dark:text-slate-300 bg-slate-500/10 dark:bg-slate-500/20 px-2 py-0.5 rounded">${label}</span>
        <pre class="flex-1 bg-gray-900/90 dark:bg-gray-900 text-gray-100 dark:text-gray-200 px-3 py-1.5 rounded-lg text-xs shadow-inner overflow-auto">${message.replace(/\\r/g, '')}</pre>
      </div>`;
    message = '';
  }

  const statusText = statusTextRaw ? String(statusTextRaw) : null;
  const isSuccess = statusText && statusText.toLowerCase() === 'success';
  const colors = isSuccess ? colorConfig.success : (colorConfig[level] || colorConfig.default);
  const badgeLabelSource = statusText || (Object.prototype.hasOwnProperty.call(messagePairs, 'status') ? messagePairs.status : levelCandidateRaw) || 'INFO';
  const badgeLabel = String(badgeLabelSource).toUpperCase();

  const tagsBlock = renderTagPills(tagValues, { inline: true });
  const bodyPieces = [];
  if (actionBlock) bodyPieces.push(actionBlock);
  if (outputBlock) bodyPieces.push(outputBlock);
  if (!outputBlock && message) {
    bodyPieces.push(actionBlock ? `<div class="mt-0.5">${message}</div>` : message);
  }
  const bodyHtml = bodyPieces.length > 0 ? bodyPieces.join('') : '<span class="opacity-50">—</span>';

  return `
    <div class="log-line-structured ${dimClass} ${selClass}">
      <div class="border-l-4 ${colors.borderColor} pl-4">
        <div class="flex flex-col">
          <div class="flex flex-wrap items-center gap-2 text-xs">
            ${showTs ? `<span class="text-[var(--text-secondary)] select-none">${ctx.ts}</span>` : ''}
            <span class="font-semibold px-2 py-0.5 rounded-full ${colors.levelBg} ${colors.levelText}">${badgeLabel}</span>
          </div>
          <div class="flex flex-wrap items-center gap-0.5 text-xs mt-1">
            ${tagsBlock || ''}
          </div>
          <div class="pl-0 text-sm text-[var(--text-primary)] leading-6 mt-1">${bodyHtml}</div>
        </div>
      </div>
    </div>`;
}

function renderTagPills(map, opts = {}) {
  if (!map || map.size === 0) return '';
  const chips = [];
  const inline = !!opts.inline;
  for (const key of tagKeys) {
    const entry = map.get(key);
    if (!entry) continue;
    const valueHtml = entry.display || entry.raw || '';
    const rawValue = entry.raw || '';
    if (!valueHtml) continue;
    const label = tagLabels[key] || key.toUpperCase();
    const title = rawValue.replace(/&/g, '&amp;').replace(/"/g, '&quot;');
    chips.push(`<span title="${title}" class="inline-flex items-center gap-1 rounded-full bg-slate-500/10 dark:bg-slate-200/10 text-slate-700 dark:text-slate-100 px-2 py-0.5 text-[11px] font-medium"><span class="uppercase tracking-wide text-[10px] text-slate-500 dark:text-slate-300">${label}</span><span class="font-mono text-[11px] text-slate-800 dark:text-slate-100 break-all">${valueHtml}</span></span>`);
  }
  if (chips.length === 0) return '';
  if (inline) {
    return `<span class="inline-flex flex-wrap items-center gap-0.5">${chips.join('')}</span>`;
  }
  return `<div class="flex flex-wrap gap-1 -mt-1.5 mb-0.25">${chips.join('')}</div>`;
}

function rowKV(k, vHtml) {
  return `<div class="grid grid-cols-[60px,1fr] gap-x-2">
    <strong class="text-right text-gray-500 dark:text-gray-400 font-normal">${k}:</strong>
    <span class="truncate">${vHtml}</span>
  </div>`;
}

function deriveLevelFromStatus(status) {
  if (!status) return 'info';
  const s = String(status).toLowerCase();
  if (s === 'success' || s === 'ok' || s === 'passed') return 'info';
  if (s === 'warn' || s === 'warning') return 'warn';
  if (s === 'debug') return 'debug';
  return 'error';
}
  }).filter(Boolean).join('');

  DOM.logsContainer.innerHTML = html || `<p class="text-[var(--text-secondary)]">No matching logs.</p>`;

  if (DOM.logsCount) {
const total = logs.length || 0;
DOM.logsCount.textContent = query && total ? `${matches} matches • ${total} lines` : (total ? `${total} lines` : '');
  }

  // focus + follow behaviors
  if (state._logsFocusFirstMatch) {
requestAnimationFrame(() => {
  let target = DOM.logsContainer.querySelector('.log-highlight');
  if (target) target = target.closest('.log-line-structured') || target.closest('.log-line') || target;
  if (target && typeof target.scrollIntoView === 'function') {
    try { target.scrollIntoView({ block: 'start' }); } catch { target.scrollIntoView(); }
  }
  state._logsFocusFirstMatch = false;
});
  } else if (DOM.followLogsCheckbox && DOM.followLogsCheckbox.checked) {
DOM.logsContainer.scrollTop = DOM.logsContainer.scrollHeight;
  }
}


    global.logs = {
        init,
        showLogsModal,
        initLogsUIControls,
        closeLogsModal,
        fetchAndRenderLogs,
        getLogStepName,
        buildLogsFilters,
        updateLogsStepList,
        renderLogsWithFilters,
    };
})(window.NopsAI = window.NopsAI || {});
