window.NopsAI = window.NopsAI || {};
document.addEventListener('DOMContentLoaded', () => {
    const API_BASE_URL = 'http://localhost:8080';
    const DOM = {
        sidebarNav: document.getElementById('sidebar-nav'),
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
    };

    let state = {
        pollingInterval: null,
        currentRunData: null,
        groups: [],
        selectedGroupId: null,
        groupToDelete: null,
        panzoomInstance: null,
        expandedGroups: new Set(),
        expandedSteps: new Set(),
        expandedStepPositions: new Map(), // stepName -> { x, y }
        stepLayoutScale: 1.0, // scales step size + gaps when needed
        taskClusterScale: 1.0, // scales inside expanded boxes (tasks + gaps)
        repoLastRunCache: new Map(),
        logPollingInterval: null,
        currentGraphView: localStorage.getItem('graphView') || 'steps',
        _logsRaw: [],
        _logsSearchMatches: [],
        _logsSearchMatchIndex: -1,
        logsSelectedSteps: new Set(),
        logsSearchText: '',
        _logsFocusFirstMatch: false,
        logsAllSteps: [],
        logsWrap: true,
        logsShowTimestamps: true,
        logsStructured: true,
        logsLevelFilter: new Set(['info','warn','error','debug']),
    };

    async function fetchData(url, options = {}) {
        try {
            const response = await fetch(API_BASE_URL + url, options);
            if (!response.ok) {
                const errorText = await response.text();
                throw new Error(errorText || `HTTP error! Status: ${response.status}`);
            }
            if (response.status === 204) return null;
            return await response.json();
        } catch (error) {
            console.error(`Fetch error for ${url}:`, error);
            alert(`Error: ${error.message}`);
            return null;
        }
    }

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

    const pageModules = (window.NopsAI && window.NopsAI.pages) ? window.NopsAI.pages : {};
    const pipelineRunsModule = pageModules.pipelineruns || null;

    if (logsModule && typeof logsModule.init === 'function') {
        logsModule.init({ state, DOM, fetchData });
    }

    if (pipelineRunsModule && typeof pipelineRunsModule.init === 'function') {
        pipelineRunsModule.init({ state, DOM, fetchData, postData, deleteData, logsModule, refresh: router });
    }

    async function router(hashOverride) {
        const hash = hashOverride || window.location.hash || '#/pipelineruns/main';
        const parts = hash.replace(/^#/,'').replace(/^\//,'').split('/');
        const path = parts[0] || 'pipelineruns';

        if (DOM.pages && DOM.pages.length) {
            DOM.pages.forEach(page => {
                page.classList.toggle('active', page.dataset.page === path);
            });
        }

        if (path === 'pipelineruns' && pipelineRunsModule && typeof pipelineRunsModule.handleRoute === 'function') {
            await pipelineRunsModule.handleRoute(hash);
            return;
        }

        if (DOM.placeholder) {
            DOM.placeholder.classList.remove('hidden');
            const heading = DOM.placeholder.querySelector('h3');
            const body = DOM.placeholder.querySelector('p');
            if (heading) heading.textContent = `Welcome to ${path}`;
            if (body) body.textContent = 'This page is under construction.';
        }
        if (DOM.mainHeader) {
            DOM.mainHeader.textContent = path.charAt(0).toUpperCase() + path.slice(1);
        }
    }

// --- THEME TOGGLE LOGIC ---
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

// --- MODAL RESIZER LOGIC ---
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


    window.addEventListener('hashchange', () => router());
    router();
});
