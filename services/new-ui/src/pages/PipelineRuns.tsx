import { NavLink } from 'react-router-dom';

const tabs = [
  { id: 'main', label: 'Main' },
  { id: 'recent', label: 'Recent' },
  { id: 'events', label: 'Events' },
];

const sampleRuns = [
  {
    id: 'run-01',
    pipeline: 'Build & Test',
    repo: 'nopsai/example-app',
    branch: 'main',
    status: 'Running',
    duration: '5m 21s',
    trigger: 'manual',
  },
  {
    id: 'run-02',
    pipeline: 'Deploy staging',
    repo: 'nopsai/example-app',
    branch: 'staging',
    status: 'Succeeded',
    duration: '12m 03s',
    trigger: 'commit',
  },
  {
    id: 'run-03',
    pipeline: 'Nightly QA',
    repo: 'nopsai/qa-suite',
    branch: 'nightly',
    status: 'Queued',
    duration: '—',
    trigger: 'schedule',
  },
];

function PipelineRunsPage() {
  return (
    <div data-page="pipelineruns" className="active h-full flex flex-col">
      <div className="px-6 pt-6 flex-shrink-0 tabs-nav-wrapper">
        <div className="border-b border-[var(--border-primary)]">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <nav className="tabs-nav" aria-label="Pipeline run tabs" role="tablist">
              {tabs.map(tab => (
                <NavLink
                  key={tab.id}
                  to={`/pipelineruns/${tab.id}`}
                  role="tab"
                  className={({ isActive }) => `tabs-nav__link ${isActive ? 'tabs-nav__link--active' : ''}`}
                >
                  {tab.label}
                </NavLink>
              ))}
            </nav>
            <div id="pipelineruns-actions" className="flex items-center gap-2 flex-shrink-0 order-1 sm:order-2">
              <button id="pipelineruns-new-folder-btn" type="button" className="glass-button-subtle" aria-label="Create new folder">
                <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 5v14M5 12h14" />
                </svg>
                <span>New Folder</span>
              </button>
            </div>
            <div id="pipelineruns-search-container" className="relative flex-1 min-w-[220px] max-w-xl order-2 sm:order-1">
              <input
                id="pipelineruns-search"
                type="search"
                placeholder="Search runs"
                aria-label="Search runs"
                className="pipelines-input w-full pl-11 pr-10 py-2 text-sm transition"
              />
              <svg className="w-4 h-4 text-[var(--text-secondary)] absolute left-4 top-1/2 -translate-y-1/2" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 21l-4.35-4.35M10 18a8 8 0 110-16 8 8 0 010 16z" />
              </svg>
            </div>
          </div>
          <div id="run-selection-bar" className="hidden mt-3">
            <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded-lg px-4 py-3 text-sm">
              <span id="run-selection-count" className="text-[var(--text-primary)] font-medium">0 runs selected</span>
              <div className="flex items-center gap-2">
                <button id="run-selection-clear-btn" type="button" className="inline-flex items-center px-3 py-1.5 border border-[var(--border-primary)] rounded-md text-[var(--text-primary)] hover:bg-[var(--border-primary)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)] text-xs">Clear</button>
                <button id="run-selection-delete-btn" type="button" className="inline-flex items-center px-3 py-1.5 border border-transparent rounded-md text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500 text-xs disabled:opacity-50 disabled:cursor-not-allowed">Delete Selected</button>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div id="main-content-runs" className="p-6 h-full">
        <div id="placeholder" className="h-full flex flex-col items-center justify-center text-center text-[var(--text-placeholder)]">
          <svg className="h-12 w-12 mb-4 placeholder-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="1" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2 -2v-6a2 2 0 01-2-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
          </svg>
          <h3 className="text-lg font-semibold text-[var(--text-primary)]">Select a pipeline run</h3>
          <p>Choose a run from the sidebar to view its progress.</p>
        </div>
        <div id="run-view-toggle-container" className="hidden mb-4 flex justify-end"></div>
        <div id="main-grid-container" className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {sampleRuns.map(run => (
            <div key={run.id} className="glass-card run-card border border-[var(--border-primary)] rounded-xl p-4 space-y-3">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-xs text-[var(--text-secondary)]">{run.repo}</p>
                  <h3 className="text-lg font-semibold">{run.pipeline}</h3>
                </div>
                <span className="runner-pill runner-pill--muted">{run.status}</span>
              </div>
              <div className="flex items-center gap-2 text-sm text-[var(--text-secondary)]">
                <span className="runner-pill runner-pill--muted">Branch {run.branch}</span>
                <span className="runner-pill runner-pill--muted">Trigger {run.trigger}</span>
                <span className="runner-pill runner-pill--muted">{run.duration}</span>
              </div>
              <button className="glass-button-primary w-full justify-center" type="button">Open run</button>
            </div>
          ))}
        </div>
        <div id="graph-container" className="hidden w-full h-full overflow-hidden cursor-grab">
          <div id="graph-wrapper" className="p-8"></div>
        </div>
        <div id="tasks-graph-container" className="hidden w-full h-full overflow-hidden cursor-grab relative">
          <div id="tasks-graph-wrapper" className="w-full h-full relative">
            <svg id="tasks-graph-connections" className="pointer-events-none absolute inset-0 z-0 w-full h-full"></svg>
          </div>
          <div id="tasks-empty" className="absolute inset-0 hidden"></div>
        </div>
      </div>
    </div>
  );
}

export default PipelineRunsPage;
