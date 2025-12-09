(function (global) {
    let state;
    let DOM;
    let fetchData;
    let postData;
    let deleteData;
    let logsModule;
    let refresh;
    let initialized = false;
    let showLogsModal = () => { };
    let closeLogsModal = () => { };
    let renderLogsWithFilters = () => { };
    let updateLogsStepList = () => { };
    let lastMainGridRender = null;
    const runViewToggle = { container: null, gridBtn: null, listBtn: null };

    const statusConfig = {
        success: { icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-green-500 dark:text-green-400', rectClass: 'stroke-green-500 fill-green-100 dark:fill-green-500/10' },
        failure: { icon: 'M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-red-500 dark:text-red-400', rectClass: 'stroke-red-500 fill-red-100 dark:fill-red-500/10' },
        running: { icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-blue-500 dark:text-blue-400', rectClass: 'stroke-blue-500 fill-blue-100 dark:fill-blue-500/10 animate-pulse' },
        pending: { icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-gray-500 dark:text-gray-400', rectClass: 'stroke-gray-500 fill-gray-100 dark:fill-gray-500/10' },
        skipped: { icon: 'M15 12H9m12 0a9 9 0 11-18 0 9 9 0 0118 0z', color: 'text-amber-500 dark:text-amber-400', rectClass: 'stroke-amber-500 fill-amber-100 dark:fill-amber-500/10' },
        cancelled: { icon: 'M6 18L18 6M6 6l12 12', color: 'text-orange-500 dark:text-orange-400', rectClass: 'stroke-orange-500 fill-orange-100 dark:fill-orange-500/10' },
        'failure (ignored)': { icon: 'M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z', color: 'text-amber-500 dark:text-amber-400', rectClass: 'stroke-amber-500 fill-amber-100 dark:fill-amber-500/10' },
    };

    const BRANCH_STATUS_PRIORITIES = ['failure', 'failure (ignored)', 'cancelled', 'running', 'pending', 'skipped', 'success'];

    function normalizeRunStatus(run) {
        if (!run) return 'pending';
        const rawStatus = run.is_complete ? run.status : 'running';
        if (typeof rawStatus !== 'string' || !rawStatus) {
            return 'pending';
        }
        const normalized = rawStatus.toLowerCase();
        return statusConfig[normalized] ? normalized : 'pending';
    }

    function getStatusPriority(statusKey) {
        const normalized = typeof statusKey === 'string' ? statusKey.toLowerCase() : 'pending';
        const index = BRANCH_STATUS_PRIORITIES.indexOf(normalized);
        return index === -1 ? BRANCH_STATUS_PRIORITIES.length : index;
    }

    function summarizeLatestTriggerStatus(runs) {
        if (!Array.isArray(runs) || runs.length === 0) {
            return null;
        }

        const latestRun = runs.reduce((current, candidate) => {
            if (!candidate) return current;
            if (!current) return candidate;
            const currentTime = current.started_at ? new Date(current.started_at).getTime() : 0;
            const candidateTime = candidate.started_at ? new Date(candidate.started_at).getTime() : 0;
            return candidateTime > currentTime ? candidate : current;
        }, null);

        if (!latestRun) {
            return null;
        }

        const latestTriggerId = latestRun.trigger_event_id || latestRun.triggerEventId || null;
        if (!latestTriggerId) {
            return {
                triggerId: null,
                status: normalizeRunStatus(latestRun),
                referenceRun: latestRun,
                runs: [latestRun],
            };
        }

        const relatedRuns = runs.filter(run => {
            const triggerId = run ? (run.trigger_event_id || run.triggerEventId || null) : null;
            return triggerId === latestTriggerId;
        });

        const aggregatedStatus = relatedRuns.reduce((current, run) => {
            const statusKey = normalizeRunStatus(run);
            if (!current) return statusKey;
            return getStatusPriority(statusKey) < getStatusPriority(current) ? statusKey : current;
        }, null) || normalizeRunStatus(latestRun);

        const referenceRun = relatedRuns.reduce((current, candidate) => {
            if (!candidate) return current;
            if (!current) return candidate;
            const currentTime = current.started_at ? new Date(current.started_at).getTime() : 0;
            const candidateTime = candidate.started_at ? new Date(candidate.started_at).getTime() : 0;
            return candidateTime > currentTime ? candidate : current;
        }, null) || latestRun;

        return {
            triggerId: latestTriggerId,
            status: aggregatedStatus,
            referenceRun,
            runs: relatedRuns,
        };
    }

    const RUN_ID_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    const groupPathCache = new Map();
    const LOG_LEVEL_FILTER_KEYS = ['info', 'warn', 'error', 'debug'];

    const TERMINAL_RUN_STATUSES = new Set(['success', 'failure', 'cancelled', 'failure (ignored)', 'skipped']);

    let searchRunsPromise = null;

    function escapeText(value) {
        if (value === null || value === undefined) return '';
        return String(value)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
    }

    function normalizeBranchRef(ref) {
        if (typeof ref !== 'string' || !ref) return '';
        return ref.startsWith('refs/heads/') ? ref.slice('refs/heads/'.length) : ref;
    }

    function formatBranchDisplay(sourceRef, targetRef, options = {}) {
        const opts = typeof options === 'object' && options !== null ? options : {};
        const arrow = opts.html ? '&gt;' : '>';
        const source = normalizeBranchRef(sourceRef);
        const target = normalizeBranchRef(targetRef);
        if (target) {
            return `${source || 'N/A'} ${arrow} ${target}`;
        }
        return source || 'N/A';
    }

    function runMatchesSearch(run, searchTermLower) {
        if (!searchTermLower) return true;
        if (!run || typeof run !== 'object') return false;
        const candidates = [
            run.pipeline_name,
            run.pipeline_path,
            run.pipeline_version,
            run.git_repo_owner,
            run.git_repo_name,
            run.git_ref,
            run.git_target_ref,
            run.git_commit_sha,
            run.git_commit_message,
            run.git_pusher_name,
            run.status,
            run.variables,
            run.trigger_event_id,
        ];
        return candidates.some(field => typeof field === 'string' && field.toLowerCase().includes(searchTermLower));
    }

    function getPipelineIdentifierFromRun(run) {
        if (!run || typeof run !== 'object') return '';
        const rawName = typeof run.pipeline_name === 'string' ? run.pipeline_name.trim() : '';
        const rawPath = typeof run.pipeline_path === 'string' ? run.pipeline_path.trim() : '';
        if (!rawName) return '';
        const cleanPath = rawPath.replace(/^\/+|\/+$/g, '');
        return cleanPath ? `${cleanPath}/${rawName}` : rawName;
    }

    function buildPipelineHashFromIdentifier(identifier) {
        const base = '#/pipelines';
        if (!identifier) return base;
        const segments = identifier
            .split('/')
            .map(segment => segment.trim())
            .filter(Boolean);
        if (!segments.length) return base;
        const encoded = segments.map(part => encodeURIComponent(part));
        return `${base}/${encoded.join('/')}`;
    }

    function buildPipelineHashFromRun(run) {
        return buildPipelineHashFromIdentifier(getPipelineIdentifierFromRun(run));
    }

    function buildSidebarLogo(svgMarkup, variant = 'list', logoClass = '') {
        const renderer = (typeof window !== 'undefined' && window.NopsAI && window.NopsAI.ui)
            ? window.NopsAI.ui.renderStepLogo
            : null;
        const extraClasses = ['sidebar-logo', logoClass].filter(Boolean).join(' ').trim();
        if (typeof renderer === 'function') {
            return renderer(variant, extraClasses, svgMarkup);
        }
        const safeVariant = variant || 'list';
        const extra = extraClasses ? ` ${extraClasses}` : '';
        return `<span class="step-logo step-logo--${safeVariant}${extra}" aria-hidden="true">${svgMarkup}</span>`;
    }

    function buildRunStatusIcon(runLike) {
        if (!runLike) {
            return '<span class="run-status-icon text-gray-400" aria-hidden="true"><svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" /></svg></span>';
        }
        const statusKey = runLike.is_complete ? runLike.status : 'running';
        const config = statusConfig[(statusKey || '').toLowerCase()] || statusConfig.pending;
        return `<span class="run-status-icon ${config.color}" aria-hidden="true"><svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg></span>`;
    }

    function getRunViewMode() {
        return (state && state.runViewMode === 'list') ? 'list' : 'grid';
    }

    function persistRunViewMode(mode) {
        try {
            localStorage.setItem('pipelinerunsViewMode', mode);
        } catch (error) {
            console.debug('Unable to persist pipeline run view mode', error);
        }
    }

    function updateRunViewToggleButtons() {
        const mode = getRunViewMode();
        const isGrid = mode !== 'list';
        const gridBtn = runViewToggle.gridBtn;
        const listBtn = runViewToggle.listBtn;
        if (gridBtn) {
            gridBtn.classList.toggle('runs-view-toggle__btn--active', isGrid);
            gridBtn.setAttribute('aria-pressed', isGrid ? 'true' : 'false');
        }
        if (listBtn) {
            listBtn.classList.toggle('runs-view-toggle__btn--active', !isGrid);
            listBtn.setAttribute('aria-pressed', !isGrid ? 'true' : 'false');
        }
    }

    function refreshMainGridView() {
        if (!lastMainGridRender) return;
        renderMainGridContent(lastMainGridRender.subgroups, lastMainGridRender.runs, lastMainGridRender.showAddButton);
    }

    function setRunViewMode(mode) {
        const normalized = mode === 'list' ? 'list' : 'grid';
        if (state.runViewMode === normalized) return;
        state.runViewMode = normalized;
        persistRunViewMode(normalized);
        updateRunViewToggleButtons();
        refreshMainGridView();
    }

    function getRunViewToggleContainer() {
        if (runViewToggle.container) return runViewToggle.container;
        const container = (DOM && DOM.runViewToggleContainer) || document.getElementById('run-view-toggle-container');
        if (container) {
            runViewToggle.container = container;
        }
        return container;
    }

    function ensureRunViewToggle() {
        const container = runViewToggle.container || getRunViewToggleContainer();
        if (!container) return;
        runViewToggle.container = container;
        if (!container.dataset.runViewToggleRendered) {
            container.innerHTML = `
                <div class="runs-view-toggle" role="group" aria-label="Pipeline run layout">
                    <button type="button" class="runs-view-toggle__btn" data-view-mode="grid" aria-pressed="true" title="Grid view">
                        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <rect x="4" y="4" width="7" height="7"></rect>
                            <rect x="13" y="4" width="7" height="7"></rect>
                            <rect x="4" y="13" width="7" height="7"></rect>
                            <rect x="13" y="13" width="7" height="7"></rect>
                        </svg>
                    </button>
                    <button type="button" class="runs-view-toggle__btn" data-view-mode="list" aria-pressed="false" title="List view">
                        <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                            <path d="M4 7h16" />
                            <path d="M4 12h16" />
                            <path d="M4 17h16" />
                        </svg>
                    </button>
                </div>`;
            container.dataset.runViewToggleRendered = 'true';
            runViewToggle.gridBtn = container.querySelector('[data-view-mode="grid"]');
            runViewToggle.listBtn = container.querySelector('[data-view-mode="list"]');
            if (runViewToggle.gridBtn) {
                runViewToggle.gridBtn.addEventListener('click', () => setRunViewMode('grid'));
            }
            if (runViewToggle.listBtn) {
                runViewToggle.listBtn.addEventListener('click', () => setRunViewMode('list'));
            }
        } else {
            runViewToggle.gridBtn = container.querySelector('[data-view-mode="grid"]');
            runViewToggle.listBtn = container.querySelector('[data-view-mode="list"]');
        }
        container.classList.remove('hidden');
        updateRunViewToggleButtons();
    }

    function hideRunViewToggle() {
        const container = getRunViewToggleContainer();
        if (container) {
            container.classList.add('hidden');
        }
    }

    const SIDEBAR_ICON_SVGS = {
        pipelineruns: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="5" width="16" height="14" rx="3"></rect><path d="M11 9l4 3-4 3V9z" fill="currentColor" stroke="none"></path></svg>`,
        pipelines: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 7h10"></path><path d="M7 17h10"></path><circle cx="7" cy="7" r="1.6" fill="currentColor" stroke="none"></circle><circle cx="17" cy="7" r="1.6" fill="currentColor" stroke="none"></circle><circle cx="7" cy="17" r="1.6" fill="currentColor" stroke="none"></circle><circle cx="17" cy="17" r="1.6" fill="currentColor" stroke="none"></circle><circle cx="12" cy="12" r="1.6" fill="currentColor" stroke="none"></circle></svg>`,
        steps: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 10l6-3 6 3-6 3-6-3z"></path><path d="M6 14l6 3 6-3"></path><path d="M6 18l6 3 6-3"></path></svg>`,
        scopes: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3" fill="currentColor" stroke="none"></circle><path d="M12 3v3"></path><path d="M12 18v3"></path><path d="M3 12h3"></path><path d="M18 12h3"></path><path d="M5.6 5.6l2.1 2.1"></path><path d="M16.3 16.3l2.1 2.1"></path><path d="M18.4 5.6l-2.1 2.1"></path><path d="M7.7 16.3l-2.1 2.1"></path><circle cx="12" cy="12" r="7"></circle></svg>`,
        triggers: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 3L6 14h6l-2 7 9-13h-6z"></path></svg>`,
        monitoring: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 12h2l2.5-6 4 12 3-8H20"></path></svg>`,
        system: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"></path></svg>`,
        branch: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="6" cy="5" r="2"></circle><circle cx="17" cy="7" r="2"></circle><circle cx="17" cy="19" r="2"></circle><path d="M6 7v10a3 3 0 003 3h5"></path><path d="M17 9v6"></path></svg>`
    };

    const RUN_TOGGLE_BASE_CLASS = 'run-select-toggle inline-flex items-center justify-center h-5 w-5 rounded-full border focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition-colors duration-150';
    const PIPELINE_BUTTON_BASE_CLASSES = 'run-view-pipeline-btn inline-flex items-center px-3 py-1.5 border border-transparent text-xs font-medium rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-offset-2 transition-colors duration-150';
    const PIPELINE_BUTTON_ACTIVE_CLASSES = `${PIPELINE_BUTTON_BASE_CLASSES} text-white bg-indigo-500 hover:bg-indigo-600 focus:ring-indigo-400`;
    const PIPELINE_BUTTON_DISABLED_CLASSES = `${PIPELINE_BUTTON_BASE_CLASSES} text-[var(--text-secondary)] bg-[var(--border-primary)] cursor-not-allowed pointer-events-none focus:ring-[var(--border-primary)]`;

    const pipelineExistenceCache = new Map();
    let apiBaseUrl = '';

    function normalizePipelineExistenceKey(identifier) {
        return (identifier || '').trim().toLowerCase();
    }

    async function pipelineExistsInDatabase(identifier) {
        const key = normalizePipelineExistenceKey(identifier);
        if (!key) return false;
        if (pipelineExistenceCache.has(key)) {
            return pipelineExistenceCache.get(key);
        }

        const segments = identifier.split('/').map(segment => segment.trim()).filter(Boolean);
        if (!segments.length) {
            pipelineExistenceCache.set(key, false);
            return false;
        }

        const encodedPath = segments.map(encodeURIComponent).join('/');
        const fallbackOrigin = (typeof window !== 'undefined' && window.location && window.location.origin) ? window.location.origin : '';
        const base = (apiBaseUrl || fallbackOrigin || '').replace(/\/+$/, '');

        if (!/^https?:/i.test(base)) {
            pipelineExistenceCache.set(key, false);
            return false;
        }

        const url = `${base}/v1/pipelines/${encodedPath}`;

        try {
            let response = await fetch(url, { method: 'HEAD' });
            if (response.status === 405 || response.status === 501) {
                response = await fetch(url, { method: 'GET' });
            }
            if (response.ok) {
                pipelineExistenceCache.set(key, true);
                return true;
            }
            if (response.status === 404) {
                pipelineExistenceCache.set(key, false);
                return false;
            }
            pipelineExistenceCache.set(key, false);
            return false;
        } catch (error) {
            if (console && typeof console.debug === 'function') {
                console.debug('Skipping pipeline existence check (request failed)', { identifier, error });
            }
            pipelineExistenceCache.set(key, false);
            return false;
        }
    }

    function applyPipelineButtonState(button, exists) {
        if (!button) return;
        if (exists) {
            button.className = PIPELINE_BUTTON_ACTIVE_CLASSES;
            const href = button.dataset.pipelineHref || '#';
            button.setAttribute('href', href);
            button.setAttribute('aria-disabled', 'false');
            button.setAttribute('tabindex', '0');
            const activeTitle = button.dataset.activeTitle || button.getAttribute('title') || '';
            if (activeTitle) button.setAttribute('title', activeTitle);
        } else {
            button.className = PIPELINE_BUTTON_DISABLED_CLASSES;
            button.removeAttribute('href');
            button.setAttribute('aria-disabled', 'true');
            button.setAttribute('tabindex', '-1');
            button.setAttribute('title', 'Pipeline definition not available in configuration repository');
        }
    }

    async function updatePipelineButtonState(identifier) {
        const button = document.getElementById('run-view-pipeline-link');
        if (!button) return;
        const expectedKey = normalizePipelineExistenceKey(identifier);
        const buttonKey = normalizePipelineExistenceKey(button.dataset.pipelineId || '');
        if (!expectedKey || expectedKey !== buttonKey) return;
        const exists = await pipelineExistsInDatabase(identifier);
        if (normalizePipelineExistenceKey(button.dataset.pipelineId || '') !== expectedKey) {
            return; // Button was re-rendered before fetch completed
        }
        applyPipelineButtonState(button, exists);
    }

    function setRunToolbarVisibility(visible) {
        if (!DOM) return;
        if (DOM.pipelineRunsSearchContainer) {
            DOM.pipelineRunsSearchContainer.classList.toggle('hidden', !visible);
        }
        if (DOM.pipelineRunsActions) {
            DOM.pipelineRunsActions.classList.toggle('hidden', !visible);
        }
    }

    function setNewFolderButtonEnabled(enabled) {
        if (!DOM) return;
        if (!DOM.pipelineRunsNewFolderBtn) return;
        DOM.pipelineRunsNewFolderBtn.classList.toggle('hidden', !enabled);
        DOM.pipelineRunsNewFolderBtn.disabled = !enabled;
        DOM.pipelineRunsNewFolderBtn.setAttribute('aria-disabled', enabled ? 'false' : 'true');
        if (DOM.pipelineRunsActions) {
            DOM.pipelineRunsActions.style.display = enabled ? '' : 'none';
        }
    }

    async function ensureSearchRunsLoaded() {
        if (Array.isArray(state.searchRuns) && state.searchRuns.length && state.searchRunsFetchedAt && (Date.now() - state.searchRunsFetchedAt) < 60 * 1000) {
            return state.searchRuns;
        }
        if (searchRunsPromise) return searchRunsPromise;
        searchRunsPromise = fetchData('/v1/runs')
            .then(runs => {
                state.searchRuns = Array.isArray(runs) ? runs : [];
                state.searchRunsFetchedAt = Date.now();
                return state.searchRuns;
            })
            .catch(() => {
                state.searchRuns = [];
                return state.searchRuns;
            })
            .finally(() => {
                searchRunsPromise = null;
            });
        return searchRunsPromise;
    }

    function parseRepoIdentifiers(fullName) {
        if (typeof fullName !== 'string') return null;
        const trimmed = fullName.trim();
        if (!trimmed) return null;
        const parts = trimmed.split('/');
        if (parts.length < 2) return null;
        const owner = parts[0].trim();
        const repo = parts.slice(1).join('/').trim();
        if (!owner || !repo) return null;
        return { owner, repo };
    }

    function getRepoIdentifiersByGroupId(groupId) {
        const group = getGroupById(Number(groupId));
        if (!group || !group.name) return null;
        return parseRepoIdentifiers(group.name);
    }

    function buildContextHashFromContext(context) {
        const ctx = context || {};
        const tab = ctx.tab || 'main';
        if (tab === 'recent') {
            return '#/pipelineruns/recent';
        }
        const segments = Array.isArray(ctx.groupSegments) ? ctx.groupSegments.filter(Boolean) : [];
        if (segments.length) {
            return `#/pipelineruns/main/${segments.join('/')}`;
        }
        return '#/pipelineruns/main';
    }

    async function refreshAfterMutation(hashOverride) {
        const targetHash = hashOverride || window.location.hash || '#/pipelineruns/main';
        if (hashOverride && window.location.hash !== hashOverride) {
            window.location.hash = hashOverride;
            return;
        }
        if (typeof refresh === 'function') {
            await refresh(targetHash);
        }
        updateSelectionBar();
    }

    async function deleteBranchRunsForRepo(owner, repo, branch) {
        if (!owner || !repo) {
            alert('Unable to determine repository information for this branch.');
            return false;
        }
        const branchLabel = branch || 'Others';
        if (!window.confirm(`Delete all pipeline runs for ${owner}/${repo} (${branchLabel})? This cannot be undone.`)) {
            return false;
        }
        const encodedOwner = encodeURIComponent(owner);
        const encodedRepo = encodeURIComponent(repo);
        const encodedBranch = branchLabel.split('/').map(part => encodeURIComponent(part)).join('/');
        await deleteData(`/v1/repositories/${encodedOwner}/${encodedRepo}/branches/${encodedBranch}`);
        clearSelectedRuns();
        await refreshAfterMutation();
        return true;
    }

    async function handleBranchDeleteButton(button) {
        if (!button) return false;
        const branch = button.dataset.branch || '';
        if (!branch) {
            alert('Unable to determine which branch to delete.');
            return true;
        }
        let owner = button.dataset.owner || '';
        let repo = button.dataset.repo || '';
        let repoGroupId = button.dataset.repoGroupId;
        if ((!owner || !repo) && !repoGroupId) {
            const hostWithGroup = button.closest('[data-repo-group-id]');
            if (hostWithGroup) repoGroupId = hostWithGroup.dataset.repoGroupId;
        }
        if ((!owner || !repo) && repoGroupId) {
            const identifiers = getRepoIdentifiersByGroupId(repoGroupId);
            if (identifiers) {
                owner = owner || identifiers.owner;
                repo = repo || identifiers.repo;
            }
        }
        await deleteBranchRunsForRepo(owner, repo, branch);
        return true;
    }

    function ensureRunSelectionSet() {
        if (!(state.selectedRunIds instanceof Set)) {
            state.selectedRunIds = new Set();
        }
        return state.selectedRunIds;
    }

    function clearSelectedRuns(options = {}) {
        const selected = ensureRunSelectionSet();
        if (selected.size === 0) return;
        selected.clear();
        document.querySelectorAll('.run-card[data-selected="true"]').forEach(card => {
            card.classList.remove('ring-2', 'ring-indigo-500', 'border-transparent');
            card.setAttribute('data-selected', 'false');
        });
        document.querySelectorAll('.run-select-toggle[aria-pressed="true"]').forEach(btn => {
            setRunToggleState(btn, false);
        });
        if (!options.silent) updateSelectionBar();
    }

    function getRunToggleClasses(isSelected) {
        return isSelected
            ? `${RUN_TOGGLE_BASE_CLASS} bg-indigo-600 border-transparent text-white shadow-sm`
            : `${RUN_TOGGLE_BASE_CLASS} border-[var(--border-primary)] bg-[var(--bg-primary)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]`;
    }

    function setRunToggleState(btn, isSelected) {
        if (!btn) return;
        btn.setAttribute('aria-pressed', isSelected ? 'true' : 'false');
        btn.className = getRunToggleClasses(isSelected);
    }

    function updateRunCardSelectionStyles(runId, isSelected) {
        if (!runId) return;
        const cards = document.querySelectorAll('.run-card[data-run-id]');
        cards.forEach(card => {
            if ((card.dataset.runId || '') !== runId) return;
            if (isSelected) {
                card.classList.add('ring-2', 'ring-indigo-500', 'border-transparent');
                card.setAttribute('data-selected', 'true');
            } else {
                card.classList.remove('ring-2', 'ring-indigo-500', 'border-transparent');
                card.setAttribute('data-selected', 'false');
            }
        });

        const buttons = document.querySelectorAll('.run-select-toggle[data-run-id]');
        buttons.forEach(btn => {
            if ((btn.dataset.runId || '') !== runId) return;
            setRunToggleState(btn, isSelected);
        });
    }

    function updateSelectionBar() {
        const selected = ensureRunSelectionSet();
        const count = selected.size;
        if (!DOM.runSelectionBar) return;
        const isDetailView = !!(state.currentRunData && state.currentRunData.run_info);
        const isPipelinePage = state.currentPath === 'pipelineruns';
        if (count > 0 && isPipelinePage && !isDetailView) {
            DOM.runSelectionBar.classList.remove('hidden');
            if (DOM.runSelectionCount) {
                DOM.runSelectionCount.textContent = `${count} run${count === 1 ? '' : 's'} selected`;
            }
        } else {
            DOM.runSelectionBar.classList.add('hidden');
        }
        if (DOM.runSelectionDeleteBtn) {
            DOM.runSelectionDeleteBtn.disabled = count === 0;
        }
        if (DOM.runSelectionClearBtn) {
            DOM.runSelectionClearBtn.disabled = count === 0;
        }
    }

    function toggleRunSelection(runId) {
        if (!runId) return;
        const selected = ensureRunSelectionSet();
        if (selected.has(runId)) {
            selected.delete(runId);
            updateRunCardSelectionStyles(runId, false);
        } else {
            selected.add(runId);
            updateRunCardSelectionStyles(runId, true);
        }
        updateSelectionBar();
    }

    async function deleteSelectedRuns() {
        const selected = ensureRunSelectionSet();
        if (selected.size === 0) return;
        if (!window.confirm(`Delete ${selected.size} selected pipeline run${selected.size === 1 ? '' : 's'}? This cannot be undone.`)) {
            return;
        }
        if (DOM.runSelectionDeleteBtn) {
            DOM.runSelectionDeleteBtn.disabled = true;
        }
        const ids = Array.from(selected);
        try {
            for (const id of ids) {
                await deleteData(`/v1/runs/${encodeURIComponent(id)}`);
            }
            clearSelectedRuns({ silent: true });
            updateSelectionBar();
            await refreshAfterMutation();
        } finally {
            if (DOM.runSelectionDeleteBtn) {
                const remaining = ensureRunSelectionSet();
                DOM.runSelectionDeleteBtn.disabled = remaining.size === 0;
            }
        }
    }

    async function deleteRunById(runId, context) {
        if (!runId) {
            alert('Missing run identifier.');
            return;
        }
        if (!window.confirm('Delete this pipeline run permanently? This action cannot be undone.')) {
            return;
        }
        await deleteData(`/v1/runs/${encodeURIComponent(runId)}`);
        const targetHash = buildContextHashFromContext(context);
        clearSelectedRuns();
        state.currentRunData = null;
        await refreshAfterMutation(targetHash);
    }

    function createDefaultLogLevelFilter() {
        return new Set(LOG_LEVEL_FILTER_KEYS);
    }

    function decodeHashSegment(segment) {
        if (typeof segment !== 'string') return '';
        try {
            return decodeURIComponent(segment);
        } catch {
            return segment;
        }
    }

    function applyLogRouteState(logSegments = [], query = {}) {
        const segments = Array.isArray(logSegments) ? logSegments : [];
        const [rawSteps, rawLevels, rawWrap, rawStructured, rawAgent, rawShort] = segments;

        const stepSpec = decodeHashSegment(rawSteps || '').trim();
        const selectedSteps = new Set();
        if (stepSpec && stepSpec.toLowerCase() !== 'all') {
            stepSpec.split(',').forEach(part => {
                const name = part.trim();
                if (name) selectedSteps.add(name);
            });
        }
        state.logsSelectedSteps = selectedSteps;

        const levelSpecRaw = decodeHashSegment(rawLevels || '').trim().toLowerCase();
        if (!levelSpecRaw || levelSpecRaw === 'all') {
            state.logsLevelFilter = createDefaultLogLevelFilter();
        } else {
            const levels = levelSpecRaw.split(',').map(l => l.trim()).filter(l => LOG_LEVEL_FILTER_KEYS.includes(l));
            state.logsLevelFilter = levels.length ? new Set(levels) : createDefaultLogLevelFilter();
        }

        const wrapSpec = decodeHashSegment(rawWrap || '').trim().toLowerCase();
        if (!wrapSpec || wrapSpec === 'wrap' || wrapSpec === 'on') {
            state.logsWrap = true;
        } else if (wrapSpec === 'unwrap' || wrapSpec === 'nowrap' || wrapSpec === 'off') {
            state.logsWrap = false;
        }

        const structuredSpec = decodeHashSegment(rawStructured || '').trim().toLowerCase();
        if (!structuredSpec || structuredSpec === 'structured' || structuredSpec === 'on') {
            state.logsStructured = true;
        } else if (structuredSpec === 'unstructured' || structuredSpec === 'raw' || structuredSpec === 'off') {
            state.logsStructured = false;
        }

        const agentSpec = decodeHashSegment(rawAgent || '').trim().toLowerCase();
        state.logsAgentOnly = agentSpec === 'agent';

        const shortSpec = decodeHashSegment(rawShort || '').trim().toLowerCase();
        state.logsShortView = shortSpec === 'short';

        const searchText = typeof query.search === 'string' ? query.search : '';
        state.logsSearchText = searchText;
        state._logsFocusFirstMatch = !!searchText;
        if (DOM && DOM.logsSearch) {
            DOM.logsSearch.value = searchText;
        }
        if (DOM && DOM.logsToggleAgent) {
            const btn = DOM.logsToggleAgent;
            const classes = ['ring-1', 'ring-[var(--border-accent)]', 'text-[var(--text-primary)]'];
            classes.forEach(cls => btn.classList.toggle(cls, state.logsAgentOnly));
            btn.setAttribute('aria-pressed', state.logsAgentOnly ? 'true' : 'false');
        }
        const shortToggle = DOM && DOM.logsToggleShort;
        if (shortToggle) {
            shortToggle.checked = !!state.logsShortView;
        }
        const wrapToggle = document.getElementById('logs-toggle-wrap');
        const structuredToggle = document.getElementById('logs-toggle-structured');
        const shortOn = !!state.logsShortView;
        if (wrapToggle) {
            wrapToggle.disabled = shortOn;
            const label = wrapToggle.closest('label');
            if (label) label.classList.toggle('opacity-50', shortOn);
        }
        if (structuredToggle) {
            structuredToggle.disabled = shortOn;
            const label = structuredToggle.closest('label');
            if (label) label.classList.toggle('opacity-50', shortOn);
        }
    }

    function buildLogSegmentsFromState() {
        const selectedSteps = state.logsSelectedSteps instanceof Set ? Array.from(state.logsSelectedSteps) : [];
        const stepSegment = selectedSteps.length ? selectedSteps.join(',') : 'all';

        const levelSet = state.logsLevelFilter instanceof Set ? state.logsLevelFilter : createDefaultLogLevelFilter();
        const orderedLevels = LOG_LEVEL_FILTER_KEYS.filter(level => levelSet.has(level));
        const levelSegment = orderedLevels.length === LOG_LEVEL_FILTER_KEYS.length || orderedLevels.length === 0
            ? 'all'
            : orderedLevels.join(',');

        const wrapSegment = state.logsWrap === false ? 'unwrap' : 'wrap';
        const structuredSegment = state.logsStructured === false ? 'unstructured' : 'structured';
        const agentSegment = state.logsAgentOnly ? 'agent' : 'all';
        const shortSegment = state.logsShortView ? 'short' : 'full';

        return [stepSegment || 'all', levelSegment || 'all', wrapSegment, structuredSegment, agentSegment, shortSegment];
    }

    function buildLogsHashFromState() {
        if (!state.currentRunData || !state.currentRunData.run_info) return null;
        const runInfo = state.currentRunData.run_info;
        const context = resolveRunContext(state.currentRunContext);
        const extras = ['logs', ...buildLogSegmentsFromState()];
        const search = (state.logsSearchText || '').trim();
        const queryOptions = search ? { query: { search } } : undefined;
        return buildRunHashWithExtras(runInfo, context, extras, queryOptions);
    }

    function syncLogsHash(options = {}) {
        const { replace = true } = options || {};
        const newHash = buildLogsHashFromState();
        if (!newHash) return;
        const currentHash = window.location.hash || '';
        if (currentHash === newHash) return;

        if (state) {
            state._suppressNextRoute = true;
            if (state._suppressRouteTimeout) {
                try { clearTimeout(state._suppressRouteTimeout); } catch { }
            }
            try {
                state._suppressRouteTimeout = setTimeout(() => {
                    state._suppressNextRoute = false;
                    state._suppressRouteTimeout = null;
                }, 100);
            } catch {
                state._suppressNextRoute = false;
            }
        }

        try {
            const url = new URL(window.location.href);
            url.hash = newHash.slice(1);
            if (replace) {
                history.replaceState(null, '', url.toString());
            } else {
                history.pushState(null, '', url.toString());
            }
        } catch {
            if (replace) {
                window.location.replace(newHash);
            } else {
                window.location.hash = newHash;
            }
        }
    }

    function openLogsForTask(stepName, taskName) {
        if (!state || !state.currentRunData) return;
        if (typeof showLogsModal !== 'function') return;

        const selectedSteps = new Set();
        if (stepName) selectedSteps.add(stepName);
        state.logsSelectedSteps = selectedSteps;
        state.logsSearchText = taskName || '';
        state._logsFocusFirstMatch = true;

        if (DOM.logsSearch) {
            DOM.logsSearch.value = state.logsSearchText;
        }
        if (DOM.logsStepSearch) {
            DOM.logsStepSearch.value = '';
        }

        const logsVisible = DOM.logsModal && !DOM.logsModal.classList.contains('hidden');

        if (logsVisible) {
            if (typeof updateLogsStepList === 'function') updateLogsStepList();
            if (typeof renderLogsWithFilters === 'function') renderLogsWithFilters({ scrollToTop: true });
            if (typeof state.syncLogsHash === 'function') state.syncLogsHash({ replace: true });
        } else {
            showLogsModal();
        }
    }

    function openLogsForStep(stepName) {
        if (!stepName) return;
        openLogsForTask(stepName, '');
    }

    function bindTaskGraphLogging(container) {
        if (!container) return;
        if (container.__taskLogsBound) return;
        container.addEventListener('click', handleTaskGraphClick);
        container.__taskLogsBound = true;
    }

    function handleTaskGraphClick(event) {
        const node = event.target.closest('g.graph-node[data-task-name]');
        if (!node) return;
        if (node.dataset && node.dataset.context) return;

        const taskName = node.dataset.taskName || '';
        if (!taskName) return;

        const container = event.currentTarget;
        let stepName = (container && container.dataset && container.dataset.stepName) || '';
        if (!stepName) {
            const host = container.closest('[data-step-name]');
            if (host && host.dataset.stepName) stepName = host.dataset.stepName;
        }
        if (!stepName) return;

        event.preventDefault();
        event.stopPropagation();
        openLogsForTask(stepName, taskName);
    }

    function normalizeParentId(id) {
        return id === null || id === undefined || id === 0 ? null : id;
    }

    function getGroupById(id) {
        if (!id || !state || !Array.isArray(state.groups)) return null;
        return state.groups.find(g => g.id === id) || null;
    }

    function getGroupPathSegmentsById(groupId) {
        if (!groupId) return [];
        if (groupPathCache.has(groupId)) return [...groupPathCache.get(groupId)];

        const segments = [];
        const visited = new Set();
        let current = getGroupById(groupId);
        while (current && !visited.has(current.id)) {
            visited.add(current.id);
            const parentId = normalizeParentId(current.parent_id);
            segments.unshift(buildGroupSegment(current, parentId));
            current = parentId ? getGroupById(parentId) : null;
        }
        groupPathCache.set(groupId, segments);
        return [...segments];
    }

    function buildGroupSegment(group, parentId) {
        const rawName = group?.name || '';
        const base = encodeURIComponent(rawName || '');
        const siblings = (state.groups || []).filter(g => normalizeParentId(g.parent_id) === parentId && g.id !== group.id);
        const hasCollision = rawName ? siblings.some(s => (s.name || '') === rawName) : false;
        const safeBase = base || `group-${group.id}`;
        return hasCollision ? `${safeBase}~${group.id}` : safeBase;
    }

    function parseGroupSegment(segment) {
        if (!segment) return { name: '', idHint: null };
        let decoded;
        try {
            decoded = decodeURIComponent(segment);
        } catch {
            decoded = segment;
        }
        let idHint = null;
        let name = decoded;
        const tildeIndex = decoded.lastIndexOf('~');
        if (tildeIndex > -1) {
            const maybeId = decoded.slice(tildeIndex + 1);
            if (/^\d+$/.test(maybeId)) {
                idHint = Number(maybeId);
                name = decoded.slice(0, tildeIndex);
            }
        }
        return { name, idHint };
    }

    function findGroupByPathSegments(segments) {
        if (!Array.isArray(segments) || segments.length === 0) return null;
        let currentParentId = null;
        let currentGroup = null;
        const visited = new Set();
        for (const segment of segments) {
            const { name, idHint } = parseGroupSegment(segment);
            const siblings = (state.groups || []).filter(g => normalizeParentId(g.parent_id) === currentParentId);
            let match = null;
            if (idHint !== null) {
                match = siblings.find(g => g.id === idHint);
            }
            if (!match) {
                match = siblings.find(g => (g.name || '') === name);
            }
            if (!match) {
                return null;
            }
            if (visited.has(match.id)) {
                return null;
            }
            visited.add(match.id);
            currentGroup = match;
            currentParentId = match.id;
        }
        return currentGroup;
    }

    function findGroupByName(name) {
        if (!name || !Array.isArray(state.groups)) return null;
        return state.groups.find(g => g.name === name) || null;
    }

    function ensureGroupAncestorsExpanded(groupId) {
        let current = getGroupById(groupId);
        const visited = new Set();
        while (current && !visited.has(current.id)) {
            visited.add(current.id);
            state.expandedGroups.add(current.id);
            const parentId = normalizeParentId(current.parent_id);
            current = parentId ? getGroupById(parentId) : null;
        }
    }

    function encodeRunContext(context) {
        try {
            return encodeURIComponent(JSON.stringify(context || {}));
        } catch {
            return '';
        }
    }

    function parseRunContextAttr(attr) {
        if (!attr) return null;
        try {
            return JSON.parse(decodeURIComponent(attr));
        } catch {
            return null;
        }
    }

    function resolveRunContext(contextOverride) {
        const resolved = contextOverride ? { ...contextOverride } : { tab: state.currentTab || 'main' };
        resolved.tab = resolved.tab || 'main';
        if (resolved.tab === 'main') {
            if (!Array.isArray(resolved.groupSegments)) {
                if (resolved.groupId) {
                    resolved.groupSegments = getGroupPathSegmentsById(resolved.groupId);
                } else if (state && Array.isArray(state.selectedGroupPathSegments)) {
                    resolved.groupSegments = [...state.selectedGroupPathSegments];
                } else {
                    resolved.groupSegments = [];
                }
            }
            if (!resolved.groupId && resolved.groupSegments.length) {
                const grp = findGroupByPathSegments(resolved.groupSegments);
                if (grp) {
                    resolved.groupId = grp.id;
                }
            }
        } else {
            resolved.groupSegments = resolved.groupSegments || [];
        }
        return resolved;
    }

    function buildRunHash(run, contextOverride) {
        if (!run) return '#/pipelineruns/main';
        const context = resolveRunContext(contextOverride);
        const segments = ['pipelineruns', context.tab || 'main'];

        if (context.tab === 'recent') {
            segments.push(run.run_id);
        } else if (context.tab === 'main') {
            let groupSegments = Array.isArray(context.groupSegments) ? [...context.groupSegments] : [];
            if (!groupSegments.length && context.groupId) {
                groupSegments = getGroupPathSegmentsById(context.groupId);
            }
            if (!groupSegments.length && run.git_repo_owner && run.git_repo_name) {
                const repoGroup = findGroupByName(`${run.git_repo_owner}/${run.git_repo_name}`);
                if (repoGroup) {
                    groupSegments = getGroupPathSegmentsById(repoGroup.id);
                }
            }
            const filteredSegments = groupSegments.filter(Boolean);
            if (filteredSegments.length) {
                segments.push(...filteredSegments);
            }
            segments.push(run.run_id);
        } else {
            segments.push(run.run_id);
        }

        return '#/' + segments.join('/');
    }

    function buildRunHashWithExtras(run, contextOverride, extras = [], options = {}) {
        let hash = buildRunHash(run, contextOverride);
        if (extras && extras.length) {
            const encodedExtras = extras.map(seg => encodeURIComponent(seg));
            hash += `/${encodedExtras.join('/')}`;
        }
        if (options && options.query) {
            const params = Object.entries(options.query)
                .filter(([, value]) => value !== undefined && value !== null && String(value).length > 0);
            if (params.length) {
                const search = new URLSearchParams(params).toString();
                hash += `?${search}`;
            }
        }
        return hash;
    }

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

    function getActiveRunId() {
        return parsePipelineRunsHash(window.location.hash).runId;
    }

    function escapeAttribute(value) {
        if (value === null || value === undefined) return '';
        return String(value)
            .replace(/&/g, '&amp;')
            .replace(/"/g, '&quot;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
    }

    function escapeForSelector(value) {
        if (value === null || value === undefined) return '';
        const str = String(value);
        if (typeof CSS !== 'undefined' && CSS && typeof CSS.escape === 'function') {
            return CSS.escape(str);
        }
        return str.replace(/["\\]/g, '\\$&');
    }

    function getTriggerGroupId(run) {
        if (!run || typeof run !== 'object') return '';
        const id = run.trigger_event_id;
        return id === undefined || id === null || id === '' ? '' : String(id);
    }

    function getTriggerGroupAttr(run) {
        const id = getTriggerGroupId(run);
        return id ? ` data-trigger-group-id="${escapeAttribute(id)}"` : '';
    }

    function formatTriggerEventInfo(id, options = {}) {
        const opts = typeof options === 'object' && options !== null ? options : {};
        const fallback = opts.fallback || 'N/A';
        if (id === undefined || id === null) {
            return { text: fallback, full: fallback };
        }
        const raw = String(id).trim();
        if (!raw) {
            return { text: fallback, full: fallback };
        }
        const limit = Number(opts.limit);
        const hasLimit = Number.isFinite(limit) && limit > 0;
        const text = hasLimit && raw.length > limit ? `${raw.slice(0, limit)}…` : raw;
        return { text, full: raw };
    }

    function formatTriggerEventCardDisplay(id, options = {}) {
        const info = formatTriggerEventInfo(id, options);
        if (info.full === 'N/A') {
            return {
                display: info.text,
                title: escapeAttribute(info.full),
            };
        }

        const raw = info.full;
        const trimmed = raw.length > 8 ? raw.slice(0, 8) : raw;
        return {
            display: trimmed,
            title: escapeAttribute(raw),
        };
    }

    function getPipelineNameHTML(run) {
        const name = `<span class="font-medium truncate flex-1 min-w-0">${run.pipeline_name}</span>`;
        const overrideIcon = run.pipeline_source === 'database override'
            ? `<span class="flex-shrink-0 text-blue-400" title="Overridden from database"><svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg></span>`
            : '';
        const includedBadge = run.parent_run_id
            ? `<span class="flex-shrink-0 text-[10px] bg-[var(--bg-primary)] text-[var(--text-accent)] font-semibold px-1.5 py-0.5 rounded-md">Included</span>`
            : '';
        return `<div class="flex items-center gap-1.5 min-w-0">${[name, overrideIcon, includedBadge].filter(Boolean).join('')}</div>`;
    }

    function normalizeRunId(value) {
        if (typeof value !== 'string') return '';
        return value.trim().toLowerCase();
    }

    function getPipelineSteps(definition) {
        if (!definition || typeof definition !== 'object') return [];
        const steps = definition.steps;
        return Array.isArray(steps) ? steps : [];
    }

    function init(context = {}) {
        if (initialized) return;
        state = context.state;
        DOM = context.DOM;
        fetchData = context.fetchData;
        postData = context.postData;
        deleteData = context.deleteData;
        logsModule = context.logsModule;
        refresh = context.refresh || (() => { });
        state.lastRunETag = null;
        state.runPollingTimer = null;
        if (!DOM.sidebarNav && DOM.sidebarDetailsNav) {
            DOM.sidebarNav = DOM.sidebarDetailsNav;
        }
        DOM.pipelineRunsSearch = document.getElementById('pipelineruns-search');
        DOM.pipelineRunsSearchContainer = DOM.pipelineRunsSearchContainer || document.getElementById('pipelineruns-search-container');
        DOM.pipelineRunsActions = DOM.pipelineRunsActions || document.getElementById('pipelineruns-actions');
        DOM.pipelineRunsNewFolderBtn = DOM.pipelineRunsNewFolderBtn || document.getElementById('pipelineruns-new-folder-btn');
        runViewToggle.container = DOM.runViewToggleContainer || document.getElementById('run-view-toggle-container');
        if (!state.runViewMode) {
            state.runViewMode = 'grid';
        }
        ensureRunViewToggle();
        if (DOM.pipelineRunsSearch) {
            DOM.pipelineRunsSearch.value = state.runSearchTerm || '';
        }
        if (DOM.pipelineRunsNewFolderBtn) {
            DOM.pipelineRunsNewFolderBtn.addEventListener('click', () => {
                showAddGroupModal(state.selectedGroupId || null);
            });
        }
        if (!Array.isArray(state.recentRuns)) state.recentRuns = [];
        if (!Array.isArray(state.searchRuns)) state.searchRuns = [];
        apiBaseUrl = typeof context.apiBaseUrl === 'string' ? context.apiBaseUrl.replace(/\/+$/, '') : '';
        if (!(state.currentRunTrackedIds instanceof Map)) {
            state.currentRunTrackedIds = new Map();
        }
        startPipelineRunsPolling();
        ensureRunSelectionSet();
        updateSelectionBar();
        if (DOM.runSelectionClearBtn) {
            DOM.runSelectionClearBtn.addEventListener('click', () => {
                clearSelectedRuns();
            });
        }
        if (DOM.runSelectionDeleteBtn) {
            DOM.runSelectionDeleteBtn.addEventListener('click', async () => {
                await deleteSelectedRuns();
            });
        }
        state.syncLogsHash = syncLogsHash;
        setupLogHelpers();
        bindDomEvents();
        setupObservers();
        initialized = true;
    }

    async function handleRealtimeUpdate(updatedRunId) {
        const normalizedUpdateId = normalizeRunId(updatedRunId);
        const currentRunIdRaw = state.currentRunData?.run_info?.run_id || '';
        const currentRunIdNormalized = normalizeRunId(currentRunIdRaw);
        const trackedIds = (state.currentRunTrackedIds instanceof Map) ? state.currentRunTrackedIds : null;
        const currentHashInfo = parsePipelineRunsHash(window.location.hash);

        if (currentHashInfo.path !== 'pipelineruns' || !currentHashInfo.runId) {
            if (state.currentTab === 'recent') {
                const runs = await fetchData('/v1/runs');
                if (runs) renderSidebarPipelineRunsList(runs);
            } else if (state.currentTab === 'main') {
                await renderHierarchy(state.groups);
            }
            return;
        }
        if (normalizedUpdateId && currentRunIdNormalized && normalizedUpdateId === currentRunIdNormalized) {
            if (currentHashInfo.runId === currentRunIdRaw) {
                await fetchActiveRun(currentRunIdRaw, true);
            }
        } else if (normalizedUpdateId && trackedIds && trackedIds.has(normalizedUpdateId) && currentRunIdRaw) {
            if (currentHashInfo.runId === currentRunIdRaw) {
                await fetchActiveRun(currentRunIdRaw, true);
            }
        }

        if (state.currentTab === 'recent') {
            const runs = await fetchData('/v1/runs');
            if (runs) renderSidebarPipelineRunsList(runs);
        } else if (state.currentTab === 'main') {
            await renderHierarchy(state.groups);
        }
    }

    function setupLogHelpers() {
        if (!logsModule) {
            showLogsModal = () => { };
            closeLogsModal = () => { };
            renderLogsWithFilters = () => { };
            updateLogsStepList = () => { };
            return;
        }
        showLogsModal = typeof logsModule.showLogsModal === 'function' ? logsModule.showLogsModal.bind(logsModule) : () => { };
        closeLogsModal = typeof logsModule.closeLogsModal === 'function' ? logsModule.closeLogsModal.bind(logsModule) : () => { };
        renderLogsWithFilters = typeof logsModule.renderLogsWithFilters === 'function' ? logsModule.renderLogsWithFilters.bind(logsModule) : () => { };
        updateLogsStepList = typeof logsModule.updateLogsStepList === 'function' ? logsModule.updateLogsStepList.bind(logsModule) : () => { };
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
            try { state.panzoomInstance.destroy(); } catch (e) { }
            state.panzoomInstance = null;
        }
        if (state._wheelTarget && state._wheelHandler) {
            try { state._wheelTarget.removeEventListener('wheel', state._wheelHandler); } catch (e) { }
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
        } catch { }

        if (!element || !element.firstElementChild) return;

        state.panzoomInstance = Panzoom(element, {
            canvas: true,
            maxScale: 5,
            minScale: 0.1,
        });
        // Ensure the pan element never lets the browser handle scrolling/zoom gestures
        try { element.style.touchAction = 'none'; } catch { }
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
            } catch (e) { }
        }
        state._pointerDownHandler = (ev) => {
            if (!state.panzoomInstance) return;
            const isControl = !!(ev.target && (ev.target.closest('#steps-graph-controls') || ev.target.closest('#tasks-graph-controls')));
            if (isControl) return;
            try {
                ev.preventDefault();
                state.panzoomInstance.handleDown(ev);
            } catch { }
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
            } catch { }
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
            const contentWidth = layout?.width || element.scrollWidth || element.offsetWidth;
            const contentHeight = layout?.height || element.scrollHeight || element.offsetHeight;
            if (!contentWidth || !contentHeight || !parentRect.width || !parentRect.height) return;

            // start from a clean slate
            if (!state.panzoomInstance) return;
            state.panzoomInstance.reset({ animate: false });

            // Centered fit behavior for both Steps and Tasks
            const padding = 40;
            const fitScale = Math.min(
                parentRect.width / (contentWidth + padding),
                parentRect.height / (contentHeight + padding)
            ) * 0.98;
            const scale = Math.min(1, fitScale);
            state.panzoomInstance.zoom(scale, { animate: false });
            const x = (parentRect.width - contentWidth * scale) / 2;
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
                                const s = Math.sqrt(a * a + b * b) || 1;
                                const px = v[4] || 0; const py = v[5] || 0;
                                state._baselineStepsTransform = { x: px, y: py, scale: s };
                            }
                        }
                    } else {
                        state._baselineStepsTransform = { x: 0, y: 0, scale: 1 };
                    }
                }
            } catch { }
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
                                scale = Math.sqrt(a * a + b * b) || 1; x = v[4] || 0; y = v[5] || 0;
                            }
                        }
                    }
                    if (isSteps) state._stepsViewTransform = { x, y, scale };
                    state._suppressInitialFit = true; // avoid recenter jump on first control click
                } catch { }
                const layout = isSteps ? null : (state._lastStepLayout || null);
                initPanAndZoom(view, layout);
            }
        } catch { }
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
                        scale = Math.sqrt(a * a + b * b) || 1;
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
            const leftMin = margin - bbox.x * scale;
            const rightMax = container.clientWidth - margin - (bbox.x + bbox.width) * scale;
            const topMin = topSafe + margin - bbox.y * scale;
            const bottomMax = container.clientHeight - margin - (bbox.y + bbox.height) * scale;

            const panX = Math.min(leftMin, Math.max(rightMax, centerPanX));
            const panY = Math.min(bottomMax, Math.max(topMin, centerPanY));
            state.panzoomInstance.pan(panX, panY, { animate: true });
        } catch { }
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
                    if (vals.length === 6) { const a = vals[0], b = vals[1]; scale = Math.sqrt(a * a + b * b) || 1; }
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
        } catch { }
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
        if (Array.isArray(runs)) {
            state.recentRuns = runs;
        } else {
            state.recentRuns = [];
        }
        renderSidebarPipelineRunsList(state.recentRuns);
        const hashInfo = parsePipelineRunsHash(window.location.hash);
        if (state.currentTab === 'recent' && !hashInfo.runId) {
            renderMainGridContent(null, state.recentRuns, false);
        }
    }

    function stopPipelineRunsPolling() {
        if (state.pipelineRunsPollingTimer) {
            clearTimeout(state.pipelineRunsPollingTimer);
            state.pipelineRunsPollingTimer = null;
        }
    }

    function startPipelineRunsPolling() {
        if (state.pipelineRunsPollingTimer) return;
        const poll = async () => {
            const info = parsePipelineRunsHash(window.location.hash || '#/pipelineruns/main');
            const onPipelineruns = info.path === 'pipelineruns';
            if (!onPipelineruns) {
                stopPipelineRunsPolling();
                return;
            }

            if (state.currentTab === 'recent') {
                await fetchAllRuns();
            } else {
                await fetchGroups();
                await renderSidebar('pipelineruns', state.currentTab || 'main');
                if (!info.runId) {
                    if (state.selectedGroupId) {
                        await fetchMainContent(state.selectedGroupId);
                    } else {
                        const rootGroups = state.groups.filter(g => normalizeParentId(g.parent_id) === null);
                        state.currentRepoRunsByBranch = null;
                        state.currentRepoGroupId = null;
                        renderMainGridContent(rootGroups, null, true);
                    }
                }
            }

            const isHidden = document.visibilityState === 'hidden';
            const interval = isHidden ? 12000 : 6000;
            state.pipelineRunsPollingTimer = setTimeout(poll, interval);
        };
        state.pipelineRunsPollingTimer = setTimeout(poll, 0);
    }

    async function fetchMainContent(groupId) {
        const runsByBranch = await fetchData(`/v1/runs?groupId=${groupId}`);
        const hasRuns = runsByBranch && Object.keys(runsByBranch).length > 0;
        const subgroups = state.groups.filter(g => normalizeParentId(g.parent_id) === normalizeParentId(groupId));

        if (hasRuns) {
            state.currentRepoRunsByBranch = runsByBranch || {};
            state.currentRepoGroupId = groupId || null;
            renderGroupedRuns(runsByBranch);
        } else {
            state.currentRepoRunsByBranch = null;
            state.currentRepoGroupId = null;
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
            const triggerGroupId = targetRunElement.dataset.triggerGroupId;

            const highlightedElements = new Set();

            if (triggerGroupId) {
                const safeId = escapeForSelector(triggerGroupId);
                if (safeId) {
                    document.querySelectorAll(`[data-trigger-group-id="${safeId}"]`).forEach(el => highlightedElements.add(el));
                }
            }

            highlightedElements.add(targetRunElement);

            highlightedElements.forEach(el => {
                if (el.matches('a, div[data-href]')) {
                    el.classList.add('run-link-highlight');
                } else {
                    el.classList.add('sidebar-link-highlight');
                }
            });

        }
    }

    function collectRelatedRunIds(runDetails) {
        const ids = new Map();
        if (!runDetails || typeof runDetails !== 'object') return ids;
        const addId = (rawId) => {
            const normalized = normalizeRunId(rawId);
            if (normalized) ids.set(normalized, rawId);
        };

        addId(runDetails.run_info && runDetails.run_info.run_id);

        const visitChildren = (children) => {
            if (!Array.isArray(children)) return;
            children.forEach(child => {
                if (!child || typeof child !== 'object') return;
                const childId = child.run_id || (child.run_info && child.run_info.run_id) || child.runId;
                addId(childId);
                if (Array.isArray(child.child_runs)) visitChildren(child.child_runs);
            });
        };

        visitChildren(runDetails.child_runs);
        return ids;
    }

    function updateTrackedRunSubscriptions(runDetails) {
        const relatedIdsMap = collectRelatedRunIds(runDetails);
        if (!(state.currentRunTrackedIds instanceof Map)) {
            state.currentRunTrackedIds = new Map();
        }
        state.currentRunTrackedIds.clear();
        relatedIdsMap.forEach((rawId, normalized) => {
            state.currentRunTrackedIds.set(normalized, rawId);
        });
    }

    function startRunPolling(runId) {
        stopRunPolling();
        const poll = async () => {
            if (state.currentRunData?.run_info?.run_id !== runId) return;
            await fetchActiveRun(runId, true);

            const isHidden = document.visibilityState === 'hidden';
            const interval = isHidden ? 30000 : 2000;
            state.runPollingTimer = setTimeout(poll, interval);
        };
        state.runPollingTimer = setTimeout(poll, 2000);
    }

    function stopRunPolling() {
        if (state.runPollingTimer) {
            clearTimeout(state.runPollingTimer);
            state.runPollingTimer = null;
        }
    }

    async function fetchActiveRun(runId, isRefresh = false) {
        if (!runId) return;
        if (!isRefresh) {
            resetMainView();
            stopRunPolling();
            state.lastRunETag = null;
        }

        const prevRunInfo = state.currentRunData?.run_info;
        const canUseConditionalGet =
            prevRunInfo &&
            prevRunInfo.run_id === runId &&
            prevRunInfo.is_complete === true &&
            !!state.lastRunETag;

        const options = { cache: 'no-store' };
        if (canUseConditionalGet) {
            options.headers = { 'If-None-Match': state.lastRunETag };
        }

        const runDetails = await fetchData(`/v1/runs/${runId}`, options);

        if (runDetails === null && fetchData.lastStatus === 304) {
            if (!isRefresh) startRunPolling(runId);
            return;
        }
        if (runDetails) {
            if (fetchData.lastETag) {
                state.lastRunETag = fetchData.lastETag;
            }
            state.currentRunData = runDetails;
            refreshSidebarRunState(runDetails.run_info || {});
            if (!isRefresh) startRunPolling(runId);
            updateTrackedRunSubscriptions(runDetails);
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
                            state.expandedStepPositions = new Map(entries.map(([k, v]) => [k, { x: Number(v.x) || 0, y: Number(v.y) || 0 }]));
                        }
                        if (saved.scale) state.stepLayoutScale = Math.max(1, Math.min(1.8, Number(saved.scale) || 1.0));
                        if (saved.tasksScale) state.taskClusterScale = Math.max(0.65, Math.min(1.0, Number(saved.tasksScale) || 1.0));
                    }
                }
            } catch { }
            renderRunView(runDetails);
            const { action, stepName } = parsePipelineRunsHash(window.location.hash);
            if (action === 'steps' && stepName) {
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
        groupPathCache.clear();
    }

    function resetMainView() {
        DOM.placeholder.classList.add('hidden');
        DOM.graphContainer.classList.add('hidden');
        DOM.tasksGraphContainer.classList.add('hidden');
        DOM.tasksEmpty.classList.add('hidden');
        DOM.mainGridContainer.classList.add('hidden');
        if (DOM.pageContentWrapper) DOM.pageContentWrapper.classList.remove('no-scroll');

        if (DOM.placeholder) {
            const heading = DOM.placeholder.querySelector('h3');
            const body = DOM.placeholder.querySelector('p');
            if (heading) heading.textContent = 'Select a pipeline run';
            if (body) body.textContent = 'Choose a run from the sidebar to view its progress.';
        }
    }

    async function renderSidebar(activeRoute, currentTab) {
        if (DOM.pipelineRunsSearch) {
            DOM.pipelineRunsSearch.value = state.runSearchTerm || '';
        }

        const navConfig = [
            { route: 'pipelineruns', title: 'Pipeline Runs', iconSvg: SIDEBAR_ICON_SVGS.pipelineruns, logoClass: 'step-logo--pipelineruns' },
            { route: 'pipelines', title: 'Pipelines', iconSvg: SIDEBAR_ICON_SVGS.pipelines, logoClass: 'step-logo--pipelines' },
            { route: 'steps', title: 'Steps', iconSvg: SIDEBAR_ICON_SVGS.steps, logoClass: 'step-logo--steps' },
            { route: 'scopes', title: 'Scopes', iconSvg: SIDEBAR_ICON_SVGS.scopes, logoClass: 'step-logo--scopes' },
            { route: 'monitoring', title: 'Monitoring', iconSvg: SIDEBAR_ICON_SVGS.monitoring, logoClass: 'step-logo--monitoring' },
            { route: 'triggers', title: 'Triggers', iconSvg: SIDEBAR_ICON_SVGS.triggers, logoClass: 'step-logo--triggers' },
            { route: 'system', title: 'System', iconSvg: SIDEBAR_ICON_SVGS.system, logoClass: 'step-logo--system' },
        ];

        let baseNavHtml = navConfig.map(item => {
            const isActive = activeRoute === item.route;
            const iconSvg = item.iconSvg || `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"></svg>`;
            const iconHtml = buildSidebarLogo(iconSvg, 'menu', item.logoClass || '');
            return `<a href="#/${item.route}" class="sidebar-link flex items-center gap-3 p-2 text-[var(--text-primary)] rounded-md transition-colors duration-200 group ${isActive ? 'active' : ''}" data-navigo>
                        ${iconHtml}
                        <span>${item.title}</span>
                    </a>`;
        }).join('');

        DOM.sidebarBaseNav.innerHTML = `<div class="space-y-1">${baseNavHtml}</div>`;

        let detailsNavHtml = '';
        if (activeRoute === 'pipelines') {
            detailsNavHtml = `<div class="px-2 mt-2 mb-2 flex items-center justify-between">
                                  <h2 class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">All Pipelines</h2>
                              </div>
                              <div id="pipelines-sidebar-tree"></div>`;
            DOM.sidebarDetailsNav.innerHTML = detailsNavHtml;
            const pipelinesModule = window.NopsAI.pages?.pipelines;
            if (pipelinesModule && typeof pipelinesModule.renderSidebarTree === 'function') {
                try {
                    pipelinesModule.renderSidebarTree(document.getElementById('pipelines-sidebar-tree'));
                } catch (error) {
                    console.error('Failed to render pipelines sidebar tree:', error);
                }
            }
        } else if (activeRoute === 'triggers') {
            detailsNavHtml = `<div class="px-2 mt-2 mb-2 flex items-center justify-between">
                                  <h2 class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">Triggers</h2>
                              </div>
                              <div id="triggers-sidebar-tree"></div>`;
            DOM.sidebarDetailsNav.innerHTML = detailsNavHtml;
            const triggersModule = window.NopsAI.pages?.triggers;
            if (triggersModule && typeof triggersModule.renderSidebarForRoute === 'function') {
                try {
                    triggersModule.renderSidebarForRoute();
                } catch (error) {
                    console.error('Failed to render triggers sidebar tree:', error);
                }
            }
        } else if (activeRoute === 'steps') {
            detailsNavHtml = `<div class="px-2 mt-2 mb-2 flex items-center justify-between">
                                  <h2 class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">Steps Library</h2>
                              </div>
                              <div id="steps-sidebar-tree"></div>`;
            DOM.sidebarDetailsNav.innerHTML = detailsNavHtml;
            const stepsModule = window.NopsAI.pages?.steps;
            if (stepsModule && typeof stepsModule.renderSidebarTree === 'function') {
                try {
                    stepsModule.renderSidebarTree(document.getElementById('steps-sidebar-tree'));
                } catch (error) {
                    console.error('Failed to render steps sidebar tree:', error);
                }
            }
        } else if (activeRoute === 'scopes') {
            detailsNavHtml = `<div class="px-2 mt-2 mb-2 flex items-center justify-between">
                                  <h2 class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">Scopes</h2>
                              </div>
                              <div id="scopes-sidebar-tree"></div>`;
            DOM.sidebarDetailsNav.innerHTML = detailsNavHtml;
            const scopesModule = window.NopsAI.pages?.scopes;
            if (scopesModule && typeof scopesModule.renderSidebarForRoute === 'function') {
                try {
                    scopesModule.renderSidebarForRoute();
                } catch (error) {
                    console.error('Failed to render scopes sidebar tree:', error);
                }
            }
        } else if (activeRoute === 'pipelineruns') {
            if (currentTab === 'recent') {
                detailsNavHtml = `<h2 class="px-2 mt-2 mb-2 text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">Recent Runs</h2>
                                  <ul id="pipeline-runs-list" class="space-y-1"></ul>`;
                DOM.sidebarDetailsNav.innerHTML = detailsNavHtml;
                fetchAllRuns();
            } else {
                detailsNavHtml = `<div class="px-2 mt-2 mb-2 flex items-center justify-between">
                                      <h2 class="text-xs font-semibold tracking-wider text-[var(--text-secondary)] uppercase">Main</h2>
                                  </div>
                                  <div id="main-hierarchy"></div>`;
                DOM.sidebarDetailsNav.innerHTML = detailsNavHtml;
                state.repoLastRunCache.clear();
                await renderHierarchy(state.groups);
            }
        } else {
            DOM.sidebarDetailsNav.innerHTML = '';
        }
    }

    async function renderHierarchy(groups, parentId = null, level = 0, container = null) {
        container = container || (level === 0 ? document.getElementById('main-hierarchy') : document.querySelector(`[data-group-id='${parentId}'] .group-children`));
        if (!container) return;

        const normalizedParentId = normalizeParentId(parentId);
        const children = groups.filter(g => normalizeParentId(g.parent_id) === normalizedParentId);

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
            const hasChildren = groups.some(g => normalizeParentId(g.parent_id) === group.id);
            const isExpanded = state.expandedGroups.has(group.id);
            const isRepo = (group.name || '').includes('/');
            const displayName = isRepo ? group.name.split('/')[1] : group.name;
            const canExpand = hasChildren || isRepo;
            const isActive = state.selectedGroupId === group.id;
            const pathSegments = getGroupPathSegmentsById(group.id);
            const groupHref = pathSegments.length ? `#/pipelineruns/main/${pathSegments.join('/')}` : '#/pipelineruns/main';

            let chevron = canExpand
                ? `<svg class="h-4 w-4 mr-1 text-[var(--text-secondary)] chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>`
                : `<div class="w-5 h-4 mr-1"></div>`;

            const folderIconSvg = `<svg class="h-4 w-4 text-[var(--text-secondary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>`;
            const repoIconSvg = `<svg class="h-4 w-4 text-[var(--text-accent)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><circle cx="8" cy="7" r="2" fill="currentColor" /><circle cx="8" cy="17" r="2" fill="currentColor" /><circle cx="16" cy="7" r="2" fill="currentColor" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 7h4"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9v6a4 4 0 004 4h4"/></svg>`;
            const iconHtml = isRepo ? repoIconSvg : folderIconSvg;
            const descriptionText = (group.description || '').trim();
            const descriptionHtml = descriptionText ? `<span class="group-tree-description truncate">${escapeText(descriptionText)}</span>` : '';
            const linkTitleAttr = escapeAttribute(descriptionText ? `${displayName} — ${descriptionText}` : displayName);

            html += `<li data-group-id="${group.id}" draggable="true">
                            <div class="flex items-center justify-between p-2 text-[var(--text-primary)] rounded-md group-header-container ${isActive ? 'bg-[var(--bg-tertiary)]' : ''}">
                                <div class="flex items-center group-header flex-grow cursor-pointer ${isExpanded ? 'expanded' : ''}">
                                    ${chevron}
                                    <a href="${groupHref}" class="flex items-center flex-grow gap-2" title="${linkTitleAttr}">
                                        ${iconHtml}
                                        <span class="flex flex-col leading-tight min-w-0">
                                            <span class="truncate">${escapeText(displayName)}</span>
                                            ${descriptionHtml}
                                        </span>
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
                    renderRepoChildren(childContainer, runsByBranch, level + 1, group.id);
                } else {
                    await renderHierarchy(groups, group.id, level + 1, childContainer);
                }
            }
        }
    }

    function renderRepoChildren(container, runsByBranch, level, repoGroupId) {
        if (!container) return;
        const repoInfo = getRepoIdentifiersByGroupId(repoGroupId);
        let html = `<ul class="pl-${level > 0 ? '4' : '0'} space-y-1">`;
        const sortedBranches = Object.keys(runsByBranch).sort();

        sortedBranches.forEach(branch => {
            const runs = runsByBranch[branch];
            const branchId = `branch-${container.closest('li').dataset.groupId}-${branch.replace(/[^a-zA-Z0-9]/g, '')}`;
            const isExpanded = state.expandedGroups.has(branchId);
            const chevron = `<svg class="h-4 w-4 mr-1 text-[var(--text-secondary)] chevron ${isExpanded ? 'rotate-90' : ''}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>`;

            const branchIconHtml = `<svg class="h-4 w-4 mr-2 text-[var(--text-accent)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>`;

            html += `<li data-branch-id="${branchId}" data-repo-group-id="${repoGroupId}">
                            <div class="flex items-center justify-between p-2 text-[var(--text-primary)] rounded-md group-header cursor-pointer">
                                <div class="flex items-center min-w-0">
                                    ${chevron}
                                    ${branchIconHtml}
                                    <span class="truncate">${escapeText(branch)}</span>
                                </div>
                                <div class="flex items-center gap-1 branch-actions">
                                    <button type="button" class="branch-delete-btn inline-flex items-center justify-center h-6 w-6 rounded-full text-[var(--text-secondary)] hover:text-red-500 hover:bg-[var(--border-primary)] focus:outline-none" data-branch="${escapeAttribute(branch)}" data-repo-group-id="${repoGroupId}" ${repoInfo ? `data-owner="${escapeAttribute(repoInfo.owner)}" data-repo="${escapeAttribute(repoInfo.repo)}"` : ''} title="Delete branch">
                                        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0 1 16.138 21H7.862a2 2 0 0 1-1.995-1.858L5 7m5-3h4m1 3H7" /></svg>
                                    </button>
                                </div>
                            </div>
                            <div class="group-children">
                                ${isExpanded ? renderRunLinks(runs, level + 1, repoGroupId) : ''}
                            </div>
                        </li>`;
        });
        html += '</ul>';
        container.innerHTML = html;
        updateSelectionBar();
    }

    function renderRunLinks(runs, level, groupId) {
        let html = `<ul class="pl-${level > 0 ? '4' : '0'} space-y-1">`;
        const groupSegments = groupId ? getGroupPathSegmentsById(groupId) : [];
        runs.forEach(run => {
            const repoFullName = `${run.git_repo_owner}/${run.git_repo_name}`;
            const context = { tab: 'main', groupId, groupSegments };
            const contextAttr = encodeRunContext(context);
            html += `<li data-run-id="${run.run_id}" data-repo-full-name="${repoFullName}"${run.parent_run_id ? ` data-parent-run-id="${run.parent_run_id}"` : ''}${getTriggerGroupAttr(run)} data-run-context="${contextAttr}">
                        ${renderSidebarRunLinkHTML(run, context)}
                     </li>`;
        });
        html += `</ul>`;
        return html;
    }

    // Update renderSidebarPipelineRunsList to use the new reusable function
    function renderSidebarPipelineRunsList(runs) {
        const listEl = document.getElementById('pipeline-runs-list');
        if (!listEl) return;
        const runsArray = Array.isArray(runs) ? runs : [];
        const searchTerm = (state.runSearchTerm || '').trim().toLowerCase();
        const filteredRuns = searchTerm ? runsArray.filter(run => runMatchesSearch(run, searchTerm)) : runsArray;
        if (!filteredRuns.length) {
            const message = searchTerm ? 'No runs match your search.' : 'No recent runs found.';
            listEl.innerHTML = `<li><p class="p-2 text-[var(--text-secondary)] text-sm">${escapeText(message)}</p></li>`;
            return;
        }
        const context = { tab: 'recent' };
        listEl.innerHTML = filteredRuns.map(run => {
            const repoFullName = `${run.git_repo_owner}/${run.git_repo_name}`;
            const contextAttr = encodeRunContext(context);
            return `<li data-run-id="${run.run_id}" data-repo-full-name="${repoFullName}"${run.parent_run_id ? ` data-parent-run-id="${run.parent_run_id}"` : ''}${getTriggerGroupAttr(run)} data-run-context="${contextAttr}">
                        ${renderSidebarRunLinkHTML(run, context)}
                    </li>`;
        }).join('');
    }

    async function applyRunSearchFilter() {
        if (DOM.pipelineRunsSearch && DOM.pipelineRunsSearch.value !== (state.runSearchTerm || '')) {
            DOM.pipelineRunsSearch.value = state.runSearchTerm || '';
        }

        if (state.currentTab === 'recent') {
            renderSidebarPipelineRunsList(state.recentRuns || []);
            const hashInfo = parsePipelineRunsHash(window.location.hash);
            if (hashInfo.runId) {
                return;
            }
            renderMainGridContent(null, state.recentRuns || [], false);
            return;
        } else {
            const hashInfo = parsePipelineRunsHash(window.location.hash);
            if (hashInfo.runId) {
                return;
            }
            if (state.runSearchTerm) {
                await ensureSearchRunsLoaded();
            }
            if (state.selectedGroupId) {
                if (state.currentRepoRunsByBranch && Object.keys(state.currentRepoRunsByBranch).length > 0) {
                    renderGroupedRuns(state.currentRepoRunsByBranch);
                } else {
                    const subgroups = state.groups.filter(g => normalizeParentId(g.parent_id) === normalizeParentId(state.selectedGroupId));
                    renderMainGridContent(subgroups, null, true);
                }
            } else {
                const rootGroups = state.groups.filter(g => normalizeParentId(g.parent_id) === null);
                state.currentRepoRunsByBranch = null;
                state.currentRepoGroupId = null;
                renderMainGridContent(rootGroups, null, true);
            }
        }
    }

    function renderGroupedRuns(runsByBranch) {
        state.currentRepoRunsByBranch = runsByBranch || {};
        state.currentRepoGroupId = state.selectedGroupId || null;
        resetMainView();
        DOM.mainGridContainer.classList.remove('hidden');
        setRunToolbarVisibility(true);
        setNewFolderButtonEnabled(false);

        if (!runsByBranch || Object.keys(runsByBranch).length === 0) {
            DOM.mainGridContainer.innerHTML = `<p class="text-[var(--text-secondary)]">No pipeline runs found for this repository.</p>`;
            return;
        }

        const sortedBranches = Object.keys(runsByBranch).sort((a, b) => {
            const lastRunA = runsByBranch[a][0];
            const lastRunB = runsByBranch[b][0];
            if (!lastRunA || !lastRunA.started_at) return 1;
            if (!lastRunB || !lastRunB.started_at) return -1;
            return new Date(lastRunB.started_at) - new Date(lastRunA.started_at);
        });

        const context = resolveRunContext({
            tab: 'main',
            groupId: state.selectedGroupId,
            groupSegments: state.selectedGroupPathSegments,
        });

        const searchTerm = (state.runSearchTerm || '').trim().toLowerCase();
        const filteredBranches = [];

        sortedBranches.forEach(branch => {
            const runs = runsByBranch[branch] || [];
            const visibleRuns = searchTerm ? runs.filter(run => runMatchesSearch(run, searchTerm)) : runs;
            if (!visibleRuns.length) return;
            const latestSummary = summarizeLatestTriggerStatus(runs);
            const fallbackRun = runs[0];
            filteredBranches.push({
                branch,
                runs: visibleRuns,
                latestRun: latestSummary?.referenceRun || fallbackRun,
                latestStatus: latestSummary?.status || normalizeRunStatus(fallbackRun),
            });
        });

        if (filteredBranches.length === 0) {
            const message = searchTerm ? 'No runs match your search in this repository.' : 'No pipeline runs found for this repository.';
            DOM.mainGridContainer.innerHTML = `<p class="text-[var(--text-secondary)]">${escapeText(message)}</p>`;
            return;
        }

        let html = '<div class="space-y-6">';

        filteredBranches.forEach((entry, index) => {
            const { branch, runs, latestRun: latestFromBranch, latestStatus } = entry;
            const latestRun = latestFromBranch || runs[0];
            const statusKey = latestStatus || normalizeRunStatus(latestRun);
            const config = statusKey ? (statusConfig[statusKey] || statusConfig.pending) : statusConfig.pending;
            const isExpanded = index === 0; // Expand first branch by default

            html += `
            <div class="bg-[var(--bg-secondary)] rounded-lg shadow-md">
                <div class="branch-header cursor-pointer flex items-center justify-between p-4 border-b border-[var(--border-primary)] ${isExpanded ? 'expanded' : ''}">
                    <div class="flex items-center min-w-0">
                        <svg class="h-5 w-5 mr-3 text-[var(--text-secondary)] chevron flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" /></svg>
                        <svg class="h-5 w-5 mr-3 text-[var(--text-accent)] flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                        <span class="font-semibold text-lg text-[var(--text-primary)] truncate">${escapeText(branch)}</span>
                        <span class="ml-4 text-sm text-[var(--text-secondary)] hidden sm:inline">(${runs.length} runs)</span>
                    </div>
                    <div class="flex items-center gap-2">
                        <span class="text-sm text-[var(--text-secondary)] mr-1 hidden sm:block">Latest run: ${latestRun ? timeAgo(latestRun.started_at) : 'N/A'}</span>
                        <button type="button" class="branch-delete-btn inline-flex items-center justify-center h-8 w-8 rounded-full text-[var(--text-secondary)] hover:text-red-500 hover:bg-[var(--border-primary)] focus:outline-none" data-branch="${escapeAttribute(branch)}" data-owner="${escapeAttribute(latestRun?.git_repo_owner || '')}" data-repo="${escapeAttribute(latestRun?.git_repo_name || '')}" title="Delete branch">
                            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0 1 16.138 21H7.862a2 2 0 0 1-1.995-1.858L5 7m5-3h4m1 3H7" /></svg>
                        </button>
                        <svg class="h-6 w-6 ${config.color}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                    </div>
                </div>
                <div class="branch-runs p-4 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4" style="${isExpanded ? 'max-height: 2000px;' : ''}">
                ${runs.map(run => renderRunCard(run, context, { viewMode: getRunViewMode() })).join('')}
            </div>
            </div>`;
        });
        html += '</div>';
        DOM.mainGridContainer.innerHTML = html;
        updateSelectionBar();
    }

    function renderRunCardHTML(run, isSelected = false) {
        const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
        const branchDisplay = formatBranchDisplay(run.git_ref, run.git_target_ref, { html: true });
        const repoFullName = `${run.git_repo_owner}/${run.git_repo_name}`;
        const pipelineNameHTML = getPipelineNameHTML(run);
        const triggerCard = formatTriggerEventCardDisplay(run.trigger_event_id);
        const statusIcon = buildRunStatusIcon(run);
        const pipelineTitleHTML = `<div class="flex items-center gap-2 min-w-0">${statusIcon}<div class="flex-1 min-w-0">${pipelineNameHTML}</div></div>`;
        return `
            <div>
                <div class="flex items-start justify-between">
                    <div class="flex-1 min-w-0 pr-4">${pipelineTitleHTML}</div>
                </div>
                <p class="text-sm text-[var(--text-secondary)] items-center mt-1">
                   ${run.git_repo_name}
                </p>
                <p class="text-sm text-[var(--text-link)] font-mono items-center mt-1">
                   <svg class="inline-block h-4 w-4 mr-1 -mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                   ${branchDisplay}
                </p>
            </div>
            <div class="mt-4 text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
                <div class="flex items-center">
                    <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                    <span class="truncate">${run.git_pusher_name || 'N/A'}</span>
                </div>
                <div class="flex items-center">
                    <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                    <span class="truncate" title="Commit Hash">${(run.git_commit_sha || '...').slice(0, 8)}</span>
                </div>
                <div class="flex items-center">
                    <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H5v-2H3v-2H1v-4a6 6 0 016-6h1.5" /></svg>
                    <span class="truncate" title="Run ID">${(run.run_id || '...').slice(0, 8)}</span>
                </div>
                 <div class="flex items-center">
                    <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7a1 1 0 011-1h3.586a1 1 0 01.707.293l6.414 6.414a1 1 0 010 1.414l-4.586 4.586a1 1 0 01-1.414 0L7.293 13.707A1 1 0 017 13V9a1 1 0 011-1z" /></svg>
                    <span class="truncate" title="Trigger Event ID">${escapeText(triggerCard.display)}</span>
                </div>
            </div>
            <div class="mt-4 pt-3 border-t border-[var(--border-primary)] flex items-center justify-between text-xs text-[var(--text-secondary)]">
                <div class="flex items-center gap-2">
                    <svg class="h-3.5 w-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                    <span class="truncate">${timeAgo(timeToDisplay)}</span>
                </div>
                <button type="button" class="${getRunToggleClasses(isSelected)}" data-run-id="${escapeAttribute(run.run_id)}" aria-pressed="${isSelected ? 'true' : 'false'}" title="${isSelected ? 'Deselect run' : 'Select run'}">
                    <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
                </button>
            </div>`;
    }

    function renderRunListRowHTML(run, isSelected = false) {
        const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
        const branchDisplay = formatBranchDisplay(run.git_ref, run.git_target_ref, { html: true });
        const pipelineNameHTML = getPipelineNameHTML(run);
        const commit = escapeText((run.git_commit_sha || '...').slice(0, 8));
        const runIdShort = escapeText((run.run_id || '...').slice(0, 8));
        const triggerCard = formatTriggerEventCardDisplay(run.trigger_event_id);
        const triggerDisplay = escapeText(triggerCard.display || 'N/A');
        const updatedDisplay = timeAgo(timeToDisplay);
        const statusIcon = buildRunStatusIcon(run);
        return `
            <div class="run-list-info">
                <span class="run-list-icon" aria-hidden="true">${statusIcon}</span>
                <div class="run-list-titles">
                    <div class="run-list-title">${pipelineNameHTML}</div>
                    <div class="run-list-sub">${branchDisplay}</div>
                </div>
            </div>
            <div class="run-list-meta">
                <span class="run-list-meta-item">
                    <span class="run-list-meta-label">Commit</span>
                    <span class="run-list-meta-value">${commit}</span>
                </span>
                <span class="run-list-meta-item">
                    <span class="run-list-meta-label">Run ID</span>
                    <span class="run-list-meta-value">${runIdShort}</span>
                </span>
                <span class="run-list-meta-item">
                    <span class="run-list-meta-label">Trigger</span>
                    <span class="run-list-meta-value">${triggerDisplay}</span>
                </span>
                <span class="run-list-meta-item">
                    <span class="run-list-meta-label">Updated</span>
                    <span class="run-list-meta-value">${updatedDisplay}</span>
                </span>
            </div>
            <div class="run-list-actions">
                <button type="button" class="${getRunToggleClasses(isSelected)}" data-run-id="${escapeAttribute(run.run_id)}" aria-pressed="${isSelected ? 'true' : 'false'}" title="${isSelected ? 'Deselect run' : 'Select run'}">
                    <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" /></svg>
                </button>
            </div>
        `;
    }

    function renderSidebarRunLinkHTML(run, contextOverride) {
        const context = resolveRunContext(contextOverride);
        const runUrl = buildRunHash(run, context);
        const contextAttr = encodeRunContext(context);
        const activeRunId = getActiveRunId();
        const config = statusConfig[(run.is_complete ? run.status : 'running').toLowerCase()] || statusConfig.pending;
        const isActive = run.run_id === activeRunId;
        const timeToDisplay = run.is_complete ? run.finished_at : run.started_at;
        const pipelineNameHTML = getPipelineNameHTML(run);
        const triggerCard = formatTriggerEventCardDisplay(run.trigger_event_id);

        return `
            <a href="${runUrl}" data-run-context="${contextAttr}"
                class="sidebar-run-link flex items-center p-2 text-sm text-[var(--text-secondary)] rounded-md ${isActive ? 'bg-[var(--bg-tertiary)] text-[var(--text-primary)] sidebar-run-link--active' : ''}">
                <svg class="h-4 w-4 mr-2 flex-shrink-0 ${config.color}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                <div class="flex-1 overflow-hidden">
                    <div class="flex justify-between items-center">
                        ${pipelineNameHTML}
                        <span class="text-xs text-[var(--text-secondary)] flex-shrink-0 ml-2">${timeAgo(timeToDisplay)}</span>
                    </div>
                    <div class="text-xs text-[var(--text-secondary)] font-mono mt-1 space-y-1">
                        <div class="flex items-center">
                            <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" /></svg>
                            <span class="truncate" title="Commit Hash">${(run.git_commit_sha || '...').slice(0, 8)}</span>
                        </div>                                          
                      <div class="flex items-center">
                          <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H5v-2H3v-2H1v-4a6 6 0 016-6h1.5" /></svg>
                          <span class="truncate" title="Run ID">${(run.run_id || '...').slice(0, 8)}</span>
                      </div>
                      <div class="flex items-center">
                          <svg class="h-3.5 w-3.5 mr-2 text-gray-500 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 7a1 1 0 011-1h3.586a1 1 0 01.707.293l6.414 6.414a1 1 0 010 1.414l-4.586 4.586a1 1 0 01-1.414 0L7.293 13.707A1 1 0 017 13V9a1 1 0 011-1z" /></svg>
                          <span class="truncate" title="Trigger Event ID">${escapeText(triggerCard.display)}</span>
                      </div>
                    </div>
                </div>
            </a>`;
    }

    // Keep sidebar entries in sync with live run updates without a full refresh.
    function refreshSidebarRunState(runInfo) {
        if (!runInfo || !runInfo.run_id) return;
        const safeId = escapeForSelector(runInfo.run_id);
        const matchingItems = document.querySelectorAll(`[data-run-id="${safeId}"]`);

        matchingItems.forEach(item => {
            const ctxAttr = item.getAttribute('data-run-context');
            const ctx = parseRunContextAttr(ctxAttr) || undefined;
            item.innerHTML = renderSidebarRunLinkHTML(runInfo, ctx);
        });

        // Upsert into the recent runs list so new runs appear without reload.
        if (!Array.isArray(state.recentRuns)) {
            state.recentRuns = [];
        }
        const existingIdx = state.recentRuns.findIndex(r => r && r.run_id === runInfo.run_id);
        if (existingIdx !== -1) {
            state.recentRuns[existingIdx] = { ...state.recentRuns[existingIdx], ...runInfo };
        } else {
            state.recentRuns.unshift({ ...runInfo });
        }
        renderSidebarPipelineRunsList(state.recentRuns);
    }

    function handleRunSummaryUpdate(runData) {
        // Find all elements representing this run (cards in main view, links in sidebar)
        const elements = document.querySelectorAll(`[data-run-id="${runData.run_id}"]`);
        const repoOwner = runData.git_repo_owner || '';
        const repoName = runData.git_repo_name || '';
        const repoFullName = `${repoOwner}/${repoName}`;
        const triggerGroupId = getTriggerGroupId(runData);
        elements.forEach(el => {
            const context = parseRunContextAttr(el.dataset.runContext) || null;
            const resolvedContext = resolveRunContext(context);
            if (repoOwner || repoName) {
                el.dataset.repoFullName = repoFullName;
            } else {
                delete el.dataset.repoFullName;
            }
            if (runData.parent_run_id) {
                el.dataset.parentRunId = runData.parent_run_id;
            } else {
                delete el.dataset.parentRunId;
            }
            if (triggerGroupId) {
                el.dataset.triggerGroupId = triggerGroupId;
            } else {
                delete el.dataset.triggerGroupId;
            }
            el.dataset.runContext = encodeRunContext(resolvedContext);
            if (el.hasAttribute('data-href')) { // It's a run card
                const viewModeAttr = el.dataset.viewMode === 'list' ? 'list' : 'grid';
                const newMarkup = renderRunCard(runData, resolvedContext, { viewMode: viewModeAttr });
                replaceRunCardElement(el, newMarkup);
            } else if (el.tagName === 'LI') { // It's a sidebar item
                el.innerHTML = renderSidebarRunLinkHTML(runData, resolvedContext);
            }
        });
        updateRunCardSelectionStyles(runData.run_id || '', ensureRunSelectionSet().has(runData.run_id || ''));
    }

    // Update renderRunCard to use the new reusable function
    function renderRunCard(run, contextOverride, options = {}) {
        const repoFullName = `${run.git_repo_owner}/${run.git_repo_name}`;
        const parentAttr = run.parent_run_id ? ` data-parent-run-id="${run.parent_run_id}"` : '';
        const pipelineIdentifier = getPipelineIdentifierFromRun(run);
        const pipelineAttr = pipelineIdentifier ? ` data-pipeline-id="${escapeAttribute(pipelineIdentifier)}"` : '';
        const context = resolveRunContext(contextOverride);
        const runUrl = buildRunHash(run, context);
        const contextAttr = encodeRunContext(context);
        const selected = ensureRunSelectionSet();
        const runId = run.run_id || '';
        const runIdAttr = escapeAttribute(runId);
        const isSelected = selected.has(runId);
        const selectedClasses = isSelected ? 'ring-2 ring-indigo-500 border-transparent' : '';
        const viewMode = (options && options.viewMode === 'list') ? 'list' : 'grid';
        const isListView = viewMode === 'list';
        const baseClasses = `run-card relative block rounded-lg bg-[var(--bg-primary)] transition-all duration-200 cursor-pointer border border-[var(--border-primary)] shadow-sm ${selectedClasses}`;
        const layoutClasses = isListView
            ? 'run-card--list'
            : 'run-card--grid p-4 flex flex-col justify-between';
        const content = isListView ? renderRunListRowHTML(run, isSelected) : renderRunCardHTML(run, isSelected);
        return `
            <div data-href="${runUrl}"
                data-run-id="${runIdAttr}"${pipelineAttr}
                data-repo-full-name="${repoFullName}"${parentAttr}${getTriggerGroupAttr(run)}
                data-run-context="${contextAttr}"
                data-view-mode="${viewMode}"
                data-selected="${isSelected ? 'true' : 'false'}"
                class="${baseClasses} ${layoutClasses}"
            >
                ${content}
            </div>`;
    }

    function replaceRunCardElement(oldEl, html) {
        if (!oldEl || !oldEl.parentNode) return;
        const temp = document.createElement('div');
        temp.innerHTML = html.trim();
        const newEl = temp.firstElementChild;
        if (newEl) {
            oldEl.parentNode.replaceChild(newEl, oldEl);
        }
    }

    async function renderMainGridContent(subgroups, runs, showAddButton = false) {
        lastMainGridRender = { subgroups, runs, showAddButton };
        const container = DOM.mainGridContainer;
        if (!container) return;
        const isListView = getRunViewMode() === 'list';
        ensureRunViewToggle();
        resetMainView();
        container.classList.remove('hidden');

        const searchTerm = (state.runSearchTerm || '').trim().toLowerCase();
        setRunToolbarVisibility(true);
        setNewFolderButtonEnabled(showAddButton && !searchTerm);

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

        const childGroupMap = new Map();
        if (Array.isArray(state.groups)) {
            state.groups.forEach(childGroup => {
                const parentKey = normalizeParentId(childGroup.parent_id);
                if (!childGroupMap.has(parentKey)) {
                    childGroupMap.set(parentKey, []);
                }
                childGroupMap.get(parentKey).push(childGroup);
            });
        }

        const visibleSubgroups = !searchTerm ? (subgroups || []) : (subgroups || []).filter(group => (group.name || '').toLowerCase().includes(searchTerm));
        let baseRuns = null;
        if (Array.isArray(runs)) {
            baseRuns = runs;
        } else if (searchTerm && Array.isArray(state.searchRuns)) {
            baseRuns = state.searchRuns;
        }
        let visibleRuns = baseRuns;
        if (Array.isArray(visibleRuns) && searchTerm) {
            visibleRuns = visibleRuns.filter(run => runMatchesSearch(run, searchTerm));
        }

        if (searchTerm && visibleSubgroups.length === 0 && (!Array.isArray(visibleRuns) || visibleRuns.length === 0)) {
            container.innerHTML = `<p class="py-10 text-center text-sm text-[var(--text-secondary)]">No runs or folders match your search.</p>`;
            return;
        }

        if (!searchTerm && !showAddButton && visibleSubgroups.length === 0 && (!Array.isArray(visibleRuns) || visibleRuns.length === 0)) {
            container.innerHTML = `<p class="py-10 text-center text-sm text-[var(--text-secondary)]">No pipeline runs found.</p>`;
            return;
        }

        const contextForRuns = resolveRunContext({
            tab: state.currentTab || (runs ? 'recent' : 'main'),
            groupId: state.selectedGroupId,
            groupSegments: state.selectedGroupPathSegments,
        });

        const wrapperClass = 'grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4';
        let html = `<div class="${wrapperClass}">`;

        visibleSubgroups.forEach(group => {
            const rawName = group?.name || '';
            const isRepo = rawName.includes('/');
            const displayName = isRepo ? rawName.split('/')[1] : rawName;
            const safeDisplayName = escapeText(displayName);
            const titleAttr = escapeAttribute(displayName);
            const descriptionText = (group.description || '').trim();
            const hasDescription = descriptionText.length > 0;
            const safeDescription = hasDescription ? escapeText(descriptionText) : '';
            const descriptionAttr = hasDescription ? escapeAttribute(descriptionText) : '';
            const groupSegments = getGroupPathSegmentsById(group.id);
            const groupHref = groupSegments.length ? `#/pipelineruns/main/${groupSegments.join('/')}` : '#/pipelineruns/main';

            if (isRepo) {
                const latestRun = state.repoLastRunCache ? state.repoLastRunCache.get(group.id) : null;
                let latestRunInfo = '';
                if (latestRun) {
                    const branchDisplay = formatBranchDisplay(latestRun.git_ref, latestRun.git_target_ref, { html: true });
                    const config = statusConfig[(latestRun.is_complete ? latestRun.status : 'running').toLowerCase()] || statusConfig.pending;
                    const timeToDisplay = latestRun.is_complete ? latestRun.finished_at : latestRun.started_at;
                    latestRunInfo = `
                        <div class="mt-4 pt-3 border-t border-[var(--border-primary)] text-xs text-[var(--text-secondary)] font-mono space-y-1.5">
                            <div class="flex items-center justify-between">
                                <div class="flex items-center">
                                    <svg class="h-4 w-4 mr-2 ${config.color}" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="${config.icon}"/></svg>
                                    <span class="font-semibold text-sm text-[var(--text-primary)] truncate">${branchDisplay}</span>
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
                    <a href="${groupHref}" draggable="true" class="relative group bg-[var(--bg-secondary)] p-4 rounded-md hover:bg-[var(--bg-tertiary)] transition-colors duration-200 group-card border border-[var(--border-primary)] hover:border-[var(--border-accent)] shadow-sm hover:shadow-lg flex flex-col justify-between" data-group-id="${group.id}">
                        <div>
                            <button class="delete-group-btn absolute top-2 right-2 text-[var(--text-secondary)] hover:text-red-500 opacity-0 group-hover:opacity-100 transition-opacity z-10" data-group-id="${group.id}" data-group-name="${escapeAttribute(group.name)}" type="button">
                                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                            </button>
                            <div class="flex items-center">
                                <svg class="h-8 w-8 text-[var(--text-accent)] mr-4" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                                    <circle cx="8" cy="7" r="2.2" fill="currentColor" />
                                    <circle cx="8" cy="17" r="2.2" fill="currentColor" />
                                    <circle cx="16" cy="7" r="2.2" fill="currentColor" />
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.2" d="M10.2 7h3.8M8 9v6a4 4 0 004 4h4" />
                                </svg>
                                <span class="text-lg font-medium text-[var(--text-primary)] truncate" title="${titleAttr}">${safeDisplayName}</span>
                            </div>
                        </div>
                        ${latestRunInfo}
                    </a>`;
                return;
            }

            const childGroups = childGroupMap.get(group.id) || [];
            let applicationCount = 0;
            let subfolderCount = 0;
            childGroups.forEach(child => {
                if ((child.name || '').includes('/')) {
                    applicationCount += 1;
                } else {
                    subfolderCount += 1;
                }
            });

            html += `
                <a href="${groupHref}" draggable="true" class="pipeline-folder-card border border-[var(--border-primary)]" data-group-id="${group.id}">
                    <div class="pipeline-folder-card-header">
                        <span class="pipeline-folder-icon">
                            <svg class="h-6 w-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                <path d="M3 7h5l2 2h9a2 2 0 012 2v7a2 2 0 01-2 2H3a2 2 0 01-2-2V9a2 2 0 012-2z" />
                            </svg>
                        </span>
                        <h3 class="pipeline-folder-title" title="${titleAttr}">${safeDisplayName}</h3>
                        <div class="pipeline-folder-actions">
                            <span class="pipeline-folder-chevron" aria-hidden="true">
                                <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                    <path d="M9 5l7 7-7 7" />
                                </svg>
                            </span>
                            <button class="pipelines-delete-button pipeline-folder-delete-btn delete-group-btn" data-group-id="${group.id}" data-group-name="${escapeAttribute(group.name)}" type="button" title="Delete folder">
                                <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                    <path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6" />
                                    <path d="M9 7V4a1 1 0 011-1h4a1 1 0 011 1v3" />
                                    <path d="M4 7h16" />
                                </svg>
                            </button>
                        </div>
                    </div>
                    ${hasDescription ? `<p class="pipeline-folder-description" title="${descriptionAttr}">${safeDescription}</p>` : ''}
                    <div class="pipeline-folder-meta">
                        <div class="pipeline-folder-meta-row">
                            <span class="pipeline-folder-meta-label">Applications:</span>
                            <span class="pipeline-folder-meta-value">${applicationCount}</span>
                        </div>
                        <div class="pipeline-folder-meta-row">
                            <span class="pipeline-folder-meta-label">Sub folders:</span>
                            <span class="pipeline-folder-meta-value">${subfolderCount}</span>
                        </div>
                    </div>
                </a>`;
        });

        (Array.isArray(visibleRuns) ? visibleRuns : []).forEach(run => {
            html += renderRunCard(run, contextForRuns, { viewMode: isListView ? 'list' : 'grid' });
        });

        html += '</div>';
        container.innerHTML = html;
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

        while (queue.length > 0) {
            const levelSize = queue.length;
            for (let i = 0; i < levelSize; i++) {
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
        } catch { }
        let totalWidth, totalHeight;

        if (isVertical) {
            let maxNodesInLevel = 0;
            levels.forEach(l => { if (l) maxNodesInLevel = Math.max(maxNodesInLevel, l.length); });
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
            levels.forEach(l => { if (l) maxNodesInLevel = Math.max(maxNodesInLevel, l.length); });
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
    function renderRunView(runDetails) {
        const currentHashInfo = parsePipelineRunsHash(window.location.hash);
        const expectedRunId = runDetails?.run_info?.run_id;

        // Only update the header IF we are currently on a pipelinerun page
        // AND the runId in the hash matches the runId we are trying to render.
        if (currentHashInfo.path !== 'pipelineruns' || currentHashInfo.runId !== expectedRunId) {
            console.warn("renderRunView aborted: View changed or Run ID mismatch.", {
                currentPath: currentHashInfo.path,
                currentHashRunId: currentHashInfo.runId,
                renderingRunId: expectedRunId
            });
            // IMPORTANT: Stop the function here to prevent overwriting the header
            return;
        }
        setRunToolbarVisibility(false);
        setNewFolderButtonEnabled(false);
        const runInfo = runDetails.run_info;
        clearSelectedRuns();
        hideRunViewToggle();
        const branchDisplay = formatBranchDisplay(runInfo.git_ref, runInfo.git_target_ref, { html: true });
        const triggerRaw = (runInfo.trigger_event_id ?? '').toString().trim();
        const triggerDisplay = triggerRaw || 'N/A';
        const repoFullName = runInfo.git_repo_owner ? `${runInfo.git_repo_owner} / ${runInfo.git_repo_name}` : runInfo.git_repo_name;
        const pipelineIdentifier = getPipelineIdentifierFromRun(runInfo);
        const pipelinePageLink = pipelineIdentifier ? buildPipelineHashFromRun(runInfo) : '';

        const runContext = resolveRunContext(state.currentRunContext || null);
        state.currentRunContext = runContext;

        const repoGroup = findGroupByName(`${runInfo.git_repo_owner}/${runInfo.git_repo_name}`);
        const repoSegments = repoGroup ? getGroupPathSegmentsById(repoGroup.id) : [];
        const repoLink = repoSegments.length ? `#/pipelineruns/main/${repoSegments.join('/')}` : '#/pipelineruns/main';

        let headerHTML = `<div class="w-full min-w-0">`;

        if (runDetails.parent_run_info) {
            const parentRunLink = buildRunHash({
                run_id: runDetails.parent_run_info.run_id,
                git_repo_owner: runInfo.git_repo_owner,
                git_repo_name: runInfo.git_repo_name,
            }, runContext);
            headerHTML += `
                <div class="mb-2">
                    <a href="${parentRunLink}" class="text-sm text-[var(--text-link)] hover:underline">
                        &larr; Back to parent: ${runDetails.parent_run_info.pipeline_name}
                    </a>
                </div>
            `;
        }

        const overrideIcon = runInfo.pipeline_source === 'database override'
            ? `<svg class="h-5 w-5 text-purple-500 ml-2" title="Overridden from database" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l4-4 4 4m0 6l-4 4-4-4"/></svg>`
            : '';
        const statusIconHtml = buildRunStatusIcon(runInfo);

        const normalizedStatus = (runInfo.status || '').trim().toLowerCase();
        const isCancelable = normalizedStatus === 'pending' || normalizedStatus === 'running';
        const primaryActionHTML = isCancelable
            ? `<button id="run-primary-action-btn" data-action="cancel" type="button" class="inline-flex items-center px-3 py-1.5 border border-transparent text-xs font-medium rounded-md shadow-sm text-white bg-red-600 hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500">
                <svg class="-ml-0.5 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
                Cancel Run
            </button>`
            : `<button id="run-primary-action-btn" data-action="rerun" type="button" class="inline-flex items-center px-3 py-1.5 border border-transparent text-xs font-medium rounded-md shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">
                <svg class="-ml-0.5 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16.023 9.348h4.992m0 0V4.356m0 4.992L18 7.5M7.977 14.652H2.985m0 0v4.992m0-4.992L6 16.5M4.5 12a7.5 7.5 0 0112.69-5.31M19.5 12a7.5 7.5 0 01-12.69 5.31" />
                </svg>
                Rerun
            </button>`;

        const pipelineActionHTML = pipelineIdentifier
            ? `<a id="run-view-pipeline-link" data-pipeline-id="${escapeAttribute(pipelineIdentifier)}" data-pipeline-href="${escapeAttribute(pipelinePageLink)}" data-active-title="View this pipeline in Pipelines tab" class="${PIPELINE_BUTTON_DISABLED_CLASSES}" aria-disabled="true" tabindex="-1" title="Checking pipeline availability…">
                <svg class="-ml-0.5 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" /></svg>
                View Pipeline
            </a>`
            : '';

        const deleteRunIconHTML = `<button id="run-delete-btn" type="button" class="inline-flex items-center justify-center h-8 w-8 rounded-full text-[var(--text-secondary)] hover:text-red-500 hover:bg-[var(--border-primary)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-500" title="Delete run">
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0 1 16.138 21H7.862a2 2 0 0 1-1.995-1.858L5 7m5-3h4m1 3H7" /></svg>
            </button>`;

        headerHTML += `
            <div class="flex flex-wrap items-baseline gap-x-3 min-w-0">
                <a href="${repoLink}" class="text-xl font-semibold text-[var(--text-secondary)] hover:text-[var(--text-accent)] transition-colors truncate">${repoFullName}</a>
                <span class="flex items-center gap-2 text-xl font-semibold text-[var(--text-primary)] truncate">
                    ${statusIconHtml}
                    <span class="truncate">${runInfo.pipeline_name}</span>
                </span>
                ${overrideIcon}
            </div>
            <div class="text-xs text-[var(--text-secondary)] mt-2 font-mono grid grid-cols-[auto,1fr] gap-x-4 w-full max-w-3xl">
                <span class="text-gray-500 justify-self-end truncate">Run ID:</span>
                <span class="truncate">${runInfo.run_id}</span>
                <span class="text-gray-500 justify-self-end truncate">Commit:</span>
                <span class="truncate">${runInfo.git_commit_sha}</span>
                <span class="text-gray-500 justify-self-end truncate">Trigger Event:</span>
                <span class="break-all" title="${escapeAttribute(triggerDisplay)}">${escapeText(triggerDisplay)}</span>
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
                    <span>${branchDisplay}</span>
                </div>
                <div class="ml-auto flex items-center gap-2">
                    ${pipelineActionHTML}
                    ${primaryActionHTML}
                    <a href="${buildRunHashWithExtras(runInfo, runContext, ['logs'])}" class="inline-flex items-center px-3 py-1.5 border border-transparent text-xs font-medium rounded-md shadow-sm text-[var(--text-primary)] bg-[var(--bg-tertiary)] hover:bg-[var(--border-primary)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)]">
                        <svg class="-ml-0.5 mr-2 h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" /></svg>
                        View Logs
                    </a>
                    ${deleteRunIconHTML}
                </div>
            </div>
        </div>`;

        if (DOM.mainHeader) {
            DOM.mainHeader.innerHTML = headerHTML;
        } else {
            console.error("DOM.mainHeader not found during renderRunView");
            return; // Avoid errors if header element isn't there
        }

        if (pipelineIdentifier) {
            updatePipelineButtonState(pipelineIdentifier);
        }

        const actionBtn = document.getElementById('run-primary-action-btn');
        if (actionBtn && runInfo?.run_id) {
            const setLoadingState = (label) => {
                actionBtn.dataset.originalHtml = actionBtn.innerHTML;
                actionBtn.disabled = true;
                actionBtn.classList.add('opacity-70', 'cursor-not-allowed');
                actionBtn.innerHTML = label;
            };
            const restoreState = () => {
                if (!document.body.contains(actionBtn)) return;
                const originalHTML = actionBtn.dataset.originalHtml;
                if (originalHTML) {
                    actionBtn.innerHTML = originalHTML;
                    delete actionBtn.dataset.originalHtml;
                }
                actionBtn.disabled = false;
                actionBtn.classList.remove('opacity-70', 'cursor-not-allowed');
            };
            const actionType = actionBtn.dataset.action;
            if (actionType === 'cancel') {
                actionBtn.addEventListener('click', async (e) => {
                    e.preventDefault();
                    if (actionBtn.disabled) return;
                    if (!window.confirm('Cancel this pipeline run?')) {
                        return;
                    }
                    setLoadingState('Cancelling…');
                    try {
                        await fetchData(`/v1/runs/${runInfo.run_id}/cancel`, { method: 'POST' });
                        await fetchActiveRun(runInfo.run_id, true);
                    } finally {
                        restoreState();
                    }
                });
            } else if (actionType === 'rerun') {
                actionBtn.addEventListener('click', async (e) => {
                    e.preventDefault();
                    if (actionBtn.disabled) return;

                    if (!window.confirm('Rerun this pipeline run?')) {
                        return;
                    }

                    setLoadingState('Rerunning…');

                    try {
                        const result = await fetchData(`/v1/runs/${runInfo.run_id}/rerun`, { method: 'POST' });
                        if (!result) return;

                        let newRunId = null;
                        let newTriggerId = null;
                        if (typeof result === 'string') {
                            const match = result.match(/[0-9a-fA-F-]{36}/);
                            if (match) {
                                newRunId = match[0];
                            } else {
                                alert(result);
                            }
                        } else if (typeof result === 'object' && result !== null) {
                            if (result.runId) {
                                newRunId = result.runId;
                            }
                            if (result.triggerEventId) {
                                newTriggerId = result.triggerEventId;
                            }
                            if (!newRunId && typeof result.message === 'string') {
                                alert(result.message);
                            }
                        }

                        if (newRunId) {
                            const context = resolveRunContext(state.currentRunContext || null);
                            const runForHash = { ...runInfo, run_id: newRunId, trigger_event_id: newTriggerId || newRunId };
                            window.location.hash = buildRunHash(runForHash, context);
                        }
                    } finally {
                        restoreState();
                    }
                });
            }
        }

        const deleteRunBtn = document.getElementById('run-delete-btn');
        if (deleteRunBtn && runInfo?.run_id) {
            deleteRunBtn.addEventListener('click', async (e) => {
                e.preventDefault();
                if (deleteRunBtn.disabled) return;
                deleteRunBtn.disabled = true;
                deleteRunBtn.classList.add('opacity-70', 'cursor-not-allowed');
                try {
                    await deleteRunById(runInfo.run_id, state.currentRunContext);
                } finally {
                    deleteRunBtn.disabled = false;
                    deleteRunBtn.classList.remove('opacity-70', 'cursor-not-allowed');
                }
            });
        }

        const isSameRun = state.stepsGraphRenderedRunId === runInfo.run_id;
        if (isSameRun && state.currentGraphView === 'steps') {
            const success = updateStepsGraphStatuses(runDetails);
            if (success) {
                if (pipelineIdentifier) updatePipelineButtonState(pipelineIdentifier);
                return;
            }
        }

        state.stepsGraphRenderedRunId = runInfo.run_id;
        resetMainView();
        try { localStorage.setItem('graphView', 'steps'); } catch { }
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
                'display_options', 'variables', 'secrets', 'volumes', 'goal', 'script',
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

        if (typeof pipelineDefinition === 'string') {
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
                        if (Array.isArray(value)) {
                            yamlString += `${spaces}${key}:\n`;
                            value.forEach(item => {
                                if (typeof item === 'object' && item !== null) {
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
        if (typeof pipelineDefinition === 'string') {
            pipelineYAML = pipelineDefinition;
        } else if (typeof pipelineDefinition === 'object' && pipelineDefinition !== null) {
            pipelineYAML = toYAML(pipelineDefinition);
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
        const runInfo = runDetails?.run_info || {};
        const runContext = resolveRunContext(state.currentRunContext || null);

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
        const baseH = isVerticalLayout ? 80 : 100;
        // tighter horizontal spacing to keep graph compact when many steps are expanded
        const baseHG = isVerticalLayout ? 40 : 90;   // was 120 for horizontal
        const baseVG = isVerticalLayout ? 100 : 16;   // was 20 for horizontal
        const nodeWidth = Math.round(baseW * scale);
        const nodeHeight = Math.round(baseH * scale);
        const hGap = Math.round(baseHG * scale);
        const vGap = Math.round(baseVG * scale);
        const { nodes, edges, width, height } = calculateGraphLayout(runDetails.steps, DOM.graphWrapper, nodeWidth, nodeHeight, hGap, vGap, isVerticalLayout);
        const pipelineSteps = getPipelineSteps(runDetails.pipeline_definition);

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
            x2: e.to.x + e.to.width / 2,
            y2: e.to.y + e.to.height / 2,
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

            const stepDef = pipelineSteps.find(s => s.name === stepNode.name);
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
            const tNodeH = Math.round((isVerticalLayout ? 70 : 72) * clusterScale);
            const tHG = Math.max(32, Math.round((isVerticalLayout ? 32 : 48) * clusterScale));
            const tVG = Math.max((isVerticalLayout ? 80 : 24), Math.round((isVerticalLayout ? 80 : 24) * clusterScale));
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
            let originX = Math.round(stepCenterX - (tLayout.width / 2));
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
                <marker id="task_arrow" viewBox="0 0 10 10" refX="9" refY="5"
                        markerWidth="8" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
                  <path d="M0,0 L10,5 L0,10 Q2.8,5 0,0 Z" class="fill-current text-gray-400 dark:text-gray-600" />
                </marker>
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
            const toBox = clustersByStep.get(edge.to.name);
            const isCompletedEdge = edge.from.status === 'Success' && (edge.to.status.toLowerCase() !== 'pending' && edge.to.status.toLowerCase() !== 'skipped');
            const marker = isCompletedEdge ? 'url(#arrowhead-completed)' : 'url(#arrowhead)';
            const edgeClasses = ['edge-path', 'edge-path--glow'];
            if (isCompletedEdge) edgeClasses.push('edge-path--completed');
            if (isRunning && isCompletedEdge) edgeClasses.push('edge-path--running');
            const fromAttr = escapeAttribute(edge.from.name);
            const toAttr = escapeAttribute(edge.to.name);

            const fromCx = edge.from.x + edge.from.width / 2;
            const fromCy = edge.from.y + edge.from.height / 2;
            const toCx = edge.to.x + edge.to.width / 2;
            const toCy = edge.to.y + edge.to.height / 2;

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
            svgEdges += `<path d="${d}" class="edge-path-halo" data-from-step="${fromAttr}" data-to-step="${toAttr}"></path>`;
            svgEdges += `<path d="${d}" class="${edgeClasses.join(' ')}" marker-end="${marker}" data-from-step="${fromAttr}" data-to-step="${toAttr}"></path>`;
        });

        // Step nodes
        nodes.forEach(node => {
            const config = statusConfig[node.status.toLowerCase()] || statusConfig.pending;
            const node_center_x = node.x + node.width / 2;
            const node_center_y = node.y + node.height / 2;
            const originalStep = pipelineSteps.find(s => s.name === node.name);
            const isExpanded = clustersByStep.has(node.name);
            if (isExpanded) return; // replaced by a box
            let subText = `<text x="${node_center_x}" y="${node_center_y + 53}" text-anchor="middle" class="text-xs fill-current text-[var(--text-secondary)]">${node.duration || '...'}</text>`;
            if (originalStep && originalStep.include) {
                let includeType = originalStep.include.startsWith('pipeline:') ? '(Included Pipeline)' : '(Included Step)';
                let linkClass = originalStep.include.startsWith('pipeline:') ? 'text-[var(--text-link)] hover:underline' : 'text-[var(--text-accent)]';
                const childRun = originalStep.include.startsWith('pipeline:') && Array.isArray(runDetails.child_runs) ? runDetails.child_runs.find(cr => cr.parent_step_name === node.name) : null;
                const yInclude = node_center_y + 53;
                const yDuration = node_center_y + 67;
                if (childRun) {
                    subText = `
                      <a data-included-link="true" href="${buildRunHash({
                        run_id: childRun.run_id,
                        git_repo_owner: runInfo.git_repo_owner,
                        git_repo_name: runInfo.git_repo_name,
                    }, runContext)}" class="fill-current ${linkClass}">
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
                const to_center_x = originX + edge.to.x + edge.to.width / 2;
                const to_center_y = originY + edge.to.y + edge.to.height / 2;
                const arrowPad = 10;
                let pathData;
                if (isVerticalLayout) {
                    const x1 = from_center_x;
                    const y1 = from_center_y + (edge.from.height / 2) + arrowPad;
                    const x2 = to_center_x;
                    const y2 = to_center_y - (edge.to.height / 2) - arrowPad;
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
                const taskAttr = escapeAttribute(itemName);
                const stepAttr = escapeAttribute(cluster.stepName || '');
                svgClusters += `
                    <g class=\"graph-node\" data-task-name=\"${taskAttr}\" data-step-name=\"${stepAttr}\" transform=\"translate(${cx}, ${cy})\"> 
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
            (function () {
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
        try { updateExpandToggleLabel(); } catch { }

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
        } catch { }

        initPanAndZoom('steps', { width: finalWidth, height: finalHeight });
        // Clear one-shot preserve flag after render
        if (state._preserveScale) delete state._preserveScale;
        // In case this ran while hidden, ensure a binding once visible
        requestAnimationFrame(() => ensurePanzoomBound('steps'));
        try { updateExpandToggleLabel(); } catch { }
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
            const startWidth = parseInt(document.defaultView.getComputedStyle(card).width, 10);
            const startHeight = parseInt(document.defaultView.getComputedStyle(card).height, 10);

            function doDrag(ev) {
                card.style.width = (startWidth + ev.clientX - startX) + 'px';
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
        if (!state.currentRunData) {
            container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">Waiting for pipeline data...</p>`;
            return;
        }
        const pipelineSteps = getPipelineSteps(state.currentRunData.pipeline_definition);
        if (pipelineSteps.length === 0) {
            container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">Waiting for pipeline data...</p>`;
            return;
        }
        if (!tasks || tasks.length === 0) {
            container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">This step has no tasks defined.</p>`;
            return;
        }

        container.dataset.stepName = stepName || '';
        bindTaskGraphLogging(container);

        const stepDef = pipelineSteps.find(s => s.name === stepName);
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
        const nodeWidth = isVerticalLayout ? 84 : 120;
        const nodeHeight = isVerticalLayout ? 48 : 72;
        const hGap = isVerticalLayout ? 24 : 60;
        const vGap = isVerticalLayout ? 64 : 28;

        const { nodes, edges, width, height } =
            calculateGraphLayout(itemsWithDeps, container, nodeWidth, nodeHeight, hGap, vGap, isVerticalLayout);

        const arrowPad = 12;
        const getEdgePath = (fromNode, toNode) => {
            const fc = { x: fromNode.x + fromNode.width / 2, y: fromNode.y + fromNode.height / 2 };
            const tc = { x: toNode.x + toNode.width / 2, y: toNode.y + toNode.height / 2 };
            if (isVerticalLayout) {
                const x1 = fc.x, y1 = fc.y + (fromNode.height / 2) + arrowPad;
                const x2 = tc.x, y2 = tc.y - (toNode.height / 2) - arrowPad;
                const curveY = y1 + (y2 - y1) * 0.5;
                return `M ${x1} ${y1} C ${x1} ${curveY}, ${x2} ${curveY}, ${x2} ${y2}`;
            } else {
                const x1 = fc.x + (fromNode.width / 2) + arrowPad, y1 = fc.y;
                const x2 = tc.x - (toNode.width / 2) - arrowPad, y2 = tc.y;
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
            nodeNames.has(e.to.task_name || e.to.name)
        );
        const connectors = filteredEdges.length ? filteredEdges : (
            nodes.length > 1
                ? nodes.slice(0, -1).map((_, i) => ({ from: nodes[i], to: nodes[i + 1] }))
                : []
        );
        edges.forEach(e => {
            // drop edges whose endpoints aren’t in the layout for any reason
            if (!nodeNames.has(e.from.task_name || e.from.name) ||
                !nodeNames.has(e.to.task_name || e.to.name)) {
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
            const taskAttr = escapeAttribute(itemName);
            const stepAttr = escapeAttribute(stepName || '');

            svg += `
  <g class="graph-node" data-task-name="${taskAttr}" data-step-name="${stepAttr}" transform="translate(${x}, ${y})">
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
        DOM.tasksGraphWrapper.style.width = `${stepLayout.width}px`;
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
                const context = resolveRunContext(state.currentRunContext || null);
                const newHash = buildRunHashWithExtras(runDetails.run_info, context, ['steps', step.name]);
                window.location.hash = newHash;
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
        DOM.tasksGraphWrapper.style.width = `${stepLayout.width}px`;
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
        const info = parsePipelineRunsHash(window.location.hash);
        if (!info.runId) return;
        const runInfo = state.currentRunData ? state.currentRunData.run_info : { run_id: info.runId };
        const context = resolveRunContext(state.currentRunContext || { tab: info.tab, groupSegments: info.groupSegments, groupId: state.selectedGroupId });
        const newHash = buildRunHash(runInfo, context);
        const currentHash = window.location.hash || '';
        if (currentHash === newHash) return;
        try {
            const url = new URL(window.location.href);
            url.hash = newHash.slice(1);
            history.replaceState(null, '', url.toString());
        } catch {
            window.location.hash = newHash;
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
                                if (v.length === 6) { const a = v[0], b = v[1]; scale = Math.sqrt(a * a + b * b) || 1; x = v[4] || 0; y = v[5] || 0; }
                            }
                        }
                        state._stepsViewTransform = { x, y, scale };
                    } catch { }
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
                    } catch { }
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
                                } catch { }
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
        try { updateExpandToggleLabel(); } catch { }
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
            const path = `M${edge.from.x + edge.from.width / 2},${edge.from.y + edge.from.height / 2} L${edge.to.x + edge.to.width / 2},${edge.to.y + edge.to.height / 2}`;
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
            const toEl = stepElements.get(edge.to.name);
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

        // Defensive check for steps array
        const step = (runDetails.steps || []).find(s => s.name === stepName);
        if (!step) return;

        const pipelineSteps = getPipelineSteps(runDetails.pipeline_definition);
        const stepDef = (pipelineSteps || []).find(s => s.name === stepName) || null;

        const config = statusConfig[step.status.toLowerCase()] || statusConfig.pending;
        const safeStepAttr = escapeAttribute(stepName);
        const modalHeader = document.querySelector('#modal-content > div:first-child');
        const closeButtonHTML = modalHeader.querySelector('#close-modal-btn').outerHTML;
        const escapeHtml = (value) => {
            if (value === undefined || value === null) return '';
            return String(value)
                .replace(/&/g, '&amp;')
                .replace(/</g, '&lt;')
                .replace(/>/g, '&gt;')
                .replace(/"/g, '&quot;')
                .replace(/'/g, '&#39;');
        };
        const addConfigRow = (label, valueHtml) => {
            if (!valueHtml) return;
            configHTML += `<div class="step-config-row"><span class="step-config-label">${label}:</span><span class="step-config-value">${valueHtml}</span></div>`;
        };
        const boolBadge = (value) => {
            if (value === undefined || value === null) return '';
            return `<span class="step-config-bool">${value ? 'true' : 'false'}</span>`;
        };
        let headerHTML = `
            <div>
                <h2 id="modal-title" class="text-xl font-semibold">Step: ${stepName}</h2>
                <div class="flex items-center space-x-6 mt-1 text-sm">
                    <div><span class="text-[var(--text-secondary)]">Status: </span><span id="modal-status" class="font-medium ${config.color}">${step.status}</span></div>
                    <div><span class="text-[var(--text-secondary)]">Duration: </span><span id="modal-duration" class="font-medium">${step.duration || '0s'}</span></div>
                    <button id="step-modal-open-logs-btn" data-step-name="${safeStepAttr}" class="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-md border border-[var(--border-primary)] text-[var(--text-primary)] bg-[var(--bg-tertiary)] hover:bg-[var(--border-primary)] focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-[var(--border-accent)]">
                        <svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h7" /></svg>
                        View Logs
                    </button>
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

        if (stepDef?.image) {
            addConfigRow('Image', escapeHtml(stepDef.image));
        }
        if (stepDef?.include) {
            addConfigRow('Include', escapeHtml(stepDef.include));
        }
        if (stepDef?.depends_on && stepDef.depends_on.length > 0) {
            const dependsOnList = stepDef.depends_on.map(d => `<li>${escapeHtml(d)}</li>`).join('');
            configHTML += `<div class="step-config-list"><h4>Depends On</h4><ul>${dependsOnList}</ul></div>`;
        }
        if (stepDef?.secrets && stepDef.secrets.length > 0) {
            const secretsList = stepDef.secrets.map(s => `<li>${escapeHtml(s)}</li>`).join('');
            configHTML += `<div class="step-config-list"><h4>Secrets</h4><ul>${secretsList}</ul></div>`;
        }
        if (stepDef?.volumes && stepDef.volumes.length > 0) {
            const volumesList = stepDef.volumes.map(v => `<li>${escapeHtml(v)}</li>`).join('');
            configHTML += `<div class="step-config-list"><h4>Volumes</h4><ul>${volumesList}</ul></div>`;
        }
        if (stepDef?.variables && Object.keys(stepDef.variables).length > 0) {
            const envList = Object.entries(stepDef.variables).map(([k, v]) => `<li class="step-config-row"><span class="step-config-label">${escapeHtml(k)}:</span><span class="step-config-value">${escapeHtml(v)}</span></li>`).join('');
            configHTML += `<div class="step-config-list"><h4>Variables</h4><ul>${envList}</ul></div>`;
        }
        addConfigRow('Ignore Failure', boolBadge(!!stepDef?.ignore_failure));
        addConfigRow('Sync', boolBadge(!!step.sync));
        if (stepDef?.llm_output_sharing !== undefined) {
            addConfigRow('LLM Output Sharing', boolBadge(!!stepDef.llm_output_sharing));
        }
        if (stepDef?.llm_content_sharing !== undefined) {
            addConfigRow('LLM Content Sharing', boolBadge(!!stepDef.llm_content_sharing));
        }

        if (stepDef?.tasks && stepDef.tasks.length > 0) {
            configHTML += `<div class="step-config-list"><h4>Tasks</h4><div class="space-y-3 pt-2">`;
            stepDef.tasks.forEach(task => {
                configHTML += `<div class="bg-[var(--bg-primary)] p-3 rounded-md border-l-2 border-[var(--border-secondary)] space-y-3">
                                <p class="font-semibold text-[var(--text-primary)]">${escapeHtml(task.name)}</p>`;
                if (task.goal) {
                    configHTML += `<div class="step-task-row"><span class="step-config-label">Goal:</span><span class="step-config-value italic">"${escapeHtml(task.goal)}"</span></div>`;
                }
                if (task.script) {
                    const escapedScript = task.script.replace(/</g, '&lt;').replace(/>/g, '&gt;');
                    configHTML += `<div class="step-config-list"><h5>Script</h5><pre class="bg-[var(--bg-code-darker)] p-2 rounded text-cyan-700 dark:text-cyan-300 text-xs overflow-x-auto"><code>${escapedScript}</code></pre></div>`;
                }
                if (task.depends_on && task.depends_on.length > 0) {
                    const dependsOnList = task.depends_on.map(d => `<li>${escapeHtml(d)}</li>`).join('');
                    configHTML += `<div class="step-config-list"><h5>Depends On</h5><ul>${dependsOnList}</ul></div>`;
                }
                if (task.ignore_failure) {
                    configHTML += `<div class="step-task-row"><span class="step-config-label">Ignore Failure:</span><span class="step-config-value">${boolBadge(true)}</span></div>`;
                }
                if (task.llm_output_sharing === false) {
                    configHTML += `<div class="step-task-row"><span class="step-config-label">LLM Output Sharing:</span><span class="step-config-value">${boolBadge(false)}</span></div>`;
                }
                if (task.variables && Object.keys(task.variables).length > 0) {
                    const taskVars = Object.entries(task.variables).map(([k, v]) => `<li class="step-config-row"><span class="step-config-label">${escapeHtml(k)}:</span><span class="step-config-value">${escapeHtml(v)}</span></li>`).join('');
                    configHTML += `<div class="step-config-list"><h5>Variables</h5><ul>${taskVars}</ul></div>`;
                }
                configHTML += '</div>';
            });
            configHTML += '</div></div>';
        }
        configContainer.innerHTML = configHTML;

        const taskGraphEl = document.getElementById('task-graph');
        const showEmptyTaskGraph = (message) => {
            if (!taskGraphEl) return;
            taskGraphEl.dataset.stepName = stepName || '';
            taskGraphEl.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">${message}</p>`;
        };

        if (stepDef?.include && stepDef.include.startsWith('pipeline:')) {
            const childRun = runDetails.child_runs.find(cr => cr.parent_step_name === stepName);
            if (childRun) {
                try {
                    const childRunDetails = await fetchData(`/v1/runs/${childRun.run_id}`);
                    if (childRunDetails && Array.isArray(childRunDetails.steps) && childRunDetails.steps.length > 0) {
                        renderTaskGraph(taskGraphEl, stepName, childRunDetails.steps, { runId: childRun.run_id, parentRunId: runId, parentStepName: stepName });
                    } else {
                        showEmptyTaskGraph('This included pipeline has no tasks to display.');
                    }
                } catch (error) {
                    console.error('Failed to load child run tasks graph', error);
                    showEmptyTaskGraph('Unable to load tasks for this included pipeline.');
                }
            } else {
                showEmptyTaskGraph('This included pipeline was skipped, so no tasks ran.');
            }
        } else {
            renderTaskGraph(taskGraphEl, stepName, step.tasks);
        }
    }

    function renderTaskGraph(container, stepName, tasks, clickableNodeContext = null) {
        if (!state.currentRunData) {
            container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">Waiting for pipeline data...</p>`;
            return;
        }
        const pipelineSteps = getPipelineSteps(state.currentRunData.pipeline_definition);
        if (pipelineSteps.length === 0) {
            container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">Waiting for pipeline data...</p>`;
            return;
        }
        if (!tasks || tasks.length === 0) {
            container.innerHTML = `<p class="text-[var(--text-secondary)] text-sm">This step has no tasks defined.</p>`;
            return;
        }

        container.dataset.stepName = stepName || '';
        bindTaskGraphLogging(container);

        const stepDef = pipelineSteps.find(s => s.name === stepName);
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
        const nodeWidth = isVerticalLayout ? (isMini ? 84 : 184) : (isMini ? 120 : 136);
        const nodeHeight = isVerticalLayout ? (isMini ? 48 : 92) : (isMini ? 72 : 112);
        const hGap = isVerticalLayout ? (isMini ? 24 : 44) : (isMini ? 60 : 100);
        const vGap = isVerticalLayout ? (isMini ? 64 : 120) : (isMini ? 28 : 36);

        const { nodes, edges, width, height } =
            calculateGraphLayout(itemsWithDeps, container, nodeWidth, nodeHeight, hGap, vGap, isVerticalLayout);

        // keep arrows clear of node icons
        const iconRadius = 14;
        const arrowPad = 12; // extra gap so the tip doesn’t sit on the icon

        const getEdgePath = (fromNode, toNode) => {
            const from_center_x = fromNode.x + fromNode.width / 2;
            const from_center_y = fromNode.y + fromNode.height / 2;
            const to_center_x = toNode.x + toNode.width / 2;
            const to_center_y = toNode.y + toNode.height / 2;

            if (isVerticalLayout) {
                const x1 = from_center_x;
                const y1 = from_center_y + iconRadius + arrowPad;
                const x2 = to_center_x;
                const y2 = to_center_y - iconRadius - arrowPad;
                const curveY = y1 + (y2 - y1) * 0.5;
                return `M ${x1} ${y1} C ${x1} ${curveY}, ${x2} ${curveY}, ${x2} ${y2}`;
            } else {
                const x1 = from_center_x + iconRadius + arrowPad;
                const y1 = from_center_y;
                const x2 = to_center_x - iconRadius - arrowPad;
                const y2 = to_center_y;
                const curveX = x1 + (x2 - x1) * 0.5;
                return `M ${x1} ${y1} C ${curveX} ${y1}, ${curveX} ${y2}, ${x2} ${y2}`;
            }
        };

        let svgContent = `<svg width="${width}" height="${height}" xmlns="http://www.w3.org/2000/svg">
<defs>
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

            const taskAttr = escapeAttribute(itemName);
            const stepAttr = escapeAttribute(stepName || '');

            svgContent += `
  <g transform="translate(0,0)" ${clickableAttrs} data-task-name="${taskAttr}" data-step-name="${stepAttr}">
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


    function updateStepsGraphStatuses(runDetails) {
        if (!DOM.graphWrapper || !runDetails?.steps) return false;

        const renderedRunId = state.stepsGraphRenderedRunId || null;
        const incomingRunId = runDetails.run_info?.run_id || null;
        if (renderedRunId && incomingRunId && renderedRunId !== incomingRunId) {
            return false;
        }

        const stepStatus = new Map();
        runDetails.steps.forEach(s => stepStatus.set(s.name, (s.status || 'pending').toLowerCase()));

        const formatItemDuration = (item) => {
            if (!item) return '...';
            if (item.duration) return item.duration;
            return formatDuration(item.started_at, item.finished_at) || '...';
        };

        for (const step of runDetails.steps) {
            const safeStep = escapeForSelector(step.name);
            const clusterEl = DOM.graphWrapper.querySelector(`.step-cluster[data-step-name="${safeStep}"]`);
            const stepNode = DOM.graphWrapper.querySelector(`.graph-node[data-step-name="${safeStep}"]:not([data-task-name])`);

            // If neither a cluster nor a node exists, structure changed — force re-render
            if (!clusterEl && !stepNode) return false;

            // Update expanded task clusters in-place
            if (clusterEl) {
                const tasks = Array.isArray(step.tasks) ? step.tasks : [];
                const taskMap = new Map();
                tasks.forEach(t => {
                    const key = t.task_name || t.name;
                    if (key) taskMap.set(key, t);
                });
                const taskNodes = clusterEl.querySelectorAll('g.graph-node[data-task-name]');
                if (taskNodes.length !== taskMap.size) return false; // structural change

                for (const node of taskNodes) {
                    const taskName = node.dataset.taskName;
                    const task = taskMap.get(taskName);
                    if (!task) return false;
                    const statusKey = (task.status || 'pending').toLowerCase();
                    const config = statusConfig[statusKey] || statusConfig.pending;
                    const path = node.querySelector('path');
                    if (path) {
                        path.setAttribute('class', `stroke-current ${config.color}`);
                        path.setAttribute('d', config.icon);
                    }
                    const texts = node.querySelectorAll('text');
                    if (texts.length > 0) {
                        texts[texts.length - 1].textContent = formatItemDuration(task);
                    }
                }
                continue;
            }

            // Update collapsed step node
            const status = stepStatus.get(step.name) || 'pending';
            const config = statusConfig[status] || statusConfig.pending;
            const path = stepNode.querySelector('path');
            if (path) {
                path.setAttribute('class', `stroke-current ${config.color}`);
                path.setAttribute('d', config.icon); // Update icon shape!
            }

            const texts = stepNode.querySelectorAll('text');
            if (texts.length > 0) {
                texts[texts.length - 1].textContent = formatItemDuration(step);
            }
        }

        // Update edge styling without rebuilding the SVG (only applies to step-level edges)
        const edgePaths = DOM.graphWrapper.querySelectorAll('.edge-path[data-from-step]');
        if (edgePaths.length) {
            const isRunStillRunning = !runDetails.run_info?.is_complete;
            const pendingOrSkipped = new Set(['pending', 'skipped']);
            edgePaths.forEach(path => {
                if (path.classList.contains('edge-path-halo')) return; // halos don't need status changes
                const from = path.getAttribute('data-from-step');
                const to = path.getAttribute('data-to-step');
                const fromStatus = stepStatus.get(from) || 'pending';
                const toStatus = stepStatus.get(to) || 'pending';
                const completedEdge = fromStatus === 'success' && !pendingOrSkipped.has(toStatus);
                path.classList.toggle('edge-path--completed', completedEdge);
                path.classList.toggle('edge-path--running', completedEdge && isRunStillRunning);
                path.setAttribute('marker-end', completedEdge ? 'url(#arrowhead-completed)' : 'url(#arrowhead)');
            });
        }

        return true;
    }

    function renderBreadcrumbs(groupId) {
        // Always set the header to "Pipeline Runs" regardless of the group ID
        if (DOM.mainHeader) {
            DOM.mainHeader.textContent = 'Pipeline Runs';
        }
    }

    async function handleRoute(hashOverride) {
        const hash = hashOverride || window.location.hash || '#/pipelineruns/main';
        const info = parsePipelineRunsHash(hash);
        const { path, tab, groupSegments, runId, action, stepName, logSegments, query } = info;

        if (path !== 'pipelineruns') {
            clearSelectedRuns({ silent: true });
            updateSelectionBar();
        }

        if (!runId) {
            state.currentRunData = null;
            state.currentRunContext = null;
            state.expandedSteps = new Set();
            state.expandedStepPositions = new Map();
            if (state.currentRunTrackedIds instanceof Map) {
                state.currentRunTrackedIds.clear();
            } else {
                state.currentRunTrackedIds = new Map();
            }
        }

        resetMainView();

        DOM.pages.forEach(p => p.classList.toggle('active', p.dataset.page === path));

        if (path === 'pipelineruns') {
            await fetchGroups();

            state.currentTab = (tab === 'recent' || tab === 'main') ? tab : 'main';
            updateTabs(state.currentTab);
            startPipelineRunsPolling();

            let selectedGroupId = null;
            if (state.currentTab === 'main' && groupSegments.length) {
                let group = findGroupByPathSegments(groupSegments);
                if (!group) {
                    const maybeId = parseInt(groupSegments[groupSegments.length - 1], 10);
                    if (!Number.isNaN(maybeId)) {
                        const fallbackGroup = getGroupById(maybeId);
                        if (fallbackGroup) {
                            const canonicalSegments = getGroupPathSegmentsById(fallbackGroup.id);
                            const newHash = canonicalSegments.length ? `#/pipelineruns/main/${canonicalSegments.join('/')}` : '#/pipelineruns/main';
                            try {
                                history.replaceState(null, '', newHash);
                            } catch {
                                window.location.hash = newHash;
                                return;
                            }
                            groupSegments.splice(0, groupSegments.length, ...canonicalSegments);
                            group = fallbackGroup;
                        }
                    }
                }
                if (!group) {
                    window.location.hash = '#/pipelineruns/main';
                    return;
                }
                selectedGroupId = group.id;
                groupSegments.splice(0, groupSegments.length, ...getGroupPathSegmentsById(group.id));
            }

            state.selectedGroupId = selectedGroupId;
            state.selectedGroupPathSegments = state.currentTab === 'main' ? groupSegments.slice() : [];
            if (selectedGroupId) {
                ensureGroupAncestorsExpanded(selectedGroupId);
            }

            await renderSidebar(path, state.currentTab);

            if (runId) {
                const shouldFetch = !state.currentRunData || state.currentRunData.run_info.run_id !== runId;
                if (shouldFetch) {
                    await fetchActiveRun(runId);
                }

                state.currentRunContext = {
                    tab: state.currentTab,
                    groupSegments: state.selectedGroupPathSegments.slice(),
                    groupId: selectedGroupId || null,
                };

                if (state.currentRunData) {
                    if (action === 'logs') {
                        applyLogRouteState(logSegments, query);
                        renderRunView(state.currentRunData);
                        showLogsModal();
                    } else {
                        renderRunView(state.currentRunData);
                        if (action === 'steps' && stepName) {
                            showStepDetails(stepName);
                        }
                    }
                }
            } else if (state.currentTab === 'recent') {
                state.selectedGroupId = null;
                state.selectedGroupPathSegments = [];
                state.currentRunContext = null;
                DOM.mainHeader.textContent = 'Recent Pipeline Runs';
                const runs = await fetchData('/v1/runs');
                if (Array.isArray(runs)) {
                    state.recentRuns = runs;
                } else {
                    state.recentRuns = [];
                }
                renderMainGridContent(null, state.recentRuns, false);
                renderSidebarPipelineRunsList(state.recentRuns);
            } else { // main tab without specific run
                state.currentRunContext = null;
                const groupId = selectedGroupId;
                renderBreadcrumbs(groupId);
                if (groupId) {
                    await fetchMainContent(groupId);
                } else {
                    const rootGroups = state.groups.filter(g => normalizeParentId(g.parent_id) === null);
                    state.currentRepoRunsByBranch = null;
                    state.currentRepoGroupId = null;
                    renderMainGridContent(rootGroups, null, true);
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
        if (!DOM.tabs) return;
        DOM.tabs.forEach(tab => {
            const isActive = (tab.dataset.tab === activeTab);
            tab.classList.toggle('tabs-nav__link--active', isActive);
            tab.setAttribute('aria-selected', isActive ? 'true' : 'false');
            tab.setAttribute('tabindex', isActive ? '0' : '-1');
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
        const description = (formData.get('description') || '').trim();

        const siblings = state.groups.filter(g => normalizeParentId(g.parent_id) === normalizeParentId(parentId));
        if (siblings.some(s => s.name === name)) {
            alert('A folder or repository with this name already exists at this level.');
            return;
        }

        const data = {
            name: name,
            parent_id: parentId,
            description,
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

    function bindDomEvents() {
        DOM.mainHeader.addEventListener('click', e => {
            if (e.target.closest('#view-logs-btn')) {
                showLogsModal();
            }
        });

        // Tabs navigation (Recent, Main, etc.) should update the hash lazily.
        if (DOM.tabs && DOM.tabs.length) {
            const navigateToTab = (targetTab) => {
                if (!targetTab) return;

                const current = parsePipelineRunsHash(window.location.hash);
                const sameTab = current.tab === targetTab;
                const hasRunOpen = !!current.runId;

                if (targetTab === 'recent') {
                    if (sameTab && !hasRunOpen) return;
                    window.location.hash = '#/pipelineruns/recent';
                    return;
                }

                if (targetTab === 'main') {
                    let segments = [];
                    if (sameTab && !hasRunOpen) {
                        segments = [];
                    } else if (sameTab && hasRunOpen && Array.isArray(current.groupSegments) && current.groupSegments.length) {
                        segments = current.groupSegments.slice();
                    } else if (state.selectedGroupPathSegments && state.selectedGroupPathSegments.length) {
                        segments = state.selectedGroupPathSegments.slice();
                    }

                    const targetHash = segments.length ? `#/pipelineruns/main/${segments.join('/')}` : '#/pipelineruns/main';
                    window.location.hash = targetHash;
                }
            };

            DOM.tabs.forEach(tab => {
                tab.addEventListener('click', (e) => {
                    e.preventDefault();
                    navigateToTab(tab.dataset.tab);
                });

                tab.addEventListener('keydown', (e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        navigateToTab(tab.dataset.tab);
                    }
                });
            });
        }

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
                    if (typeof state.syncLogsHash === 'function') state.syncLogsHash({ replace: true });
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
                if (typeof state.syncLogsHash === 'function') state.syncLogsHash({ replace: true });
            });
        }
        if (DOM.logsStepsClear) {
            DOM.logsStepsClear.addEventListener('click', () => {
                state.logsSelectedSteps = new Set();
                updateLogsStepList();
                state._logsFocusFirstMatch = true;
                renderLogsWithFilters();
                if (typeof state.syncLogsHash === 'function') state.syncLogsHash({ replace: true });
            });
        }
        // No 'only selected' toggle; selecting none means show all

        if (DOM.pipelineRunsSearch) {
            DOM.pipelineRunsSearch.addEventListener('input', () => {
                const nextValue = DOM.pipelineRunsSearch.value.trim();
                if (state.runSearchTerm === nextValue) return;
                state.runSearchTerm = nextValue;
                applyRunSearchFilter();
            });
        }
        if (DOM.logsSearch) {
            DOM.logsSearch.addEventListener('input', (e) => {
                state.logsSearchText = e.target.value || '';
                state._logsFocusFirstMatch = true;
                renderLogsWithFilters();
                if (typeof state.syncLogsHash === 'function') state.syncLogsHash({ replace: true });
            });
        }
        if (DOM.logsClearSearch) {
            DOM.logsClearSearch.addEventListener('click', () => {
                if (DOM.logsSearch) DOM.logsSearch.value = '';
                state.logsSearchText = '';
                renderLogsWithFilters();
                if (typeof state.syncLogsHash === 'function') state.syncLogsHash({ replace: true });
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

        if (DOM.addGroupForm) {
            DOM.addGroupForm.addEventListener('submit', createGroup);
        }
        if (DOM.closeAddGroupModalBtn) {
            DOM.closeAddGroupModalBtn.addEventListener('click', closeAddGroupModal);
        }
        if (DOM.cancelAddGroupBtn) {
            DOM.cancelAddGroupBtn.addEventListener('click', closeAddGroupModal);
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
                    const newHash = buildRunHashWithExtras(state.currentRunData?.run_info, resolveRunContext(state.currentRunContext || null), ['steps', stepName]);
                    window.location.hash = newHash;
                    showStepDetails(stepName);
                    return;
                }

                const stepEl = e.target.closest('[data-step-name]');
                const taskNode = e.target.closest('g.graph-node[data-task-name]');
                if (taskNode) {
                    if (!(taskNode.dataset && taskNode.dataset.context)) {
                        const taskStepName = taskNode.dataset.stepName || (stepEl && stepEl.dataset.stepName) || '';
                        const taskName = taskNode.dataset.taskName || '';
                        if (taskStepName && taskName) {
                            e.preventDefault();
                            e.stopPropagation();
                            openLogsForTask(taskStepName, taskName);
                            return;
                        }
                    }
                }

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
                    } catch { }
                    state._fitOnNextStepsRender = false;
                    const stepName = stepEl.dataset.stepName;
                    if (e.ctrlKey || e.metaKey) {
                        const newHash = buildRunHashWithExtras(state.currentRunData?.run_info, resolveRunContext(state.currentRunContext || null), ['steps', stepName]);
                        window.location.hash = newHash;
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
                if (e.target.closest('.run-select-toggle')) return;
                const card = e.target.closest('[data-href]');
                if (!card) return;

                const startX = e.clientX;
                const startY = e.clientY;
                const button = e.button;
                if (button !== 0 && button !== 1) return;

                const onMouseUp = (upEvent) => {
                    document.removeEventListener('mouseup', onMouseUp);

                    const dx = upEvent.clientX - startX;
                    const dy = upEvent.clientY - startY;
                    if (Math.hypot(dx, dy) > 5) return;

                    const selection = window.getSelection();
                    if (selection && selection.toString().trim().length > 0 && button === 0) {
                        try {
                            navigator.clipboard.writeText(selection.toString());
                        } catch { }
                        return;
                    }

                    const url = card.dataset.href;
                    const context = parseRunContextAttr(card.dataset.runContext);
                    if (context) {
                        state.currentRunContext = context;
                    }

                    if (button === 1 || upEvent.ctrlKey || upEvent.metaKey) {
                        try {
                            window.open(url, '_blank');
                        } catch {
                            window.location.hash = url;
                        }
                    } else if (button === 0) {
                        window.location.hash = url;
                    }
                };

                document.addEventListener('mouseup', onMouseUp, { once: true });
            });

            DOM.mainGridContainer.addEventListener('click', async e => {
                const selectToggle = e.target.closest('.run-select-toggle');
                if (selectToggle) {
                    e.preventDefault();
                    e.stopPropagation();
                    toggleRunSelection(selectToggle.dataset.runId || '');
                    return;
                }

                const card = e.target.closest('a[data-run-id]');
                if (card && !e.ctrlKey && !e.metaKey && e.button === 0) {
                    e.preventDefault();
                    const context = parseRunContextAttr(card.dataset.runContext);
                    if (context) {
                        state.currentRunContext = context;
                    }
                    window.location.hash = card.getAttribute('href');
                }

                const branchDeleteBtn = e.target.closest('.branch-delete-btn');
                if (branchDeleteBtn) {
                    e.preventDefault();
                    e.stopPropagation();
                    await handleBranchDeleteButton(branchDeleteBtn);
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

                const logsBtn = e.target.closest('#step-modal-open-logs-btn');
                if (logsBtn && logsBtn.dataset.stepName) {
                    openLogsForStep(logsBtn.dataset.stepName);
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
                const branchDeleteBtn = e.target.closest('.branch-delete-btn');
                if (branchDeleteBtn) {
                    e.preventDefault();
                    e.stopPropagation();
                    await handleBranchDeleteButton(branchDeleteBtn);
                    return;
                }

                const link = e.target.closest('a[href]');
                const groupHeader = e.target.closest('.group-header');

                if (link) {
                    e.preventDefault();
                    const context = parseRunContextAttr(link.dataset.runContext);
                    if (context) {
                        state.currentRunContext = context;
                    }
                    const href = link.getAttribute('href');
                    if (window.location.hash !== href) {
                        window.location.hash = href;
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
            const taskNode = e.target.closest('g.graph-node[data-task-name]');
            if (taskNode) { show('Logs', 'accent', e.clientX, e.clientY); return; }
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

    async function handleNewRunStarted(runData) {
        // This function triggers a refresh of both the sidebar and the main content area.

        // 1. Refresh the sidebar (existing logic, which is working)
        if (state.repoLastRunCache) {
            state.repoLastRunCache.clear();
        }
        await renderSidebar(state.currentPath || 'pipelineruns', state.currentTab || 'main');

        // --- FIX START ---
        // 2. Refresh the main content view if it's a relevant page.
        // We only need to do this if we are on the main 'pipelineruns' page.
        const hashInfo = parsePipelineRunsHash(window.location.hash);
        if (state.currentPath === 'pipelineruns' && !hashInfo.runId) {
            if (state.currentTab === 'recent') {
                // If on the "Recent" tab, re-fetch all runs and re-render the main grid.
                const runs = await fetchData('/v1/runs');
                if (Array.isArray(runs)) {
                    state.recentRuns = runs;
                } else {
                    state.recentRuns = [];
                }
                renderMainGridContent(null, state.recentRuns, false);
                renderSidebarPipelineRunsList(state.recentRuns);
            } else if (state.currentTab === 'main') {
                // If on the "Main" tab, re-fetch the content for the currently selected group.
                // This will show new branches or update the grouped run lists.
                if (state.selectedGroupId) {
                    await fetchMainContent(state.selectedGroupId);
                } else {
                    // If at the root, re-render the top-level groups.
                    const rootGroups = state.groups.filter(g => normalizeParentId(g.parent_id) === null);
                    state.currentRepoRunsByBranch = null;
                    state.currentRepoGroupId = null;
                    renderMainGridContent(rootGroups, null, true);
                }
            }
        }
        // --- FIX END ---

        const newRunIdRaw = runData?.run_id || runData?.runId || '';
        const parentRunIdRaw = runData?.parent_run_id || runData?.parentRunId || '';
        const trackedIds = (state.currentRunTrackedIds instanceof Map) ? state.currentRunTrackedIds : null;
        const currentRunId = state.currentRunData?.run_info?.run_id;
        const parentNormalized = normalizeRunId(parentRunIdRaw);
        const newRunNormalized = normalizeRunId(newRunIdRaw);

        if (trackedIds && parentNormalized && trackedIds.has(parentNormalized) && newRunNormalized && currentRunId) {
            trackedIds.set(newRunNormalized, newRunIdRaw);
            await fetchActiveRun(currentRunId, true);
        }
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
        handleNewRunStarted,
        renderSidebarForRoute: async (route) => {
            await renderSidebar(route, state.currentTab || 'main');
        },
    };
})(window.NopsAI = window.NopsAI || {});
