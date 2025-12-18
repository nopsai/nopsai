function LabPage() {
  return (
    <div data-page="lab" className="h-full flex flex-col">
      <div className="px-6 pt-6 pb-4 flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-semibold">Lab</h2>
          <p className="text-sm text-[var(--text-secondary)]">Experiment with new features and prototypes.</p>
        </div>
        <button className="glass-button-primary" type="button">New experiment</button>
      </div>
      <div className="flex-1 overflow-auto px-6 pb-8">
        <div className="glass-card p-6 border border-[var(--border-primary)] rounded-xl">
          <h3 className="text-lg font-semibold mb-2">Playground</h3>
          <p className="text-sm text-[var(--text-secondary)]">This area mirrors the legacy Lab page and will host experimental tools as they are ported.</p>
        </div>
      </div>
    </div>
  );
}

export default LabPage;
