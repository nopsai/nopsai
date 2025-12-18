function TriggersPage() {
  return (
    <div data-page="triggers" className="h-full flex flex-col">
      <div className="px-6 pt-6 pb-4 flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold">Triggers</h2>
          <p className="text-sm text-[var(--text-secondary)]">Manage repo events, schedules, and webhooks.</p>
        </div>
        <button className="glass-button-primary" type="button">New trigger</button>
      </div>
      <div className="flex-1 overflow-auto px-6 pb-8">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-2">
            <p className="text-xs text-[var(--text-secondary)]">Git webhook</p>
            <h3 className="text-lg font-semibold">Push to main</h3>
            <p className="text-sm text-[var(--text-secondary)]">Runs Build & Test when commits land on main.</p>
          </div>
          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-2">
            <p className="text-xs text-[var(--text-secondary)]">Scheduler</p>
            <h3 className="text-lg font-semibold">Nightly QA</h3>
            <p className="text-sm text-[var(--text-secondary)]">Runs at 02:00 UTC to validate staging.</p>
          </div>
          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-2">
            <p className="text-xs text-[var(--text-secondary)]">Manual</p>
            <h3 className="text-lg font-semibold">Ad-hoc deployment</h3>
            <p className="text-sm text-[var(--text-secondary)]">Use the Deploy Staging pipeline on demand.</p>
          </div>
        </div>
      </div>
    </div>
  );
}

export default TriggersPage;
