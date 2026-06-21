import { useEffect, useMemo, useRef, useState } from 'react';
import { useSystemLogs } from './useSystemLogs.js';
import type { SystemLogEntry, SystemLogStream } from './types.js';

const connectionTone: Record<string, string> = {
  connected: 'bg-emerald-500',
  connecting: 'bg-amber-500',
  reconnecting: 'bg-amber-500',
  error: 'bg-rose-500',
  idle: 'bg-slate-400',
};

const toggleButtonClass = (selected: boolean) => [
  'inline-flex items-center gap-2 rounded border px-3 py-2 text-sm font-medium transition-colors',
  'focus:outline-none focus:ring-2 focus:ring-[var(--border-accent-focus-ring)]',
  selected
    ? 'border-[var(--border-accent)] bg-[var(--bg-tertiary)] text-[var(--text-primary)] ring-1 ring-[var(--border-accent)]'
    : 'border-[var(--border-primary)] text-[var(--text-secondary)] hover:border-[var(--border-accent)] hover:text-[var(--text-primary)]',
].join(' ');

function ToggleStateDot({ selected }: { selected: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`h-2 w-2 rounded-full ${selected ? 'bg-[var(--border-accent)]' : 'border border-[var(--border-secondary)]'}`}
    />
  );
}

const formatLogText = (entries: SystemLogEntry[]) => entries
  .map(entry => `${entry.emitted_at} ${entry.stream} ${entry.line}`)
  .join('\n');

