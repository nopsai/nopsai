import { useCallback, useEffect, useMemo, useRef, useState, type Ref, type UIEvent } from 'react';
import { X } from 'lucide-react';
import { useDialogFocus } from './useDialogFocus';
import {
  buildConfigRepositoryContentDiff,
  changedConfigRepositoryDriftItems,
  type ConfigRepositoryCommitResponse,
  type ConfigRepositoryDriftLine,
  type ConfigRepositoryDriftResponse,
} from '../lib/configRepositoryDrift';

function statusTone(status: string) {
  switch (status) {
    case 'added':
      return 'border-emerald-500/40 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200';
    case 'modified':
      return 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-200';
    case 'deleted':
      return 'border-red-500/40 bg-red-500/10 text-red-700 dark:text-red-200';
    default:
      return 'border-[var(--border-primary)] bg-[var(--bg-tertiary)] text-[var(--text-secondary)]';
  }
}

function contentValue(value?: string | null) {
  return typeof value === 'string' ? value : '';
}

function lineTone(kind: ConfigRepositoryDriftLine['kind']) {
  switch (kind) {
    case 'added':
      return 'bg-emerald-500/15 text-emerald-950 dark:text-emerald-50';
    case 'removed':
      return 'bg-red-500/15 text-red-950 dark:text-red-50';
    default:
      return 'text-[var(--text-secondary)]';
  }
}

function shortCommit(sha?: string) {
  return sha ? sha.slice(0, 12) : '';
}

function DriftCodePane({
  label,
  lines,
  emptyLabel,
  paneRef,
  onScroll,
}: {
  label: string;
  lines: ConfigRepositoryDriftLine[];
  emptyLabel: string;
  paneRef?: Ref<HTMLDivElement>;
  onScroll?: (event: UIEvent<HTMLDivElement>) => void;
}) {
  const scrollClassName = 'h-[52vh] min-h-[360px] max-h-[620px] overflow-auto';
  if (lines.length === 0) {
    return (
      <div ref={paneRef} onScroll={onScroll} className={`${scrollClassName} p-3 font-mono text-xs text-[var(--text-secondary)]`}>
        {emptyLabel}
      </div>
    );
  }
  return (
    <div ref={paneRef} onScroll={onScroll} className={`${scrollClassName} py-2 font-mono text-xs`} aria-label={`${label} highlighted drift`}>
      {lines.map((line, index) => (
        <div
          key={`${line.number}-${index}-${line.kind}`}
          className={`grid min-w-max grid-cols-[3.25rem_minmax(28rem,1fr)] gap-3 px-3 py-0.5 ${lineTone(line.kind)}`}
          data-drift-line-kind={line.kind}
        >
          <span className="select-none text-right tabular-nums text-[var(--text-muted)]">{line.number}</span>
          <code className="whitespace-pre">{line.text || ' '}</code>
        </div>
      ))}
    </div>
  );
}

