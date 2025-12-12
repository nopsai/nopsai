window.NopsAI = window.NopsAI || {};
document.addEventListener('DOMContentLoaded', () => {
    const API_BASE_URL = 'http://localhost:8080';

const normalizeRunId = (runId) => typeof runId === 'string' ? runId.trim().toLowerCase() : '';

    const showErrorToast = (message) => {
        const container = document.getElementById('toast-container');
        if (!container) return;

        const toast = document.createElement('div');
        toast.className = 'pointer-events-auto w-full max-w-sm overflow-hidden rounded-lg bg-[var(--bg-secondary)] shadow-lg ring-1 ring-black ring-opacity-5 border-l-4 border-red-500 transition-transform transform translate-x-full duration-300 ease-in-out';

        toast.innerHTML = `
            <div class="p-4">
                <div class="flex items-start">
                    <div class="flex-shrink-0">
                        <svg class="h-6 w-6 text-red-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                        </svg>
                    </div>
                    <div class="ml-3 w-0 flex-1 pt-0.5">
                        <p class="text-sm font-medium text-[var(--text-primary)]">Error</p>
                        <p class="mt-1 text-sm text-[var(--text-secondary)] break-words">
                            ${String(message).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}
                        </p>
                    </div>
                    <div class="ml-4 flex flex-shrink-0">
                        <button class="inline-flex rounded-md bg-[var(--bg-secondary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-2">
                            <span class="sr-only">Close</span>
                            <svg class="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor"><path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z"></path></svg>
                        </button>
                    </div>
                </div>
            </div>
        `;

        const closeButton = toast.querySelector('button');
        const closeToast = () => {
            toast.classList.add('translate-x-full', 'opacity-0');
            setTimeout(() => toast.remove(), 300);
        };
        closeButton.addEventListener('click', closeToast);

        container.appendChild(toast);

        // Animate in
        setTimeout(() => toast.classList.remove('translate-x-full'), 10);
        // Auto-dismiss after 8 seconds
        setTimeout(closeToast, 8000);
    };

    const showToast = (runData) => {
        const container = document.getElementById('toast-container');
        if (!container) return;

        const toast = document.createElement('div');
        toast.className = 'pointer-events-auto w-full max-w-sm overflow-hidden rounded-lg bg-[var(--bg-secondary)] shadow-lg ring-1 ring-black ring-opacity-5 transition-transform transform translate-x-full duration-300 ease-in-out';

        const repoName = runData.git_repo_owner && runData.git_repo_name
            ? `${runData.git_repo_owner}/${runData.git_repo_name}`
            : (runData.git_repo_name || 'N/A');

        toast.innerHTML = `
            <div class="p-4">
                <div class="flex items-start">
                    <div class="flex-shrink-0">
                        <svg class="h-6 w-6 text-blue-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"></path></svg>
                    </div>
                    <div class="ml-3 w-0 flex-1 pt-0.5">
                        <p class="text-sm font-medium text-[var(--text-primary)]">New Run Started</p>
                        <p class="mt-1 text-sm text-[var(--text-secondary)] truncate">
                            ${runData.pipeline_name} on ${repoName}
                        </p>
                    </div>
                    <div class="ml-4 flex flex-shrink-0">
                        <button class="inline-flex rounded-md bg-[var(--bg-secondary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent)] focus:ring-offset-2">
                            <span class="sr-only">Close</span>
                            <svg class="h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor"><path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z"></path></svg>
                        </button>
                    </div>
                </div>
            </div>
        `;

        const closeButton = toast.querySelector('button');
        const closeToast = () => {
            toast.classList.add('translate-x-full', 'opacity-0');
            setTimeout(() => toast.remove(), 300);
        };
        closeButton.addEventListener('click', closeToast);

        container.appendChild(toast);

        // Animate in
        setTimeout(() => toast.classList.remove('translate-x-full'), 10);
        // Auto-dismiss after 5 seconds
        setTimeout(closeToast, 5000);
    };

    const DOM = {
        sidebarBaseNav: document.getElementById('sidebar-base-nav'),
        sidebarDetailsNav: document.getElementById('sidebar-details-nav'),
        mainHeader: document.getElementById('main-header'),
        pages: document.querySelectorAll('[data-page]'),
        pageContentWrapper: document.getElementById('page-content-wrapper'),
        hoverHint: document.getElementById('hover-hint'),
        placeholder: document.getElementById('placeholder'),
        graphContainer: document.getElementById('graph-container'),
        graphWrapper: document.getElementById('graph-wrapper'),
        tasksGraphContainer: document.getElementById('tasks-graph-container'),
        tasksGraphWrapper: document.getElementById('tasks-graph-wrapper'),
        tasksGraphConnections: document.getElementById('tasks-graph-connections'),
        tasksEmpty: document.getElementById('tasks-empty'),
        mainGridContainer: document.getElementById('main-grid-container'),
        openSidebarBtn: document.getElementById('open-sidebar-btn'),
        closeSidebarBtn: document.getElementById('close-sidebar-btn'),
        sidebar: document.getElementById('sidebar'),
        modal: document.getElementById('step-modal'),
        modalContent: document.getElementById('modal-content'),
        closeModalBtn: document.getElementById('close-modal-btn'),
        tabs: document.querySelectorAll('[data-tab]'),
        addGroupModal: document.getElementById('add-group-modal'),
        closeAddGroupModalBtn: document.getElementById('close-add-group-modal-btn'),
        cancelAddGroupBtn: document.getElementById('cancel-add-group-btn'),
        addGroupForm: document.getElementById('add-group-form'),
        deleteGroupModal: document.getElementById('delete-group-modal'),
        confirmDeleteBtn: document.getElementById('confirm-delete-btn'),
        cancelDeleteBtn: document.getElementById('cancel-delete-btn'),
        deleteItemName: document.getElementById('delete-item-name'),
        logsModal: document.getElementById('logs-modal'),
        logsModalContent: document.getElementById('logs-modal-content'),
        closeLogsModalBtn: document.getElementById('close-logs-modal-btn'),
        logsContainer: document.getElementById('logs-container'),
        followLogsCheckbox: document.getElementById('follow-logs-checkbox'),
        copyLogsBtn: document.getElementById('copy-logs-btn'),
        logsStepsSidebar: document.getElementById('logs-steps-sidebar'),
        logsStepList: document.getElementById('logs-step-list'),
        logsStepSearch: document.getElementById('logs-step-search'),
        logsStepsSelectAll: document.getElementById('logs-steps-select-all'),
        logsStepsClear: document.getElementById('logs-steps-clear'),
        logsSearch: document.getElementById('logs-search'),
        logsClearSearch: document.getElementById('logs-clear-search'),
        logSearchNav: document.getElementById('log-search-nav'),
        logsSearchPrev: document.getElementById('logs-search-prev'),
        logsSearchNext: document.getElementById('logs-search-next'),
        logsSearchMatches: document.getElementById('logs-search-matches'),
        logsCount: document.getElementById('logs-count'),
        downloadLogsBtn: document.getElementById('download-logs-btn'),
        logsToggleAgent: document.getElementById('logs-toggle-agent'),
        logsToggleShort: document.getElementById('logs-toggle-short'),
        runSelectionBar: document.getElementById('run-selection-bar'),
        runSelectionCount: document.getElementById('run-selection-count'),
        runSelectionDeleteBtn: document.getElementById('run-selection-delete-btn'),
        runSelectionClearBtn: document.getElementById('run-selection-clear-btn'),
        pipelineRunsSearchContainer: document.getElementById('pipelineruns-search-container'),
        pipelineRunsActions: document.getElementById('pipelineruns-actions'),
        pipelineRunsNewFolderBtn: document.getElementById('pipelineruns-new-folder-btn'),
        runViewToggleContainer: document.getElementById('run-view-toggle-container'),
    };
    let savedRunViewMode = 'grid';
    try {
        const storedView = localStorage.getItem('pipelinerunsViewMode');
        if (storedView === 'list') {
            savedRunViewMode = 'list';
        }
    } catch (error) {
        savedRunViewMode = 'grid';
    }

    let state = {
        pollingInterval: null, // This will be removed
        currentRunData: null,
        groups: [],
        selectedGroupId: null,
        selectedGroupPathSegments: [],
        groupToDelete: null,
        panzoomInstance: null,
        expandedGroups: new Set(),
        expandedSteps: new Set(),
        expandedStepPositions: new Map(), // stepName -> { x, y }
        stepLayoutScale: 1.0, // scales step size + gaps when needed
        taskClusterScale: 1.0, // scales inside expanded boxes (tasks + gaps)
        repoLastRunCache: new Map(),
        logPollingInterval: null, // This will be removed
        currentGraphView: localStorage.getItem('graphView') || 'steps',
        currentPath: 'pipelineruns',
        currentHash: window.location.hash || '#/pipelineruns/main',
        pollingInterval: null,
        currentRunTrackedIds: new Map(),
        _logsRaw: [],
        _logsSearchMatches: [],
        _logsSearchMatchIndex: -1,
        logsSelectedSteps: new Set(),
        logsSearchText: '',
        _logsFocusFirstMatch: false,
        logsAllSteps: [],
        logsStepStatuses: new Map(),
        logsWrap: true,
        logsShowTimestamps: true,
        logsStructured: true,
        logsLevelFilter: new Set(['info', 'warn', 'error', 'debug']),
        logsShortView: false,
        logsAgentOnly: false,
        currentRunContext: null,
        _suppressNextRoute: false,
        _suppressRouteTimeout: null,
        selectedRunIds: new Set(),
        runSearchTerm: '',
        recentRuns: [],
        pipelineRunsPollingTimer: null,
        currentRepoRunsByBranch: null,
        currentRepoGroupId: null,
        searchRuns: [],
        searchRunsFetchedAt: 0,
        runViewMode: savedRunViewMode,
    };

    async function fetchData(url, options = {}) {
        try {
            const response = await fetch(API_BASE_URL + url, options);
            fetchData.lastStatus = response.status;
            fetchData.lastETag = response.headers.get('etag');
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(errorText || `HTTP error! Status: ${response.status}`);
            }
            if (response.status === 204 || response.status === 304) return null;

            const contentType = (response.headers.get('content-type') || '').toLowerCase();
            if (contentType.includes('application/json')) {
                return await response.json();
            }
            return await response.text();
        } catch (error) {
            console.error(`Fetch error for ${url}:`, error);
            fetchData.lastError = error;
            fetchData.lastStatus = null;
            showErrorToast(error.message || 'Network error occurred');
            return null;
        }
    }

    fetchData.lastStatus = null;
    fetchData.lastError = null;

    async function postData(url, data) {
        return await fetchData(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(data),
        });
    }

    async function deleteData(url) {
        return await fetchData(url, { method: 'DELETE' });
    }

    const logsModule = (window.NopsAI && window.NopsAI.logs) ? window.NopsAI.logs : null;

    const context = { state, DOM, fetchData, postData, deleteData, refresh: router, logsModule, apiBaseUrl: API_BASE_URL, showErrorToast };

    const pageModules = (window.NopsAI && window.NopsAI.pages) ? window.NopsAI.pages : {};

    const pipelineRunsModule = pageModules.pipelineruns || null;
    const pipelinesModule = pageModules.pipelines || null;
    const stepsModule = pageModules.steps || null;
    const triggersModule = pageModules.triggers || null;
    const scopesModule = pageModules.scopes || null;
    const labModule = pageModules.lab || null;
    const systemModule = pageModules.system || null;

    if (logsModule && typeof logsModule.init === 'function') {
        logsModule.init({ state, DOM, fetchData });
    }

    if (pipelineRunsModule && typeof pipelineRunsModule.init === 'function') {
        pipelineRunsModule.init(context);
    }

    if (pipelinesModule && typeof pipelinesModule.init === 'function') {
        pipelinesModule.init(context);
    }

    if (stepsModule && typeof stepsModule.init === 'function') {
        stepsModule.init(context);
    }

    if (triggersModule && typeof triggersModule.init === 'function') {
        triggersModule.init(context);
    }

    if (scopesModule && typeof scopesModule.init === 'function') {
        scopesModule.init(context);
    }

    if (labModule && typeof labModule.init === 'function') {
        labModule.init(context);
    }

    if (systemModule && typeof systemModule.init === 'function') {
        systemModule.init(context);
    }

    async function router(hashOverride) {
        // --- Helper function (ensure this is available or copy from pipeline-runs.js) ---
        const RUN_ID_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
        function parsePipelineRunsHash(hash) {
            const raw = (hash || '').replace(/^#/, '');
            const questionIndex = raw.indexOf('?');
            const pathPart = questionIndex === -1 ? raw : raw.slice(0, questionIndex);
            const queryPart = questionIndex === -1 ? '' : raw.slice(questionIndex + 1);
            const normalizedPath = (pathPart || '').replace(/^\/?/, '');
            const parts = normalizedPath ? normalizedPath.split('/').filter(Boolean) : [];
            const searchParams = new URLSearchParams(queryPart || '');
            const query = {};
            searchParams.forEach((value, key) => {
                query[key] = value;
            });
            const path = parts[0] || 'pipelineruns';
            const tab = parts[1] || 'main';
            const rest = parts.slice(2);

            let runId = null;
            let action = null;
            let stepName = null;
            let actionSegments = [];
            let logSegments = [];
            let groupSegments = rest;

            const runIndex = rest.findIndex(seg => RUN_ID_REGEX.test(seg));
            if (runIndex !== -1) {
                runId = rest[runIndex];
                groupSegments = rest.slice(0, runIndex);
                actionSegments = rest.slice(runIndex + 1);
                action = actionSegments[0] || null;
                if (action === 'steps' && actionSegments[1]) {
                    try {
                        stepName = decodeURIComponent(actionSegments[1]);
                    } catch {
                        stepName = actionSegments[1];
                    }
                }
                if (action === 'logs') {
                    logSegments = actionSegments.slice(1);
                }
            }

            return { path, tab, groupSegments, runId, action, stepName, actionSegments, logSegments, query };
        }
        // --- End Helper ---

        if (state._suppressNextRoute) {
            state._suppressNextRoute = false;
            if (state._suppressRouteTimeout) {
                clearTimeout(state._suppressRouteTimeout);
                state._suppressRouteTimeout = null;
            }
            return;
        }
        if (window.location.search) {
            try {
                const clean = window.location.pathname + (window.location.hash || '');
                history.replaceState(null, '', clean);
            } catch {
                window.location.search = '';
            }
        }

        const hash = hashOverride || window.location.hash || '#/pipelineruns/main';
        // Use the helper to get info about the TARGET route
        const info = parsePipelineRunsHash(hash);
        const { path, runId } = info; // 'path' is the new page, 'runId' exists if it's a run detail page

        const wasOnLab = state.currentPath === 'lab';
        if (wasOnLab && path !== 'lab' && labModule && typeof labModule.preventNavigation === 'function') {
            const blocked = labModule.preventNavigation(hash, state.currentHash || '#/lab');
            if (blocked) {
                return;
            }
        }

        if (wasOnLab && path !== 'lab' && labModule && typeof labModule.onLeave === 'function') {
            try {
                labModule.onLeave();
            } catch (error) {
                console.error('Failed to clean up lab page state:', error);
            }
        }

        if (path !== 'system' && systemModule && typeof systemModule.onLeave === 'function') {
            try {
                systemModule.onLeave();
            } catch (error) {
                console.error('Failed to clean up system page state:', error);
            }
        }

        // --- ADDED/MODIFIED SCROLL MANAGEMENT BLOCK ---
        // Check the state *before* updating state.currentPath
        const wasOnRunDetail = state.currentPath === 'pipelineruns' && state.currentRunData?.run_info?.run_id;
        const isNowOnRunDetail = path === 'pipelineruns' && runId;
        const isNowOnPipelinesPage = path === 'pipelines'; // Check if navigating TO pipelines

        // Manage the scroll lock on the main wrapper
        if (DOM.pageContentWrapper) {
            if (isNowOnRunDetail) {
                // Navigating TO a run detail page that uses pan/zoom: ADD no-scroll
                DOM.pageContentWrapper.classList.add('no-scroll');
            } else if (wasOnRunDetail || isNowOnPipelinesPage) {
                // Navigating AWAY from a run detail page OR TO the pipelines page: REMOVE no-scroll
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
            // For other pages, ensure no-scroll is removed (if they need scrolling)
            if (!isNowOnRunDetail) {
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
        }
        // --- END SCROLL MANAGEMENT BLOCK ---

        // Update the current path *after* checking the previous state
        state.currentPath = path;
        state.currentHash = hash;

        // --- Standard router logic continues ---
        if (DOM.mainHeader) {
            DOM.mainHeader.innerHTML = ''; // Clear potentially complex HTML
            DOM.mainHeader.textContent = path.charAt(0).toUpperCase() + path.slice(1);
        }

        if (DOM.pageContentWrapper) {
            // Scroll to top unless we are staying on the same run detail page (e.g., opening/closing modal)
            if (!wasOnRunDetail || !isNowOnRunDetail || state.currentRunData?.run_info?.run_id !== runId) {
                DOM.pageContentWrapper.scrollTop = 0;
            }
        }
        if (DOM.pages && DOM.pages.length) {
            DOM.pages.forEach(page => {
                page.classList.toggle('active', page.dataset.page === path);
            });
        }

        // --- Call page-specific handlers ---
        if (path === 'pipelineruns' && pipelineRunsModule && typeof pipelineRunsModule.handleRoute === 'function') {
            await pipelineRunsModule.handleRoute(hash);
            // Re-apply no-scroll specifically if needed after module handling
            if (runId && DOM.pageContentWrapper) {
                DOM.pageContentWrapper.classList.add('no-scroll');
            }
            return;
        }

        if (path === 'pipelines' && pipelinesModule && typeof pipelinesModule.handleRoute === 'function') {
            // Ensure scroll IS possible on pipelines page (redundant check, but safe)
            if (DOM.pageContentWrapper) {
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
            await pipelinesModule.handleRoute(hash);
            return;
        }

        if (path === 'steps' && stepsModule && typeof stepsModule.handleRoute === 'function') {
            if (DOM.pageContentWrapper) {
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
            if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
                try {
                    await pipelineRunsModule.renderSidebarForRoute('steps');
                } catch (error) {
                    console.error('Failed to render steps sidebar navigation:', error);
                }
            }
            await stepsModule.handleRoute(hash);
            return;
        }

        if (path === 'triggers' && triggersModule && typeof triggersModule.handleRoute === 'function') {
            if (DOM.pageContentWrapper) {
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
            if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
                try {
                    await pipelineRunsModule.renderSidebarForRoute('triggers');
                } catch (error) {
                    console.error('Failed to render triggers sidebar navigation:', error);
                }
            }
            await triggersModule.handleRoute(hash);
            return;
        }

        if (path === 'scopes' && scopesModule && typeof scopesModule.handleRoute === 'function') {
            if (DOM.pageContentWrapper) {
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
            if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
                try {
                    await pipelineRunsModule.renderSidebarForRoute('scopes');
                } catch (error) {
                    console.error('Failed to render scopes sidebar navigation:', error);
                }
            }
            await scopesModule.handleRoute(hash);
            return;
        }

        if (path === 'lab' && labModule && typeof labModule.handleRoute === 'function') {
            if (DOM.pageContentWrapper) {
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
            if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
                try {
                    await pipelineRunsModule.renderSidebarForRoute('lab');
                } catch (error) {
                    console.error('Failed to render lab sidebar navigation:', error);
                }
            }
            await labModule.handleRoute(hash);
            return;
        }

        if (path === 'system' && systemModule && typeof systemModule.handleRoute === 'function') {
            if (DOM.pageContentWrapper) {
                DOM.pageContentWrapper.classList.remove('no-scroll');
            }
            if (pipelineRunsModule && typeof pipelineRunsModule.renderSidebarForRoute === 'function') {
                try {
                    await pipelineRunsModule.renderSidebarForRoute('system');
                } catch (error) {
                    console.error('Failed to render system sidebar navigation:', error);
                }
            }
            await systemModule.handleRoute(hash);
            return;
        }

        // --- Fallback for other/placeholder pages ---
        if (DOM.placeholder) {
            DOM.placeholder.classList.remove('hidden');
            const heading = DOM.placeholder.querySelector('h3');
            const body = DOM.placeholder.querySelector('p');
            if (heading) heading.textContent = `Welcome to ${path}`;
            if (body) body.textContent = 'This page is under construction.';
        }
        if (DOM.mainHeader && !DOM.mainHeader.textContent) { // Update header if not already set by specific logic
            DOM.mainHeader.textContent = path.charAt(0).toUpperCase() + path.slice(1);
        }
        // Ensure scrolling is possible for placeholder/other pages
        if (DOM.pageContentWrapper) {
            DOM.pageContentWrapper.classList.remove('no-scroll');
        }
    }

    (() => {
        const themeToggleDarkIcon = document.getElementById('theme-toggle-dark-icon');
        const themeToggleLightIcon = document.getElementById('theme-toggle-light-icon');
        const themeToggleButton = document.getElementById('theme-toggle');

        if (!themeToggleButton || !themeToggleDarkIcon || !themeToggleLightIcon) return;

        const applyTheme = () => {
            if (localStorage.getItem('theme') === 'dark' || (!('theme' in localStorage) && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
                document.documentElement.classList.add('dark');
                themeToggleLightIcon.classList.remove('hidden');
                themeToggleDarkIcon.classList.add('hidden');
            } else {
                document.documentElement.classList.remove('dark');
                themeToggleDarkIcon.classList.remove('hidden');
                themeToggleLightIcon.classList.add('hidden');
            }
        };

        applyTheme();

        themeToggleButton.addEventListener('click', () => {
            const isDark = document.documentElement.classList.contains('dark');
            if (isDark) {
                localStorage.setItem('theme', 'light');
            } else {
                localStorage.setItem('theme', 'dark');
            }
            applyTheme();
        });
    })();

    (() => {
        const resizer = document.getElementById('modal-resizer');
        const container = document.getElementById('modal-grid-container');

        if (!resizer || !container) return;

        let isResizing = false;

        resizer.addEventListener('mousedown', (e) => {
            isResizing = true;
            document.body.style.cursor = 'col-resize';
            document.body.style.userSelect = 'none';

            const mouseMoveHandler = (moveEvent) => {
                if (!isResizing) return;

                const containerRect = container.getBoundingClientRect();
                let mouseXRelative = moveEvent.clientX - containerRect.left;

                const minPercent = 20;
                const maxPercent = 80;
                const minWidth = (containerRect.width * minPercent) / 100;
                const maxWidth = (containerRect.width * maxPercent) / 100;

                if (mouseXRelative < minWidth) mouseXRelative = minWidth;
                if (mouseXRelative > maxWidth) mouseXRelative = maxWidth;

                const newLeftWidth = mouseXRelative;

                container.style.gridTemplateColumns = `${newLeftWidth}px 10px 1fr`;
            };

            const mouseUpHandler = () => {
                isResizing = false;
                document.body.style.cursor = '';
                document.body.style.userSelect = '';
                document.removeEventListener('mousemove', mouseMoveHandler);
                document.removeEventListener('mouseup', mouseUpHandler);
            };

            document.addEventListener('mousemove', mouseMoveHandler);
            document.addEventListener('mouseup', mouseUpHandler, { once: true });
        });
    })();

    (() => {
        const sidebar = document.getElementById('sidebar');
        const resizer = document.getElementById('sidebar-resizer');
        if (!sidebar || !resizer) return;

        const MIN_WIDTH = 220;
        const MAX_WIDTH = 600;
        const DEFAULT_WIDTH = 288;

        const savedWidth = localStorage.getItem('sidebarWidth');
        sidebar.style.width = savedWidth ? savedWidth : `${DEFAULT_WIDTH}px`;

        const resize = (e) => {
            window.requestAnimationFrame(() => {
                let newWidth = e.clientX;
                if (newWidth < MIN_WIDTH) newWidth = MIN_WIDTH;
                if (newWidth > MAX_WIDTH) newWidth = MAX_WIDTH;
                sidebar.style.width = `${newWidth}px`;
            });
        };

        resizer.addEventListener('mousedown', (e) => {
            e.preventDefault();
            sidebar.classList.add('sidebar-no-transition');
            document.body.style.cursor = 'col-resize';
            document.body.style.userSelect = 'none';

            const mouseMoveHandler = (moveEvent) => {
                resize(moveEvent);
            };

            const mouseUpHandler = () => {
                sidebar.classList.remove('sidebar-no-transition');
                localStorage.setItem('sidebarWidth', sidebar.style.width);
                document.body.style.cursor = '';
                document.body.style.userSelect = '';
                document.removeEventListener('mousemove', mouseMoveHandler);
            };

            document.addEventListener('mousemove', mouseMoveHandler);
            document.addEventListener('mouseup', mouseUpHandler, { once: true });
        });
    })();


    router();
    window.addEventListener('hashchange', () => router());
});
