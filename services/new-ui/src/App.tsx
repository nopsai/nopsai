import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { HashRouter, NavLink, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import PipelineRunsPage from './pages/PipelineRuns';
import PipelinesPage from './pages/Pipelines';
import TriggersPage from './pages/Triggers';
import ScopesPage from './pages/Scopes';
import LabPage from './pages/Lab';
import StepsPage from './pages/Steps';
import SystemPage from './pages/System';
import { buildApiUrl } from './lib/api';
import { PIPELINE_DRAFTS_CHANGED_EVENT, PIPELINE_DRAFTS_STORAGE_KEY, loadPipelineDrafts } from './lib/pipelineDrafts';
import { STEP_DRAFTS_CHANGED_EVENT, STEP_DRAFTS_STORAGE_KEY, loadStepDrafts } from './lib/stepDrafts';

type Theme = 'light' | 'dark';

type NavItem = {
  label: string;
  path: string;
  icon: ReactNode;
};

type PipelineTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: PipelineTreeNode[];
  pipelineIds: string[];
};

type TriggerTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: TriggerTreeNode[];
  triggerSlugs: string[];
};

type StepTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: StepTreeNode[];
  stepIds: string[];
};

const navItems: NavItem[] = [
  {
    label: 'Pipeline runs',
    path: '/pipelineruns/main',
    icon: <IconPlay />, 
  },
  {
    label: 'Pipelines',
    path: '/pipelines',
    icon: <IconFlow />, 
  },
  {
    label: 'Triggers',
    path: '/triggers',
    icon: <IconBell />, 
  },
  {
    label: 'Scopes',
    path: '/scopes',
    icon: <IconGlobe />, 
  },
  {
    label: 'Lab',
    path: '/lab',
    icon: <IconFlask />, 
  },
  {
    label: 'Steps',
    path: '/steps',
    icon: <IconSteps />,
  },
  {
    label: 'System',
    path: '/system/config',
    icon: <IconCog />, 
  },
];

const titleMap: Record<string, string> = {
  pipelineruns: 'Pipeline runs',
  pipelines: 'Pipelines',
  triggers: 'Triggers',
  scopes: 'Scopes',
  lab: 'Lab',
  steps: 'Steps',
  system: 'System',
};

const getInitialTheme = (): Theme => {
  if (typeof window === 'undefined') return 'light';
  const stored = localStorage.getItem('theme');
  if (stored === 'dark' || stored === 'light') return stored;
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
};

function App() {
  return (
    <HashRouter>
      <AppShell />
    </HashRouter>
  );
}