export function ConfigRepositoryDriftModal({
  title,
  drift,
  loading,
  error,
  pushing,
  pushResult,
  canPush,
  onClose,
  onRefresh,
  onPush,
}: {
  title: string;
  drift: ConfigRepositoryDriftResponse | null;
  loading: boolean;
  error: string | null;
  pushing: boolean;
  pushResult: ConfigRepositoryCommitResponse | null;
  canPush: boolean;
  onClose: () => void;
  onRefresh: () => Promise<void>;
  onPush: () => Promise<void>;
}) {
  const changedItems = useMemo(() => changedConfigRepositoryDriftItems(drift), [drift]);
  const [selectedPath, setSelectedPath] = useState('');
  const selected = changedItems.find(item => item.path === selectedPath) ?? changedItems[0] ?? null;
  const selectedDiff = useMemo(() => (selected ? buildConfigRepositoryContentDiff(selected) : null), [selected]);
  const summary = drift?.summary ?? {};
  const gitOnlyCount = summary.deleted ?? changedItems.filter(item => item.status === 'deleted').length;
  const pushDisabled = loading || pushing || !canPush || changedItems.length === 0 || Boolean(pushResult);
  const dialogRef = useDialogFocus(onClose);
  const gitPaneRef = useRef<HTMLDivElement | null>(null);
  const nopsaiPaneRef = useRef<HTMLDivElement | null>(null);
  const syncingScrollRef = useRef(false);
  const syncDriftPaneScroll = useCallback((sourcePane: HTMLDivElement | null, targetPane: HTMLDivElement | null) => {
    if (syncingScrollRef.current || !sourcePane || !targetPane) return;
    const nextScrollTop = sourcePane.scrollTop;
    const nextScrollLeft = sourcePane.scrollLeft;
    if (targetPane.scrollTop === nextScrollTop && targetPane.scrollLeft === nextScrollLeft) return;
    syncingScrollRef.current = true;
    targetPane.scrollTop = nextScrollTop;
    targetPane.scrollLeft = nextScrollLeft;
    syncingScrollRef.current = false;
  }, []);
  const syncNopsaiToGit = useCallback((event: UIEvent<HTMLDivElement>) => {
    syncDriftPaneScroll(event.currentTarget, gitPaneRef.current);
  }, [syncDriftPaneScroll]);
  const syncGitToNopsai = useCallback((event: UIEvent<HTMLDivElement>) => {
    syncDriftPaneScroll(event.currentTarget, nopsaiPaneRef.current);
  }, [syncDriftPaneScroll]);

  useEffect(() => {
    [gitPaneRef.current, nopsaiPaneRef.current].forEach(pane => {
      if (!pane) return;
      pane.scrollTop = 0;
      pane.scrollLeft = 0;
    });
  }, [selected?.path]);

  return (
    <div
      className="fixed inset-0 z-[70] flex items-center justify-center bg-[var(--bg-overlay)] px-4 py-6"
      onPointerDown={event => {
        if (!pushing && event.target === event.currentTarget) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className="flex max-h-[94vh] w-full max-w-7xl flex-col overflow-hidden rounded-lg border border-[var(--border-primary)] bg-white shadow-xl dark:bg-slate-900"
        role="dialog"
        aria-modal="true"
        aria-labelledby="config-repository-drift-title"
        tabIndex={-1}
      >
        <div className="flex items-start justify-between gap-3 border-b border-[var(--border-primary)] px-4 py-3">
          <div>
            <p className="text-xs uppercase tracking-wide text-[var(--text-secondary)] font-semibold">Config Repository Drift</p>
            <h3 id="config-repository-drift-title" className="text-lg font-semibold text-[var(--text-primary)]">{title}</h3>
            {drift && (
              <p className="text-xs text-[var(--text-secondary)]">
                {drift.base_branch || 'main'} to {drift.push_branch || 'push branch'}
              </p>
            )}
          </div>
          <button type="button" className="pipelines-icon-only" aria-label="Close" onClick={onClose} data-dialog-initial-focus>
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-5 space-y-4">
          <div className="flex flex-wrap gap-2 text-xs">
            {(['added', 'modified', 'deleted', 'unchanged'] as const).map(status => (
              <span key={status} className={`rounded-full border px-2 py-1 font-medium ${statusTone(status)}`}>
                {status}: {summary[status] ?? 0}
              </span>
            ))}
          </div>

          {loading && <div className="text-sm text-[var(--text-secondary)]">Checking drift...</div>}

          {error && (
            <div className="rounded-lg border border-red-500/30 px-4 py-3 text-sm text-red-600 dark:text-red-300">
              {error}
            </div>
          )}

          {pushResult && (
            <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-200">
              Pushed {pushResult.files_changed ?? changedItems.length} file{(pushResult.files_changed ?? changedItems.length) === 1 ? '' : 's'} to {pushResult.branch || drift?.push_branch || 'the push branch'}
              {pushResult.commit_sha ? ` at ${shortCommit(pushResult.commit_sha)}` : ''}.
              {pushResult.commit_url && (
                <>
                  {' '}
                  <a className="underline" href={pushResult.commit_url} target="_blank" rel="noreferrer">
                    View commit
                  </a>
                </>
              )}
            </div>
          )}

          {!loading && drift && changedItems.length === 0 && !error && (
            <div className="rounded-lg border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-4 py-6 text-sm text-[var(--text-secondary)]">
              No drift found between Nopsai and the sync branch.
            </div>
          )}

          {!loading && drift && gitOnlyCount > 0 && !error && (
            <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-200">
              {gitOnlyCount} file{gitOnlyCount === 1 ? '' : 's'} exist in Git but do not have a matching Nopsai resource. If that is unexpected, sync the matching config repository and check its sync status before pushing.
            </div>
          )}

          {changedItems.length > 0 && (
            <div className="grid min-h-[520px] grid-cols-1 gap-4 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.7fr)]">
              <div className="overflow-hidden rounded-lg border border-[var(--border-primary)]">
                <div className="border-b border-[var(--border-primary)] px-3 py-2 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
                  Files
                </div>
                <div className="max-h-[700px] overflow-y-auto">
                  {changedItems.map(item => (
                    <button
                      key={item.path}
                      type="button"
                      className={`flex w-full items-center justify-between gap-3 border-b border-[var(--border-primary)] px-3 py-2 text-left text-sm last:border-b-0 ${selected?.path === item.path ? 'bg-[var(--bg-secondary)]' : 'hover:bg-[var(--bg-tertiary)]'}`}
                      onClick={() => setSelectedPath(item.path)}
                    >
                      <span className="min-w-0 flex-1 truncate text-[var(--text-primary)]" title={item.path}>{item.path}</span>
                      <span className={`shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-medium ${statusTone(item.status)}`}>{item.status}</span>
                    </button>
                  ))}
                </div>
              </div>

              {selected && (
                <div className="min-w-0 rounded-lg border border-[var(--border-primary)]">
                  <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border-primary)] px-3 py-2">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold text-[var(--text-primary)]" title={selected.path}>{selected.path}</p>
                      <p className="text-xs text-[var(--text-secondary)]">
                        {selected.delete ? 'Will be deleted' : 'Will be written to Git'}
                        {selectedDiff && selectedDiff.changedLines > 0 ? ` - ${selectedDiff.changedLines} highlighted line${selectedDiff.changedLines === 1 ? '' : 's'}` : ''}
                      </p>
                    </div>
                    <span className={`rounded-full border px-2 py-1 text-xs font-medium ${statusTone(selected.status)}`}>{selected.status}</span>
                  </div>
                  <div className="grid grid-cols-1 gap-0 lg:grid-cols-2">
                    <div className="min-w-0 border-b border-[var(--border-primary)] lg:border-b-0 lg:border-r">
                      <div className="border-b border-[var(--border-primary)] px-3 py-2 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
                        Git
                      </div>
                      <DriftCodePane
                        label="Git"
                        lines={selectedDiff?.git ?? []}
                        emptyLabel={contentValue(selected.git_content) || '(no file)'}
                        paneRef={gitPaneRef}
                        onScroll={syncGitToNopsai}
                      />
                    </div>
                    <div className="min-w-0">
                      <div className="border-b border-[var(--border-primary)] px-3 py-2 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">
                        Nopsai
                      </div>
                      <DriftCodePane
                        label="Nopsai"
                        lines={selectedDiff?.desired ?? []}
                        emptyLabel={selected.delete ? '(deleted)' : contentValue(selected.desired_content) || '(no file)'}
                        paneRef={nopsaiPaneRef}
                        onScroll={syncNopsaiToGit}
                      />
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-end gap-2 border-t border-[var(--border-primary)] px-5 py-4">
          {drift && !drift.can_push && (
            <span className="mr-auto text-xs text-[var(--text-secondary)]">Enable Git push and set a push branch before committing changes.</span>
          )}
          <button type="button" className="glass-button-subtle" onClick={() => void onRefresh()} disabled={loading || pushing}>
            Refresh
          </button>
          <button type="button" className="glass-button-subtle" onClick={onClose} disabled={pushing}>
            Close
          </button>
          <button type="button" className="glass-button-primary" onClick={() => void onPush()} disabled={pushDisabled}>
            {pushing ? 'Pushing...' : pushResult ? 'Pushed' : 'Push to Git'}
          </button>
        </div>
      </div>
    </div>
  );
}
