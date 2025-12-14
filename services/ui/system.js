(function (global) {
    const state = {
        syncPollTimer: null,
        lastStatus: null,
        isSaving: false,
        activeTab: 'config',
    };

    const DOM = {};
    let context = null;

    function init(ctx) {
        context = ctx;
        const ids = [
            'system-config-form', 'system-config-repo', 'system-agent-image', 'system-docker-network',
            'system-default-timeout', 'system-llm-timeout', 'system-auto-remove', 'system-sync-btn',
            'system-reload-btn', 'system-save-btn', 'system-sync-report', 'system-sync-empty',
            'system-repo-display', 'system-repo-helper', 'system-repo-chip', 'system-sync-updated',
            'system-sync-status-label', 'system-config-status', 'system-sync-refresh-btn',
            'system-agent-api', 'system-gitbot-api', 'system-nopsai-gitbot-api',
            'system-config-section', 'system-dispatcher-section', 'dispatcher-queue-count',
            'dispatcher-runner-count', 'dispatcher-active-count', 'dispatcher-runner-list',
            'dispatcher-empty', 'dispatcher-updated',
        ];
        ids.forEach(id => {
            const el = document.getElementById(id);
            if (el) DOM[id] = el;
        });

        const tabButtons = document.querySelectorAll('.system-tab-btn');
        tabButtons.forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.preventDefault();
                const target = btn.getAttribute('data-target');
                switchTab(target);
            });
        });

        if (DOM['system-config-form']) {
            DOM['system-config-form'].addEventListener('submit', handleSave);
        }
        if (DOM['system-sync-btn']) {
            DOM['system-sync-btn'].addEventListener('click', (e) => {
                e.preventDefault();
                triggerSync();
            });
        }
        if (DOM['system-sync-refresh-btn']) {
            DOM['system-sync-refresh-btn'].addEventListener('click', (e) => {
                e.preventDefault();
                loadSyncStatus(true);
            });
        }
    }

    async function handleRoute() {
        stopSyncPolling();
        switchTab(state.activeTab || 'config');
        await loadSystemConfig(true);
        await loadDispatcherStatus(true);
        startSyncPolling();
    }

    function onLeave() {
        stopSyncPolling();
        state.activeTab = 'config';
        switchTab('config');
    }

    function startSyncPolling() {
        stopSyncPolling();
        state.syncPollTimer = window.setInterval(() => {
            if (window.location.hash.startsWith('#/system')) {
                loadSyncStatus();
                loadDispatcherStatus();
            }
        }, 5000);
    }

    function stopSyncPolling() {
        if (state.syncPollTimer) {
            clearInterval(state.syncPollTimer);
            state.syncPollTimer = null;
        }
    }

    async function loadSystemConfig() {
        if (!context || typeof context.fetchData !== 'function') return null;
        const data = await context.fetchData('/v1/system/config');
        if (!data) return null;

        applyConfigToForm(data);
        renderMeta(data);
        renderSyncStatus(data.config_sync_status);
        return data;
    }

    async function loadDispatcherStatus(force = false) {
        if (!context || typeof context.fetchData !== 'function') return;
        if (!force && state.activeTab !== 'dispatcher') {
            return;
        }
        const status = await context.fetchData('/v1/system/dispatcher');
        renderDispatcherStatus(status);
    }

    function applyConfigToForm(data) {
        if (!data || !DOM['system-config-form']) return;
        if (DOM['system-config-repo']) DOM['system-config-repo'].value = data.config_repo_url || '';
        if (DOM['system-agent-image']) DOM['system-agent-image'].value = data.agent_image || '';
        if (DOM['system-docker-network']) DOM['system-docker-network'].value = data.docker_network_name || '';
        if (DOM['system-default-timeout']) DOM['system-default-timeout'].value = data.default_pipeline_timeout || '';
        if (DOM['system-llm-timeout']) DOM['system-llm-timeout'].value = data.llm_agent_timeout || '';
        if (DOM['system-auto-remove']) DOM['system-auto-remove'].checked = !!data.auto_removal_agent_container;
        if (DOM['system-agent-api']) DOM['system-agent-api'].value = data.agent_nopsai_api_url || '';
        if (DOM['system-gitbot-api']) DOM['system-gitbot-api'].value = data.git_bot_nopsai_api_url || '';
        if (DOM['system-nopsai-gitbot-api']) DOM['system-nopsai-gitbot-api'].value = data.nopsai_git_bot_api_url || '';
    }

    function collectFormPayload() {
        const repoInput = DOM['system-config-repo'];
        const agentInput = DOM['system-agent-image'];
        const networkInput = DOM['system-docker-network'];
        const defaultTimeoutInput = DOM['system-default-timeout'];
        const llmTimeoutInput = DOM['system-llm-timeout'];
        const autoRemoveInput = DOM['system-auto-remove'];
        const agentApiInput = DOM['system-agent-api'];
        const gitbotApiInput = DOM['system-gitbot-api'];
        const nopsaiGitbotApiInput = DOM['system-nopsai-gitbot-api'];

        return {
            config_repo_url: ((repoInput && repoInput.value) || '').trim(),
            agent_image: ((agentInput && agentInput.value) || '').trim(),
            docker_network_name: ((networkInput && networkInput.value) || '').trim(),
            default_pipeline_timeout: ((defaultTimeoutInput && defaultTimeoutInput.value) || '').trim(),
            llm_agent_timeout: ((llmTimeoutInput && llmTimeoutInput.value) || '').trim(),
            auto_removal_agent_container: !!(autoRemoveInput && autoRemoveInput.checked),
            agent_nopsai_api_url: ((agentApiInput && agentApiInput.value) || '').trim(),
            git_bot_nopsai_api_url: ((gitbotApiInput && gitbotApiInput.value) || '').trim(),
            nopsai_git_bot_api_url: ((nopsaiGitbotApiInput && nopsaiGitbotApiInput.value) || '').trim(),
        };
    }

    async function handleSave(event) {
        event.preventDefault();
        if (!context || typeof context.fetchData !== 'function' || state.isSaving) return;

        state.isSaving = true;
        toggleSaving(true);
        setConfigStatus('Saving…', 'muted');

        try {
            const payload = collectFormPayload();
            const response = await context.fetchData('/v1/system/config', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            });
            if (response) {
                applyConfigToForm(response);
                renderMeta(response);
                renderSyncStatus(response.config_sync_status);
                setConfigStatus('Settings saved', 'success');
                showToast('System settings saved.', 'success');
            } else {
                setConfigStatus('Save failed', 'error');
                showToast('Failed to save settings. Check console for details.', 'error');
            }
        } catch (error) {
            console.error('Failed to save system settings', error);
            setConfigStatus('Save failed', 'error');
            showToast('Failed to save settings. Check console for details.', 'error');
        } finally {
            state.isSaving = false;
            toggleSaving(false);
        }
    }

    async function triggerSync() {
        if (!context || typeof context.fetchData !== 'function') return;
        setSyncButtonState(true);
        try {
            const status = await context.fetchData('/v1/system/config/sync', { method: 'POST' });
            if (status) {
                renderSyncStatus(status);
                startSyncPolling();
                showToast('Config sync requested.', 'info');
            } else {
                showToast('Unable to start config sync.', 'error');
            }
        } catch (error) {
            console.error('Failed to trigger config sync', error);
            showToast('Unable to start config sync.', 'error');
        } finally {
            setSyncButtonState(false);
        }
    }

    async function loadSyncStatus() {
        if (!context || typeof context.fetchData !== 'function') return;
        const status = await context.fetchData('/v1/system/config/sync');
        renderSyncStatus(status);
    }

    function renderMeta(data) {
        const repo = (data && data.config_repo_url) ? data.config_repo_url : '';
        if (DOM['system-repo-display']) {
            DOM['system-repo-display'].textContent = repo || 'Not configured';
        }
        if (DOM['system-repo-helper']) {
            DOM['system-repo-helper'].textContent = repo
                ? 'Definitions will sync from this repository.'
                : 'Set the Git URL to enable sync from source control.';
        }

        const syncStatus = data && data.config_sync_status ? data.config_sync_status : state.lastStatus;
        const statusKey = normalizeStatus(syncStatus && syncStatus.status, repo);
        const statusLabel = statusKey === 'success'
            ? 'Synced'
            : statusKey === 'running'
                ? 'In progress'
                : statusKey === 'error'
                    ? 'Sync failed'
                    : repo ? 'Ready' : 'Not started';

        if (DOM['system-sync-status-label']) {
            DOM['system-sync-status-label'].textContent = statusLabel;
        }
        if (DOM['system-sync-updated']) {
            const timestamp = (syncStatus && syncStatus.completed_at) || (syncStatus && syncStatus.started_at);
            DOM['system-sync-updated'].textContent = formatTimestamp(timestamp);
        }
        updateChip(statusKey, repo);
    }

    function updateChip(statusKey, repo) {
        if (!DOM['system-repo-chip']) return;
        const chip = DOM['system-repo-chip'];
        chip.className = 'system-chip';
        if (!repo) {
            chip.classList.add('system-chip--muted');
            chip.textContent = 'Not configured';
            return;
        }
        if (statusKey === 'running') {
            chip.classList.add('system-chip--warning');
            chip.textContent = 'Syncing';
            return;
        }
        if (statusKey === 'error') {
            chip.classList.add('system-chip--error');
            chip.textContent = 'Sync failed';
            return;
        }
        if (statusKey === 'success') {
            chip.classList.add('system-chip--success');
            chip.textContent = 'Synced';
            return;
        }
        chip.classList.add('system-chip--muted');
        chip.textContent = 'Ready';
    }

    function renderSyncStatus(status) {
        state.lastStatus = status || null;
        const container = DOM['system-sync-report'];
        const empty = DOM['system-sync-empty'];
        if (!container) return;

        if (!status || !status.status) {
            container.innerHTML = '';
            if (empty) empty.classList.remove('hidden');
            const repoInput = DOM['system-config-repo'];
            renderMeta({ config_repo_url: repoInput ? repoInput.value : '', config_sync_status: status });
            return;
        }

        if (empty) empty.classList.add('hidden');
        const statusKey = normalizeStatus(status.status);
        const message = status.message || defaultStatusMessage(statusKey);
        const detailsHtml = renderDetails(status.details);
        const timing = buildTiming(status);
        const toneClass = statusTone(statusKey);

        container.innerHTML = `
            <div class="pipeline-sync-card ${toneClass}">
                <div class="sync-icon">
                    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        <path d="${chooseIcon(statusKey)}" />
                    </svg>
                </div>
                <div class="flex-1 min-w-0">
                    <h3>${escapeHtml(capitalizeStatus(statusKey))}</h3>
                    <p>${escapeHtml(message)}</p>
                    ${timing}
                    ${detailsHtml}
                </div>
            </div>
        `;

        const repoInput = DOM['system-config-repo'];
        renderMeta({ config_repo_url: repoInput ? repoInput.value : '', config_sync_status: status });
    }

    function renderDispatcherStatus(status) {
        const queueCountEl = DOM['dispatcher-queue-count'];
        const runnerCountEl = DOM['dispatcher-runner-count'];
        const activeCountEl = DOM['dispatcher-active-count'];
        const listEl = DOM['dispatcher-runner-list'];
        const emptyEl = DOM['dispatcher-empty'];
        const updatedEl = DOM['dispatcher-updated'];

        if (!status || !queueCountEl || !runnerCountEl || !activeCountEl || !listEl || !emptyEl) {
            return;
        }

        const runnersRaw = Array.isArray(status.runners) ? status.runners : [];
        const runners = sortRunnersById(runnersRaw);
        const activeSum = runners.reduce((sum, r) => sum + (r.activeJobs || 0), 0);

        queueCountEl.textContent = status.queuedJobs != null ? status.queuedJobs : '0';
        runnerCountEl.textContent = runners.length.toString();
        activeCountEl.textContent = activeSum.toString();
        if (updatedEl) {
            updatedEl.textContent = `Updated ${formatRelative(new Date())}`;
        }

        listEl.innerHTML = '';
        if (runners.length === 0) {
            emptyEl.classList.remove('hidden');
            return;
        }

        emptyEl.classList.add('hidden');
        const now = Date.now();
        runners.forEach(runner => {
            const stale = isStale(now, runner.lastHeartbeatUnix);
            const scopes = (runner.scopes || []).length > 0 ? runner.scopes.join(', ') : 'All scopes';
            const badgeClass = stale ? 'runner-pill--error' : 'runner-pill--ok';
            const badgeLabel = stale ? 'Stale' : 'Healthy';
            const meta = getRunnerMeta(runner);
            const connectionLabel = formatConnectionId(meta.connectionId);

            const card = document.createElement('div');
            card.className = 'glass-card p-4 space-y-2';
            const metaGrid = [];
            metaGrid.push(`
                <div class="runner-meta">
                    <span class="runner-meta__label">Active</span>
                    <span class="runner-meta__value">${runner.activeJobs || 0}/${runner.capacity || 0}</span>
                </div>
            `);
            metaGrid.push(`
                <div class="runner-meta">
                    <span class="runner-meta__label">Inflight</span>
                    <span class="runner-meta__value">${runner.inflightJobs || 0}</span>
                </div>
            `);
            metaGrid.push(`
                <div class="runner-meta">
                    <span class="runner-meta__label">Dispatch</span>
                    <span class="runner-meta__value">${runner.allowDispatch ? 'Yes' : 'Paused'}</span>
                </div>
            `);
            metaGrid.push(`
                <div class="runner-meta">
                    <span class="runner-meta__label">Heartbeat</span>
                    <span class="runner-meta__value">${formatSince(runner.lastHeartbeatUnix)}</span>
                </div>
            `);
            if (connectionLabel) {
                metaGrid.push(`
                    <div class="runner-meta">
                        <span class="runner-meta__label">Instance</span>
                        <span class="runner-meta__value runner-meta__value--mono">${escapeHtml(connectionLabel)}</span>
                    </div>
                `);
            }
            if (meta.hostname) {
                metaGrid.push(`
                    <div class="runner-meta">
                        <span class="runner-meta__label">Host</span>
                        <span class="runner-meta__value">${escapeHtml(meta.hostname)}</span>
                    </div>
                `);
            }
            if (meta.network) {
                metaGrid.push(`
                    <div class="runner-meta">
                        <span class="runner-meta__label">Network</span>
                        <span class="runner-meta__value runner-meta__value--mono">${escapeHtml(meta.network)}</span>
                    </div>
                `);
            }

            card.innerHTML = `
                <div class="flex items-start justify-between gap-2">
                    <div class="space-y-1">
                        <p class="text-sm font-semibold text-[var(--text-primary)]">${escapeHtml(runner.runnerId || 'unnamed')}</p>
                        <p class="text-xs text-[var(--text-secondary)]">${escapeHtml(scopes)}</p>
                    </div>
                    <div class="flex items-center gap-2">
                        ${connectionLabel ? `<span class="runner-pill runner-pill--muted">${escapeHtml(connectionLabel)}</span>` : ''}
                        <span class="runner-pill ${badgeClass}">${badgeLabel}</span>
                    </div>
                </div>
                <div class="grid grid-cols-2 gap-2 text-xs">
                    ${metaGrid.join('')}
                </div>
            `;
            listEl.appendChild(card);
        });
    }

    function switchTab(target) {
        if (!target || (target !== 'config' && target !== 'dispatcher')) return;
        state.activeTab = target;
        const configSection = DOM['system-config-section'];
        const dispatcherSection = DOM['system-dispatcher-section'];
        if (configSection && dispatcherSection) {
            if (target === 'config') {
                configSection.classList.remove('hidden');
                dispatcherSection.classList.add('hidden');
            } else {
                configSection.classList.add('hidden');
                dispatcherSection.classList.remove('hidden');
                loadDispatcherStatus(true);
            }
        }
        document.querySelectorAll('.system-tab-btn').forEach(btn => {
            const tab = btn.getAttribute('data-target');
            if (tab === target) {
                btn.classList.add('system-tab-btn--active');
            } else {
                btn.classList.remove('system-tab-btn--active');
            }
        });
    }

    function renderDetails(details) {
        if (!details || typeof details !== 'object') return '';
        const entries = Object.entries(details);
        if (!entries.length) return '';
        const items = entries.map(([key, value]) => {
            return `<li><strong>${escapeHtml(key)}:</strong> <span>${escapeHtml(String(value))}</span></li>`;
        }).join('');
        return `<ul class="sync-detail-list">${items}</ul>`;
    }

    function buildTiming(status) {
        const started = formatTimestamp(status && status.started_at);
        const finished = formatTimestamp(status && status.completed_at);
        if (!started && !finished) return '';
        if (started && !finished) {
            return `<p class="text-xs text-[var(--text-secondary)] mt-1">Started ${escapeHtml(started)}</p>`;
        }
        if (finished) {
            return `<p class="text-xs text-[var(--text-secondary)] mt-1">Finished ${escapeHtml(finished)}</p>`;
        }
        return '';
    }

    function toggleSaving(isSaving) {
        const btn = DOM['system-save-btn'];
        if (btn) {
            btn.disabled = isSaving;
            btn.classList.toggle('cursor-wait', isSaving);
        }
    }

    function setSyncButtonState(isBusy) {
        const btn = DOM['system-sync-btn'];
        if (btn) {
            btn.disabled = isBusy;
            btn.classList.toggle('cursor-wait', isBusy);
        }
    }

    function setConfigStatus(message, tone = 'muted') {
        const el = DOM['system-config-status'];
        if (!el) return;
        el.textContent = message || '';
        el.className = 'text-xs';
        if (tone === 'error') {
            el.classList.add('text-red-500');
        } else if (tone === 'success') {
            el.classList.add('text-green-600');
        } else {
            el.classList.add('text-[var(--text-secondary)]');
        }
    }

    function statusTone(statusKey) {
        if (statusKey === 'success') return 'success';
        if (statusKey === 'error') return 'error';
        if (statusKey === 'running') return 'loading';
        return 'info';
    }

    function chooseIcon(statusKey) {
        if (statusKey === 'success') return 'M5 13l4 4L19 7';
        if (statusKey === 'error') return 'M12 9v4m0 4h.01M5.455 5.455l13.09 13.09';
        if (statusKey === 'running') return 'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99';
        return 'M12 18.5a6.5 6.5 0 1 1 6.32-8.13';
    }

    function normalizeStatus(value, repo = '') {
        const key = (value || '').toString().toLowerCase();
        if (!key && !repo) return 'missing';
        if (['running', 'loading', 'in_progress'].includes(key)) return 'running';
        if (['success', 'completed', 'complete', 'done'].includes(key)) return 'success';
        if (['error', 'failed', 'failure'].includes(key)) return 'error';
        return key || 'idle';
    }

    function defaultStatusMessage(statusKey) {
        if (statusKey === 'running') return 'Synchronization is in progress.';
        if (statusKey === 'success') return 'Configuration synchronization completed successfully.';
        if (statusKey === 'error') return 'Configuration synchronization failed.';
        return 'Awaiting the first synchronization.';
    }

    function escapeHtml(value) {
        return String(value ?? '').replace(/[&<>"'`]/g, (char) => ({
            '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;', '`': '&#96;',
        }[char]));
    }

    function formatTimestamp(value) {
        if (!value) return '—';
        const date = new Date(value);
        if (Number.isNaN(date.getTime())) return '—';
        return date.toLocaleString();
    }

    function formatSince(unixSeconds) {
        if (!unixSeconds) return 'never';
        const diff = Date.now() - unixSeconds * 1000;
        if (diff < 0) return 'now';
        const seconds = Math.floor(diff / 1000);
        if (seconds < 60) return `${seconds}s ago`;
        const minutes = Math.floor(seconds / 60);
        if (minutes < 60) return `${minutes}m ago`;
        const hours = Math.floor(minutes / 60);
        return `${hours}h ago`;
    }

    function formatRelative(date) {
        const d = date instanceof Date ? date : new Date(date);
        if (Number.isNaN(d.getTime())) return '—';
        return d.toLocaleTimeString();
    }

    function getRunnerMeta(runner) {
        const meta = (runner && runner.metadata) || {};
        return {
            connectionId: (meta.connection_id || meta.instance_id || '').trim(),
            hostname: (meta.hostname || meta.host || meta.runner_host || '').trim(),
            network: (meta.docker_network || meta.docker_network_name || meta.docker_networkname || '').trim(),
        };
    }

    function formatConnectionId(value) {
        const trimmed = (value || '').trim();
        if (!trimmed) return '';
        if (trimmed.length <= 14) return trimmed;
        return `${trimmed.slice(0, 6)}...${trimmed.slice(-4)}`;
    }

    function sortRunnersById(runners) {
        const copy = Array.isArray(runners) ? runners.slice() : [];
        copy.sort((a, b) => {
            const idA = ((a && a.runnerId) || '').toLowerCase();
            const idB = ((b && b.runnerId) || '').toLowerCase();
            if (idA !== idB) return idA.localeCompare(idB);
            const connA = getRunnerMeta(a).connectionId.toLowerCase();
            const connB = getRunnerMeta(b).connectionId.toLowerCase();
            return connA.localeCompare(connB);
        });
        return copy;
    }

    function isStale(nowMs, lastHeartbeatUnix) {
        if (!lastHeartbeatUnix) return true;
        return (nowMs - lastHeartbeatUnix * 1000) > 30000;
    }

    function capitalizeStatus(value) {
        if (!value) return '';
        return value.charAt(0).toUpperCase() + value.slice(1);
    }

    function showToast(message, variant = 'info') {
        const container = document.getElementById('toast-container');
        if (!container) return;
        const toast = document.createElement('div');
        toast.className = `pipelines-toast pipelines-toast--${variant}`;
        toast.innerHTML = `<div class="pipelines-toast__content">${escapeHtml(message)}</div>`;
        container.appendChild(toast);
        requestAnimationFrame(() => toast.classList.add('show'));
        setTimeout(() => {
            toast.classList.remove('show');
            setTimeout(() => toast.remove(), 200);
        }, 3200);
    }

    global.pages = global.pages || {};
    global.pages.system = {
        init,
        handleRoute,
        onLeave,
    };
})(window.NopsAI = window.NopsAI || {});