function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const [theme, setTheme] = useState<Theme>(getInitialTheme);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [pipelines, setPipelines] = useState<string[]>([]);
  const serverPipelinesRef = useRef<string[]>([]);
  const [pipelineTreeOpen, setPipelineTreeOpen] = useState<Set<string>>(new Set());

  const [triggers, setTriggers] = useState<string[]>([]);
  const [triggerTreeOpen, setTriggerTreeOpen] = useState<Set<string>>(new Set());

  const [steps, setSteps] = useState<string[]>([]);
  const serverStepsRef = useRef<string[]>([]);
  const [stepTreeOpen, setStepTreeOpen] = useState<Set<string>>(new Set());

  useEffect(() => {
    const root = document.documentElement;
    if (theme === 'dark') {
      root.classList.add('dark');
      localStorage.setItem('theme', 'dark');
    } else {
      root.classList.remove('dark');
      localStorage.setItem('theme', 'light');
    }
  }, [theme]);

  useEffect(() => {
    if (!sidebarOpen) return;
    const handle = window.setTimeout(() => setSidebarOpen(false), 0);
    return () => window.clearTimeout(handle);
  }, [location.pathname, sidebarOpen]);

  useEffect(() => {
    const load = async () => {
      try {
        const response = await fetch(buildApiUrl('/v1/pipelines'));
        if (!response.ok) return;
        const payload = await response.json();
        const asRecord = (value: unknown): Record<string, unknown> | null => {
          if (!value || typeof value !== 'object') return null;
          return value as Record<string, unknown>;
        };
        const ids = Array.isArray(payload)
          ? payload
              .map((item: unknown) => {
                if (typeof item === 'string') return item;
                const record = asRecord(item);
                if (!record) return '';
                if (typeof record.id === 'string') return record.id;
                if (typeof record.identifier === 'string') return record.identifier;
                return '';
              })
              .filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        serverPipelinesRef.current = ids;
        const draftIds = loadPipelineDrafts().map(draft => draft.id);
        const merged = Array.from(new Set([...ids, ...draftIds])).sort((a, b) => a.localeCompare(b));
        setPipelines(merged);
      } catch (error) {
        console.warn('Failed to load pipelines for sidebar', error);
      }
    };
    if (location.pathname.startsWith('/pipelines')) {
      void load();
    }
  }, [location.pathname]);

  useEffect(() => {
    const load = async () => {
      try {
        const response = await fetch(buildApiUrl('/v1/overrides'));
        if (!response.ok) return;
        const payload = await response.json();
        const slugs = Array.isArray(payload)
          ? payload.map((item: unknown) => (typeof item === 'string' ? item.trim() : '')).filter(Boolean)
          : [];
        slugs.sort((a: string, b: string) => a.localeCompare(b));
        setTriggers(slugs);
      } catch (error) {
        console.warn('Failed to load triggers for sidebar', error);
      }
    };
    if (location.pathname.startsWith('/triggers')) {
      void load();
    }
  }, [location.pathname]);

  useEffect(() => {
    const load = async () => {
      try {
        const response = await fetch(buildApiUrl('/v1/steps'));
        if (!response.ok) return;
        const payload = await response.json();
        const ids = Array.isArray(payload)
          ? payload.map((item: unknown) => (typeof item === 'string' ? item.trim() : '')).filter(Boolean)
          : [];
        ids.sort((a, b) => a.localeCompare(b));
        serverStepsRef.current = ids;
        const draftIds = loadStepDrafts().map(draft => draft.id);
        const merged = Array.from(new Set([...ids, ...draftIds])).sort((a, b) => a.localeCompare(b));
        setSteps(merged);
      } catch (error) {
        console.warn('Failed to load steps for sidebar', error);
      }
    };
    if (location.pathname.startsWith('/steps')) {
      void load();
    }
  }, [location.pathname]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const handleDraftsChanged = () => {
      if (!location.pathname.startsWith('/pipelines')) return;
      const draftIds = loadPipelineDrafts().map(draft => draft.id);
      const merged = Array.from(new Set([...serverPipelinesRef.current, ...draftIds])).sort((a, b) => a.localeCompare(b));
      setPipelines(merged);
    };
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== PIPELINE_DRAFTS_STORAGE_KEY) return;
      handleDraftsChanged();
    };
    window.addEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
    window.addEventListener('storage', handleStorage);
    return () => {
      window.removeEventListener(PIPELINE_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
      window.removeEventListener('storage', handleStorage);
    };
  }, [location.pathname]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const handleDraftsChanged = () => {
      if (!location.pathname.startsWith('/steps')) return;
      const draftIds = loadStepDrafts().map(draft => draft.id);
      const merged = Array.from(new Set([...serverStepsRef.current, ...draftIds])).sort((a, b) => a.localeCompare(b));
      setSteps(merged);
    };
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== STEP_DRAFTS_STORAGE_KEY) return;
      handleDraftsChanged();
    };
    window.addEventListener(STEP_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
    window.addEventListener('storage', handleStorage);
    return () => {
      window.removeEventListener(STEP_DRAFTS_CHANGED_EVENT, handleDraftsChanged);
      window.removeEventListener('storage', handleStorage);
    };
  }, [location.pathname]);

  const title = useMemo(() => {
    const key = location.pathname.split('/').filter(Boolean)[0] || 'pipelineruns';
    return titleMap[key] || 'Dashboard';
  }, [location.pathname]);

  const splitIdentifier = (id: string) => {
    const parts = id.split('/').filter(Boolean);
    const name = decodeURIComponent(parts.pop() || '');
    const path = parts.map(decodeURIComponent).join('/');
    return { name, path };
  };

  const buildTree = useMemo(() => {
    const root: PipelineTreeNode = { id: '__root__', name: 'All pipelines', fullPath: '', children: [], pipelineIds: [] };
    pipelines.forEach(id => {
      const parts = id.split('/').filter(Boolean);
      const pipelineName = parts.pop();
      if (!pipelineName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], pipelineIds: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.pipelineIds.push(id);
      current.pipelineIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [pipelines]);

  const buildTriggerTree = useMemo(() => {
    const root: TriggerTreeNode = { id: '__root__', name: 'All triggers', fullPath: '', children: [], triggerSlugs: [] };
    triggers.forEach(slug => {
      const parts = slug.split('/').filter(Boolean);
      const repoName = parts.pop();
      if (!repoName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], triggerSlugs: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.triggerSlugs.push(slug);
      current.triggerSlugs.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [triggers]);

  const buildStepTree = useMemo(() => {
    const root: StepTreeNode = { id: '__root__', name: 'All steps', fullPath: '', children: [], stepIds: [] };
    steps.forEach(id => {
      const parts = id.split('/').filter(Boolean);
      const stepName = parts.pop();
      if (!stepName) return;
      let current = root;
      let pathSoFar = '';
      parts.forEach(segment => {
        pathSoFar = pathSoFar ? `${pathSoFar}/${segment}` : segment;
        let child = current.children.find(c => c.name === segment);
        if (!child) {
          child = { id: pathSoFar, name: segment, fullPath: pathSoFar, children: [], stepIds: [] };
          current.children.push(child);
          current.children.sort((a, b) => a.name.localeCompare(b.name));
        }
        current = child;
      });
      current.stepIds.push(id);
      current.stepIds.sort((a, b) => a.localeCompare(b));
    });
    return root;
  }, [steps]);

  const handleTogglePipelineNode = (id: string) => {
    setPipelineTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleToggleTriggerNode = (id: string) => {
    setTriggerTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleToggleStepNode = (id: string) => {
    setStepTreeOpen(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <div className="min-h-screen bg-[var(--bg-primary)] text-[var(--text-primary)]">
      <div id="hover-hint" aria-hidden="true"></div>
      <div className="flex h-screen overflow-hidden">
          <Sidebar
            navItems={navItems}
            open={sidebarOpen}
            onClose={() => setSidebarOpen(false)}
            pipelineTree={buildTree}
            pipelineTreeOpen={pipelineTreeOpen}
            onTogglePipelineNode={handleTogglePipelineNode}
            triggerTree={buildTriggerTree}
            triggerTreeOpen={triggerTreeOpen}
            onToggleTriggerNode={handleToggleTriggerNode}
            stepTree={buildStepTree}
            stepTreeOpen={stepTreeOpen}
            onToggleStepNode={handleToggleStepNode}
            splitIdentifier={splitIdentifier}
            locationPathname={location.pathname}
            onSelectPipelineFolder={path => navigate(path ? `/pipelines?folder=${encodeURIComponent(path)}` : '/pipelines')}
            onSelectTriggerFolder={path => navigate(path ? `/triggers?folder=${encodeURIComponent(path)}` : '/triggers')}
            onSelectStepFolder={path => navigate(path ? `/steps?folder=${encodeURIComponent(path)}` : '/steps')}
          />
        <div
          id="sidebar-resizer"
          className="w-1.5 cursor-col-resize flex-shrink-0 bg-[var(--bg-tertiary)] hover:bg-[var(--border-accent)] transition-colors duration-200 hidden sm:block"
        ></div>
        <main className="flex-1 flex flex-col overflow-hidden">
          <Header
            title={title}
            theme={theme}
            onToggleTheme={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            onOpenSidebar={() => setSidebarOpen(true)}
          />
          <div id="page-content-wrapper" className="flex-1 overflow-auto">
            <Routes>
              <Route path="/" element={<Navigate to="/pipelineruns/main" replace />} />
              <Route path="/pipelineruns/:tab?" element={<PipelineRunsPage />} />
              <Route path="/pipelines/*" element={<PipelinesPage />} />
              <Route path="/triggers/*" element={<TriggersPage />} />
              <Route path="/scopes/*" element={<ScopesPage />} />
              <Route path="/lab/*" element={<LabPage />} />
              <Route path="/steps/*" element={<StepsPage />} />
              <Route path="/system/:tab?" element={<SystemPage />} />
              <Route path="*" element={<Navigate to="/pipelineruns/main" replace />} />
            </Routes>
          </div>
        </main>
      </div>
      <div id="toast-container" className="fixed top-6 right-6 z-[100] w-full max-w-sm space-y-3"></div>
    </div>
  );
}

function Sidebar({
  navItems,
  open,
  onClose,
  pipelineTree,
  pipelineTreeOpen,
  onTogglePipelineNode,
  triggerTree,
  triggerTreeOpen,
  onToggleTriggerNode,
  stepTree,
  stepTreeOpen,
  onToggleStepNode,
  splitIdentifier,
  locationPathname,
  onSelectPipelineFolder,
  onSelectTriggerFolder,
  onSelectStepFolder,
}: {
  navItems: NavItem[];
  open: boolean;
  onClose: () => void;
  pipelineTree: PipelineTreeNode;
  pipelineTreeOpen: Set<string>;
  onTogglePipelineNode: (id: string) => void;
  triggerTree: TriggerTreeNode;
  triggerTreeOpen: Set<string>;
  onToggleTriggerNode: (id: string) => void;
  stepTree: StepTreeNode;
  stepTreeOpen: Set<string>;
  onToggleStepNode: (id: string) => void;
  splitIdentifier: (id: string) => { name: string; path: string };
  locationPathname: string;
  onSelectPipelineFolder: (path: string) => void;
  onSelectTriggerFolder: (path: string) => void;
  onSelectStepFolder: (path: string) => void;
}) {
  const isPipelinesRoute = locationPathname.startsWith('/pipelines');
  const isTriggersRoute = locationPathname.startsWith('/triggers');
  const isStepsRoute = locationPathname.startsWith('/steps');
  const searchParams = new URLSearchParams(location.search);
  const activeFolder = searchParams.get('folder') || '';

  const renderPipelineTreeNode = (node: PipelineTreeNode) => {
    const isOpen = pipelineTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={node.id} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item">
            <button className="pipeline-tree-toggle" onClick={() => onTogglePipelineNode(node.id)} aria-label="Toggle folder">
              <span className="text-sm">{isOpen ? '▾' : '▸'}</span>
            </button>
            <button
              className={`pipeline-tree-folder ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onTogglePipelineNode(node.id);
                onSelectPipelineFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderPipelineTreeNode(child))}
            {node.pipelineIds.map(pid => {
              const { name } = splitIdentifier(pid);
              const active = locationPathname.includes(`/pipelines/${pid}`);
              return (
                <li key={pid} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink className="pipeline-tree-leaf-btn" to={`/pipelines/${pid.split('/').map(encodeURIComponent).join('/')}`}>
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <circle cx="12" cy="12" r="2.5" />
                        <path d="M4 12h3m10 0h3M12 4v3m0 10v3" />
                      </svg>
                    </span>
                    <span className="truncate">{name || pid}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  const renderTriggerTreeNode = (node: TriggerTreeNode) => {
    const isOpen = triggerTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={`tr-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item">
            <button className="pipeline-tree-toggle" onClick={() => onToggleTriggerNode(node.id)} aria-label="Toggle folder">
              <span className="text-sm">{isOpen ? '▾' : '▸'}</span>
            </button>
            <button
              className={`pipeline-tree-folder ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleTriggerNode(node.id);
                onSelectTriggerFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderTriggerTreeNode(child))}
            {node.triggerSlugs.map(slug => {
              const { name } = splitIdentifier(slug);
              const active = locationPathname.includes(`/triggers/${slug}`);
              return (
                <li key={`slug-${slug}`} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink className="pipeline-tree-leaf-btn" to={`/triggers/${slug.split('/').map(encodeURIComponent).join('/')}`}>
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
                      </svg>
                    </span>
                    <span className="truncate">{name || slug}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  const renderStepTreeNode = (node: StepTreeNode) => {
    const isOpen = stepTreeOpen.has(node.id);
    const isRoot = node.id === '__root__';
    const isActiveFolder = activeFolder === node.fullPath;
    return (
      <li key={`step-${node.id}`} className="pipeline-tree-row">
        {!isRoot && (
          <div className="pipeline-tree-item">
            <button className="pipeline-tree-toggle" onClick={() => onToggleStepNode(node.id)} aria-label="Toggle folder">
              <span className="text-sm">{isOpen ? '▾' : '▸'}</span>
            </button>
            <button
              className={`pipeline-tree-folder ${isActiveFolder ? 'active' : ''}`}
              onClick={() => {
                if (!isOpen) onToggleStepNode(node.id);
                onSelectStepFolder(node.fullPath);
              }}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                <path d="M3 7h5l2 2h11v9a2 2 0 0 1-2 2H3z" />
                <path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v2" />
              </svg>
              <span className="truncate">{node.name}</span>
            </button>
          </div>
        )}
        {(isRoot || isOpen) && (
          <ul className="pipeline-tree-children">
            {node.children.map(child => renderStepTreeNode(child))}
            {node.stepIds.map(stepId => {
              const { name } = splitIdentifier(stepId);
              const active = locationPathname.includes(`/steps/${stepId}`);
              return (
                <li key={`step-id-${stepId}`} className={`pipeline-tree-leaf ${active ? 'active' : ''}`}>
                  <NavLink className="pipeline-tree-leaf-btn" to={`/steps/${stepId.split('/').map(encodeURIComponent).join('/')}`}>
                    <span className="pipeline-tree-leaf-icon" aria-hidden="true">
                      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
                        <path d="M12 2l8 4.5v11L12 22 4 17.5v-11L12 2z" />
                        <path d="M12 22v-7.5" />
                        <path d="M20 6.5l-8 4.5-8-4.5" />
                      </svg>
                    </span>
                    <span className="truncate">{name || stepId}</span>
                  </NavLink>
                </li>
              );
            })}
          </ul>
        )}
      </li>
    );
  };

  return (
    <>
      <div
        className={`fixed inset-0 bg-[var(--bg-overlay)] sm:hidden transition-opacity duration-200 ${open ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}
        onClick={onClose}
      ></div>
      <aside
        id="sidebar"
        className={`bg-[var(--bg-secondary)] border-r border-[var(--border-primary)] flex-shrink-0 flex flex-col transition-transform duration-300 ease-in-out h-full z-20 w-72 sidebar-scrollbar overflow-hidden
          ${open ? 'translate-x-0' : '-translate-x-full'} sm:translate-x-0 fixed sm:static`}
      >
        <div className="flex items-center justify-between px-6 h-16 border-b border-[var(--border-primary)] flex-shrink-0">
          <div className="flex items-center gap-3">
            <span className="step-logo step-logo--sidebar step-logo--brand" aria-hidden="true">
              <IconLogo />
            </span>
            <span className="text-xl font-semibold">NopsAI</span>
          </div>
          <button
            id="close-sidebar-btn"
            className="sm:hidden text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
            onClick={onClose}
            aria-label="Close sidebar"
          >
            <IconX />
          </button>
        </div>
        <nav id="sidebar-base-nav" className="px-4 py-4 flex-shrink-0 space-y-1">
          {navItems.map(item => (
            <NavLink
              key={item.path}
              to={item.path}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm transition-colors sidebar-link ${isActive ? 'active text-[var(--text-primary)] bg-[var(--bg-tertiary)]' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-tertiary)]'}`
              }
            >
              <span className="text-[var(--text-secondary)]">{item.icon}</span>
              <span className="truncate">{item.label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="flex-1 overflow-y-auto sidebar-scrollbar border-t border-[var(--border-primary)]">
          <nav id="sidebar-details-nav" className="px-4 py-4 space-y-2">
            {isPipelinesRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All pipelines</p>
                </div>
                <ul className="pipeline-tree-list">
                  {renderPipelineTreeNode(pipelineTree)}
                </ul>
              </div>
            ) : isTriggersRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All triggers</p>
                </div>
                <ul className="pipeline-tree-list">
                  {renderTriggerTreeNode(triggerTree)}
                </ul>
              </div>
            ) : isStepsRoute ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-2">
                  <p className="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wide">All steps</p>
                </div>
                <ul className="pipeline-tree-list">{renderStepTreeNode(stepTree)}</ul>
              </div>
            ) : (
              <p className="text-xs text-[var(--text-secondary)]">Contextual navigation will appear here as features are migrated.</p>
            )}
          </nav>
        </div>
      </aside>
    </>
  );
}

function Header({ title, onOpenSidebar, theme, onToggleTheme }: { title: string; onOpenSidebar: () => void; theme: Theme; onToggleTheme: () => void; }) {
  return (
    <header
      className="flex items-center justify-between px-6 py-4 themed-bg-blur backdrop-blur-sm shadow-sm z-10 border-b border-[var(--border-primary)] flex-shrink-0"
      style={{ paddingTop: '11px' }}
    >
      <button
        id="open-sidebar-btn"
        className="sm:hidden text-[var(--text-secondary)] hover:text-[var(--text-primary)] mr-4"
        onClick={onOpenSidebar}
        aria-label="Open sidebar"
      >
        <IconMenu />
      </button>
      <div id="main-header" className="flex-1 text-xl font-semibold min-w-0 truncate">{title}</div>
      <button
        id="theme-toggle"
        type="button"
        className="ml-4 p-2 rounded-full text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)]"
        aria-label="Toggle theme"
        onClick={onToggleTheme}
      >
        {theme === 'dark' ? <IconSun /> : <IconMoon />}
      </button>
    </header>
  );
}

function IconLogo() {
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" strokeWidth="2" stroke="currentColor" fill="none" strokeLinecap="round" strokeLinejoin="round">
      <path stroke="none" d="M0 0h24v24H0z" />
      <path d="M12.5 21H6a1 1 0 0 1 -1 -1V4a1 1 0 0 1 1 -1h12a1 1 0 0 1 1 1v5" />
      <path d="M18 22v-6" />
      <path d="M21 19l-3 -3l-3 3" />
      <path d="M3 13h8" />
      <path d="M5 10v6" />
    </svg>
  );
}

function IconX() {
  return (
    <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
    </svg>
  );
}

function IconMenu() {
  return (
    <svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16m-7 6h7" />
    </svg>
  );
}

function IconSun() {
  return (
    <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M12 7a5 5 0 100 10 5 5 0 000-10z" />
    </svg>
  );
}

function IconMoon() {
  return (
    <svg className="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z" />
    </svg>
  );
}

function IconPlay() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M14.752 11.168l-4.197-2.42A1 1 0 009 9.58v4.84a1 1 0 001.555.832l4.197-2.42a1 1 0 000-1.664z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

function IconFlow() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />
    </svg>
  );
}

function IconBell() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" />
    </svg>
  );
}

function IconGlobe() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 12c2.28 0 4-1.79 4-4s-1.72-4-4-4-4 1.79-4 4 1.72 4 4 4z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.477 0 8.268 2.943 9.542 7-1.274 4.057-5.065 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
    </svg>
  );
}

function IconFlask() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M10 2h4m-2 0v8m0 0H8m4 0h4m-6 4h4m-6 4h8M6 10h12" />
    </svg>
  );
}

function IconCog() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0a1.724 1.724 0 002.573 1.02c.842-.488 1.91.27 1.662 1.2a1.724 1.724 0 001.091 2.062c.9.3.9 1.603 0 1.902a1.724 1.724 0 00-1.09 2.062c.247.93-.82 1.688-1.663 1.2a1.724 1.724 0 00-2.572 1.02c-.3.921-1.603.921-1.902 0a1.724 1.724 0 00-2.573-1.02c-.842.488-1.91-.27-1.662-1.2a1.724 1.724 0 00-1.091-2.062c-.9-.3-.9-1.603 0-1.902a1.724 1.724 0 001.09-2.062c-.247-.93.82-1.688 1.663-1.2a1.724 1.724 0 002.572-1.02z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
    </svg>
  );
}

function IconSteps() {
  return (
    <svg className="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 2l8 4.5v11L12 22 4 17.5v-11L12 2z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M12 22v-7.5" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M20 6.5l-8 4.5-8-4.5" />
    </svg>
  );
}

export default App;
