function ScopesPage() {
  return (
    <div data-page="scopes" className="h-full flex flex-col">
      <div className="px-6 pt-6 pb-4 flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold">Scopes</h2>
          <p className="text-sm text-[var(--text-secondary)]">Organize runners and pipelines by scope.</p>
        </div>
        <button className="glass-button-primary" type="button">New scope</button>
      </div>
      <div className="flex-1 overflow-auto px-6 pb-8">
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-2">
            <h3 className="text-lg font-semibold">Prod</h3>
            <p className="text-sm text-[var(--text-secondary)]">High-trust runners for protected branches.</p>
          </div>
          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-2">
            <h3 className="text-lg font-semibold">Staging</h3>
            <p className="text-sm text-[var(--text-secondary)]">Shared runners for integration testing.</p>
          </div>
          <div className="glass-card p-5 border border-[var(--border-primary)] rounded-xl space-y-2">
            <h3 className="text-lg font-semibold">QA</h3>
            <p className="text-sm text-[var(--text-secondary)]">Isolated runs for nightly suites.</p>
          </div>
        </div>
      </div>
    </div>
  );
}

export default ScopesPage;