function SystemLogsPanel() {
  const logs = useSystemLogs();
  const [query, setQuery] = useState('');
  const [streams, setStreams] = useState<Set<SystemLogStream>>(new Set(['stdout', 'stderr']));
  const [wrapLines, setWrapLines] = useState(false);
  const viewportRef = useRef<HTMLDivElement>(null);
  const normalizedQuery = query.trim().toLowerCase();
  const visibleEntries = useMemo(() => logs.entries.filter(entry =>
    streams.has(entry.stream) && (!normalizedQuery || entry.line.toLowerCase().includes(normalizedQuery))
  ), [logs.entries, normalizedQuery, streams]);

  useEffect(() => {
    if (logs.paused || !logs.live || !viewportRef.current) return;
    viewportRef.current.scrollTop = viewportRef.current.scrollHeight;
  }, [logs.entries, logs.live, logs.paused]);

  const toggleStream = (stream: SystemLogStream) => {
    setStreams(current => {
      const next = new Set(current);
      if (next.has(stream)) next.delete(stream);
      else next.add(stream);
      return next;
    });
  };

  const copyVisible = async () => {
    await navigator.clipboard?.writeText(formatLogText(visibleEntries));
  };

  const downloadVisible = () => {
    const blob = new Blob([formatLogText(visibleEntries)], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `nopsai-${logs.selectedSourceID || 'system'}-logs.txt`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  return (
    <section className="space-y-4" aria-labelledby="system-logs-heading">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 id="system-logs-heading" className="text-xl font-semibold">System logs</h2>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">Live operational logs from allow-listed NopsAI platform services.</p>
        </div>
        <button type="button" className="rounded border px-3 py-2 text-sm" onClick={() => void logs.refreshSources()}>Refresh sources</button>
      </div>

      {logs.redactionWarning && (
        <div role="note" className="rounded border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200">
          {logs.redactionWarning}
        </div>
      )}
      {logs.cursorExpired && (
        <div role="alert" className="rounded border border-rose-300 bg-rose-50 px-3 py-2 text-sm text-rose-900 dark:bg-rose-950/30 dark:text-rose-200">
          Log gap detected. The replay cursor expired, so some lines may be missing.
        </div>
      )}
      {logs.error && <div role="alert" className="rounded border border-rose-300 px-3 py-2 text-sm text-rose-700">{logs.error}</div>}

      <div className="grid gap-3 rounded border border-[var(--border)] bg-[var(--bg-secondary)] p-4 lg:grid-cols-[minmax(220px,1fr)_auto_auto]">
        <label className="text-sm font-medium">
          Service
          <select
            aria-label="System log service"
            value={logs.selectedSourceID}
            onChange={event => logs.selectSource(event.target.value)}
            className="mt-1 w-full rounded border bg-[var(--bg-primary)] px-3 py-2"
          >
            {logs.sources.map(source => (
              <option key={source.id} value={source.id} disabled={!source.available}>
                {source.display_name} · {source.available ? source.health || source.state : 'unavailable'}
              </option>
            ))}
          </select>
        </label>
        <div className="flex items-end gap-2 text-sm" aria-label="Connection status">
          <span className={`mb-3 h-2.5 w-2.5 rounded-full ${connectionTone[logs.connectionState]}`} />
          <span className="mb-2 capitalize">{logs.connectionState}{logs.reconnectAttempt > 0 ? ` (${logs.reconnectAttempt})` : ''}</span>
        </div>
        <div className="flex items-end gap-2">
          <button type="button" aria-pressed={logs.live} className={toggleButtonClass(logs.live)} onClick={() => logs.setLive(!logs.live)}>
            <ToggleStateDot selected={logs.live} />
            {logs.live ? 'Stop live' : 'Go live'}
          </button>
          <button type="button" aria-pressed={logs.paused} className={toggleButtonClass(logs.paused)} onClick={logs.togglePaused}>
            <ToggleStateDot selected={logs.paused} />
            {logs.paused ? `Resume (${logs.unseenCount})` : 'Pause'}
          </button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <input aria-label="Search system logs" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search lines" className="min-w-56 flex-1 rounded border bg-[var(--bg-primary)] px-3 py-2 text-sm" />
        <div role="group" aria-label="Log stream filters" className="flex items-center gap-2">
          {(['stdout', 'stderr'] as const).map(stream => {
            const selected = streams.has(stream);
            return (
              <button key={stream} type="button" aria-pressed={selected} onClick={() => toggleStream(stream)} className={toggleButtonClass(selected)}>
                <ToggleStateDot selected={selected} />
                {stream}
              </button>
            );
          })}
        </div>
        <button type="button" aria-pressed={wrapLines} onClick={() => setWrapLines(value => !value)} className={toggleButtonClass(wrapLines)}>
          <ToggleStateDot selected={wrapLines} />
          Wrap lines
        </button>
        <button type="button" onClick={() => void copyVisible()} className="rounded border px-3 py-2 text-sm">Copy visible</button>
        <button type="button" onClick={downloadVisible} className="rounded border px-3 py-2 text-sm">Download visible</button>
        <button type="button" onClick={logs.clear} className="rounded border px-3 py-2 text-sm">Clear</button>
      </div>

      <div ref={viewportRef} role="log" aria-live={logs.paused ? 'off' : 'polite'} className="h-[58vh] overflow-auto rounded border border-slate-700 bg-slate-950 p-3 font-mono text-xs text-slate-100">
        {visibleEntries.length === 0 && <p className="text-slate-400">No matching log lines.</p>}
        {visibleEntries.map((entry, index) => {
          const previousInstance = visibleEntries[index - 1]?.container_instance;
          const restarted = Boolean(previousInstance && previousInstance !== entry.container_instance);
          return (
            <div key={entry.id}>
              {restarted && <div className="my-2 border-y border-amber-500/50 py-1 text-center text-amber-300">Service instance restarted</div>}
              <div className={`grid grid-cols-[11rem_3.5rem_1fr] gap-2 ${wrapLines ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'}`}>
                <time className="text-slate-500">{new Date(entry.emitted_at).toLocaleTimeString()}</time>
                <span className={entry.stream === 'stderr' ? 'text-rose-300' : 'text-sky-300'}>{entry.stream}</span>
                <span>{entry.line}</span>
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

export default SystemLogsPanel;
