(function (global) {
    let state;
    let DOM;
    let fetchData;
    let postData;
    let deleteData;
    let logsModule;
    let refresh;
    let initialized = false;
    let showLogsModal = () => {};
    let closeLogsModal = () => {};
    let renderLogsWithFilters = () => {};
    let updateLogsStepList = () => {};

    const statusConfig = {
        success: { icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-green-500 dark:text-green-400', rectClass: 'stroke-green-500 fill-green-100 dark:fill-green-500/10' },
        failure: { icon: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-red-500 dark:text-red-400', rectClass: 'stroke-red-500 fill-red-100 dark:fill-red-500/10' },
        running: { icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-blue-500 dark:text-blue-400', rectClass: 'stroke-blue-500 fill-blue-100 dark:fill-blue-500/10 animate-pulse' },
        pending: { icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-gray-500 dark:text-gray-400', rectClass: 'stroke-gray-500 fill-gray-100 dark:fill-gray-500/10' },
        skipped: { icon: 'M15 12H9m12 0a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-amber-500 dark:text-amber-400', rectClass: 'stroke-amber-500 fill-amber-100 dark:fill-amber-500/10' },
        'failure (ignored)': { icon: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z', color: 'text-amber-500 dark:text-amber-400', rectClass: 'stroke-amber-500 fill-amber-100 dark:fill-amber-500/10' },
    };


    function init(context = {}) {
        if (initialized) return;
        state = context.state;
        DOM = context.DOM;
        fetchData = context.fetchData;
        postData = context.postData;
        deleteData = context.deleteData;
        logsModule = context.logsModule;
        refresh = context.refresh || (() => {});
        setupLogHelpers();
        bindDomEvents();
        setupObservers();
        initialized = true;
    }

function setupLogHelpers() {
        if (!logsModule) {
            showLogsModal = () => {};
            closeLogsModal = () => {};
            renderLogsWithFilters = () => {};
            updateLogsStepList = () => {};
            return;
        }
        showLogsModal = typeof logsModule.showLogsModal === 'function' ? logsModule.showLogsModal.bind(logsModule) : () => {};
        closeLogsModal = typeof logsModule.closeLogsModal === 'function' ? logsModule.closeLogsModal.bind(logsModule) : () => {};
        renderLogsWithFilters = typeof logsModule.renderLogsWithFilters === 'function' ? logsModule.renderLogsWithFilters.bind(logsModule) : () => {};
        updateLogsStepList = typeof logsModule.updateLogsStepList === 'function' ? logsModule.updateLogsStepList.bind(logsModule) : () => {};
    }

function setupObservers() {
        if (typeof ResizeObserver === 'undefined' || !DOM || !DOM.graphWrapper) return;
        try {
            if (state._graphResizeObserver) {
                state._graphResizeObserver.disconnect();
            }
            const observer = new ResizeObserver(() => {
                requestAnimationFrame(() => {
                    if (state.currentRunData) {
                        renderRunView(state.currentRunData);
                    }
                });
            });
            observer.observe(DOM.graphWrapper);
            state._graphResizeObserver = observer;
        } catch (err) {
            console.error('ResizeObserver setup failed:', err);
        }
    }

function initPanAndZoom(view, layout = null) {
  // Clean up any previous panzoom + wheel handler
  if (state.panzoomInstance) {
try { state.panzoomInstance.destroy(); } catch (e) {}
state.panzoomInstance = null;
  }
  if (state._wheelTarget && state._wheelHandler) {
try { state._wheelTarget.removeEventListener('wheel', state._wheelHandler); } catch (e) {}
state._wheelTarget = null;
state._wheelHandler = null;
  }

  let container, element;
  if (view === 'steps') {
container = DOM.graphContainer;
element = DOM.graphWrapper;
  } else if (view === 'tasks') {
container = DOM.tasksGraphContainer;
element = DOM.tasksGraphWrapper;
  } else {
return;
  }

  // If the target container is hidden or has no size yet, defer binding.
  try {
const isHidden = container.classList.contains('hidden') || container.clientWidth === 0 || container.clientHeight === 0;
if (isHidden) {
  state._pendingPanzoom = { view, layout };
  return;
}
  } catch {}

  if (!element || !element.firstElementChild) return;

  state.panzoomInstance = Panzoom(element, {
canvas: true,
maxScale: 5,
minScale: 0.1,
  });
  // Ensure the pan element never lets the browser handle scrolling/zoom gestures
  try { element.style.touchAction = 'none'; } catch {}
  state._wheelTarget = container;
  state._wheelHandler = state.panzoomInstance.zoomWithWheel;
  // Attach wheel to both the container and the element for robustness
  container.addEventListener('wheel', state._wheelHandler, { passive: false });
  element.addEventListener('wheel', state._wheelHandler, { passive: false });
  // remember which element panzoom is bound to
  state._panElement = element;

  // Also bind pointer/touch start on the container so users can pan immediately,
  // even if the pointer starts outside the inner SVG element.
  if (state._pointerDownHandler) {
try {
  container.removeEventListener('mousedown', state._pointerDownHandler);
  container.removeEventListener('pointerdown', state._pointerDownHandler);
  container.removeEventListener('touchstart', state._pointerDownHandler);
  element.removeEventListener('mousedown', state._pointerDownHandler);
  element.removeEventListener('pointerdown', state._pointerDownHandler);
  element.removeEventListener('touchstart', state._pointerDownHandler);
} catch (e) {}
  }
  state._pointerDownHandler = (ev) => {
if (!state.panzoomInstance) return;
const isControl = !!(ev.target && (ev.target.closest('#steps-graph-controls') || ev.target.closest('#tasks-graph-controls')));
if (isControl) return;
try {
  ev.preventDefault();
  state.panzoomInstance.handleDown(ev);
} catch {}
  };
  // Bind to pointerdown for widest browser coverage; keep mouse/touch as fallback
  container.addEventListener('pointerdown', state._pointerDownHandler, { passive: false });
  container.addEventListener('mousedown', state._pointerDownHandler, { passive: false });
  container.addEventListener('touchstart', state._pointerDownHandler, { passive: false });
  // Also bind directly on the pan element so dragging on the SVG works immediately
  element.addEventListener('pointerdown', state._pointerDownHandler, { passive: false });
  element.addEventListener('mousedown', state._pointerDownHandler, { passive: false });
  element.addEventListener('touchstart', state._pointerDownHandler, { passive: false });

  container.addEventListener('dblclick', () => {
if (!state.panzoomInstance || state._panElement !== element) return;
state.panzoomInstance.reset({ animate: true });
fitToView();
  });

  // Restore transform for Steps view if requested. When preserving scale (e.g., expand/collapse all),
  // ignore any pending auto-fit requests set by layout passes.
  const restore = (view === 'steps' && state._stepsViewTransform && (!state._fitOnNextStepsRender || state._preserveScale)) ? state._stepsViewTransform : null;
  if (restore) {
try {
  state.panzoomInstance.zoom(restore.scale || 1, { animate: false });
  state.panzoomInstance.pan(restore.x || 0, restore.y || 0, { animate: false });
} catch {}
// one-time restore
delete state._stepsViewTransform;
  } else {
// Do not auto-fit on bind; keep current zoom/pan unless explicitly requested.
// Initial fit is triggered by external caller (first render) or when _fitOnNextStepsRender is set.
if (state._suppressInitialFit) delete state._suppressInitialFit;
  }

  function fitToView() {
// Another initPanAndZoom call might have destroyed/rebound the instance.
// Only fit if the current instance is bound to this element.
if (!state.panzoomInstance || state._panElement !== element) return;
const parentRect = container.getBoundingClientRect();
// prefer layout dims (exact), fallback to element’s current box
const contentWidth  = layout?.width  || element.scrollWidth || element.offsetWidth;
const contentHeight = layout?.height || element.scrollHeight || element.offsetHeight;
if (!contentWidth || !contentHeight || !parentRect.width || !parentRect.height) return;

// start from a clean slate
if (!state.panzoomInstance) return;
state.panzoomInstance.reset({ animate: false });

// Centered fit behavior for both Steps and Tasks
const padding = 40;
const fitScale = Math.min(
  parentRect.width  / (contentWidth  + padding),
  parentRect.height / (contentHeight + padding)
) * 0.98;
const scale = Math.min(1, fitScale);
state.panzoomInstance.zoom(scale, { animate: false });
const x = (parentRect.width  - contentWidth  * scale) / 2;
const y = (parentRect.height - contentHeight * scale) / 2;
state.panzoomInstance.pan(x, y, { animate: false });
// Record baseline so Reset reproduces this
if (view === 'steps') state._baselineStepsTransform = { x, y, scale };

// Record baseline transform for Steps view so Reset matches refresh
try {
  if (view === 'steps' && !state._baselineStepsTransform) {
    const tr = window.getComputedStyle(element).transform;
    if (tr && tr !== 'none') {
      const m = tr.match(/matrix\(([^)]+)\)/);
      if (m) {
        const v = m[1].split(',').map(parseFloat);
        if (v.length === 6) {
          const a = v[0], b = v[1];
          const s = Math.sqrt(a*a + b*b) || 1;
          const px = v[4] || 0; const py = v[5] || 0;
          state._baselineStepsTransform = { x: px, y: py, scale: s };
        }
      }
    } else {
      state._baselineStepsTransform = { x: 0, y: 0, scale: 1 };
    }
  }
} catch {}
  }

  state.__fitToView = fitToView;
  if (view === 'steps' && state._fitOnNextStepsRender && !state._preserveScale) {
// ensure a final synchronous fit
setTimeout(() => fitToView(), 0);
delete state._fitOnNextStepsRender;
  }
}

// Ensure there is a working Panzoom bound to the currently visible view
function ensurePanzoomBound(view) {
  try {
const isSteps = view === 'steps';
const container = isSteps ? DOM.graphContainer : DOM.tasksGraphContainer;
const element = isSteps ? DOM.graphWrapper : DOM.tasksGraphWrapper;
if (!container || !element) return;
if (container.classList.contains('hidden')) return; // will bind on switch
// Require content to be present
if (!element.firstElementChild) return;
// If missing or bound to the wrong element, (re)bind
if (!state.panzoomInstance || state._panElement !== element) {
  // Preserve current transform if re-binding to the same element (or to steps target)
  try {
    const tr = window.getComputedStyle(element).transform;
    let scale = 1, x = 0, y = 0;
    if (tr && tr !== 'none') {
      const m = tr.match(/matrix\(([^)]+)\)/);
      if (m) {
        const v = m[1].split(',').map(parseFloat);
        if (v.length === 6) {
          const a = v[0], b = v[1];
          scale = Math.sqrt(a*a + b*b) || 1; x = v[4] || 0; y = v[5] || 0;
        }
      }
    }
    if (isSteps) state._stepsViewTransform = { x, y, scale };
    state._suppressInitialFit = true; // avoid recenter jump on first control click
  } catch {}
  const layout = isSteps ? null : (state._lastStepLayout || null);
  initPanAndZoom(view, layout);
}
  } catch {}
}


// Center a particular step (node or expanded cluster) in view
function focusStepIntoView(stepName) {
  try {
const container = DOM.graphContainer;
const svg = DOM.graphWrapper.querySelector('svg');
if (!container || !svg || !state.panzoomInstance) return;

// Prefer the expanded cluster bbox; fallback to the step node bbox
let target = svg.querySelector(`.step-cluster[data-step-name="${stepName}"]`);
if (!target) target = svg.querySelector(`g.graph-node[data-step-name="${stepName}"]`);
if (!target || !target.getBBox) return;

const bbox = target.getBBox();
const cx = bbox.x + bbox.width / 2;
const cy = bbox.y + bbox.height / 2;

// Read current scale from computed transform matrix
const panEl = state._panElement || svg; // actual pan element
const tr = window.getComputedStyle(panEl).transform;
let scale = 1;
if (tr && tr !== 'none') {
  const m = tr.match(/matrix\(([^)]+)\)/);
  if (m) {
    const vals = m[1].split(',').map(parseFloat);
    if (vals.length === 6) {
      const a = vals[0], b = vals[1];
      scale = Math.sqrt(a*a + b*b) || 1;
    }
  }
}

// Compute a zoom that keeps the cluster fully visible with margins
const margin = 20;
const topSafe = 72; // room for tabs/header
const availW = Math.max(100, container.clientWidth - 2 * margin);
const availH = Math.max(100, container.clientHeight - topSafe - 2 * margin);
const fitScaleX = availW / Math.max(1, bbox.width);
const fitScaleY = availH / Math.max(1, bbox.height);
const fitScale = Math.min(fitScaleX, fitScaleY);
if (fitScale < scale - 1e-6) {
  state.panzoomInstance.zoom(fitScale, { animate: true });
  scale = fitScale;
}

// Clamp pan so the bbox is fully inside the viewport margins
const centerPanX = (container.clientWidth / 2) - (cx * scale);
const centerPanY = ((container.clientHeight - topSafe) / 2 + topSafe) - (cy * scale);
const leftMin   = margin - bbox.x * scale;
const rightMax  = container.clientWidth - margin - (bbox.x + bbox.width) * scale;
const topMin    = topSafe + margin - bbox.y * scale;
const bottomMax = container.clientHeight - margin - (bbox.y + bbox.height) * scale;

const panX = Math.min(leftMin, Math.max(rightMax, centerPanX));
const panY = Math.min(bottomMax, Math.max(topMin, centerPanY));
state.panzoomInstance.pan(panX, panY, { animate: true });
  } catch {}
}

// Minimal pan-only nudge to keep a step fully visible without changing zoom
function nudgeStepIntoView(stepName) {
  try {
const container = DOM.graphContainer;
const svg = DOM.graphWrapper.querySelector('svg');
if (!container || !svg || !state.panzoomInstance) return;
let target = svg.querySelector(`.step-cluster[data-step-name="${stepName}"]`);
if (!target) target = svg.querySelector(`g.graph-node[data-step-name="${stepName}"]`);
if (!target || !target.getBBox) return;

const bbox = target.getBBox();
const panEl = state._panElement || svg;
const tr = window.getComputedStyle(panEl).transform;
let scale = 1;
if (tr && tr !== 'none') {
  const m = tr.match(/matrix\(([^)]+)\)/);
  if (m) {
    const vals = m[1].split(',').map(parseFloat);
    if (vals.length === 6) { const a = vals[0], b = vals[1]; scale = Math.sqrt(a*a + b*b) || 1; }
  }
}

const margin = 20, topSafe = 84; // extra room for sticky header/tabs
// Current pan from transform matrix
let panX = 0, panY = 0;
if (tr && tr !== 'none') {
  const m = tr.match(/matrix\(([^)]+)\)/);
  if (m) {
    const vals = m[1].split(',').map(parseFloat);
    if (vals.length === 6) { panX = vals[4] || 0; panY = vals[5] || 0; }
  }
}

// Compute deltas so bbox lies within margins
const left = bbox.x * scale + panX;
const top = bbox.y * scale + panY;
const right = (bbox.x + bbox.width) * scale + panX;
const bottom = (bbox.y + bbox.height) * scale + panY;

let dx = 0, dy = 0;
if (left < margin) dx = margin - left;
if (right > container.clientWidth - margin) dx = (container.clientWidth - margin) - right;
if (top < topSafe + margin) dy = (topSafe + margin) - top;
if (bottom > container.clientHeight - margin) dy = (container.clientHeight - margin) - bottom;

if (dx !== 0 || dy !== 0) {
  state.panzoomInstance.pan(panX + dx, panY + dy, { animate: true });
}
  } catch {}
}



    function timeAgo(dateString) {
        if (!dateString || new Date(dateString).getFullYear() < 2000) return '';
        const seconds = Math.round((new Date() - new Date(dateString)) / 1000);
        const minutes = Math.round(seconds / 60);
        const hours = Math.round(minutes / 60);
        const days = Math.round(hours / 24);
        if (seconds < 60) return `${seconds}s ago`;
        if (minutes < 60) return `${minutes}m ago`;
        if (hours < 24) return `${hours}h ago`;
        return `${days}d ago`;
    }

    function formatDuration(startStr, endStr) {
        if (!startStr || new Date(startStr).getFullYear() < 2000) return 'N/A';
        const start = new Date(startStr);
        const end = (endStr && new Date(endStr).getFullYear() > 2000) ? new Date(endStr) : new Date();
        return `${((end - start) / 1000).toFixed(2)}s`;
    }



    async function fetchAllRuns() {
        const runs = await fetchData('/v1/runs');
        if (runs) renderSidebarPipelineRunsList(runs);
    }

    async function fetchMainContent(groupId) {
        const runsByBranch = await fetchData(`/v1/runs?groupId=${groupId}`);
        const hasRuns = runsByBranch && Object.keys(runsByBranch).length > 0;
        const subgroups = state.groups.filter(g => g.parent_id == groupId);

        if (hasRuns) {
            renderGroupedRuns(runsByBranch);
        } else {
            renderMainGridContent(subgroups, null, true);
        }
    }

    function handleRunHighlight(event) {
        // Find the closest element that represents a run, either in the main content or sidebar
        const targetRunElement = event.target.closest('[data-run-id]');

        // Clear all previous highlights
        document.querySelectorAll('.run-link-highlight').forEach(el => el.classList.remove('run-link-highlight'));
        document.querySelectorAll('.sidebar-link-highlight').forEach(el => el.classList.remove('sidebar-link-highlight'));

        if (event.type === 'mouseover' && targetRunElement) {
            // --- Hovering over a RUN CARD or a SIDEBAR RUN LINK ---
            const runId = targetRunElement.dataset.runId;
            const parentRunId = targetRunElement.dataset.parentRunId;
            const repoFullName = targetRunElement.dataset.repoFullName;

            // Determine the main parent run ID to group included pipelines
            const mainParentId = parentRunId || runId;

            // Highlight all related run cards/links (the parent and all of its children)
            const runSelector = `[data-run-id="${mainParentId}"], [data-parent-run-id="${mainParentId}"]`;
            document.querySelectorAll(runSelector).forEach(el => {
                // The element itself could be a card or a sidebar li
                if (el.matches('a, div[data-href]')) {
                    el.classList.add('run-link-highlight');
                } else { // It's a sidebar li
                    el.classList.add('sidebar-link-highlight');
                }
            });

            // Highlight the corresponding repository folder in the sidebar
            if (repoFullName) {
                let currentGroup = state.groups.find(g => g.name === repoFullName);
                while (currentGroup) {
                    const sidebarElement = document.querySelector(`#sidebar-nav li[data-group-id='${currentGroup.id}']`);
                    if (sidebarElement) {
                        sidebarElement.classList.add('sidebar-link-highlight');
                    }
                    currentGroup = currentGroup.parent_id ? state.groups.find(g => g.id === currentGroup.parent_id) : null;
                }
            }
        }
    }

    async function fetchActiveRun(runId) {
        if (!runId) return;
        resetMainView();
        const runDetails = await fetchData(`/v1/runs/${runId}`);
        if (runDetails) {
            state.currentRunData = runDetails;
            // Restore persisted Steps layout for this run (expanded, positions, scale)
            try {
                const key = `nopsai_steps_layout:${runDetails.run_info?.run_id || ''}`;
                const raw = localStorage.getItem(key);
                if (raw) {
                    const saved = JSON.parse(raw);
                    if (saved && typeof saved === 'object') {
                        if (Array.isArray(saved.expanded)) {
                            state.expandedSteps = new Set(saved.expanded);
                        }
                        if (saved.positions && typeof saved.positions === 'object') {
                            const entries = Object.entries(saved.positions);
                            state.expandedStepPositions = new Map(entries.map(([k, v]) => [k, { x: Number(v.x)||0, y: Number(v.y)||0 }]));
                        }
                        if (saved.scale) state.stepLayoutScale = Math.max(1, Math.min(1.8, Number(saved.scale) || 1.0));
                        if (saved.tasksScale) state.taskClusterScale = Math.max(0.65, Math.min(1.0, Number(saved.tasksScale) || 1.0));
                    }
                }
            } catch {}
            renderRunView(runDetails);
            const stepName = window.location.hash.split('/steps/')[1];
            if (stepName) {
                showStepDetails(stepName);
            }
        }
    }

    async function fetchGroups() {
        const groups = await fetchData('/v1/groups');
        if (groups) {
            state.groups = groups;
        } else {
            state.groups = [];
        }
    }

    function resetMainView() {
        DOM.placeholder.classList.add('hidden');
        DOM.graphContainer.classList.add('hidden');
        DOM.tasksGraphContainer.classList.add('hidden');
        DOM.tasksEmpty.classList.add('hidden');
        DOM.mainGridContainer.classList.add('hidden');
        if (DOM.pageContentWrapper) DOM.pageContentWrapper.classList.remove('no-scroll');
    }

    async function renderSidebar(activeRoute, currentTab) {
        const navConfig = [
            { route: 'pipelineruns', title: 'Pipeline Runs', icon: 'M4 6h16M4 12h16M4 18h7' },
            { route: 'pipelines', title: 'Pipelines', icon: 'M9 17V7h2v10H9zm4-12h2v12h-2V5zm4 4h2v8h-2V9zM3 3h18v2H3V3z' },
            { route: 'secrets', title: 'Secrets', icon: 'M12 15l-3.3-3.3a4.7 4.7 0 116.6 0L12 15zm0 0l-1.4-1.4' },
            { route: 'steps', title: 'Steps', icon: 'M12 15l-3.3-3.3a4.7 4.7 0 116.6 0L12 15zm0 0l-1.4-1.4' },
            { route: 'environment', title: 'Environment', icon: 'M12 15l-3.3-3.3a4.7 4.7 0 116.6 0L12 15zm0 0l-1.4-1.4' },
            { route: 'monitoring', title: 'Monitoring', icon: 'M12 15l-3.3-3.3a4.7 4.7 0 116.6 0L12 15zm0 0l-1.4-1.4' },
            { route: 'triggers', title: 'Triggers', icon: 'M12 15l-3.3-3.3a4.7 4.7 0 116.6 0L12 15zm0 0l-1.4-1.4' },
            { route: 'system', title: 'System', icon: 'M12 15l-3.3-3.3a4.7 4.7 0 116.6 0L12 15zm0 0l-1.4-1.4' },
        ];

        let navHtml = navConfig.map(item => {
            const isActive = activeRoute === item.route;
            return `<a href="#/${item.route}" class="sidebar-link flex items-center p-2 text-[var(--text-primary)] rounded-md transition-colors duration-200 group ${isActive ? 'active' : ''}" data-navigo>
                        <svg class="h-5 w-5 mr-3 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${item.icon}"/></svg>
                        <span>${item.title}</span>
                    </a>`;
        }).join('');

        DOM.sidebarNav.innerHTML = `<div class="space-y-1">${navHtml}</div>`;

        if (activeRoute === 'pipelineruns') {
            if (currentTab === 'recent') {
                DOM.sidebarNav.innerHTML += `<h2 class="px-2 mt-6 mb-2 text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">Recent Runs</h2>
                                             <ul id="pipeline-runs-list" class="space-y-1"></ul>`;
                fetchAllRuns();
            } else {
                DOM.sidebarNav.innerHTML += `<div class="px-2 mt-6 mb-2 flex items-center justify-between">
                                                <h2 class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">Main</h2>
                                            </div>
                                            <div id="main-hierarchy"></div>`;

                state.repoLastRunCache.clear();
                await renderHierarchy(state.groups);
            }
        }
    }

    async function renderHierarchy(groups, parentId = null, level = 0, container = null) {
        container = container || (level === 0 ? document.getElementById('main-hierarchy') : document.querySelector(`[data-group-id='${parentId}'] .group-children`));
        if (!container) return;

        const children = groups.filter(g => {
            const gParentId = (g.parent_id === 0 || g.parent_id === null) ? null : g.parent_id;
            return gParentId === parentId;
        });

        const childrenWithDataPromises = children.map(async (group) => {
            const isRepo = (group.name || '').includes('/');
            if (!isRepo) {
                return { ...group, lastRunAt: null };
            }
            if (state.repoLastRunCache && state.repoLastRunCache.has(group.id)) {
                const latestRun = state.repoLastRunCache.get(group.id);
                return { ...group, lastRunAt: latestRun ? latestRun.started_at : null };
            }
            const runsByBranch = await fetchData(`/v1/runs?groupId=${group.id}`);
            let latestRun = null;
            if (runsByBranch && Object.keys(runsByBranch).length > 0) {
                for (const branch in runsByBranch) {
                    if (runsByBranch[branch].length > 0) {
                        const latestBranchRun = runsByBranch[branch][0];
                        if (!latestRun || (latestBranchRun.started_at && new Date(latestBranchRun.started_at) > new Date(latestRun.started_at))) {
                            latestRun = latestBranchRun;
                        }
                    }
                }
            }
            const lastRunAt = latestRun ? latestRun.started_at : null;
            if (!state.repoLastRunCache) state.repoLastRunCache = new Map();
            state.repoLastRunCache.set(group.id, latestRun);
            return { ...group, lastRunAt };
        });

        const childrenWithData = await Promise.all(childrenWithDataPromises);

        childrenWithData.sort((a, b) => {
            const isRepoA = (a.name || '').includes('/');
            const isRepoB = (b.name || '').includes('/');
            if (isRepoA !== isRepoB) {
                return isRepoA ? 1 : -1;
            }
            if (isRepoA && a.lastRunAt && b.lastRunAt) {
                return new Date(b.lastRunAt) - new Date(a.lastRunAt);
            }
            return a.name.localeCompare(b.name);
        });

        let html = `<ul class="pl-${level > 0 ? '4' : '0'} space-y-1">`;
        for (const group of childrenWithData) {
            const hasChildren = groups.some(g => g.parent_id === group.id);
            const isExpanded = state.expandedGroups.has(group.id);
            const isRepo = (group.name || '').includes('/');
            const displayName = isRepo ? group.name.split('/')[1] : group.name;
            const canExpand = hasChildren || isRepo;
            const isActive = state.selectedGroupId === group.id;

            let chevron = canExpand 
                ? `<svg class="h-4 w-4 mr-1 text-[var(--text-secondary)] chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>` 
                : `<div class="w-5 h-4 mr-1"></div>`;

            html += `<li data-group-id="${group.id}" draggable="true">
                            <div class="flex items-center justify-between p-2 text-[var(--text-primary)] rounded-md group-header-container ${isActive ? 'bg-[var(--bg-tertiary)]' : ''}">
                                <div class="flex items-center group-header flex-grow cursor-pointer ${isExpanded ? 'expanded' : ''}">
                                    ${chevron}
                                    <a href="#/pipelineruns/main/${group.id}" class="flex items-center flex-grow">
                                        <svg class="h-4 w-4 mr-2 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                                        <span class="truncate">${displayName}</span>
                                    </a>
                                </div>
                                <button class="delete-group-btn text-[var(--text-secondary)] hover:text-red-500 opacity-0 transition-opacity" data-group-id="${group.id}" data-group-name="${group.name}"><svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg></button>
                            </div>
                            <div class="group-children"></div>
                        </li>`;
        }
        html += '</ul>';
        container.innerHTML = html;

        for (const group of childrenWithData) {
            if (state.expandedGroups.has(group.id)) {
                const childContainer = document.querySelector(`[data-group-id='${group.id}'] .group-children`);
                const isRepo = (group.name || '').includes('/');
                if (isRepo) {
                    const runsByBranch = await fetchData(`/v1/runs?groupId=${group.id}`);
                    renderRepoChildren(childContainer, runsByBranch, level + 1);
                } else {
                    await renderHierarchy(groups, group.id, level + 1, childContainer);
                }
            }
        }
    }

    function renderRepoChildren(container, runsByBranch, level) {
        if (!container) return;
        let html = `<ul class="pl-${level > 0 ? '4' : '0'} space-y-1">`;
        const sortedBranches = Object.keys(runsByBranch).sort();

        sortedBranches.forEach(branch => {
            const runs = runsByBranch[branch];
            const branchId = `branch-${container.closest('li').dataset.groupId}-${branch.replace(/[^a-zA-Z0-9]/g, '')}`;
            const isExpanded = state.expandedGroups.has(branchId);
            const chevron = `<svg class="h-4 w-4 mr-1 text-[var(--text-secondary)] chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>`;

            html += `<li data-branch-id="${branchId}">
                            <div class="flex items-center p-2 text-[var(--text-primary)] rounded-md group-header cursor-pointer">
                                ${chevron}
                                <svg class="h-4 w-4 mr-2 text-[var(--text-accent)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                                <span>${branch}</span>
                            </div>
                            <div class="group-children">
                                ${isExpanded ? renderRunLinks(runs, level + 1) : ''}
                            </div>
                        </li>`;
        });
        html += '</ul>';
        container.innerHTML = html;
    }

    function renderRunLinks(runs, level) {
        const activeRunId = window.location.hash.split('/')[3];
        let html = `<ul class="pl-${level > 0 ? '4' : '0'} space-y-1">`;
        runs.forEach(run => {
            const config = statusConfig[(run.is_complete ? run.status : 'running').toLowerCase()] || statusConfig.pending;
            const isActive = run.run_id === activeRunId;
            const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
            const repoFullName = `${run.git_repo_owner}/${run.git_repo_name}`;

            let pipelineNameHTML = `<span class="font-medium truncate text-[var(--text-primary)]">${run.pipeline_name}</span>`;
            if (run.parent_run_id) {
                pipelineNameHTML = `
                    <div class="flex items-baseline">
                        <span class="font-medium truncate text-[var(--text-primary)]">${run.pipeline_name}</span>
                        <span class="ml-1.5 text-[10px] bg-[var(--bg-primary)] text-[var(--text-accent)] font-semibold px-1.5 py-0.5 rounded-md">Included</span>
                    </div>`;
            }

            html += `<li data-run-id="${run.run_id}" data-repo-full-name="${repoFullName}" ${run.parent_run_id ? `data-parent-run-id="${run.parent_run_id}"` : ''}>
                            <a href="#/pipelineruns/run/${run.run_id}" class="flex items-center p-2 text-sm text-[var(--text-secondary)] rounded-md ${isActive ? 'bg-[var(--bg-tertiary)] text-[var(--text-primary)]' : ''}">
                                <svg class="h-4 w-4 mr-2 flex-shrink-0 ${config.color}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                                <div class="flex-1 overflow-hidden">
                                    <div class="flex justify-between items-center">
                                        ${pipelineNameHTML}
                                        <span class="text-xs text-[var(--text-secondary)] flex-shrink-0 ml-2">${timeAgo(timeToDisplay)}</span>
                                    </div>
                                    <div class="text-xs text-[var(--text-secondary)] font-mono mt-1 space-y-1">
                                        <div class="flex items-center">
                                            <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                                            <span class="truncate">${(run.git_commit_sha || '...').slice(0, 8)}</span>
                                        </div>                                          
                                      <div class="flex items-center">
                                          <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H5v-2H3v-2H1v-4a6 6 0 016-6h1.5" /></svg>
                                          <span class="truncate">${(run.run_id || '...').slice(0, 8)}</span>
                                      </div>
                                    </div>
                                </div>
                            </a>
                        </li>`;
        });
        html += `</ul>`;
        return html;
    }

    function renderSidebarPipelineRunsList(runs) {
        const listEl = document.getElementById('pipeline-runs-list');
        if (!listEl) return;
        const activeRunId = window.location.hash.split('/')[3];
         if (!runs || runs.length === 0) {
             listEl.innerHTML = `<li><p class="p-2 text-[var(--text-secondary)] text-sm">No recent runs found.</p></li>`;
             return;
         }
        listEl.innerHTML = (runs || []).map(run => {
            const status = run.is_complete ? run.status : 'running';
            const config = statusConfig[status.toLowerCase()] || statusConfig.pending;
            const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
            const isActive = run.run_id === activeRunId;
            const branchName = (run.git_ref || '').startsWith('refs/heads/') ? run.git_ref.split('/')[2] : 'N/A';
            const repoFullName = `${run.git_repo_owner}/${run.git_repo_name}`;

            let pipelineNameHTML = `<span class="font-medium truncate">${run.pipeline_name}</span>`;
            if (run.parent_run_id) {
                pipelineNameHTML = `
                    <div class="flex items-baseline">
                        <span class="font-medium truncate">${run.pipeline_name}</span>
                        <span class="ml-1.5 text-[10px] bg-[var(--bg-primary)] text-[var(--text-accent)] font-semibold px-1.5 py-0.5 rounded-md">Included</span>
                    </div>`;
            }

            return `<li data-run-id="${run.run_id}" data-repo-full-name="${repoFullName}" ${run.parent_run_id ? `data-parent-run-id="${run.parent_run_id}"` : ''}>
                    <a href="#/pipelineruns/run/${run.run_id}" class="flex items-center p-2 text-[var(--text-primary)] rounded-md ${isActive ? 'bg-[var(--bg-tertiary)]' : ''}">
                        <svg class="h-5 w-5 mr-3 flex-shrink-0 ${config.color}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                        <div class="flex-1 overflow-hidden">
                            <div class="flex justify-between items-center">
                                ${pipelineNameHTML}
                                <span class="text-xs text-[var(--text-secondary)] flex-shrink-0 ml-2">${timeAgo(timeToDisplay)}</span>
                            </div>
                            <div class="text-xs text-[var(--text-secondary)] font-mono mt-1 space-y-1">
                                <div class="flex items-center">
                                    <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                                    <span class="truncate">${run.git_repo_name}</span>
                                </div>
                                <div class="flex items-center">
                                   <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                                   <span class="truncate">${branchName}</span>
                                </div>
                                <div class="flex items-center">
                                    <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                                    <span class="truncate">${(run.git_commit_sha || '...').slice(0, 8)}</span>
                                </div>                                    
                                <div class="flex items-center">
                                    <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H5v-2H3v-2H1v-4a6 6 0 016-6h1.5" /></svg>
                                    <span class="truncate">${(run.run_id || '...').slice(0, 8)}</span>
                                </div>
                            </div>
                        </div>
                    </a>
                </li>`;
        }).join('');
    }

    function renderGroupedRuns(runsByBranch) {
        resetMainView();
        DOM.mainGridContainer.classList.remove('hidden');

        if (!runsByBranch || Object.keys(runsByBranch).length === 0) {
            DOM.mainGridContainer.innerHTML = `<p class="text-[var(--text-secondary)]">No pipeline runs found for this repository.</p>`;
            return;
        }

        let html = '<div class="space-y-6">';

        const sortedBranches = Object.keys(runsByBranch).sort((a, b) => {
            const lastRunA = runsByBranch[a][0];
            const lastRunB = runsByBranch[b][0];
            if (!lastRunA || !lastRunA.started_at) return 1;
            if (!lastRunB || !lastRunB.started_at) return -1;
            return new Date(lastRunB.started_at) - new Date(lastRunA.started_at);
        });

        sortedBranches.forEach((branch, index) => {
            const runs = runsByBranch[branch];
            const latestRun = runs[0];
            const config = statusConfig[(latestRun.is_complete ? latestRun.status : 'running').toLowerCase()] || statusConfig.pending;
            const isExpanded = index === 0; // Expand first branch by default

            html += `
            <div class="bg-[var(--bg-secondary)] rounded-lg shadow-md">
                <div class="branch-header cursor-pointer flex items-center justify-between p-4 border-b border-[var(--border-primary)] ${isExpanded ? 'expanded' : ''}">
                    <div class="flex items-center min-w-0">
                        <svg class="h-5 w-5 mr-3 text-[var(--text-secondary)] chevron flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                        <svg class="h-5 w-5 mr-3 text-[var(--text-accent)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                        <span class="font-semibold text-lg text-[var(--text-primary)] truncate">${branch}</span>
                        <span class="ml-4 text-sm text-[var(--text-secondary)] hidden sm:inline">(${runs.length} runs)</span>
                    </div>
                    <div class="flex items-center">
                        <span class="text-sm text-[var(--text-secondary)] mr-3 hidden sm:block">Latest run: ${timeAgo(latestRun.started_at)}</span>
                        <svg class="h-6 w-6 ${config.color}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                    </div>
                </div>
                <div class="branch-runs p-4 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4" style="${isExpanded ? 'max-height: 2000px;' : ''}">
                    ${runs.map(run => renderRunCard(run)).join('')}
                </div>
            </div>`;
        });
        html += '</div>';
        DOM.mainGridContainer.innerHTML = html;
    }

    function renderRunCard(run) {
        const status = run.is_complete ? run.status : 'running';
        const config = statusConfig[status.toLowerCase()] || statusConfig.pending;
        const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
        const branchName = (run.git_ref || '').startsWith('refs/heads/') ? run.git_ref.split('/')[2] : 'N/A';
        const repoFullName = `${run.git_repo_owner}/${run.git_repo_name}`;

        let pipelineNameHTML = `<p class="text-base font-semibold text-[var(--text-primary)] truncate pr-4">${run.pipeline_name}</p>`;
        if (run.parent_run_id) {
            pipelineNameHTML = `
                <div class="flex items-center gap-x-2">
                    <p class="text-base font-semibold text-[var(--text-primary)]">${run.pipeline_name}</p>
                    <span class="ml-1.5 text-[10px] bg-[var(--bg-primary)] text-[var(--text-accent)] font-semibold px-1.5 py-0.5 rounded-md">Included</span>
                </div>`;
        }

        return `
            <div data-href="#/pipelineruns/run/${run.run_id}"
                data-run-id="${run.run_id}" 
                data-repo-full-name="${repoFullName}"
                ${run.parent_run_id ? `data-parent-run-id="${run.parent_run_id}"` : ''}
                class="block bg-[var(--bg-primary)] transition-all duration-200 rounded-lg p-4 flex flex-col justify-between cursor-pointer border border-[var(--border-primary)] shadow-sm">
                <div>
                    <div class="flex items-start justify-between">
                        ${pipelineNameHTML}
                        <div class="flex-shrink-0 h-6 w-6 rounded-full flex items-center justify-center ${config.color}">
                            <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                        </div>
                    </div>
                    <p class="text-sm text-[var(--text-secondary)] items-center mt-1">
                       ${run.git_repo_name}
                    </p>
                    <p class="text-sm text-[var(--text-link)] font-mono items-center mt-1">
                       <svg class="inline-block h-4 w-4 mr-1 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                       ${branchName}
                    </p>
                </div>
                <div class="mt-4 text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
                    <div class="flex items-center">
                        <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                        <span class="truncate">${run.git_pusher_name || 'N/A'}</span>
                    </div>
                    <div class="flex items-center">
                        <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                        <span class="truncate">${(run.git_commit_sha || '...').slice(0, 8)}</span>
                    </div>
                     <div class="flex items-center">
                        <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H5v-2H3v-2H1v-4a6 6 0 016-6h1.5" /></svg>
                        <span class="truncate">${(run.run_id).slice(0, 8)}</span>
                    </div>
                </div>
                 <div class="mt-4 pt-3 border-t border-[var(--border-primary)] flex items-center justify-between text-xs text-[var(--text-secondary)]">
                    <span class="font-medium">${run.is_complete ? 'Completed' : 'Started'}</span>
                    <span>${timeAgo(timeToDisplay)}</span>
                </div>
            </div>`;
    }

    async function renderMainGridContent(subgroups, runs, showAddButton = false) {
         resetMainView();
         DOM.mainGridContainer.classList.remove('hidden');

         // Pre-fetch latest run info for all subgroups if not in cache
        const subgroupsWithDataPromises = (subgroups || []).map(async (group) => {
            const isRepo = (group.name || '').includes('/');
            if (isRepo && (!state.repoLastRunCache || !state.repoLastRunCache.has(group.id))) {
                const runsByBranch = await fetchData(`/v1/runs?groupId=${group.id}`);
                let latestRun = null;
                if (runsByBranch && Object.keys(runsByBranch).length > 0) {
                    for (const branch in runsByBranch) {
                        if (runsByBranch[branch].length > 0) {
                            const latestBranchRun = runsByBranch[branch][0];
                            if (!latestRun || (latestBranchRun.started_at && new Date(latestBranchRun.started_at) > new Date(latestRun.started_at))) {
                                latestRun = latestBranchRun;
                            }
                        }
                    }
                }
                if (!state.repoLastRunCache) state.repoLastRunCache = new Map();
                state.repoLastRunCache.set(group.id, latestRun);
            }
            return group;
        });

        await Promise.all(subgroupsWithDataPromises);


         let html = '<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">';

        (subgroups || []).forEach(group => {
            const isRepo = (group.name || '').includes('/');
            const displayName = isRepo ? group.name.split('/')[1] : group.name;
            const latestRun = isRepo ? state.repoLastRunCache.get(group.id) : null;
            let latestRunInfo = '';
            if (latestRun) {
                const branchName = (latestRun.git_ref || '').startsWith('refs/heads/') ? latestRun.git_ref.split('/')[2] : 'N/A';
                const config = statusConfig[(latestRun.is_complete ? latestRun.status : 'running').toLowerCase()] || statusConfig.pending;
                const timeToDisplay = latestRun.is_complete ? latestRun.finished_at : latestRun.started_at;
                latestRunInfo = `
                    <div class="mt-4 pt-3 border-t border-[var(--border-primary)] text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
                        <div class="flex items-center justify-between">
                            <div class="flex items-center">
                                <svg class="h-4 w-4 mr-2 ${config.color}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                                <span class="font-semibold text-sm text-[var(--text-primary)] truncate">${branchName}</span>
                            </div>
                            <span class="text-xs text-[var(--text-secondary)] flex-shrink-0 ml-2">${timeAgo(timeToDisplay)}</span>
                        </div>
                        <div class="flex items-center">
                            <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                            <span class="truncate">${(latestRun.git_commit_sha || '...').slice(0, 7)}</span>
                        </div>
                        <div class="flex items-center">
                            <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                            <span class="truncate">${latestRun.git_pusher_name || 'N/A'}</span>
                        </div>
                    </div>`;
            }


            html += `
                <a href="#/pipelineruns/main/${group.id}" draggable="true" class="relative group bg-[var(--bg-secondary)] p-4 rounded-md hover:bg-[var(--bg-tertiary)] transition-colors duration-200 group-card border border-[var(--border-primary)] hover:border-[var(--border-accent)] shadow-sm hover:shadow-lg flex flex-col justify-between" data-group-id="${group.id}">
                    <div>
                        <button class="delete-group-btn absolute top-2 right-2 text-[var(--text-secondary)] hover:text-red-500 opacity-0 group-hover:opacity-100 transition-opacity z-10" data-group-id="${group.id}" data-group-name="${group.name}">
                            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                        </button>
                        <div class="flex items-center">
                            <svg class="h-8 w-8 text-[var(--text-accent)] mr-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>
                            <span class="text-lg font-medium text-[var(--text-primary)] truncate">${displayName}</span>
                        </div>
                    </div>
                    ${latestRunInfo}
                </a>`;
        });

        (runs || []).forEach(run => {
            html += renderRunCard(run);
        });

         if (showAddButton) {
              html += `
                <div id="add-group-card" class="relative group bg-[var(--bg-secondary)] p-4 rounded-md border-2 border-dashed border-[var(--border-secondary)] hover:border-[var(--border-accent)] hover:bg-[var(--bg-tertiary)] transition-colors duration-200 cursor-pointer flex items-center justify-center min-h-[120px]">
                    <div class="text-center">
                       <svg class="mx-auto h-8 w-8 text-[var(--text-secondary)] group-hover:text-[var(--text-accent)]" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" /></svg>
                       <p class="mt-2 text-sm font-medium text-[var(--text-secondary)] group-hover:text-[var(--text-accent)]">New Folder</p>
                    </div>
                </div>`;
        }
        html += '</div>';
        DOM.mainGridContainer.innerHTML = html;
    }

    function calculateGraphLayout(items, container, nodeWidth, nodeHeight, hGap, vGap, isVertical = false) {
        if (!items || items.length === 0) return { nodes: [], edges: [], width: 0, height: 0 };

        const nodes = {};
        const adj = {};
        const itemNameKey = 'task_name' in items[0] ? 'task_name' : 'name';

        items.forEach((item, index) => {
            const name = item[itemNameKey];
            nodes[name] = { ...item, id: name, level: -1, parents: new Set(), children: new Set() };
            adj[name] = [];
        });

        items.forEach(item => {
            const name = item[itemNameKey];
            (item.depends_on || []).forEach(dep => {
                if (nodes[dep] && nodes[name]) {
                    adj[dep].push(name);
                    nodes[name].parents.add(dep);
                    nodes[dep].children.add(name);
                }
            });
        });

        // Assign levels (topological sort)
        let level = 0;
        let queue = Object.values(nodes).filter(node => node.parents.size === 0);
        let processedCount = 0;

        while(queue.length > 0) {
            const levelSize = queue.length;
            for(let i=0; i < levelSize; i++){
                const node = queue.shift();
                node.level = level;
                processedCount++;

                (adj[node.id] || []).forEach(childId => {
                    const childNode = nodes[childId];
                    childNode.parents.delete(node.id);
                    if (childNode.parents.size === 0) {
                        queue.push(childNode);
                    }
                });
            }
            level++;
        }

        if (processedCount < Object.keys(nodes).length) {
            console.warn("Cycle detected in graph, layout may be incorrect.");
            // Handle cycles by assigning remaining nodes to a final level
            Object.values(nodes).filter(n => n.level === -1).forEach(n => n.level = level);
        }

        const levels = [];
        Object.values(nodes).forEach(node => {
            if (!levels[node.level]) levels[node.level] = [];
            levels[node.level].push(node);
        });

        // Dynamic padding: generous for step layouts, compact for task graphs
        let PADDING_X = 80;
        let PADDING_Y = 136; // extra top/bottom breathing room so top badges aren't obscured
        try {
            const isTaskGraph = (itemNameKey === 'task_name');
            const isMiniContainer = !!(container && container.classList && container.classList.contains('task-graph-mini-container'));
            if (isTaskGraph) {
                // Task graphs (expanded step box, modal, or mini): much tighter paddings
                PADDING_X = isMiniContainer ? 12 : 24;
                PADDING_Y = isMiniContainer ? 12 : 24;
            }
        } catch {}
        let totalWidth, totalHeight;

        if (isVertical) {
            let maxNodesInLevel = 0;
            levels.forEach(l => { if(l) maxNodesInLevel = Math.max(maxNodesInLevel, l.length); });
            totalWidth = maxNodesInLevel * nodeWidth + (maxNodesInLevel > 1 ? (maxNodesInLevel - 1) * hGap : 0);
            totalHeight = levels.length * nodeHeight + (levels.length > 1 ? (levels.length - 1) * vGap : 0);

            levels.forEach((levelNodes, i) => {
                if (!levelNodes) return;
                const levelWidth = levelNodes.length * nodeWidth + (levelNodes.length > 1 ? (levelNodes.length - 1) * hGap : 0);
                const xOffset = (totalWidth - levelWidth) / 2;
                levelNodes.forEach((node, j) => {
                    node.x = j * (nodeWidth + hGap) + xOffset + PADDING_X / 2;
                    node.y = i * (nodeHeight + vGap) + PADDING_Y / 2;
                });
            });
        } else { // Horizontal layout
            let maxNodesInLevel = 0;
            levels.forEach(l => { if(l) maxNodesInLevel = Math.max(maxNodesInLevel, l.length); });
            totalWidth = levels.length * nodeWidth + (levels.length > 1 ? (levels.length - 1) * hGap : 0);
            totalHeight = maxNodesInLevel * nodeHeight + (maxNodesInLevel > 1 ? (maxNodesInLevel - 1) * vGap : 0);

            levels.forEach((levelNodes, i) => {
                if (!levelNodes) return;
                const levelHeight = levelNodes.length * nodeHeight + (levelNodes.length > 1 ? (levelNodes.length - 1) * vGap : 0);
                const yOffset = (totalHeight - levelHeight) / 2;
                levelNodes.forEach((node, j) => {
                    node.x = i * (nodeWidth + hGap) + PADDING_X / 2;
                    node.y = j * (nodeHeight + vGap) + yOffset + PADDING_Y / 2;
                });
            });
        }

        Object.values(nodes).forEach(node => {
            node.width = nodeWidth;
            node.height = nodeHeight;
        });

        const edges = [];
        Object.values(nodes).forEach(item => {
            (item.depends_on || []).forEach(dep => {
                const fromNode = nodes[dep];
                const toNode = nodes[item[itemNameKey]];
                if (fromNode && toNode) {
                    edges.push({ from: fromNode, to: toNode });
                }
            });
        });

        return { 
            nodes: Object.values(nodes), 
            edges, 
            width: totalWidth + PADDING_X, 
            height: totalHeight + PADDING_Y
        };
    }

// services/ui/index.html

    function renderRunView(runDetails) {
    const runInfo = runDetails.run_info;
    const branchName = (runInfo.git_ref || '').startsWith('refs/heads/') ? runInfo.git_ref.split('/')[2] : runInfo.git_ref;
    const repoFullName = runInfo.git_repo_owner ? `${runInfo.git_repo_owner} / ${runInfo.git_repo_name}` : runInfo.git_repo_name;

    const repoGroup = state.groups.find(g => g.name === `${runInfo.git_repo_owner}/${runInfo.git_repo_name}`);
    const repoLink = repoGroup ? `#/pipelineruns/main/${repoGroup.id}` : '#/pipelineruns/main';

    let headerHTML = `<div class="w-full min-w-0">`;

    if (runDetails.parent_run_info) {
        headerHTML += `
            <div class="mb-2">
                <a href="#/pipelineruns/run/${runDetails.parent_run_info.run_id}" class="text-sm text-[var(--text-link)] hover:underline">
                    &larr; Back to parent: ${runDetails.parent_run_info.pipeline_name}
                </a>
            </div>
        `;
    }

    headerHTML += `
            <div class="flex flex-wrap items-baseline gap-x-3 min-w-0">
                <a href="${repoLink}" class="text-xl font-semibold text-[var(--text-secondary)] hover:text-[var(--text-accent)] transition-colors truncate">${repoFullName}</a>
                <a href="#" id="view-pipeline-definition-link" class="text-xl font-semibold text-[var(--text-primary)] hover:text-[var(--text-accent)] transition-colors truncate">${runInfo.pipeline_name}</a>
            </div>
            <div class="text-xs text-[var(--text-secondary)] mt-2 font-mono grid grid-cols-[auto,1fr] gap-x-4 w-full max-w-3xl">
                <span class="text-gray-500 justify-self-end truncate">Run ID:</span>
                <span class="truncate">${runInfo.run_id}</span>
                <span class="text-gray-500 justify-self-end truncate">Commit:</span>
                <span class="truncate">${runInfo.git_commit_sha}</span>
            </div>
            <div class="text-sm text-[var(--text-secondary)] mt-2 flex flex-wrap items-center gap-x-6 gap-y-1">
                <div class="flex items-center" title="Duration">
                    <svg class="h-4 w-4 mr-1.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                    <span>${runInfo.duration || '0s'}</span>
                </div>
                <div class="flex items-center" title="Committer">
                    <svg class="h-4 w-4 mr-1.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                    <span>${runInfo.git_pusher_name || 'N/A'}</span>
                </div>
                <div class="flex items-center" title="Branch">
                    <svg class="h-4 w-4 mr-1.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                    <span>${branchName}</span>
                </div>
                <div class="ml-auto flex items-center gap-3">
                    <a href="#/pipelineruns/run/${runInfo.run_id}/logs" class="inline-flex items-center px-3 py-1.5 border border-transparent text-xs font-medium rounded-md shadow-sm text-[var(--text-primary)] bg-[var(--bg-tertiary)] hover:bg-[var(--border-primary)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)]">
                        <svg class="-ml-0.5 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" /></svg>
                        View Logs
                    </a>
                </div>
            </div>
        </div>`;

    DOM.mainHeader.innerHTML = headerHTML;

    document.getElementById('view-pipeline-definition-link').addEventListener('click', (e) => {
        e.preventDefault();
        showPipelineDefinitionModal(runDetails.pipeline_definition);
    });

    resetMainView();
    try { localStorage.setItem('graphView', 'steps'); } catch {}
    state.currentGraphView = 'steps';

    if (runInfo.failure_reason) {
        DOM.mainGridContainer.classList.remove('hidden');
        DOM.mainGridContainer.innerHTML = `
            <div class="bg-red-50 dark:bg-red-900/50 border border-red-200 dark:border-red-500/50 text-red-800 dark:text-red-200 px-4 py-3 rounded-lg relative" role="alert">
                <strong class="font-bold">Pipeline Failed to Start</strong>
                <span class="block sm:inline mt-2 sm:mt-0 sm:ml-2">This pipeline could not be launched due to a configuration error.</span>
                <div class="mt-3 bg-red-100 dark:bg-gray-900/70 p-3 rounded font-mono">
                    <code class="text-sm text-red-900 dark:text-red-100">${runInfo.failure_reason}</code>
                </div>
            </div>`;
    } else {
        renderStepsGraph(runDetails);
        ensureStepsControls();
        switchGraphView('steps');
        setTimeout(() => {
          const view = 'steps';
          initPanAndZoom(view);
          if (typeof state.__fitToView === 'function') state.__fitToView();
        }, 0);
    }
}

function showPipelineDefinitionModal(pipelineDefinition) {
        const modal = document.getElementById('pipeline-definition-modal');
        const modalContent = document.getElementById('pipeline-definition-modal-content');
        const contentEl = document.getElementById('pipeline-definition-content').querySelector('code');
        const copyBtn = document.getElementById('copy-pipeline-btn');
        const downloadBtn = document.getElementById('download-pipeline-btn');
        const closeBtn = document.getElementById('close-pipeline-definition-modal-btn');

        // Helper to convert an object to a YAML string with a defined key order
        const toYAML = (obj, indent = 0, isListItem = false) => {
            const keyOrder = [
                'name', 'description', 'container_image', 'working_directory', 'image', 'include', 'sync',
                'display_options', 'environment', 'secrets', 'volumes', 'goal', 'script',
                'tasks', 'depends_on', 'ignore_failure', 'llm_content_sharing', 'llm_output_sharing', 'timeout'
            ];

            const colorPalette = ["#A5B4FC", "#6EE7B7", "#FCD34D", "#FDBA74", "#F9A8D4", "#93C5FD", "#A78BFA", "#F472B6"];

            let yamlString = '';
            const spaces = '  '.repeat(indent);
            let firstKey = true;
            const processedKeys = new Set();

            const processKey = (key, value) => {
                if (value === null || value === undefined) return;
                processedKeys.add(key);

                let currentSpaces = spaces;
                if (isListItem && firstKey) {
                    currentSpaces = '  '.repeat(indent - 1) + '- ';
                }

                if (key === 'script' && typeof value === 'string' && value.includes('\n')) {
                    const lines = value.trim().split('\n');
                    yamlString += `${currentSpaces}${key}: |\n`;
                    lines.forEach(line => {
                        yamlString += `${'  '.repeat(indent + 1)}${line}\n`;
                    });
                } else if (typeof value === 'object') {
                    if (Array.isArray(value)) {
                        yamlString += `${currentSpaces}${key}:\n`;
                        value.forEach((item, index) => {
                            if (typeof item === 'object' && item !== null) {
                                if (key === 'steps') {
                                    const color = colorPalette[index % colorPalette.length];
                                    yamlString += `<span style="color: ${color}">`;
                                    yamlString += toYAML(item, indent + 1, true);
                                    yamlString += `</span>`;
                                } else {
                                    yamlString += toYAML(item, indent + 1, true);
                                }
                            } else {
                                yamlString += `${'  '.repeat(indent + 1)}- ${item}\n`;
                            }
                        });
                    } else {
                        yamlString += `${currentSpaces}${key}:\n${toYAML(value, indent + 1, false)}`;
                    }
                } else {
                    yamlString += `${currentSpaces}${key}: ${value}\n`;
                }
                firstKey = false;
            };

            // Process keys in the defined order
            keyOrder.forEach(key => {
                if (Object.prototype.hasOwnProperty.call(obj, key)) {
                    processKey(key, obj[key]);
                }
            });

            // Process any remaining keys
            for (const key in obj) {
                if (Object.prototype.hasOwnProperty.call(obj, key) && !processedKeys.has(key)) {
                    processKey(key, obj[key]);
                }
            }

            return yamlString;
        };

        let pipelineYAML;
        let rawYAML;

        if(typeof pipelineDefinition === 'string') {
            pipelineYAML = pipelineDefinition.replace(/</g, '&lt;').replace(/>/g, '&gt;');
            rawYAML = pipelineDefinition;
        } else if (typeof pipelineDefinition === 'object' && pipelineDefinition !== null) {
            rawYAML = toYAML(pipelineDefinition).replace(/<span style="color: #[0-9a-fA-F]{6}">|<\/span>/g, '');
            pipelineYAML = toYAML(pipelineDefinition);
        } else {
            pipelineYAML = 'Pipeline definition is not available.';
            rawYAML = pipelineYAML;
        }

        contentEl.innerHTML = pipelineYAML;

        const closeModal = () => {
            modal.classList.remove('opacity-100');
            modalContent.classList.add('scale-95');
            setTimeout(() => modal.classList.add('hidden'), 300);
        };

        copyBtn.onclick = () => {
            navigator.clipboard.writeText(rawYAML).then(() => {
                const originalIcon = copyBtn.innerHTML;
                copyBtn.innerHTML = '<svg class="h-5 w-5 text-green-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>';
                setTimeout(() => copyBtn.innerHTML = originalIcon, 2000);
            });
        };

        downloadBtn.onclick = () => {
            const blob = new Blob([rawYAML], { type: 'text/yaml' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = 'pipeline.yaml';
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        };

        closeBtn.onclick = closeModal;
        modal.onclick = (e) => { if (e.target === modal) closeModal(); };

        modal.classList.remove('hidden');
        setTimeout(() => {
            modal.classList.add('opacity-100');
            modalContent.classList.remove('scale-95');
        }, 10);
    }

    function setupPipelineDetailsPopover(runDetails) {
        const container = document.getElementById('pipeline-details-container');
        const btn = document.getElementById('pipeline-details-btn');
        const popover = document.getElementById('pipeline-details-popover');
        const contentEl = document.getElementById('pipeline-details-content');
        const copyBtn = document.getElementById('copy-pipeline-details-btn');

        if (!container || !btn || !popover || !contentEl || !copyBtn) return;

        // Helper to convert an object to a YAML string
        const toYAML = (obj, indent = 0) => {
            let yamlString = '';
            const spaces = '  '.repeat(indent);
            for (const key in obj) {
                if (Object.prototype.hasOwnProperty.call(obj, key)) {
                    const value = obj[key];
                    if (typeof value === 'object' && value !== null) {
                        if(Array.isArray(value)) {
                            yamlString += `${spaces}${key}:\n`;
                            value.forEach(item => {
                                if(typeof item === 'object' && item !== null) {
                                    yamlString += `${spaces}  - \n${toYAML(item, indent + 2)}`;
                                } else {
                                    yamlString += `${spaces}  - ${item}\n`;
                                }
                            });
                        } else {
                            yamlString += `${spaces}${key}:\n${toYAML(value, indent + 1)}`;
                        }
                    } else {
                        yamlString += `${spaces}${key}: ${value}\n`;
                    }
                }
            }
            return yamlString;
        };

        let pipelineYAML;
        if(typeof runDetails.pipeline_definition === 'string') {
            pipelineYAML = runDetails.pipeline_definition;
        } else if (typeof runDetails.pipeline_definition === 'object' && runDetails.pipeline_definition !== null) {
            pipelineYAML = toYAML(runDetails.pipeline_definition);
        } else {
            pipelineYAML = 'Pipeline definition is not available.';
        }

        // Escape HTML to safely render it within a <pre> tag
        const escapedYAML = pipelineYAML.replace(/</g, '&lt;').replace(/>/g, '&gt;');

        const detailsHTML = `<pre class="bg-[var(--bg-code-darker)] p-2 rounded text-cyan-700 dark:text-cyan-300 text-xs overflow-x-auto"><code>${escapedYAML}</code></pre>`;

        contentEl.innerHTML = detailsHTML;

        let enterTimeout, leaveTimeout;

        const showPopover = () => {
            clearTimeout(leaveTimeout);
            enterTimeout = setTimeout(() => {
                popover.classList.remove('hidden');
                btn.setAttribute('aria-expanded', 'true');
                setTimeout(() => {
                    popover.classList.remove('opacity-0', 'scale-95');
                }, 10);
            }, 150);
        };

        const hidePopover = () => {
            clearTimeout(enterTimeout);
            leaveTimeout = setTimeout(() => {
                popover.classList.add('opacity-0', 'scale-95');
                btn.setAttribute('aria-expanded', 'false');
                setTimeout(() => {
                    popover.classList.add('hidden');
                }, 200);
            }, 300);
        };

        container.addEventListener('mouseenter', showPopover);
        container.addEventListener('mouseleave', hidePopover);

        copyBtn.addEventListener('click', () => {
            navigator.clipboard.writeText(pipelineYAML).then(() => {
                const originalText = copyBtn.textContent;
                copyBtn.textContent = 'Copied!';
                copyBtn.disabled = true;
                setTimeout(() => {
                    copyBtn.textContent = originalText;
                    copyBtn.disabled = false;
                }, 2000);
            });
        });
    }

    function renderStepsGraph(runDetails) {
        // If nothing is expanded, optionally reset step layout scale.
        // When triggered by "toggle all" actions, preserve the current scale so user zoom feel remains.
        if (!state.expandedSteps || state.expandedSteps.size === 0) {
            if (!state._preserveScale) {
                state.stepLayoutScale = 1.0;
            }
        }

        // Only horizontal layout
        const isVerticalLayout = false;
        // Allow the main steps layout to expand more to avoid any overlap
        const MAX_STEP_LAYOUT_SCALE = 3.0;
        // Keep internal task clusters close to 1.0; we prefer growing the main layout
        const MIN_TASK_CLUSTER_SCALE = 0.9;
        const scale = Math.max(1, Math.min(MAX_STEP_LAYOUT_SCALE, state.stepLayoutScale || 1.0));
        const clusterScale = Math.max(MIN_TASK_CLUSTER_SCALE, Math.min(1.0, state.taskClusterScale || 1.0));
        // Use icon-only task graphs inside expanded steps for clarity
        const miniStyle = 'icon';
        const baseW = isVerticalLayout ? 160 : 120;
        const baseH = isVerticalLayout ? 80  : 100;
        // tighter horizontal spacing to keep graph compact when many steps are expanded
        const baseHG = isVerticalLayout ? 40  : 90;   // was 120 for horizontal
        const baseVG = isVerticalLayout ? 100 : 16;   // was 20 for horizontal
        const nodeWidth = Math.round(baseW * scale);
        const nodeHeight = Math.round(baseH * scale);
        const hGap = Math.round(baseHG * scale);
        const vGap = Math.round(baseVG * scale);
        const { nodes, edges, width, height } = calculateGraphLayout(runDetails.steps, DOM.graphWrapper, nodeWidth, nodeHeight, hGap, vGap, isVerticalLayout);

        const getEdgePath = (fromNode, toNode) => {
            const iconRadius = 14;
            const from_center_x = fromNode.x + fromNode.width / 2;
            const from_center_y = fromNode.y + fromNode.height / 2;
            const to_center_x = toNode.x + toNode.width / 2;
            const to_center_y = toNode.y + toNode.height / 2;
            let x1, y1, x2, y2, curveX, curveY;
            if (isVerticalLayout) {
                x1 = from_center_x; y1 = from_center_y + iconRadius;
                x2 = to_center_x; y2 = to_center_y - iconRadius;
                curveX = x1; curveY = y1 + (y2 - y1) * 0.5;
                return `M ${x1} ${y1} C ${curveX} ${curveY}, ${x2} ${curveY}, ${x2} ${y2}`;
            } else {
                x1 = from_center_x + iconRadius; y1 = from_center_y;
                x2 = to_center_x - iconRadius; y2 = to_center_y;
                curveX = x1 + (x2 - x1) * 0.5;
                return `M ${x1} ${y1} C ${curveX} ${y1}, ${curveX} ${y2}, ${x2} ${y2}`;
            }
        };

        // Pre-compute expanded clusters to size SVG properly
        const isRunning = !runDetails.run_info.is_complete;
        let finalWidth = width;
        let finalHeight = height;
        const clusters = [];
        const placed = []; // track placed cluster boxes to avoid overlap
        // Precompute step rectangles to avoid collisions between expanded tasks and other steps
        const stepRects = nodes.map(n => ({
            name: n.name,
            x1: n.x,
            y1: n.y,
            x2: n.x + n.width,
            y2: n.y + n.height,
        }));

        // Helper geometry for dynamic spacing: line/rect intersection
        function pointInRect(px, py, rx1, ry1, rx2, ry2) {
          return px >= rx1 && px <= rx2 && py >= ry1 && py <= ry2;
        }
        function segmentsIntersect(ax, ay, bx, by, cx, cy, dx, dy) {
          function orient(px, py, qx, qy, rx, ry) { return (qy - py) * (rx - qx) - (qx - px) * (ry - qy); }
          function onSeg(px, py, qx, qy, rx, ry) { return Math.min(px, qx) <= rx && rx <= Math.max(px, qx) && Math.min(py, qy) <= ry && ry <= Math.max(py, qy); }
          const o1 = orient(ax, ay, bx, by, cx, cy);
          const o2 = orient(ax, ay, bx, by, dx, dy);
          const o3 = orient(cx, cy, dx, dy, ax, ay);
          const o4 = orient(cx, cy, dx, dy, bx, by);
          if ((o1 > 0 && o2 < 0 || o1 < 0 && o2 > 0) && (o3 > 0 && o4 < 0 || o3 < 0 && o4 > 0)) return true;
          if (o1 === 0 && onSeg(ax, ay, bx, by, cx, cy)) return true;
          if (o2 === 0 && onSeg(ax, ay, bx, by, dx, dy)) return true;
          if (o3 === 0 && onSeg(cx, cy, dx, dy, ax, ay)) return true;
          if (o4 === 0 && onSeg(cx, cy, dx, dy, bx, by)) return true;
          return false;
        }
        function lineIntersectsRect(x1, y1, x2, y2, rx1, ry1, rx2, ry2) {
          if (pointInRect(x1, y1, rx1, ry1, rx2, ry2) || pointInRect(x2, y2, rx1, ry1, rx2, ry2)) return true;
          // check against each edge of rectangle
          return (
            segmentsIntersect(x1, y1, x2, y2, rx1, ry1, rx2, ry1) ||
            segmentsIntersect(x1, y1, x2, y2, rx2, ry1, rx2, ry2) ||
            segmentsIntersect(x1, y1, x2, y2, rx2, ry2, rx1, ry2) ||
            segmentsIntersect(x1, y1, x2, y2, rx1, ry2, rx1, ry1)
          );
        }

        // Build simple step-edge segments (center-to-center) for overlap checks
        const simpleEdgeSegments = edges.map(e => ({
          from: e.from.name,
          to: e.to.name,
          x1: e.from.x + e.from.width / 2,
          y1: e.from.y + e.from.height / 2,
          x2: e.to.x   + e.to.width   / 2,
          y2: e.to.y   + e.to.height  / 2,
        }));

        // Place expanded step clusters in flow order to reduce collisions
        const expandedStepNodes = nodes.filter(n => state.expandedSteps && state.expandedSteps.has(n.name));
        expandedStepNodes.sort((a, b) => {
            return isVerticalLayout
              ? (a.y - b.y) || (a.x - b.x)
              : (a.x - b.x) || (a.y - b.y);
        });

        expandedStepNodes.forEach(stepNode => {
            const step = runDetails.steps.find(s => s.name === stepNode.name);
            if (!step || !Array.isArray(step.tasks) || step.tasks.length === 0) return;

            const stepDef = runDetails.pipeline_definition?.steps?.find(s => s.name === stepNode.name);
            const itemsWithDeps = step.tasks.map(item => {
                if (item.task_name) {
                    const taskDef = stepDef && stepDef.tasks ? stepDef.tasks.find(t => t.name === item.task_name) : null;
                    return { ...item, depends_on: taskDef ? (taskDef.depends_on || []) : [] };
                } else {
                    return { ...item, depends_on: item.depends_on || [] };
                }
            });

        // Size the inner task graph and ALIGN its direction with the main graph
        // Horizontal steps => horizontal inner graph; Vertical steps => vertical inner graph
        // Compact task nodes inside expanded step clusters
        const tNodeW = Math.round((isVerticalLayout ? 140 : 100) * clusterScale);
        const tNodeH = Math.round((isVerticalLayout ?  70 :  72) * clusterScale);
        const tHG    = Math.max(32,  Math.round((isVerticalLayout ? 32 : 48) * clusterScale));
        const tVG    = Math.max( (isVerticalLayout ? 80 :  24), Math.round((isVerticalLayout ? 80 : 24) * clusterScale));
        const tLayout = calculateGraphLayout(itemsWithDeps, DOM.graphWrapper, tNodeW, tNodeH, tHG, tVG, isVerticalLayout);
            const stepCenterX = stepNode.x + stepNode.width / 2;
            const stepCenterY = stepNode.y + stepNode.height / 2;
            // Tighter padding around tasks inside a cluster box
            const pad = Math.max(6, Math.round(10 * Math.max(0.85, clusterScale)));
            const stackGap = 28;// spacing between clusters when pushing forward

            // Place box initially centered on the step position
            const children = edges
              .filter(e => e.from.name === stepNode.name)
              .map(e => e.to);
            const anchorX = stepCenterX;
            const anchorY = stepCenterY;

            // Corridor band between step and its children to keep the box visually “in flow”
            const corridorMargin = pad + 12;
            let bandMin, bandMax;
            if (isVerticalLayout) {
              const xs = [stepCenterX].concat(children.map(n => n.x + n.width / 2));
              const minX = Math.min.apply(null, xs);
              const maxX = Math.max.apply(null, xs);
              bandMin = (isFinite(minX) ? minX : stepCenterX) - corridorMargin;
              bandMax = (isFinite(maxX) ? maxX : stepCenterX) + corridorMargin;
            } else {
              const ys = [stepCenterY].concat(children.map(n => n.y + n.height / 2));
              const minY = Math.min.apply(null, ys);
              const maxY = Math.max.apply(null, ys);
              // For horizontal layout, widen the corridor by half the cluster height to allow vertical sliding
              const corridorBoost = Math.round((tLayout.height / 2) || 0);
              bandMin = (isFinite(minY) ? minY : stepCenterY) - corridorMargin - corridorBoost;
              bandMax = (isFinite(maxY) ? maxY : stepCenterY) + corridorMargin + corridorBoost;
            }

            // Always anchor box to the step center so it stays in place across renders
            let originX = Math.round(stepCenterX - (tLayout.width  / 2));
            let originY = Math.round(stepCenterY - (tLayout.height / 2));
            // Ensure background with padding stays inside the SVG viewBox
            if (originX < pad) originX = pad;
            // Keep expanded clusters clear of the sticky header/tabs area so the
            // collapse badge (−) is always visible. This clamps the content origin
            // to be below an estimated header height, plus some breathing room.
            const HEADER_SAFE = 84; // px — header + tabs safe zone
            const topClamp = Math.max(pad + 28, HEADER_SAFE + 16);
            if (originY < topClamp) originY = topClamp;


            // Compute background box for overlap checks
            let boxX1 = originX - pad;
            let boxX2 = originX + tLayout.width + pad;
            let boxY1 = originY - pad;
            let boxY2 = originY + tLayout.height + pad;

            // Helper to test overlap against placed boxes and step rects
            const overlapsAny = (x1, y1, x2, y2) => {
              for (const pc of placed) {
                if (x1 < pc.x2 && x2 > pc.x1 && y1 < pc.y2 && y2 > pc.y1) return true;
              }
              for (const sr of stepRects) {
                if (sr.name === stepNode.name) continue;
                if (x1 < sr.x2 && x2 > sr.x1 && y1 < sr.y2 && y2 > sr.y1) return true;
              }
              return false;
            };

            const overlapsStepsOnly = (x1, y1, x2, y2) => {
              for (const sr of stepRects) {
                if (sr.name === stepNode.name) continue;
                if (x1 < sr.x2 && x2 > sr.x1 && y1 < sr.y2 && y2 > sr.y1) return true;
              }
              return false;
            };

            // If the box cannot fit within the corridor band, mark for scale-up
            let needsCorridorScale = false;
            if (isVerticalLayout) {
              const corridorWidth = (bandMax - bandMin);
              if ((tLayout.width + 2 * pad) > (corridorWidth - 8)) needsCorridorScale = true;
            } else {
              const corridorHeight = (bandMax - bandMin);
              if ((tLayout.height + 2 * pad) > (corridorHeight - 8)) needsCorridorScale = true;
            }

            // (Edge intersection check moved to after final placement)

            // Try to smart-position the cluster within the corridor band to avoid overlap
            const fits = (x1, y1, x2, y2) => !overlapsAny(x1, y1, x2, y2);
            const centerX = () => originX + (tLayout.width / 2);
            const centerY = () => originY + (tLayout.height / 2);
            const clamp = (val, min, max) => Math.max(min, Math.min(max, val));
            let bumpedByStep = false;

            const attemptReposition = () => {
              // scan along the orthogonal axis, alternating +/-, up to corridor bounds
              const maxIter = 80; // 80 * step ~ reasonable spread
              const step = 12;    // scan resolution in px
              if (isVerticalLayout) {
                // move left/right but keep within bandMin..bandMax
                const baseCenter = stepCenterX;
                for (let i = 0; i <= maxIter; i++) {
                  const dir = (i % 2 === 0) ? 1 : -1;
                  const delta = Math.floor(i / 2) * step * dir;
                  const candCenter = baseCenter + delta;
                  if (candCenter < bandMin || candCenter > bandMax) continue;
                  const candOriginX = Math.round(candCenter - (tLayout.width / 2));
                  const cx1 = candOriginX - pad;
                  const cx2 = candOriginX + tLayout.width + pad;
                  const cy1 = originY - pad;
                  const cy2 = originY + tLayout.height + pad;
                  if (fits(cx1, cy1, cx2, cy2)) {
                    originX = candOriginX;
                    boxX1 = cx1; boxX2 = cx2; boxY1 = cy1; boxY2 = cy2;
                    return true;
                  }
                }
              } else {
                // move up/down but keep within bandMin..bandMax
                const baseCenter = stepCenterY;
                // Honor header safe area when testing candidate positions
                const HEADER_SAFE = 84; // must match clamp above
                const minCenter = (Math.max(topClamp, HEADER_SAFE + 16)) + (tLayout.height / 2);
                for (let i = 0; i <= maxIter; i++) {
                  const dir = (i % 2 === 0) ? 1 : -1;
                  const delta = Math.floor(i / 2) * step * dir;
                  const candCenter = baseCenter + delta;
                  if (candCenter < bandMin || candCenter > bandMax) continue;
                  if (candCenter < minCenter) continue; // keep below sticky header
                  const candOriginY = Math.round(candCenter - (tLayout.height / 2));
                  const cx1 = originX - pad;
                  const cx2 = originX + tLayout.width + pad;
                  const cy1 = candOriginY - pad;
                  const cy2 = candOriginY + tLayout.height + pad;
                  if (fits(cx1, cy1, cx2, cy2)) {
                    originY = candOriginY;
                    boxX1 = cx1; boxX2 = cx2; boxY1 = cy1; boxY2 = cy2;
                    return true;
                  }
                }
              }
              return false;
            };

            if (overlapsAny(boxX1, boxY1, boxX2, boxY2)) {
              bumpedByStep = !attemptReposition();
            }

            // Ensure overall SVG fully contains the expanded cluster box (including padding)
            finalWidth = Math.max(finalWidth, boxX2 + 24);
            finalHeight = Math.max(finalHeight, boxY2 + 24);

            const rootNames = itemsWithDeps.filter(t => !t.depends_on || t.depends_on.length === 0)
              .map(t => t.task_name || t.name);

            // Save placed cluster rect and remember position
            placed.push({ x1: boxX1, x2: boxX2, y1: boxY1, y2: boxY2 });
            if (!state.expandedStepPositions) state.expandedStepPositions = new Map();
            state.expandedStepPositions.set(stepNode.name, { x: originX, y: originY });

            // Detect intersections with other step edges (not involving this step) at the final position
            const hitsEdges = simpleEdgeSegments
              .filter(s => s.from !== stepNode.name && s.to !== stepNode.name)
              .some(seg => lineIntersectsRect(seg.x1, seg.y1, seg.x2, seg.y2, boxX1, boxY1, boxX2, boxY2));

            clusters.push({ stepNode, stepName: stepNode.name, layout: tLayout, originX, originY, stepCenterX, rootNames, pad, bumpedByStep, needsCorridorScale, hitsEdges });
        });

        // Dynamic spacing: if any cluster overlaps steps/boxes/edges or corridor is too narrow,
        // iteratively adjust sizes and re-render until clear or caps reached.
        const needMoreSpace = clusters.some(c => c.bumpedByStep || c.needsCorridorScale || c.hitsEdges);
        if (needMoreSpace) {
            const tryIncreaseStepScale = () => {
              const prevStep = state.stepLayoutScale || 1.0;
              const inc = prevStep < 1.4 ? 0.2 : 0.12;
              const nextStep = Math.min(MAX_STEP_LAYOUT_SCALE, prevStep + inc);
              if (nextStep > prevStep + 1e-6) {
                  state.stepLayoutScale = nextStep;
                  if (!state._preserveScale) state._fitOnNextStepsRender = true;
                  renderStepsGraph(runDetails);
                  return true;
              }
              return false;
            };
            const tryDecreaseClusterScale = () => {
              const prevCluster = state.taskClusterScale || 1.0;
              const dec = prevCluster > 0.95 ? 0.05 : 0.03;
              const nextCluster = Math.max(MIN_TASK_CLUSTER_SCALE, prevCluster - dec);
              if (nextCluster < prevCluster - 1e-6) {
                  state.taskClusterScale = nextCluster;
                  if (!state._preserveScale) state._fitOnNextStepsRender = true;
                  renderStepsGraph(runDetails);
                  return true;
              }
              return false;
            };

            // Always prefer expanding the main step layout first.
            if (tryIncreaseStepScale()) return;
            if (tryDecreaseClusterScale()) return;
        }

        let svgContent = `<svg width="${finalWidth}" height="${finalHeight}" xmlns="http://www.w3.org/2000/svg">
            <defs>
                <marker id="arrowhead" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
                  <path d="M0,0 L8,4 L0,8 Q2.4,4 0,0 Z" class="fill-current text-gray-300 dark:text-gray-600" />
                </marker>
                <marker id="arrowhead-completed" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
                  <path d="M0,0 L8,4 L0,8 Q2.4,4 0,0 Z" class="fill-current text-[var(--text-accent)]" />
                </marker>
                <!-- Compact, crisp arrows for task graphs -->
                <marker id="task_arrow" viewBox="0 0 10 10" refX="9" refY="5"
                        markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
                  <path d="M0,0 L10,5 L0,10 Q2.8,5 0,0 Z" class="fill-current text-gray-400 dark:text-gray-600" />
                </marker>
                <!-- Markers for task box connectors (match Tasks view) -->
                <marker id="task_arrow_box_secondary" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
                  <path d="M0,0 L10,5 L0,10 Q2.8,5 0,0 Z" fill="var(--border-secondary)"></path>
                </marker>
                <marker id="task_arrow_box_accent" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
                  <path d="M0,0 L10,5 L0,10 Q2.8,5 0,0 Z" fill="var(--border-accent)"></path>
                </marker>
                ${isRunning ? `<linearGradient id="edge-gradient" x1="0%" y1="0%" x2="100%" y2="0%"><stop offset="0%" style="stop-color:var(--border-secondary);" /><stop offset="50%" style="stop-color:var(--border-accent);" /><stop offset="100%" style="stop-color:var(--border-secondary);" /></linearGradient>` : ''}
            </defs>
            <rect x="0" y="0" width="${finalWidth}" height="${finalHeight}" fill="transparent" style="pointer-events:all"></rect>`;

        // Build quick lookups for step indegree/outdegree to integrate tasks into flow
        const stepIn = new Map();
        const stepOut = new Map();
        nodes.forEach(n => { stepIn.set(n.name, 0); stepOut.set(n.name, 0); });
        edges.forEach(e => {
            stepOut.set(e.from.name, (stepOut.get(e.from.name) || 0) + 1);
            stepIn.set(e.to.name, (stepIn.get(e.to.name) || 0) + 1);
        });

        const clustersByStep = new Map();
        clusters.forEach(c => clustersByStep.set(c.stepName, c));

        const iconRadius = 14, arrowPad = 8;
        const pathBetween = (x1, y1, x2, y2) => {
            if (isVerticalLayout) {
                const curveY = y1 + (y2 - y1) * 0.5;
                return `M ${x1} ${y1} C ${x1} ${curveY}, ${x2} ${curveY}, ${x2} ${y2}`;
            } else {
                const curveX = x1 + (x2 - x1) * 0.5;
                return `M ${x1} ${y1} C ${curveX} ${y1}, ${curveX} ${y2}, ${x2} ${y2}`;
            }
        };

        // We collect SVG layers to keep main edges above clusters for readability
        let svgEdges = '';
        let svgNodes = '';
        let svgClusters = '';
        let svgOverlays = '';

        // Step-level edges (single input/output per expanded box) – will be appended after clusters
        edges.forEach(edge => {
            const fromBox = clustersByStep.get(edge.from.name);
            const toBox   = clustersByStep.get(edge.to.name);
            const isCompletedEdge = edge.from.status === 'Success' && (edge.to.status.toLowerCase() !== 'pending' && edge.to.status.toLowerCase() !== 'skipped');
            const marker = isCompletedEdge ? 'url(#arrowhead-completed)' : 'url(#arrowhead)';
            const edgeClasses = ['edge-path', 'edge-path--glow'];
            if (isCompletedEdge) edgeClasses.push('edge-path--completed');
            if (isRunning && isCompletedEdge) edgeClasses.push('edge-path--running');

            const fromCx = edge.from.x + edge.from.width / 2;
            const fromCy = edge.from.y + edge.from.height / 2;
            const toCx   = edge.to.x   + edge.to.width   / 2;
            const toCy   = edge.to.y   + edge.to.height  / 2;

            let sx, sy, tx, ty;
            if (fromBox) {
                sx = isVerticalLayout ? (fromBox.originX + fromBox.layout.width / 2) : (fromBox.originX + fromBox.layout.width);
                sy = isVerticalLayout ? (fromBox.originY + fromBox.layout.height) : (fromBox.originY + fromBox.layout.height / 2);
            } else {
                sx = isVerticalLayout ? fromCx : (fromCx + iconRadius + arrowPad);
                sy = isVerticalLayout ? (fromCy + iconRadius) : fromCy;
            }

            if (toBox) {
                tx = isVerticalLayout ? (toBox.originX + toBox.layout.width / 2) : (toBox.originX);
                ty = isVerticalLayout ? (toBox.originY) : (toBox.originY + toBox.layout.height / 2);
            } else {
                tx = isVerticalLayout ? toCx : (toCx - iconRadius - arrowPad);
                ty = isVerticalLayout ? (toCy - iconRadius) : toCy;
            }

            const d = pathBetween(sx, sy, tx, ty);
            // halo then main stroke for readability
            svgEdges += `<path d="${d}" class="edge-path-halo"></path>`;
            svgEdges += `<path d="${d}" class="${edgeClasses.join(' ')}" marker-end="${marker}"></path>`;
        });

        // Step nodes
        nodes.forEach(node => {
            const config = statusConfig[node.status.toLowerCase()] || statusConfig.pending;
            const node_center_x = node.x + node.width / 2;
            const node_center_y = node.y + node.height / 2;
            const originalStep = runDetails.pipeline_definition.steps.find(s => s.name === node.name);
            const isExpanded = clustersByStep.has(node.name);
            if (isExpanded) return; // replaced by a box
            let subText = `<text x="${node_center_x}" y="${node_center_y + 53}" text-anchor="middle" class="text-xs fill-current text-[var(--text-secondary)]">${node.duration || '...'}</text>`;
            if (originalStep && originalStep.include) {
                let includeType = originalStep.include.startsWith('pipeline:') ? '(Included Pipeline)' : '(Included Step)';
                let linkClass = originalStep.include.startsWith('pipeline:') ? 'text-[var(--text-link)] hover:underline' : 'text-[var(--text-accent)]';
                const childRun = originalStep.include.startsWith('pipeline:') ? runDetails.child_runs.find(cr => cr.parent_step_name === node.name) : null;
                const yInclude = node_center_y + 53;
                const yDuration = node_center_y + 67;
                if (childRun) {
                     subText = `
                       <a data-included-link="true" href="#/pipelineruns/run/${childRun.run_id}" class="fill-current ${linkClass}">
                         <text x="${node_center_x}" y="${yInclude}" text-anchor="middle" class="text-xs">${includeType}</text>
                       </a>
                       <text x="${node_center_x}" y="${yDuration}" text-anchor="middle" class="text-xs fill-current text-[var(--text-secondary)]">${node.duration || '...'}</text>`;
                } else {
                     subText = `
                       <text x="${node_center_x}" y="${yInclude}" text-anchor="middle" class="text-xs fill-current ${linkClass}">${includeType}</text>
                       <text x="${node_center_x}" y="${yDuration}" text-anchor="middle" class="text-xs fill-current text-[var(--text-secondary)]">${node.duration || '...'}</text>`;
                }
            }
            const caret = (state.expandedSteps && state.expandedSteps.has(node.name)) ? '▾' : '▸';
            const infoCx = node_center_x + 22;
            const infoCy = node_center_y - 22;
            svgNodes += `
              <g class="graph-node" data-step-name="${node.name}">
                <g transform="translate(${node_center_x}, ${node_center_y})">
                  <path d="${config.icon}" transform="translate(-12, -12) scale(1.2)" stroke-width="2" class="stroke-current ${config.color}" fill="none"/>
                </g>
                <text x="${node_center_x}" y="${node_center_y + 35}" text-anchor="middle" class="text-sm font-semibold fill-current text-[var(--text-primary)]">${caret} ${node.name}</text>
                ${subText}
                <g class="step-info" data-step-info="true" data-step-name="${node.name}">
                  <circle cx="${infoCx}" cy="${infoCy}" r="7" fill="var(--icon-info-bg)"></circle>
                  <circle cx="${infoCx - 3.8}" cy="${infoCy}" r="1.1" fill="var(--icon-glyph)"></circle>
                  <circle cx="${infoCx}" cy="${infoCy}" r="1.1" fill="var(--icon-glyph)"></circle>
                  <circle cx="${infoCx + 3.8}" cy="${infoCy}" r="1.1" fill="var(--icon-glyph)"></circle>
                </g>
              </g>`;
        });

        // Expanded task clusters per step (boxed with single input/output)
        clusters.forEach(cluster => {
            const { layout, originX, originY, stepCenterX, rootNames, pad } = cluster;
            svgClusters += `<g class="step-cluster" data-step-name="${cluster.stepName}">`;
            // Background card (box in flow)
            const bgX = originX - pad;
            const bgY = originY - pad;
            const bgW = layout.width + pad * 2;
            const bgH = layout.height + pad * 2;
            svgClusters += `<rect x="${bgX}" y="${bgY}" width="${bgW}" height="${bgH}" rx="12" ry="12" class="step-cluster-box" style="pointer-events:none"></rect>`;
            // Ports: keep only the RIGHT-side indicator (different color), remove others
            // Do this consistently for both orientations so the cue is always on the right.
            {
                const portRightX = originX + layout.width + 2;
                const portRightY = originY + layout.height / 2;
                svgClusters += `<circle cx="${portRightX}" cy="${portRightY}" r="3" fill="var(--border-secondary)"></circle>`;
            }

            // Task edges inside cluster – icon style only, aligned with main direction
            layout.edges.forEach(edge => {
                const from_center_x = originX + edge.from.x + edge.from.width / 2;
                const from_center_y = originY + edge.from.y + edge.from.height / 2;
                const to_center_x   = originX + edge.to.x   + edge.to.width   / 2;
                const to_center_y   = originY + edge.to.y   + edge.to.height  / 2;
                const arrowPad = 10;
                let pathData;
                if (isVerticalLayout) {
                    const x1 = from_center_x;
                    const y1 = from_center_y + (edge.from.height / 2) + arrowPad;
                    const x2 = to_center_x;
                    const y2 = to_center_y - (edge.to.height / 2)  - arrowPad;
                    const curveY = y1 + (y2 - y1) * 0.5;
                    pathData = `M ${x1} ${y1} C ${x1} ${curveY}, ${x2} ${curveY}, ${x2} ${y2}`;
                } else {
                    const x1 = from_center_x + (edge.from.width / 2) + arrowPad;
                    const y1 = from_center_y;
                    const x2 = to_center_x - (edge.to.width / 2) - arrowPad;
                    const y2 = to_center_y;
                    const curveX = x1 + (x2 - x1) * 0.5;
                    pathData = `M ${x1} ${y1} C ${curveX} ${y1}, ${curveX} ${y2}, ${x2} ${y2}`;
                }
                // Use the same simple arrow as the modal task graph; fade internals for readability
                svgClusters += `<path d="${pathData}" class="edge-path edge-path--internal" marker-end="url(#task_arrow)"></path>`;
            });

            // (No external connections to internal tasks; box presents single in/out)

            // Task nodes: icon-only nodes for clarity
            layout.nodes.forEach(n => {
                const status = (n.status || 'pending').toLowerCase();
                const config = statusConfig[status] || statusConfig.pending;
                const duration = formatDuration(n.started_at, n.finished_at);
                const itemName = n.task_name || n.name;
                const cx = originX + n.x + n.width / 2;
                const cy = originY + n.y + n.height / 2;
                svgClusters += `
                    <g class=\"graph-node\" transform=\"translate(${cx}, ${cy})\"> 
                      <path d=\"${config.icon}\" transform=\"translate(-12, -12) scale(1.1)\" stroke-width=\"2\" class=\"stroke-current ${config.color}\" fill=\"none\"></path>
                      <text x=\"0\" y=\"30\" text-anchor=\"middle\" class=\"text-xs font-medium fill-current text-[var(--text-secondary)]\">${itemName}</text>
                      <text x=\"0\" y=\"46\" text-anchor=\"middle\" class=\"text-[10px] fill-current text-[var(--text-secondary)]\">${duration}</text>
                    </g>`;
            });
            // Collapse badge (minus) in top-right corner of the cluster box
            // Place collapse badge slightly inside the content area so it's never clipped by the top border
            const tbx = originX + layout.width + pad - 14;
            const tby = (originY - pad) + 20;   
            svgClusters += `
              <g class=\"step-collapse-badge\"> 
                <circle cx=\"${tbx}\" cy=\"${tby}\" r=\"7\" fill=\"var(--icon-collapse-bg)\"></circle>
                <path d=\"M ${tbx - 4} ${tby + 1.5} L ${tbx} ${tby - 2.5} L ${tbx + 4} ${tby + 1.5}\" stroke=\"var(--icon-glyph)\" stroke-width=\"1.6\" stroke-linecap=\"round\" stroke-linejoin=\"round\" fill=\"none\"></path>
              </g>
            </g>`;
            // Top overlay collapse badge (duplicate for z-order)
            (function(){
              const tbx_ = originX + layout.width + pad - 14;
              // Keep the badge inside the visible SVG even if an upstream calc misplaces originY
              const HEADER_SAFE = 84; // header + tabs
              const minBadgeY = Math.max(8, HEADER_SAFE + 8);
              const tby_ = Math.max(minBadgeY, (originY - pad) + 20);
              svgOverlays += `\n                  <g class=\"step-collapse-badge\" data-step-name=\"${cluster.stepName}\">\n                    <circle cx=\"${tbx_}\" cy=\"${tby_}\" r=\"7\" fill=\"var(--icon-collapse-bg)\"></circle>\n                    <path d=\"M ${tbx_ - 4} ${tby_ + 1.5} L ${tbx_} ${tby_ - 2.5} L ${tbx_ + 4} ${tby_ + 1.5}\" stroke=\"var(--icon-glyph)\" stroke-width=\"1.6\" stroke-linecap=\"round\" stroke-linejoin=\"round\" fill=\"none\"></path>\n                  </g>`;
              // Info badge: move to right side near collapse (accent circle with modern "i")
              const ibx_ = tbx_ - 22;
              const iby_ = tby_;
              svgOverlays += `\n                  <g class=\"step-info\" data-step-info=\"true\" data-step-name=\"${cluster.stepName}\">\n                    <circle cx=\"${ibx_}\" cy=\"${iby_}\" r=\"7\" fill=\"var(--icon-info-bg)\"></circle>\n                    <circle cx=\"${ibx_ - 3.8}\" cy=\"${iby_}\" r=\"1.1\" fill=\"var(--icon-glyph)\"></circle>\n                    <circle cx=\"${ibx_}\" cy=\"${iby_}\" r=\"1.1\" fill=\"var(--icon-glyph)\"></circle>\n                    <circle cx=\"${ibx_ + 3.8}\" cy=\"${iby_}\" r=\"1.1\" fill=\"var(--icon-glyph)\"></circle>\n                  </g>`;
            })();
        });

  // Compose final SVG layers: clusters (background), edges, nodes, overlays
  DOM.graphWrapper.innerHTML = svgContent + svgClusters + svgEdges + svgNodes + svgOverlays + `</svg>`;
  try { updateExpandToggleLabel(); } catch {}

        // Clear the one-shot just-expanded hints after render cycle completes (allows scale pass to reuse them in same tick)
        setTimeout(() => { if (state._justExpandedSteps) state._justExpandedSteps.clear(); }, 0);

        // Persist current Steps layout (expanded set, positions, scale) per run
        try {
          const runId = state.currentRunData?.run_info?.run_id;
          if (runId) {
            const key = `nopsai_steps_layout:${runId}`;
            const positions = {};
            if (state.expandedStepPositions && typeof state.expandedStepPositions.forEach === 'function') {
              state.expandedStepPositions.forEach((pos, name) => { positions[name] = { x: pos.x, y: pos.y }; });
            }
            const payload = {
              expanded: Array.from(state.expandedSteps || []),
              positions,
              scale: state.stepLayoutScale || 1.0,
              tasksScale: state.taskClusterScale || 1.0,
            };
            localStorage.setItem(key, JSON.stringify(payload));
          }
        } catch {}

        initPanAndZoom('steps', { width: finalWidth, height: finalHeight });
        // Clear one-shot preserve flag after render
        if (state._preserveScale) delete state._preserveScale;
        // In case this ran while hidden, ensure a binding once visible
        requestAnimationFrame(() => ensurePanzoomBound('steps'));
        try { updateExpandToggleLabel(); } catch {}
        if (state._justExpandedSteps && state._justExpandedSteps.size > 0) {
          const last = Array.from(state._justExpandedSteps).pop();
          // Nudge only; avoid surprising recenter
          setTimeout(() => nudgeStepIntoView(last), 50);
        }
    }

    function makeResizable(card, step) {
  const handle = card.querySelector('.resize-handle');
  const miniGraphContainer = card.querySelector('.task-graph-mini-container');
  if (!handle) return;

  handle.addEventListener('mousedown', function (e) {
e.preventDefault();
e.stopPropagation();

const startX = e.clientX;
const startY = e.clientY;
const startWidth  = parseInt(document.defaultView.getComputedStyle(card).width, 10);
const startHeight = parseInt(document.defaultView.getComputedStyle(card).height, 10);

function doDrag(ev) {
  card.style.width  = (startWidth  + ev.clientX - startX) + 'px';
  card.style.height = (startHeight + ev.clientY - startY) + 'px';
}

function stopDrag() {
  document.documentElement.removeEventListener('mousemove', doDrag, false);
  document.documentElement.removeEventListener('mouseup', stopDrag, false);

  const style = localStorage.getItem('tasksMiniStyle') || 'icon';
  renderMiniForStep(miniGraphContainer, step, style);
  // Refresh inter-card connectors so arrow endpoints match the new card edge
  if (state._lastStepLayout && state._lastStepElements) {
    const isRunning = !state.currentRunData?.run_info?.is_complete;
    drawStepConnections(state._lastStepLayout, state._lastStepElements, { running: isRunning });
  }
}

document.documentElement.addEventListener('mousemove', doDrag, false);
document.documentElement.addEventListener('mouseup', stopDrag, false);
  });
}

function renderMiniForStep(container, step, style) {
  // Always use icon-only task graph for clarity
  renderTaskGraph(container, step.name, step.tasks);
}

function renderTaskGraphBoxes(container, stepName, tasks) {
  if (!state.currentRunData || !state.currentRunData.pipeline_definition || !state.currentRunData.pipeline_definition.steps) {
container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">Waiting for pipeline data...</p>`;
return;
  }
  if (!tasks || tasks.length === 0) {
container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">This step has no tasks defined.</p>`;
return;
  }

  const stepDef = state.currentRunData.pipeline_definition.steps.find(s => s.name === stepName);
  const itemsWithDeps = tasks.map(item => {
if (item.task_name) {
  const taskDef = stepDef && stepDef.tasks ? stepDef.tasks.find(t => t.name === item.task_name) : null;
  return { ...item, depends_on: taskDef ? (taskDef.depends_on || []) : [] };
} else {
  return { ...item, depends_on: item.depends_on || [] };
}
  });

  const isVerticalLayout = container.clientWidth < 600;
  // Compact mini graphs to fit smaller task cards
  const nodeWidth  = isVerticalLayout ? 84  : 120;
  const nodeHeight = isVerticalLayout ? 48  : 72;
  const hGap = isVerticalLayout ? 24 : 60;
  const vGap = isVerticalLayout ? 64 : 28;

  const { nodes, edges, width, height } =
calculateGraphLayout(itemsWithDeps, container, nodeWidth, nodeHeight, hGap, vGap, isVerticalLayout);

  const arrowPad = 12;
  const getEdgePath = (fromNode, toNode) => {
const fc = { x: fromNode.x + fromNode.width / 2, y: fromNode.y + fromNode.height / 2 };
const tc = { x: toNode.x   + toNode.width   / 2, y: toNode.y   + toNode.height   / 2 };
if (isVerticalLayout) {
  const x1 = fc.x, y1 = fc.y + (fromNode.height / 2) + arrowPad;
  const x2 = tc.x, y2 = tc.y - (toNode.height / 2)  - arrowPad;
  const curveY = y1 + (y2 - y1) * 0.5;
  return `M ${x1} ${y1} C ${x1} ${curveY}, ${x2} ${curveY}, ${x2} ${y2}`;
} else {
  const x1 = fc.x + (fromNode.width / 2) + arrowPad, y1 = fc.y;
  const x2 = tc.x - (toNode.width  / 2) - arrowPad, y2 = tc.y;
  const curveX = x1 + (x2 - x1) * 0.5;
  return `M ${x1} ${y1} C ${curveX} ${y1}, ${curveX} ${y2}, ${x2} ${y2}`;
}
  };

  // Build SVG with CSS vars via inline "style" (so they resolve inside SVG reliably)
let svg = `<svg width="${width}" height="${height}" xmlns="http://www.w3.org/2000/svg"
           style="max-width:100%;max-height:100%;display:block">
  <defs>
<marker id="task_arrow_box_secondary" viewBox="0 0 10 10" refX="9" refY="5"
        markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
  <path d="M0,0 L10,5 L0,10 Q2.8,5 0,0 Z" fill="var(--border-secondary)"></path>
</marker>
<marker id="task_arrow_box_accent" viewBox="0 0 10 10" refX="9" refY="5"
        markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
  <path d="M0,0 L10,5 L0,10 Q2.8,5 0,0 Z" fill="var(--border-accent)"></path>
</marker>
  </defs>`;

const nodeNames = new Set(nodes.map(n => n.task_name || n.name));
const filteredEdges = edges.filter(e =>
  nodeNames.has(e.from.task_name || e.from.name) &&
  nodeNames.has(e.to.task_name   || e.to.name)
);
const connectors = filteredEdges.length ? filteredEdges : (
  nodes.length > 1
? nodes.slice(0, -1).map((_, i) => ({ from: nodes[i], to: nodes[i + 1] }))
: []
);
edges.forEach(e => {
  // drop edges whose endpoints aren’t in the layout for any reason
  if (!nodeNames.has(e.from.task_name || e.from.name) ||
  !nodeNames.has(e.to.task_name   || e.to.name)) {
e.__drop = true;
  }
});
const validEdges = edges.filter(e => !e.__drop);

// edges: pick color + marker per edge
connectors.forEach(edge => {
  const pathData = getEdgePath(edge.from, edge.to);
  const isCompleted = (edge.from.status || '').toLowerCase() === 'success';
  const stroke = isCompleted ? 'var(--border-accent)' : 'var(--border-secondary)';
  const marker = isCompleted ? 'url(#task_arrow_box_accent)' : 'url(#task_arrow_box_secondary)';
  svg += `<path d="${pathData}" class="edge-path" stroke="${stroke}" marker-end="${marker}"></path>`;
});


  // nodes as boxes + icon + labels
  nodes.forEach(node => {
const status = (node.status || 'pending').toLowerCase();
const config = statusConfig[status] || statusConfig.pending;
const duration = formatDuration(node.started_at, node.finished_at);
const itemName = node.task_name || node.name;

const x = node.x, y = node.y, w = node.width, h = node.height;
const cx = x + w / 2;

svg += `
  <g transform="translate(${x}, ${y})">
    <rect width="${w}" height="${h}" rx="10" ry="10"
          style="fill:var(--bg-primary);stroke:var(--border-primary);stroke-width:1.5"
          vector-effect="non-scaling-stroke"></rect>
    <g transform="translate(10, 10)">
      <path d="${config.icon}" stroke="currentColor" class="${config.color}" fill="none" stroke-width="2"></path>
    </g>
    <text x="${w - 6}" y="16" text-anchor="end"
          class="text-[10px] fill-current ${config.color} font-medium">${(status || '').toUpperCase()}</text>
    <text x="${cx}" y="${h - 18}" text-anchor="middle"
          class="fill-current text-[var(--text-primary)] font-semibold" style="font-size:11px">${itemName}</text>
    <text x="${cx}" y="${h - 6}" text-anchor="middle"
          class="text-[10px] fill-current text-[var(--text-secondary)]">${duration}</text>
  </g>`;
  });

  svg += `</svg>`;
  container.innerHTML = svg;
}

    function renderTasksGraph(runDetails) {
  // Clear old
  DOM.tasksGraphWrapper.querySelectorAll('[id^="step-card-"]').forEach(el => el.remove());
  DOM.tasksGraphConnections.innerHTML = '';

  const stepsWithTasks = runDetails.steps.filter(s => s.tasks && s.tasks.length > 0);
  if (stepsWithTasks.length === 0) {
DOM.tasksEmpty.classList.remove('hidden');
return;
  }
  DOM.tasksEmpty.classList.add('hidden');

  // Layout for step cards
  const stepLayout = calculateGraphLayout(
runDetails.steps,
DOM.tasksGraphWrapper,
120, 96,    // much smaller cards
32, 24,     // compact gaps
false
  );

  // wrapper size == content size (for precise centering)
  DOM.tasksGraphWrapper.style.width  = `${stepLayout.width}px`;
  DOM.tasksGraphWrapper.style.height = `${stepLayout.height}px`;

  const stepElements = new Map();
  const miniStyle = (localStorage.getItem('tasksMiniStyle') || 'icon'); // 'icon' | 'box'

  stepLayout.nodes.forEach(stepNode => {
const step = runDetails.steps.find(s => s.name === stepNode.name);
if (!step) return;

const card = document.createElement('div');
card.id = `step-card-${step.name}`;
card.className = 'task-step-card bg-[var(--bg-secondary)] rounded-md p-1 shadow-sm border flex-shrink-0 flex flex-col absolute overflow-hidden z-10';
card.style.left = `${stepNode.x}px`;
card.style.top = `${stepNode.y}px`;
card.style.width = `${stepNode.width}px`;
card.style.height = `${stepNode.height}px`;
card.dataset.stepName = step.name;

const stepStatusConfig = statusConfig[step.status.toLowerCase()] || statusConfig.pending;

card.innerHTML = `
  <div class="flex items-center justify-between mb-0 flex-shrink-0 cursor-pointer" data-role="header">
    <h3 class="text-sm font-semibold text-[var(--text-primary)] truncate pr-1">${step.name}</h3>
    <span class="task-status-pill ${stepStatusConfig.color}">${step.status}</span>
  </div>
  <div class="task-graph-mini-container bg-[var(--bg-primary)] rounded-sm p-0.5 flex-1 flex items-center justify-center overflow-auto"></div>
  <div class="mt-0.5 text-[9px] text-[var(--text-secondary)] flex items-center justify-between">
    <span class="font-mono truncate">${step.duration || '0s'}</span>
    <span class="font-mono">${Array.isArray(step.tasks) ? step.tasks.length : 0}</span>
  </div>
  <div class="resize-handle"></div>
`;

// header opens modal (keeps "inside a step" behavior unchanged)
card.querySelector('[data-role="header"]').addEventListener('click', () => {
  const [, subpath, runId] = window.location.hash.slice(2).split('/');
  window.location.hash = `#/pipelineruns/${subpath}/${runId}/steps/${step.name}`;
});

DOM.tasksGraphWrapper.appendChild(card);
stepElements.set(step.name, card);

// render mini graph with chosen style
const mini = card.querySelector('.task-graph-mini-container');
renderMiniForStep(mini, step, miniStyle);

// resizable: re-render with current style on release
makeResizable(card, step);

// highlight connectors on hover
card.addEventListener('mouseenter', () => {
  DOM.tasksGraphConnections
    .querySelectorAll(`[data-from="${step.name}"] ,[data-to="${step.name}"]`)
    .forEach(p => p.classList.add('edge-path--highlight'));
});
card.addEventListener('mouseleave', () => {
  DOM.tasksGraphConnections
    .querySelectorAll(`[data-from="${step.name}"] ,[data-to="${step.name}"]`)
    .forEach(p => p.classList.remove('edge-path--highlight'));
});
  });
DOM.tasksGraphWrapper.style.width  = `${stepLayout.width}px`;
DOM.tasksGraphWrapper.style.height = `${stepLayout.height}px`;

// 🔧 Make sure the connections SVG lives inside the wrapper so it gets the same pan/zoom transform
if (DOM.tasksGraphConnections.parentElement !== DOM.tasksGraphWrapper) {
  DOM.tasksGraphWrapper.appendChild(DOM.tasksGraphConnections);
}
  // connectors between step cards
  const isRunning = !runDetails.run_info.is_complete;
  // Save to state for later refresh (e.g., after resize)
  state._lastStepLayout = stepLayout;
  state._lastStepElements = stepElements;
  drawStepConnections(stepLayout, stepElements, { running: isRunning });

  // controls (+/−/Fit + style toggle)
  ensureTasksControls();

  // center & fit
  setTimeout(() => initPanAndZoom('tasks', stepLayout), 40);
}


function stripStepFromHashWithoutRouting() {
  const hash = window.location.hash || '';
  const newHash = hash.split('/steps/')[0] || '#/pipelineruns/main';
  if (hash !== newHash) {
try {
  const url = new URL(window.location.href);
  url.hash = newHash.slice(1); // drop leading '#'
  history.replaceState(null, '', url.toString()); // no hashchange event
} catch {
  // Fallback: minimal disruption if replaceState fails
  window.location.hash = newHash;
}
  }
}

function ensureTasksControls() {
  let controls = document.getElementById('tasks-graph-controls');
  if (!controls) {
controls = document.createElement('div');
controls.id = 'tasks-graph-controls';
controls.innerHTML = `
  <button class="ctrl" data-zoom="in"  title="Zoom In">+</button>
  <button class="ctrl" data-zoom="out" title="Zoom Out">−</button>
  <button class="ctrl" data-fit       title="Fit to Screen">Fit</button>
`;
DOM.tasksGraphContainer.appendChild(controls);

// Zoom controls
controls.addEventListener('click', (e) => {
  const btn = e.target.closest('button');
  if (!btn) return;

  if (btn.dataset.zoom === 'in' && state.panzoomInstance) {
    state.panzoomInstance.zoomIn();
  } else if (btn.dataset.zoom === 'out' && state.panzoomInstance) {
    state.panzoomInstance.zoomOut();
  } else if (btn.hasAttribute('data-fit')) {
    if (typeof state.__fitToView === 'function') state.__fitToView();
  }
});
  }

  // no style toggle; always icon mode
}

    function ensureStepsControls() {
      let controls = document.getElementById('steps-graph-controls');
      if (!controls) {
        controls = document.createElement('div');
        controls.id = 'steps-graph-controls';
        controls.innerHTML = `
          <button class="ctrl" data-zoom="in"  title="Zoom In">+</button>
          <button class="ctrl" data-zoom="out" title="Zoom Out">−</button>
          <button class="ctrl" data-fit       title="Fit to Screen">Fit</button>
          <button class="ctrl" data-toggleall data-mode="expand" title="Expand All" aria-pressed="false">
            <span class="label">Expand All</span>
          </button>
          <button class="ctrl" data-reset    title="Reset Layout">Reset</button>
        `;
// Reuse tasks control styles
controls.style.position = 'absolute';
controls.style.right = '12px';
controls.style.bottom = '12px';
controls.style.zIndex = '30';
controls.style.display = 'flex';
controls.style.gap = '8px';
DOM.graphContainer.appendChild(controls);

        // Controls: zoom/fit/reset
        controls.addEventListener('click', (e) => {
          const btn = e.target.closest('button');
          if (!btn) return;
          // Guarantee there is a bound panzoom before acting
          if (!state.panzoomInstance || state._panElement !== DOM.graphWrapper) {
            ensurePanzoomBound('steps');
          }
          if (btn.dataset.zoom === 'in') {
            if (state.panzoomInstance) state.panzoomInstance.zoomIn();
          } else if (btn.dataset.zoom === 'out') {
            if (state.panzoomInstance) state.panzoomInstance.zoomOut();
          } else if (btn.hasAttribute('data-fit')) {
            if (typeof state.__fitToView === 'function') state.__fitToView();
          } else if (btn.hasAttribute('data-toggleall')) {
            // Toggle all: expand all if some are collapsed; otherwise collapse all
            const shouldExpand = (() => {
              try {
                const total = (state.currentRunData?.steps || []).length;
                const expanded = state.expandedSteps ? state.expandedSteps.size : 0;
                return total > 0 && expanded < total;
              } catch { return true; }
            })();
            try {
              const panEl = state._panElement || DOM.graphWrapper;
              const tr = panEl ? window.getComputedStyle(panEl).transform : null;
              let scale = 1, x = 0, y = 0;
              if (tr && tr !== 'none') {
                const m = tr.match(/matrix\(([^)]+)\)/);
                if (m) {
                  const v = m[1].split(',').map(parseFloat);
                  if (v.length === 6) { const a=v[0], b=v[1]; scale = Math.sqrt(a*a+b*b)||1; x=v[4]||0; y=v[5]||0; }
                }
              }
              state._stepsViewTransform = { x, y, scale };
            } catch {}
            if (shouldExpand) {
              // Do not change internal step layout scale; keep user's zoom feel
              state._preserveScale = true;
              if (state.currentRunData && Array.isArray(state.currentRunData.steps)) {
                state.expandedSteps = new Set(state.currentRunData.steps.map(s => s && s.name).filter(Boolean));
              }
            } else {
              state._preserveScale = true;
              state.expandedSteps = new Set();
              state.expandedStepPositions = new Map();
            }
            state._fitOnNextStepsRender = false;
            if (state.currentRunData) renderStepsGraph(state.currentRunData);
          } else if (btn.hasAttribute('data-reset')) {
            // Clear expanded steps, positions, and scale. Remove persisted storage for this run.
            try {
              const runId = state.currentRunData?.run_info?.run_id;
              if (runId) localStorage.removeItem(`nopsai_steps_layout:${runId}`);
        } catch {}
        state.expandedSteps = new Set();
        state.expandedStepPositions = new Map();
        state.stepLayoutScale = 1.0;
        if (state.currentRunData) {
          renderStepsGraph(state.currentRunData);
          // Apply the same baseline transform used on first load
          setTimeout(() => {
            const t = state._baselineStepsTransform;
            if (state.panzoomInstance && t && typeof t.scale === 'number') {
              try {
                state.panzoomInstance.zoom(t.scale, { animate: false });
                state.panzoomInstance.pan(t.x || 0, t.y || 0, { animate: false });
              } catch {}
            } else if (typeof state.__fitToView === 'function') {
              state.__fitToView();
            }
          }, 0);
        }
      }
    });

  }

  // match tasks controls visual for container and ctrl buttons
  controls.querySelectorAll('.ctrl').forEach(b => {
b.style.background = 'var(--bg-secondary)';
b.style.border = '1px solid var(--border-primary)';
b.style.borderRadius = '10px';
b.style.padding = '6px 10px';
b.style.fontSize = '12px';
b.style.color = 'var(--text-primary)';
  });

  // Sync the toggle button label based on current expanded state
  try { updateExpandToggleLabel(); } catch {}
}

// Update label and aria state for the expand/collapse toggle
function updateExpandToggleLabel() {
  const btn = document.querySelector('#steps-graph-controls [data-toggleall]');
  if (!btn) return;
  const total = (state.currentRunData?.steps || []).length;
  const expanded = state.expandedSteps ? state.expandedSteps.size : 0;
  const all = total > 0 && expanded >= total;
  const label = all ? 'Collapse All' : 'Expand All';
  btn.dataset.mode = all ? 'collapse' : 'expand';
  btn.setAttribute('title', label);
  btn.setAttribute('aria-pressed', all ? 'true' : 'false');
  const span = btn.querySelector('.label');
  if (span) span.textContent = label; else btn.textContent = label;
}


    function renderMiniTaskGraph(step, container) {
        if (!step.tasks || step.tasks.length === 0) {
            container.innerHTML = `<span class="text-xs text-[var(--text-secondary)]">No tasks</span>`;
            return;
        }
        const { nodes, edges, width, height } = calculateGraphLayout(step.tasks, container, 60, 50, 10, 20, false);
        let svg = `<svg width="${width}" height="${height}" class="max-w-full max-h-full">`;
        edges.forEach(edge => {
            const path = `M${edge.from.x + edge.from.width/2},${edge.from.y + edge.from.height/2} L${edge.to.x + edge.to.width/2},${edge.to.y + edge.to.height/2}`;
            svg += `<path d="${path}" class="edge-path" />`;
        });
        nodes.forEach(node => {
            const config = statusConfig[node.status.toLowerCase()] || statusConfig.pending;
            svg += `<g transform="translate(${node.x}, ${node.y})">
                <rect width="60" height="50" rx="8" ry="8" class="fill-current ${config.color} opacity-10" />
                <svg x="22" y="5" width="16" height="16" viewBox="0 0 24 24" class="${config.color}"><path d="${config.icon}" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></svg>
                <text x="30" y="38" text-anchor="middle" class="text-[10px] font-medium fill-current text-[var(--text-primary)] truncate">${node.task_name}</text>
            </g>`;
        });
        svg += `</svg>`;
        container.innerHTML = svg;
    }

    function drawStepConnections(stepLayout, stepElements, opts = { running: false }) {
  const svg = DOM.tasksGraphConnections;

  // Match the SVG’s coordinate space to the wrapper’s content box
  const w = Math.max(stepLayout.width || 0, DOM.tasksGraphWrapper.scrollWidth);
  const h = Math.max(stepLayout.height || 0, DOM.tasksGraphWrapper.scrollHeight);
  svg.setAttribute('width', w);
  svg.setAttribute('height', h);
  svg.setAttribute('viewBox', `0 0 ${w} ${h}`);

  const defs = `
<defs>
  <linearGradient id="tasks-edge-gradient" x1="0%" y1="0%" x2="100%" y2="0%">
    <stop offset="0%"   style="stop-color:var(--border-secondary);" />
    <stop offset="50%"  style="stop-color:var(--border-accent);" />
    <stop offset="100%" style="stop-color:var(--border-secondary);" />
  </linearGradient>
  <marker id="arrowhead" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
    <path d="M 0 0 L 10 5 L 0 10 z" class="fill-current text-gray-300 dark:text-gray-600" />
  </marker>
  <marker id="arrowhead-completed" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
    <path d="M 0 0 L 10 5 L 0 10 z" class="fill-current text-[var(--text-accent)]" />
  </marker>
</defs>
  `;

  let paths = '';
  stepLayout.edges.forEach(edge => {
const fromEl = stepElements.get(edge.from.name);
const toEl   = stepElements.get(edge.to.name);
if (!fromEl || !toEl) return;

// Offsets are relative to #tasks-graph-wrapper (perfect for this SVG)
const x1 = fromEl.offsetLeft + fromEl.offsetWidth;
const y1 = fromEl.offsetTop + fromEl.offsetHeight / 2;
const x2 = toEl.offsetLeft;
const y2 = toEl.offsetTop + toEl.offsetHeight / 2;
const curveX = x1 + (x2 - x1) / 2;

const isCompleted = edge.from.status === 'Success';
const marker = isCompleted ? 'url(#arrowhead-completed)' : 'url(#arrowhead)';
const extra = [
  'edge-path',
  isCompleted ? 'edge-path--completed' : '',
  'edge-path--glow',
  opts.running && isCompleted ? 'edge-path--running' : ''
].join(' ').trim();

const strokeStyle = isCompleted ? 'var(--border-accent)' : 'url(#tasks-edge-gradient)';
const d = `M ${x1} ${y1} C ${curveX} ${y1}, ${curveX} ${y2}, ${x2} ${y2}`;
paths += `<path d="${d}" class="edge-path-halo"></path>`;
paths += `<path d="${d}"
             class="${extra}"
             style="stroke:${strokeStyle};"
             marker-end="${marker}"
             data-from="${edge.from.name}"
             data-to="${edge.to.name}"></path>`;
  });

  svg.innerHTML = defs + paths;
}




    function switchGraphView(view) {
        state.currentGraphView = view;
        localStorage.setItem('graphView', view);
        DOM.graphContainer.classList.toggle('hidden', view !== 'steps');
        DOM.tasksGraphContainer.classList.toggle('hidden', view !== 'tasks');
        // Hide page scrollbars when a graph view is active
        if (DOM.pageContentWrapper) {
          const active = (view === 'steps' || view === 'tasks');
          DOM.pageContentWrapper.classList.toggle('no-scroll', active);
        }
        // Ensure pan/zoom is bound for the visible view
        if (view === 'steps') {
          initPanAndZoom('steps');
        } else if (view === 'tasks') {
          // if we have a saved layout, pass it to fit accurately
          const layout = state._lastStepLayout || null;
          initPanAndZoom('tasks', layout);
        }
        // If any previous attempt deferred binding due to hidden container, try now
        if (state._pendingPanzoom && state._pendingPanzoom.view === view) {
          const { view: pv, layout: pl } = state._pendingPanzoom;
          delete state._pendingPanzoom;
          initPanAndZoom(pv, pl || null);
        }
        // Final safety: after paint, verify there is a working panzoom
        requestAnimationFrame(() => ensurePanzoomBound(view));
    }

    function initGraphViewToggle() {
        // Tasks view removed: nothing to initialize
        return;
    }

    async function showStepDetails(stepName) {
  if (!state.currentRunData) return;

  // show the modal first so the graph container has dimensions
  DOM.modal.classList.remove('hidden');
  setTimeout(() => {
DOM.modal.classList.add('opacity-100');
DOM.modalContent.classList.remove('scale-95');
  }, 10);

  // render the content (fetch + graph)
  await renderModalForStep(state.currentRunData.run_info.run_id, stepName);

  // safety: if the SVG didn't render for any reason, re-draw once after layout
  setTimeout(() => {
const el = document.getElementById('task-graph');
if (el && el.querySelector('svg') == null) {
  const step = state.currentRunData?.steps?.find(s => s.name === stepName);
  if (step) renderTaskGraph(el, stepName, step.tasks);
}
  }, 80);
}



    async function renderModalForStep(runId, stepName, parentContext = null) {
        const runDetails = await fetchData(`/v1/runs/${runId}`);
        if (!runDetails) return;
        const step = runDetails.steps.find(s => s.name === stepName);
        if (!step) return;

        const stepDef = runDetails.pipeline_definition.steps.find(s => s.name === stepName);
        if (!stepDef) return;

        const config = statusConfig[step.status.toLowerCase()] || statusConfig.pending;
        const modalHeader = document.querySelector('#modal-content > div:first-child');
        const closeButtonHTML = modalHeader.querySelector('#close-modal-btn').outerHTML;
        let headerHTML = `
            <div>
                <h2 id="modal-title" class="text-xl font-semibold">Step: ${stepName}</h2>
                <div class="flex items-center space-x-6 mt-1 text-sm">
                    <div><span class="text-[var(--text-secondary)]">Status: </span><span id="modal-status" class="font-medium ${config.color}">${step.status}</span></div>
                    <div><span class="text-[var(--text-secondary)]">Duration: </span><span id="modal-duration" class="font-medium">${step.duration || '0s'}</span></div>
                </div>
            </div>`;
        if (parentContext) {
            const parentContextString = JSON.stringify(parentContext).replace(/"/g, '&quot;');
            headerHTML = `
            <div class="flex items-center space-x-4">
               <button id="modal-back-btn" data-parent-context='${parentContextString}' class="p-1 rounded-full text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]" title="Back to parent step">
                   <svg class="h-5 w-5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18" /></svg>
               </button>
                ${headerHTML}
            </div>`;
        }
        modalHeader.innerHTML = headerHTML + closeButtonHTML;
        const configContainer = document.getElementById('step-config-container');
        let configHTML = '';

        if (stepDef.image) {
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Image</h4><code class="text-cyan-600 dark:text-cyan-400 bg-[var(--bg-code)] px-2 py-1 rounded">${stepDef.image}</code></div>`;
        }
        if (stepDef.include) {
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Include</h4><code class="text-cyan-600 dark:text-cyan-400 bg-[var(--bg-code)] px-2 py-1 rounded">${stepDef.include}</code></div>`;
        }
        if (stepDef.depends_on && stepDef.depends_on.length > 0) {
            const dependsOnList = stepDef.depends_on.map(d => `<li class="inline-block mr-2 mb-1"><code class="text-gray-600 dark:text-gray-400 bg-[var(--bg-code)] px-2 py-1 rounded">${d}</code></li>`).join('');
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Depends On</h4><ul class="flex flex-wrap">${dependsOnList}</ul></div>`;
        }
        if (stepDef.secrets && stepDef.secrets.length > 0) {
            const secretsList = stepDef.secrets.map(s => `<li class="inline-block mr-2 mb-1"><code class="text-purple-600 dark:text-purple-400 bg-[var(--bg-code)] px-2 py-1 rounded">${s}</code></li>`).join('');
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Secrets</h4><ul class="flex flex-wrap">${secretsList}</ul></div>`;
        }
        if (stepDef.volumes && stepDef.volumes.length > 0) {
            const volumesList = stepDef.volumes.map(v => `<li class="inline-block mr-2 mb-1"><code class="text-green-600 dark:text-green-400 bg-[var(--bg-code)] px-2 py-1 rounded">${v}</code></li>`).join('');
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Volumes</h4><ul class="flex flex-wrap">${volumesList}</ul></div>`;
        }
        if (stepDef.environment && Object.keys(stepDef.environment).length > 0) {
            const envList = Object.entries(stepDef.environment).map(([k, v]) => `<li><code class="text-[var(--text-secondary)]"><span class="text-orange-600 dark:text-orange-400">${k}</span>=${v}</code></li>`).join('');
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Environment</h4><ul class="space-y-1">${envList}</ul></div>`;
        }
        if (stepDef.ignore_failure) {
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Ignore Failure</h4><span class="text-amber-600 dark:text-amber-400">true</span></div>`;
        }
         if (stepDef.include && stepDef.sync) {
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Sync</h4><span class="text-blue-600 dark:text-blue-400">true</span></div>`;
        }
        if (stepDef.llm_output_sharing === false) {
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">LLM Output Sharing</h4><span class="text-[var(--text-secondary)]">false</span></div>`;
        }
        if (stepDef.llm_content_sharing === false) {
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">LLM Content Sharing</h4><span class="text-[var(--text-secondary)]">false</span></div>`;
        }

        if (stepDef.tasks && stepDef.tasks.length > 0) {
            configHTML += `<div><h4 class="font-semibold text-[var(--text-secondary)] mb-1">Tasks</h4><div class="space-y-3 pt-2">`;
            stepDef.tasks.forEach(task => {
                configHTML += `<div class="bg-[var(--bg-primary)] p-3 rounded-md border-l-2 border-[var(--border-secondary)] space-y-3">
                                <p class="font-semibold text-[var(--text-primary)]">${task.name}</p>`;
                if (task.goal) {
                    configHTML += `<div><h5 class="text-xs font-semibold text-[var(--text-secondary)] mb-1">Goal</h5><p class="text-[var(--text-primary)] italic">"${task.goal}"</p></div>`;
                }
                if (task.script) {
                    const escapedScript = task.script.replace(/</g, '&lt;').replace(/>/g, '&gt;');
                    configHTML += `<div><h5 class="text-xs font-semibold text-[var(--text-secondary)] mb-1">Script</h5><pre class="bg-[var(--bg-code-darker)] p-2 rounded text-cyan-700 dark:text-cyan-300 text-xs overflow-x-auto"><code>${escapedScript}</code></pre></div>`;
                }
                if (task.depends_on && task.depends_on.length > 0) {
                    const dependsOnList = task.depends_on.map(d => `<li class="inline-block mr-2 mb-1"><code class="text-gray-600 dark:text-gray-400 bg-[var(--bg-code)] px-2 py-1 rounded">${d}</code></li>`).join('');
                    configHTML += `<div><h5 class="text-xs font-semibold text-[var(--text-secondary)] mb-1">Depends On</h5><ul class="flex flex-wrap">${dependsOnList}</ul></div>`;
                }
                if (task.ignore_failure) {
                    configHTML += `<div><h5 class="text-xs font-semibold text-[var(--text-secondary)] mb-1">Ignore Failure</h5><span class="text-amber-600 dark:text-amber-400">true</span></div>`;
                }
                if (task.llm_output_sharing === false) {
                    configHTML += `<div><h5 class="text-xs font-semibold text-[var(--text-secondary)] mb-1">LLM Output Sharing</h5><span class="text-[var(--text-secondary)]">false</span></div>`;
                }
                configHTML += '</div>';
            });
            configHTML += '</div></div>';
        }
        configContainer.innerHTML = configHTML;

        const taskGraphEl = document.getElementById('task-graph');
        if (stepDef.include && stepDef.include.startsWith('pipeline:')) {
            const childRun = runDetails.child_runs.find(cr => cr.parent_step_name === stepName);
            if (childRun) {
                const childRunDetails = await fetchData(`/v1/runs/${childRun.run_id}`);
                if (childRunDetails) {
                    renderTaskGraph(taskGraphEl, stepName, childRunDetails.steps, { runId: childRun.run_id, parentRunId: runId, parentStepName: stepName });
                }
            }
        } else {
            renderTaskGraph(taskGraphEl, stepName, step.tasks);
        }
    }

function renderTaskGraph(container, stepName, tasks, clickableNodeContext = null) {
  if (!state.currentRunData || !state.currentRunData.pipeline_definition || !state.currentRunData.pipeline_definition.steps) {
container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">Waiting for pipeline data...</p>`;
return;
  }
  if (!tasks || tasks.length === 0) {
container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">This step has no tasks defined.</p>`;
return;
  }

  const stepDef = state.currentRunData.pipeline_definition.steps.find(s => s.name === stepName);
  const itemsWithDeps = tasks.map(item => {
if (item.task_name) {
  const taskDef = stepDef && stepDef.tasks ? stepDef.tasks.find(t => t.name === item.task_name) : null;
  return { ...item, depends_on: taskDef ? (taskDef.depends_on || []) : [] };
} else {
  return { ...item, depends_on: item.depends_on || [] };
}
  });

  const isVerticalLayout = container.clientWidth < 700;
  const isMini = container.classList.contains('task-graph-mini-container');
  // Compact sizing for mini-graphs inside task cards; keep larger sizing elsewhere (modal)
  const nodeWidth  = isVerticalLayout ? (isMini ? 84  : 184) : (isMini ? 120 : 136);
  const nodeHeight = isVerticalLayout ? (isMini ? 48  : 92)  : (isMini ? 72  : 112);
  const hGap       = isVerticalLayout ? (isMini ? 24  : 44)  : (isMini ? 60  : 100);
  const vGap       = isVerticalLayout ? (isMini ? 64  : 120) : (isMini ? 28  : 36);

  const { nodes, edges, width, height } =
calculateGraphLayout(itemsWithDeps, container, nodeWidth, nodeHeight, hGap, vGap, isVerticalLayout);

  // keep arrows clear of node icons
  const iconRadius = 14;
  const arrowPad   = 12; // extra gap so the tip doesn’t sit on the icon

  const getEdgePath = (fromNode, toNode) => {
const from_center_x = fromNode.x + fromNode.width / 2;
const from_center_y = fromNode.y + fromNode.height / 2;
const to_center_x   = toNode.x   + toNode.width   / 2;
const to_center_y   = toNode.y   + toNode.height  / 2;

if (isVerticalLayout) {
  const x1 = from_center_x;
  const y1 = from_center_y + iconRadius + arrowPad;
  const x2 = to_center_x;
  const y2 = to_center_y   - iconRadius - arrowPad;
  const curveY = y1 + (y2 - y1) * 0.5;
  return `M ${x1} ${y1} C ${x1} ${curveY}, ${x2} ${curveY}, ${x2} ${y2}`;
} else {
  const x1 = from_center_x + iconRadius + arrowPad;
  const y1 = from_center_y;
  const x2 = to_center_x   - iconRadius - arrowPad;
  const y2 = to_center_y;
  const curveX = x1 + (x2 - x1) * 0.5;
  return `M ${x1} ${y1} C ${curveX} ${y1}, ${curveX} ${y2}, ${x2} ${y2}`;
}
  };

  let svgContent = `<svg width="${width}" height="${height}" xmlns="http://www.w3.org/2000/svg">
<defs>
  <!-- compact, crisp arrow that doesn’t distort with stroke width -->
  <marker id="task_arrow" viewBox="0 0 10 10" refX="9" refY="5"
          markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
    <path d="M0,0 L10,5 L0,10 Q2.8,5 0,0 Z" class="fill-current text-gray-400 dark:text-gray-600" />
  </marker>
</defs>`;

  // edges
  edges.forEach(edge => {
svgContent += `<path d="${getEdgePath(edge.from, edge.to)}"
                     class="edge-path"
                     marker-end="url(#task_arrow)"></path>`;
  });

  // nodes (icon + labels only)
  nodes.forEach(node => {
const status = (node.status || 'pending').toLowerCase();
const config = statusConfig[status] || statusConfig.pending;
const duration = formatDuration(node.started_at, node.finished_at);
const itemName = node.task_name || node.name;
const cx = node.x + node.width / 2;
const cy = node.y + node.height / 2;

let clickableAttrs = '';
if (clickableNodeContext) {
  const contextString = JSON.stringify({ ...clickableNodeContext, childStepName: itemName }).replace(/"/g, '&quot;');
  clickableAttrs = `class="graph-node" data-context='${contextString}'`;
}

svgContent += `
  <g transform="translate(0,0)" ${clickableAttrs} data-task-name="${itemName}">
    <g transform="translate(${cx}, ${cy})">
      <path d="${config.icon}" transform="translate(-12, -12) scale(1.1)"
            stroke-width="2" class="stroke-current ${config.color}" fill="none"/>
    </g>
    <text x="${cx}" y="${cy + 30}" text-anchor="middle"
          class="text-xs font-semibold fill-current text-[var(--text-primary)]">${itemName}</text>
    <text x="${cx}" y="${cy + 48}" text-anchor="middle"
          class="text-[10px] fill-current text-[var(--text-secondary)]">${duration}</text>
  </g>`;
  });

  container.innerHTML = svgContent + `</svg>`;
}


    function renderBreadcrumbs(groupId) {
        let breadcrumbs = [];
        let currentGroup = state.groups.find(g => g.id == groupId);
        while (currentGroup) {
            const isRepo = (currentGroup.name || '').includes('/');
            const displayName = isRepo ? currentGroup.name.split('/')[1] : currentGroup.name;
            breadcrumbs.unshift({ id: currentGroup.id, name: displayName });
            currentGroup = currentGroup.parent_id ? state.groups.find(g => g.id == currentGroup.parent_id) : null;
        }

        let html = `<a href="#/pipelineruns/main" class="text-[var(--text-secondary)] hover:text-[var(--text-accent)]">Main</a>`;
        breadcrumbs.forEach(breadcrumb => {
            html += ` <span class="mx-2 text-gray-400 dark:text-gray-500">/</span> <a href="#/pipelineruns/main/${breadcrumb.id}" class="text-[var(--text-secondary)] hover:text-[var(--text-accent)]">${breadcrumb.name}</a>`;
        });
        DOM.mainHeader.innerHTML = `<div>${html}</div>`;
    }

    async function handleRoute(hashOverride) {
    stopPolling();
    state.currentRunData = null;
    resetMainView();

    const hash = hashOverride || window.location.hash || '#/pipelineruns/main';
    const parts = hash.slice(2).split('/');
    const path = parts[0];
    const subpath = parts[1];
    const id = parts[2];
    const action = parts[3];

    DOM.pages.forEach(p => p.classList.toggle('active', p.dataset.page === path));

    if (path === 'pipelineruns') {
        await fetchGroups();

        const currentTab = (subpath === 'recent' || subpath === 'main') ? subpath : (state.currentTab || 'main');

        state.currentTab = currentTab;
        updateTabs(currentTab);

        await renderSidebar(path, currentTab);

        if (subpath === 'run' && id) {
            await fetchActiveRun(id);
            if (action === 'logs') {
                showLogsModal();
            } else {
                const stepName = parts.length > 3 && parts[3] === 'steps' ? parts[4] : null;
                if (stepName) {
                    showStepDetails(stepName);
                }
            }
        } else if (subpath === 'recent') {
            DOM.mainHeader.textContent = "Recent Pipeline Runs";
            const runs = await fetchData('/v1/runs');
            renderMainGridContent(null, runs, false);
        } else if (subpath === 'main') {
            const groupId = id ? parseInt(id, 10) : null;
            state.selectedGroupId = groupId;
            renderBreadcrumbs(groupId);
            if (groupId) {
                await fetchMainContent(groupId);
            } else {
                const rootGroups = state.groups.filter(g => g.parent_id === null || g.parent_id === 0);
                renderMainGridContent(rootGroups, null, true);
            }
        } else {
            const groupId = subpath ? parseInt(subpath, 10) : null;
            if (groupId && !isNaN(groupId)) {
                window.location.hash = `#/pipelineruns/main/${groupId}`;
            } else {
                window.location.hash = '#/pipelineruns/main';
            }
        }
    } else {
        await renderSidebar(path, 'main');
        DOM.mainHeader.textContent = path.charAt(0).toUpperCase() + path.slice(1);
        DOM.placeholder.classList.remove('hidden');
        DOM.placeholder.querySelector('h3').textContent = `Welcome to ${path}`;
        DOM.placeholder.querySelector('p').textContent = 'This page is under construction.';
    }
}

    function updateTabs(activeTab) {
        DOM.tabs.forEach(tab => {
            const isActive = (tab.dataset.tab === activeTab);
            tab.classList.toggle('border-[var(--border-accent)]', isActive);
            tab.classList.toggle('text-[var(--text-accent)]', isActive);
            tab.classList.toggle('border-transparent', !isActive);
            tab.classList.toggle('text-[var(--text-secondary)]', !isActive);
        });
    }

    function showAddGroupModal(parentId = null) {
        DOM.addGroupForm.reset();
        document.getElementById('parent-id').value = parentId;
        DOM.addGroupModal.classList.remove('hidden');
        setTimeout(() => DOM.addGroupModal.classList.add('opacity-100'), 10);
    }

    function closeAddGroupModal() {
        DOM.addGroupModal.classList.remove('opacity-100');
        setTimeout(() => DOM.addGroupModal.classList.add('hidden'), 300);
    }

    function showDeleteGroupModal(groupId, groupName) {
        state.groupToDelete = { id: groupId, name: groupName };
        DOM.deleteItemName.textContent = groupName;
        DOM.deleteGroupModal.classList.remove('hidden');
        setTimeout(() => DOM.deleteGroupModal.classList.add('opacity-100'), 10);
    }

    function closeDeleteGroupModal() {
        DOM.deleteGroupModal.classList.remove('opacity-100');
        setTimeout(() => DOM.deleteGroupModal.classList.add('hidden'), 300);
    }

    async function createGroup(event) {
        event.preventDefault();
        const formData = new FormData(event.target);
        const parentId = formData.get('parent_id') ? parseInt(formData.get('parent_id'), 10) : null;
        const name = formData.get('name');

        const siblings = state.groups.filter(g => g.parent_id === parentId);
        if (siblings.some(s => s.name === name)) {
            alert('A folder or repository with this name already exists at this level.');
            return;
        }

        const data = {
            name: name,
            parent_id: parentId,
        };
        const newGroup = await postData('/v1/groups', data);
        if (newGroup) {
            closeAddGroupModal();
            refresh();
        }
    }

    async function deleteGroup() {
        if (!state.groupToDelete) return;
        await deleteData(`/v1/groups/${state.groupToDelete.id}`);
        closeDeleteGroupModal();

        if (state.selectedGroupId == state.groupToDelete.id) {
            state.selectedGroupId = null;
             window.location.hash = '#/pipelineruns/main';
        } else {
            refresh();
        }
    }

    function closeModal() {
  // hide animation
  DOM.modal.classList.remove('opacity-100');
  DOM.modalContent.classList.add('scale-95');

  setTimeout(() => {
DOM.modal.classList.add('hidden');

// Safety: ensure the main view is still visible after closing the modal
// without relying solely on router timing.
if (false && state.currentGraphView === 'tasks') {
  DOM.graphContainer.classList.add('hidden');
  DOM.tasksGraphContainer.classList.remove('hidden');

  // If tasks graph somehow got cleared, rebuild current view.
  const hasSvg = !!DOM.tasksGraphWrapper.querySelector('svg');
  if (!hasSvg && state.currentRunData) {
    renderRunView(state.currentRunData);
    switchGraphView('steps');
  }

  // Re-center after layout settles
  if (typeof state.__fitToView === 'function') {
    requestAnimationFrame(() => state.__fitToView());
  }
}
  }, 300);
}


    function startPolling(pollingFunc, interval) {
        if (state.pollingInterval) clearInterval(state.pollingInterval);
        pollingFunc();
        state.pollingInterval = setInterval(pollingFunc, interval);
    }

    function stopPolling() {
        clearInterval(state.pollingInterval);
        state.pollingInterval = null;
    }

    function bindDomEvents() {
        DOM.mainHeader.addEventListener('click', e => {
        if (e.target.closest('#view-logs-btn')) {
            showLogsModal();
        }
    });

    DOM.closeLogsModalBtn.addEventListener('click', closeLogsModal);
    DOM.logsModal.addEventListener('click', e => { if (e.target === DOM.logsModal) closeLogsModal(); });
    DOM.copyLogsBtn.addEventListener('click', () => {
        try {
            const hasQuery = !!(state.logsSearchText && state.logsSearchText.trim());
            let linesToCopy = [];

            if (hasQuery) {
                const highlightedElements = DOM.logsContainer.querySelectorAll('.log-highlight');
                const uniqueLogEntries = new Set();

                highlightedElements.forEach(highlight => {
                    // This selector is key: it finds the parent container for BOTH structured and unstructured logs.
                    const logEntry = highlight.closest('.log-line-raw, .flex.flex-col');
                    if (logEntry) {
                        uniqueLogEntries.add(logEntry);
                    }
                });

                uniqueLogEntries.forEach(entry => {
                    linesToCopy.push(entry.innerText);
                });
            }

            // Fallback to copying everything if there's no active search, otherwise join the found entries.
            const textToCopy = linesToCopy.length > 0 ? linesToCopy.join('\n\n') : DOM.logsContainer.innerText;
            navigator.clipboard.writeText(textToCopy);

        } catch (e) {
            console.error("Copy to clipboard failed:", e);
        }

        // Provide visual feedback to the user.
        const originalIcon = DOM.copyLogsBtn.innerHTML;
        DOM.copyLogsBtn.innerHTML = '<svg class="h-5 w-5 text-green-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>';
        setTimeout(() => DOM.copyLogsBtn.innerHTML = originalIcon, 2000);
    });

    // Steps sidebar interactions
    if (DOM.logsStepsSidebar) {
      DOM.logsStepsSidebar.addEventListener('click', (e) => {
        const item = e.target.closest('.logs-step-item');
        if (item) {
          const name = item.dataset.step;
          if (state.logsSelectedSteps.has(name)) state.logsSelectedSteps.delete(name); else state.logsSelectedSteps.add(name);
          updateLogsStepList();
          state._logsFocusFirstMatch = true;
          renderLogsWithFilters({ scrollToTop: true }); 
          return;
        }
      });
    }
    if (DOM.logsStepSearch) {
      DOM.logsStepSearch.addEventListener('input', () => updateLogsStepList());
    }

    // Select all / clear
    if (DOM.logsStepsSelectAll) {
      DOM.logsStepsSelectAll.addEventListener('click', () => {
        (state.logsAllSteps || []).forEach(n => state.logsSelectedSteps.add(n));
        updateLogsStepList();
        state._logsFocusFirstMatch = true;
        renderLogsWithFilters();
      });
    }
    if (DOM.logsStepsClear) {
      DOM.logsStepsClear.addEventListener('click', () => {
        state.logsSelectedSteps = new Set();
        updateLogsStepList();
        state._logsFocusFirstMatch = true;
        renderLogsWithFilters();
      });
    }
    // No 'only selected' toggle; selecting none means show all
    if (DOM.logsSearch) {
      DOM.logsSearch.addEventListener('input', (e) => {
        state.logsSearchText = e.target.value || '';
        state._logsFocusFirstMatch = true;
        renderLogsWithFilters();
      });
    }
    if (DOM.logsClearSearch) {
      DOM.logsClearSearch.addEventListener('click', () => {
        if (DOM.logsSearch) DOM.logsSearch.value = '';
        state.logsSearchText = '';
        renderLogsWithFilters();
      });
    }
        if (DOM.downloadLogsBtn) {
      DOM.downloadLogsBtn.addEventListener('click', () => {
        try {
          const text = DOM.logsContainer?.innerText || '';
          if (!text) {
            alert('No logs available to download yet.');
            return;
          }
          const blob = new Blob([text], { type: 'text/plain' });
          const url = URL.createObjectURL(blob);
          const a = document.createElement('a');
          a.href = url;
          const runId = state.currentRunData?.run_info?.run_id || 'logs';
          a.download = `logs-${runId.slice(0, 8)}.txt`;
          document.body.appendChild(a);
          a.click();
          document.body.removeChild(a);
          URL.revokeObjectURL(url);
        } catch (error) {
          console.error('Failed to download logs:', error);
          alert('Could not download the logs.');
        }
      });
    }
        if (DOM.graphContainer) {
      DOM.graphContainer.addEventListener('click', (e) => {
        const includedLink = e.target.closest('[data-included-link="true"]');
        if (includedLink) {
          e.stopPropagation();
          return;
        }

        const infoBtn = e.target.closest('[data-step-info="true"]');
        if (infoBtn && infoBtn.dataset.stepName) {
          e.stopPropagation();
          const stepName = infoBtn.dataset.stepName;
          try {
            const [, subpath, runId] = window.location.hash.slice(2).split('/');
            if (subpath && runId) {
              window.location.hash = `#/pipelineruns/${subpath}/${runId}/steps/${stepName}`;
            }
          } catch {}
          showStepDetails(stepName);
          return;
        }

        const innerTaskNode = e.target.closest('g.graph-node');
        if (innerTaskNode && !innerTaskNode.dataset.stepName) return;

        const stepEl = e.target.closest('[data-step-name]');
        if (stepEl && stepEl.dataset.stepName) {
          try {
            const panEl = state._panElement || DOM.graphWrapper;
            const tr = panEl ? window.getComputedStyle(panEl).transform : null;
            let scale = 1, x = 0, y = 0;
            if (tr && tr !== 'none') {
              const m = tr.match(/matrix\(([^)]+)\)/);
              if (m) {
                const v = m[1].split(',').map(parseFloat);
                if (v.length === 6) {
                  const a = v[0], b = v[1];
                  scale = Math.sqrt(a * a + b * b) || 1;
                  x = v[4] || 0;
                  y = v[5] || 0;
                }
              }
            }
            state._stepsViewTransform = { x, y, scale };
          } catch {}
          state._fitOnNextStepsRender = false;
          const stepName = stepEl.dataset.stepName;
          if (e.ctrlKey || e.metaKey) {
            const [, subpath, runId] = window.location.hash.slice(2).split('/');
            if (subpath && runId) {
              window.location.hash = `#/pipelineruns/${subpath}/${runId}/steps/${stepName}`;
            }
          } else {
            if (!state.expandedSteps) state.expandedSteps = new Set();
            state._preserveScale = true;
            if (state.expandedSteps.has(stepName)) {
              state.expandedSteps.delete(stepName);
            } else {
              state.expandedSteps.add(stepName);
              if (!state._justExpandedSteps) state._justExpandedSteps = new Set();
              state._justExpandedSteps.add(stepName);
            }
            if (state.currentRunData) renderStepsGraph(state.currentRunData);
          }
        }
      });
    }
        if (DOM.cancelDeleteBtn) DOM.cancelDeleteBtn.addEventListener('click', closeDeleteGroupModal);
        if (DOM.confirmDeleteBtn) DOM.confirmDeleteBtn.addEventListener('click', deleteGroup);

        if (DOM.mainGridContainer) {
            DOM.mainGridContainer.addEventListener('mousedown', e => {
                const card = e.target.closest('[data-href]');
                if (!card || e.button !== 0) return;

                const onMouseUp = (upEvent) => {
                    document.removeEventListener('mouseup', onMouseUp);

                    const selection = window.getSelection().toString();
                    if (selection && selection.length > 0) {
                        navigator.clipboard.writeText(selection).then(() => {
                            const copiedMessage = document.createElement('div');
                            copiedMessage.textContent = 'Copied!';
                            copiedMessage.style.position = 'fixed';
                            copiedMessage.style.top = `${upEvent.clientY - 30}px`;
                            copiedMessage.style.left = `${upEvent.clientX}px`;
                            copiedMessage.style.background = '#2d3748';
                            copiedMessage.style.color = 'white';
                            copiedMessage.style.padding = '5px 10px';
                            copiedMessage.style.borderRadius = '5px';
                            copiedMessage.style.zIndex = '1000';
                            copiedMessage.style.pointerEvents = 'none';
                            document.body.appendChild(copiedMessage);
                            setTimeout(() => { copiedMessage.remove(); }, 1000);
                        });
                    } else {
                        const url = card.dataset.href;
                        if (upEvent.ctrlKey || upEvent.metaKey) {
                            window.open(url, '_blank');
                        } else {
                            window.location.hash = url;
                        }
                    }
                };

                document.addEventListener('mouseup', onMouseUp, { once: true });
            });

            DOM.mainGridContainer.addEventListener('click', e => {
                const card = e.target.closest('a[data-run-id]');
                if (card && !e.ctrlKey && !e.metaKey && e.button === 0) {
                    e.preventDefault();
                    window.location.hash = card.getAttribute('href');
                }

                const addCard = e.target.closest('#add-group-card');
                if (addCard) {
                    showAddGroupModal(state.selectedGroupId);
                    return;
                }

                const deleteBtn = e.target.closest('.delete-group-btn');
                if (deleteBtn) {
                    e.preventDefault();
                    e.stopPropagation();
                    const groupId = deleteBtn.dataset.groupId;
                    const groupName = deleteBtn.dataset.groupName;
                    showDeleteGroupModal(groupId, groupName);
                    return;
                }

                const branchHeader = e.target.closest('.branch-header');
                if (branchHeader) {
                    const chevron = branchHeader.querySelector('.chevron');
                    if (chevron && chevron.parentElement) {
                        chevron.parentElement.classList.toggle('expanded');
                    }
                    const runsContainer = branchHeader.nextElementSibling;
                    if (runsContainer) {
                        if (runsContainer.style.maxHeight && runsContainer.style.maxHeight !== '0px') {
                            runsContainer.style.maxHeight = '0px';
                        } else {
                            runsContainer.style.maxHeight = `${runsContainer.scrollHeight}px`;
                        }
                    }
                }
            });

            DOM.mainGridContainer.addEventListener('mouseover', handleRunHighlight);
            DOM.mainGridContainer.addEventListener('mouseout', handleRunHighlight);
        }

        if (DOM.modalContent) {
            DOM.modalContent.addEventListener('click', e => {
                const closeBtn = e.target.closest('#close-modal-btn');
                if (closeBtn) {
                    stripStepFromHashWithoutRouting();
                    closeModal();
                    return;
                }

                const backBtn = e.target.closest('#modal-back-btn');
                if (backBtn) {
                    const parentContext = JSON.parse(backBtn.dataset.parentContext);
                    renderModalForStep(parentContext.runId, parentContext.stepName);
                    return;
                }

                const node = e.target.closest('.graph-node[data-context]');
                if (node) {
                    const context = JSON.parse(node.dataset.context);
                    renderModalForStep(context.runId, context.childStepName, { runId: context.parentRunId, stepName: context.parentStepName });
                    return;
                }
            });
        }

        if (DOM.modal) {
            DOM.modal.addEventListener('click', e => {
                if (e.target === DOM.modal) {
                    stripStepFromHashWithoutRouting();
                    closeModal();
                }
            });
        }

        if (DOM.openSidebarBtn && DOM.sidebar) {
            DOM.openSidebarBtn.addEventListener('click', () => DOM.sidebar.classList.remove('-translate-x-full'));
        }
        if (DOM.closeSidebarBtn && DOM.sidebar) {
            DOM.closeSidebarBtn.addEventListener('click', () => DOM.sidebar.classList.add('-translate-x-full'));
        }

        if (DOM.sidebarNav) {
            DOM.sidebarNav.addEventListener('click', async (e) => {
                const link = e.target.closest('a[href]');
                const groupHeader = e.target.closest('.group-header');

                if (link) {
                    e.preventDefault();
                    if (window.location.hash !== link.getAttribute('href')) {
                        window.location.hash = link.getAttribute('href');
                    }
                } else if (groupHeader) {
                    e.preventDefault();
                    const groupLi = groupHeader.closest('li[data-group-id]');
                    const branchLi = groupHeader.closest('li[data-branch-id]');

                    if (branchLi) {
                        const branchId = branchLi.dataset.branchId;
                        if (state.expandedGroups.has(branchId)) {
                            state.expandedGroups.delete(branchId);
                        } else {
                            state.expandedGroups.add(branchId);
                        }
                    } else if (groupLi) {
                        const groupId = parseInt(groupLi.dataset.groupId);
                        if (state.expandedGroups.has(groupId)) {
                            state.expandedGroups.delete(groupId);
                        } else {
                            state.expandedGroups.add(groupId);
                        }
                    }
                    await renderHierarchy(state.groups);
                }
            });

            DOM.sidebarNav.addEventListener('mouseover', handleRunHighlight);
            DOM.sidebarNav.addEventListener('mouseout', handleRunHighlight);
        }

        setupMainGridDragAndDrop();
        setupSidebarDragAndDrop();
        setupHoverHint();
        setupGlobalTitleTooltips();
    }

    function setupMainGridDragAndDrop() {
        if (!DOM.mainGridContainer) return;
        let draggedElement = null;

        DOM.mainGridContainer.addEventListener('dragstart', e => {
            const target = e.target.closest('a[data-group-id]');
            if (!target) return;

            draggedElement = target;
            e.dataTransfer.setData('text/plain', target.dataset.groupId);
            e.dataTransfer.effectAllowed = 'move';
            setTimeout(() => target.classList.add('dragging'), 0);
        });

        DOM.mainGridContainer.addEventListener('dragend', e => {
            if (draggedElement) {
                draggedElement.classList.remove('dragging');
                draggedElement = null;
            }
        });

        DOM.mainGridContainer.addEventListener('dragover', e => {
            e.preventDefault();
            const target = e.target.closest('a[data-group-id]');
            if (target && target !== draggedElement) {
                target.classList.add('drop-target-highlight');
            }
        });

        DOM.mainGridContainer.addEventListener('dragleave', e => {
            const target = e.target.closest('a[data-group-id]');
             if (target) {
                target.classList.remove('drop-target-highlight');
            }
        });

        DOM.mainGridContainer.addEventListener('drop', async e => {
            e.preventDefault();
            e.stopPropagation();

            const dropTarget = e.target.closest('a[data-group-id]');
            if (dropTarget) {
                dropTarget.classList.remove('drop-target-highlight');
            }

            const movedGroupId = e.dataTransfer.getData('text/plain');
            const targetGroupId = dropTarget ? dropTarget.dataset.groupId : state.selectedGroupId;

            if (movedGroupId === targetGroupId) return;

            try {
                await fetchData(`/v1/groups/${movedGroupId}/move`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ parent_id: targetGroupId ? parseInt(targetGroupId, 10) : null }),
                });
                state.repoLastRunCache.clear(); // Clear the cache to force a refresh
                await refresh();
            } catch (error) {
                console.error("Move failed:", error);
            }
        });
    }


    function setupHoverHint() {
        const hint = DOM?.hoverHint;
        if (!hint || !DOM.graphContainer) return;

        const show = (text, variant, x, y) => {
            hint.textContent = text;
            if (variant) hint.setAttribute('data-variant', variant); else hint.removeAttribute('data-variant');
            hint.style.left = (x + 12) + 'px';
            hint.style.top = (y + 12) + 'px';
            hint.classList.add('show');
        };
        const hide = () => hint.classList.remove('show');

        DOM.graphContainer.addEventListener('mousemove', (e) => {
            if (e.buttons === 1) { hide(); return; }
            const isStepsVisible = !DOM.graphContainer.classList.contains('hidden');
            if (!isStepsVisible) { hide(); return; }

            const info = e.target.closest('[data-step-info="true"]');
            if (info) { show('Details', 'accent', e.clientX, e.clientY); return; }
            const collapse = e.target.closest('.step-collapse-badge');
            if (collapse) { show('Collapse', 'accent', e.clientX, e.clientY); return; }
            const cluster = e.target.closest('.step-cluster');
            if (cluster) { show('Collapse', 'accent', e.clientX, e.clientY); return; }
            const node = e.target.closest('g.graph-node[data-step-name]');
            if (node) { show('Expand', null, e.clientX, e.clientY); return; }
            hide();
        });
        DOM.graphContainer.addEventListener('mouseleave', hide);
    }

    function setupGlobalTitleTooltips() {
        const hint = DOM?.hoverHint;
        if (!hint) return;
        let currentEl = null;

        const show = (text, variant, x, y) => {
            hint.textContent = text;
            if (variant) hint.setAttribute('data-variant', variant); else hint.removeAttribute('data-variant');
            hint.style.left = (x + 12) + 'px';
            hint.style.top = (y + 12) + 'px';
            hint.classList.add('show');
        };
        const hide = () => hint.classList.remove('show');

        document.addEventListener('mouseover', (e) => {
            const el = e.target.closest('[title]');
            if (!el) return;
            currentEl = el;
            const text = el.getAttribute('title') || '';
            el.dataset.__title = text;
            el.removeAttribute('title');
            show(text, 'accent', e.clientX, e.clientY);
        });

        document.addEventListener('mousemove', (e) => {
            if (!currentEl) return;
            hint.style.left = (e.clientX + 12) + 'px';
            hint.style.top = (e.clientY + 12) + 'px';
        });

        document.addEventListener('mouseout', (e) => {
            if (!currentEl) return;
            const leavingEl = currentEl;
            if (!e.relatedTarget || !leavingEl.contains(e.relatedTarget)) {
                if (leavingEl.dataset.__title !== undefined) {
                    leavingEl.setAttribute('title', leavingEl.dataset.__title);
                    delete leavingEl.dataset.__title;
                }
                currentEl = null;
                hide();
            }
        });
    }

    function setupSidebarDragAndDrop() {
        if (!DOM.sidebarNav) return;
        let draggedElement = null;

        DOM.sidebarNav.addEventListener('dragstart', e => {
            const isRecentRunsActive = state.currentTab === 'recent';
            if (isRecentRunsActive) {
                e.preventDefault();
                return;
            }
            const target = e.target.closest('li[data-group-id]');
            if (!target) return;

            draggedElement = target;
            e.dataTransfer.setData('text/plain', target.dataset.groupId);
            e.dataTransfer.effectAllowed = 'move';
            setTimeout(() => target.classList.add('dragging'), 0);
        });

        DOM.sidebarNav.addEventListener('dragend', () => {
            if (draggedElement) {
                draggedElement.classList.remove('dragging');
                draggedElement = null;
            }
        });

        DOM.sidebarNav.addEventListener('dragover', e => {
            e.preventDefault();
            const target = e.target.closest('li[data-group-id] .group-header-container');
            if (target) {
                target.classList.add('drop-target-highlight');
            }
        });

        DOM.sidebarNav.addEventListener('dragleave', e => {
            const target = e.target.closest('li[data-group-id] .group-header-container');
            if (target) {
                target.classList.remove('drop-target-highlight');
            }
        });

        DOM.sidebarNav.addEventListener('drop', async e => {
            e.preventDefault();
            e.stopPropagation();

            const dropTarget = e.target.closest('li[data-group-id] .group-header-container');
            if (dropTarget) {
                dropTarget.classList.remove('drop-target-highlight');
            }

            const movedGroupId = e.dataTransfer.getData('text/plain');
            const targetGroupId = dropTarget ? dropTarget.closest('li[data-group-id]').dataset.groupId : null;

            if (movedGroupId === targetGroupId) return;

            try {
                await fetchData(`/v1/groups/${movedGroupId}/move`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ parent_id: targetGroupId ? parseInt(targetGroupId, 10) : null }),
                });
                state.repoLastRunCache.clear();
                await refresh();
            } catch (error) {
                console.error('Move failed:', error);
            }
        });
    }


    global.pages = global.pages || {};
    global.pages.pipelineruns = {
        init,
        handleRoute,
    };
})(window.NopsAI = window.NopsAI || {});
