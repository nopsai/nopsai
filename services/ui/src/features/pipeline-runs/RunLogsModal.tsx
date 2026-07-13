import { useEffect, useMemo, useRef, useState } from 'react';
import { useDialogFocus } from '../../components/useDialogFocus';
import { formatRunLogDownload, isAgentRunLogLine, normalizeRunLogLevel } from './runLogs';
import { useRunLogs } from './useRunLogs';
import { getStatusMeta } from './statusPresentation';

type RunLogStep = {
  name: string;
  status?: string;
};

export function RunLogsModal({
  runId,
  runName,
  onClose,
  steps,
  stepNames,
  initialStep,
  initialSearch,
}: {
  runId: string;
  runName?: string | null;
  onClose: () => void;
  steps?: RunLogStep[];
  stepNames?: string[];
  initialStep?: string | null;
  initialSearch?: string | null;
}) {
  const {
    agentOnly,
    error,
    follow,
    hasUnseen,
    levelOptions,
    lines,
    loading,
    presentLevels,
    searchText,
    selectedLevels,
    selectedSteps,
    shortView,
    structured,
    visibleLines,
    wrap,
    resetFilters,
    setAgentOnly,
    setFollow,
    setHasUnseen,
    setSearchText,
    setSelectedSteps,
    setShortView,
    setStructured,
    setWrap,
    toggleLevel,
    toggleStep,
  } = useRunLogs({ runID: runId, initialStep, initialSearch });
  const [stepSearch, setStepSearch] = useState('');
  const logContainerRef = useRef<HTMLDivElement | null>(null);
  const dialogRef = useDialogFocus(onClose);

  const stepItems = useMemo(() => {
    const fromSteps = (steps || []).map(step => ({
      name: step.name,
      status: step.status,
    }));
    const provided = (stepNames || []).map(name => ({ name, status: undefined }));
    const derived = Array.from(new Set(lines.map(line => line.step).filter(Boolean) as string[])).map(name => ({
      name,
      status: undefined,
    }));
    const merged = [...fromSteps, ...provided, ...derived];
    const seen = new Set<string>();
    return merged.filter(item => {
      if (!item.name || seen.has(item.name)) return false;
      seen.add(item.name);
      return true;
    });
  }, [lines, stepNames, steps]);

  const filteredStepItems = useMemo(() => {
    const term = stepSearch.trim().toLowerCase();
    if (!term) return stepItems;
    return stepItems.filter(item => item.name.toLowerCase().includes(term));
  }, [stepItems, stepSearch]);

  const stepColorMap = useMemo(() => {
    const palette = [
      {
        pillClass: 'bg-sky-500 text-white border-sky-600 dark:bg-sky-400 dark:text-slate-900 dark:border-sky-500',
        dotClass: 'bg-sky-500',
        lineClass: 'border-sky-500 dark:border-sky-400',
      },
      {
        pillClass: 'bg-emerald-500 text-white border-emerald-600 dark:bg-emerald-400 dark:text-slate-900 dark:border-emerald-500',
        dotClass: 'bg-emerald-500',
        lineClass: 'border-emerald-500 dark:border-emerald-400',
      },
      {
        pillClass: 'bg-indigo-500 text-white border-indigo-600 dark:bg-indigo-400 dark:text-slate-900 dark:border-indigo-500',
        dotClass: 'bg-indigo-500',
        lineClass: 'border-indigo-500 dark:border-indigo-400',
      },
      {
        pillClass: 'bg-amber-500 text-white border-amber-600 dark:bg-amber-400 dark:text-slate-900 dark:border-amber-500',
        dotClass: 'bg-amber-500',
        lineClass: 'border-amber-500 dark:border-amber-400',
      },
      {
        pillClass: 'bg-rose-500 text-white border-rose-600 dark:bg-rose-400 dark:text-slate-900 dark:border-rose-500',
        dotClass: 'bg-rose-500',
        lineClass: 'border-rose-500 dark:border-rose-400',
      },
      {
        pillClass: 'bg-teal-500 text-white border-teal-600 dark:bg-teal-400 dark:text-slate-900 dark:border-teal-500',
        dotClass: 'bg-teal-500',
        lineClass: 'border-teal-500 dark:border-teal-400',
      },
      {
        pillClass: 'bg-purple-500 text-white border-purple-600 dark:bg-purple-400 dark:text-slate-900 dark:border-purple-500',
        dotClass: 'bg-purple-500',
        lineClass: 'border-purple-500 dark:border-purple-400',
      },
      {
        pillClass: 'bg-lime-500 text-white border-lime-600 dark:bg-lime-400 dark:text-slate-900 dark:border-lime-500',
        dotClass: 'bg-lime-500',
        lineClass: 'border-lime-500 dark:border-lime-400',
      },
    ];
    const map = new Map<string, (typeof palette)[number]>();
    Array.from(selectedSteps).forEach((step, index) => {
      map.set(step, palette[index % palette.length]);
    });
    return map;
  }, [selectedSteps]);

  const handleResetFilters = () => {
    resetFilters();
    setStepSearch('');
  };

  const handleDownload = () => {
    const source = (visibleLines.length ? visibleLines : lines) || [];
    if (!source.length) {
      alert('No logs available to download yet.');
      return;
    }
    const blob = new Blob([formatRunLogDownload(source)], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `logs-${(runId || 'run').slice(0, 8)}.txt`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const formatTime = (iso: string) => {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return '—';
    return date.toLocaleTimeString(undefined, { hour12: true });
  };

  const levelTone = (level: string) => {
    const normalized = level.toLowerCase();
    if (normalized === 'error') return 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-100';
    if (normalized === 'warn' || normalized === 'warning') return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-100';
    if (normalized === 'debug') return 'bg-slate-200 text-slate-700 dark:bg-slate-800 dark:text-[var(--text-primary)]';
    if (normalized === 'agent') return 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-100';
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-100';
  };

  useEffect(() => {
    const container = logContainerRef.current;
    if (!container) return;
    const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 80;
    if (follow && nearBottom) {
      container.scrollTop = container.scrollHeight;
      setHasUnseen(false);
    }
  }, [follow, setHasUnseen, visibleLines.length]);

  const handleScroll = () => {
    const container = logContainerRef.current;
    if (!container) return;
    const nearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 80;
    if (!nearBottom) {
      setFollow(false);
    } else {
      setFollow(true);
      setHasUnseen(false);
    }
  };

  const logCountLabel = `${visibleLines.length} line${visibleLines.length === 1 ? '' : 's'} + ${lines.length} total`;

  return (
    <div className="fixed inset-0 z-[60] flex items-center justify-center bg-[var(--bg-overlay)] px-4 py-6">
      <div
        ref={dialogRef}
        className="w-full max-w-6xl bg-[var(--bg-primary)] rounded-2xl shadow-2xl border border-[var(--border-primary)] flex flex-col max-h-[90vh] overflow-hidden"
        role="dialog"
        aria-modal="true"
        aria-labelledby="run-logs-title"
        aria-describedby="run-logs-description"
        tabIndex={-1}
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-primary)]">
          <div>
            <h2 id="run-logs-title" className="text-base font-semibold text-[var(--text-primary)]">
              Agent Logs for {runName || runId}
            </h2>
            <p id="run-logs-description" className="text-xs text-[var(--text-secondary)]">Run ID: {runId}</p>
          </div>
          <div className="flex items-center gap-2">
            <button className="runner-pill runner-pill--ghost" type="button" onClick={handleDownload}>
              Download
            </button>
            <button className="glass-button-ghost" type="button" onClick={onClose}>
              Close
            </button>
          </div>
        </div>

        <div className="flex flex-col gap-3 border-b border-[var(--border-primary)] bg-[var(--bg-secondary)] px-5 py-3 md:flex-row md:items-start md:gap-6">
          <div className="flex-1 min-w-[280px]">
            <div className="relative">
              <input
                type="search"
                aria-label="Search run logs"
                className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
                placeholder="Search logs..."
                value={searchText}
                onChange={event => setSearchText(event.target.value)}
                data-dialog-initial-focus
              />
              {searchText && (
                <button
                  type="button"
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-[var(--text-secondary)] text-xs"
                  onClick={() => setSearchText('')}
                >
                  Clear
                </button>
              )}
            </div>
            <p className="text-[11px] text-[var(--text-secondary)] mt-1" aria-live="polite">{logCountLabel}</p>
          </div>
          <div className="flex flex-col gap-2 flex-1 min-w-[240px] w-full items-end">
            <div className="flex items-center gap-2 flex-wrap justify-end">
              {levelOptions.map(level => {
                const isDefault = selectedLevels.size === 0;
                const active = !isDefault && selectedLevels.has(level);
                const available = presentLevels.has(level);
                return (
                  <button
                    key={level}
                    type="button"
                    aria-pressed={active}
                    disabled={!available && lines.length > 0}
                    className={`px-2.5 py-1 rounded-full text-xs font-semibold border border-[var(--border-primary)] ${active ? 'bg-[var(--bg-primary)] text-[var(--text-primary)] ring-1 ring-[var(--border-accent)]' : 'text-[var(--text-secondary)]'} ${!available && lines.length > 0 ? 'opacity-40 cursor-not-allowed' : ''}`}
                    onClick={() => toggleLevel(level)}
                    title={`Toggle ${level} logs`}
                  >
                    {level.toUpperCase()}
                  </button>
                );
              })}
              <button
                type="button"
                disabled={!presentLevels.has('agent') && lines.length > 0}
                className={`px-2.5 py-1 rounded-full text-xs font-semibold border border-[var(--border-primary)] ${agentOnly ? 'bg-[var(--bg-primary)] text-[var(--text-primary)] ring-1 ring-[var(--border-accent)]' : 'text-[var(--text-secondary)]'} ${!presentLevels.has('agent') && lines.length > 0 ? 'opacity-40 cursor-not-allowed' : ''}`}
                onClick={() => {
                  setAgentOnly(prev => !prev);
                  setFollow(true);
                  setHasUnseen(false);
                }}
                title="Show only agent logs"
              >
                AGENT
              </button>
            </div>
            <div className="flex items-center gap-2 flex-wrap justify-end w-full">
              {[
                { label: 'Follow', value: follow, setter: setFollow },
                { label: 'Wrap', value: wrap, setter: setWrap },
                { label: 'Structured', value: structured, setter: setStructured },
                { label: 'Short', value: shortView, setter: setShortView },
              ].map(toggle => (
                <button
                  key={toggle.label}
                  type="button"
                  onClick={() => {
                    const next = !toggle.value;
                    toggle.setter(next);
                    if (toggle.label === 'Follow' && next) {
                      const container = logContainerRef.current;
                      if (container) container.scrollTop = container.scrollHeight;
                      setHasUnseen(false);
                    }
                  }}
                  disabled={shortView && (toggle.label === 'Wrap' || toggle.label === 'Structured')}
                  className={`px-3 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-2 ${toggle.value ? 'bg-[var(--bg-primary)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)]'} ${shortView && (toggle.label === 'Wrap' || toggle.label === 'Structured') ? 'opacity-50 cursor-not-allowed' : ''}`}
                  title={`Toggle ${toggle.label.toLowerCase()}`}
                >
                  <span
                    className={`h-3.5 w-3.5 rounded-sm flex items-center justify-center ${toggle.value ? 'bg-[var(--text-primary)] text-[var(--bg-primary)]' : 'bg-[var(--bg-primary)] text-[var(--text-secondary)]'}`}
                    aria-hidden="true"
                  >
                    {toggle.value && (
                      <svg className="h-2.5 w-2.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M5 12l4 4L19 7" />
                      </svg>
                    )}
                  </span>
                  {toggle.label}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="flex flex-1 min-h-0">
          <aside className="w-64 border-r border-[var(--border-primary)] bg-[var(--bg-primary)] flex flex-col">
            <div className="p-3">
              <input
                type="search"
                aria-label="Filter run steps"
                className="w-full rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)]"
                placeholder="Filter steps..."
                value={stepSearch}
                onChange={event => setStepSearch(event.target.value)}
              />
            </div>
            <div className="flex-1 overflow-auto px-2 pb-2 space-y-2">
              {filteredStepItems.map(item => {
                const active = selectedSteps.has(item.name);
                const meta = getStatusMeta(item.status, true);
                const color = stepColorMap.get(item.name);
                return (
                  <button
                    key={item.name}
                    type="button"
                    className={`w-full text-left px-3 py-2 rounded-lg border border-[var(--border-primary)] flex items-center justify-between gap-2 ${active ? 'bg-[var(--bg-secondary)]' : 'bg-[var(--bg-primary)] hover:bg-[var(--bg-secondary)]'}`}
                    onClick={() => toggleStep(item.name)}
                    title={item.name}
                  >
                    <span className="text-sm text-[var(--text-primary)] truncate flex items-center gap-2">
                      {active && color && <span className={`h-2.5 w-2.5 rounded-full ${color.dotClass}`} aria-hidden="true" />}
                      <span className="truncate">{item.name}</span>
                    </span>
                    {item.status && <span className={`text-[10px] px-2 py-1 rounded-full border ${meta.pillClass}`}>{meta.text}</span>}
                  </button>
                );
              })}
              {!filteredStepItems.length && (
                <div className="text-xs text-[var(--text-secondary)] px-3 py-2">No steps found.</div>
              )}
            </div>
            <div className="border-t border-[var(--border-primary)] p-3 flex items-center gap-2 justify-between">
              <button
                type="button"
                className="runner-pill runner-pill--ghost text-xs"
                onClick={() => setSelectedSteps(new Set(stepItems.map(item => item.name)))}
                disabled={!stepItems.length}
              >
                All
              </button>
              <button type="button" className="runner-pill runner-pill--ghost text-xs" onClick={() => setSelectedSteps(new Set())}>
                Clear
              </button>
            </div>
          </aside>

          <section className="flex-1 flex flex-col bg-[var(--bg-secondary)] min-h-0 min-w-0">
            <div className="flex items-center gap-3 px-5 py-3 border-b border-[var(--border-primary)] bg-[var(--bg-primary)]">
              {error && <div className="text-red-500 text-sm" role="alert">{error}</div>}
              {hasUnseen && !follow && (
                <button
                  type="button"
                  className="runner-pill runner-pill--ghost text-xs"
                  onClick={() => {
                    setFollow(true);
                    const container = logContainerRef.current;
                    if (container) container.scrollTop = container.scrollHeight;
                    setHasUnseen(false);
                  }}
                >
                  Jump to latest
                </button>
              )}
              <button className="runner-pill runner-pill--ghost text-xs" type="button" onClick={handleResetFilters}>
                Reset filters
              </button>
              {loading && <span className="text-[var(--text-secondary)] text-xs" role="status">Fetching new logs…</span>}
            </div>
            <div
              ref={logContainerRef}
              onScroll={handleScroll}
              role="log"
              aria-live="polite"
              aria-label={`Logs for ${runName || runId}`}
              className={`flex-1 overflow-y-auto overflow-x-auto px-5 py-4 font-mono text-sm space-y-1 ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre'} bg-[var(--bg-secondary)] min-w-0`}
            >
              {loading && !lines.length && <div className="text-[var(--text-secondary)]">Loading…</div>}
              {!loading && visibleLines.length === 0 && <div className="text-[var(--text-secondary)]">No log lines match the current filters.</div>}
              {visibleLines.map(line => {
                const level = normalizeRunLogLevel(line.level);
                const isAgent = isAgentRunLogLine(line);
                const levelLabel = isAgent ? 'AGENT' : level.toUpperCase();
                const rawLine = line.line || '';
                const stepColor = line.step ? stepColorMap.get(line.step) : undefined;
                const content = structured
                  ? (() => {
                      const jsonStart = rawLine.indexOf('{');
                      if (jsonStart !== -1) {
                        try {
                          const parsed = JSON.parse(rawLine.slice(jsonStart));
                          return JSON.stringify(parsed, null, 2);
                        } catch {
                          return rawLine;
                        }
                      }
                      return rawLine;
                    })()
                  : rawLine;
                if (shortView) {
                  const messageOnly = (() => {
                    try {
                      const jsonStart = rawLine.indexOf('{');
                      if (jsonStart !== -1) {
                        const parsed = JSON.parse(rawLine.slice(jsonStart));
                        const msg = parsed.message ?? parsed.msg ?? parsed.output ?? '';
                        if (msg) {
                          return typeof msg === 'string' ? msg : JSON.stringify(msg);
                        }
                      }
                    } catch {
                      // ignore
                    }
                    return content || '';
                  })();
                  return (
                    <div
                      key={line.id}
                      className={`flex items-start gap-3 rounded-lg px-2 py-1 hover:bg-[var(--bg-primary)] ${stepColor ? `border-l-4 ${stepColor.lineClass}` : ''}`}
                    >
                      <span className="text-[var(--text-secondary)] text-xs w-20 flex-shrink-0">{formatTime(line.timestamp)}</span>
                      <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold ${levelTone(levelLabel)}`}>
                        {levelLabel}
                      </span>
                      <pre
                        className={`flex-1 text-[var(--text-primary)] leading-6 ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre min-w-max'}`}
                      >
                        {messageOnly || '—'}
                      </pre>
                    </div>
                  );
                }
                return (
                  <div key={line.id} className="flex items-start gap-3 rounded-lg px-2 py-1 hover:bg-[var(--bg-primary)]">
                    <span className="text-[var(--text-secondary)] text-xs w-20 flex-shrink-0">{formatTime(line.timestamp)}</span>
                    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-semibold ${levelTone(levelLabel)}`}>
                      {levelLabel}
                    </span>
                    {line.step && (
                      <span
                        className={`inline-flex items-center px-2 py-0.5 rounded-full border text-[11px] font-semibold ${
                          stepColor ? stepColor.pillClass : 'bg-[var(--bg-primary)] border-[var(--border-primary)] text-[var(--text-primary)]'
                        }`}
                      >
                        {line.step}
                      </span>
                    )}
                    <pre
                      className={`flex-1 text-[var(--text-primary)] leading-6 ${wrap ? 'whitespace-pre-wrap break-words' : 'whitespace-pre min-w-max'}`}
                    >
                      {content}
                    </pre>
                  </div>
                );
              })}
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}
