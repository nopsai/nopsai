import { useCallback, useEffect, useMemo, useState } from 'react';
import { RefreshCw, UsersRound } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import { ConfigRepositoryDriftModal } from '../components/ConfigRepositoryDriftModal';
import { useAuth } from '../auth/AuthContext';
import {
  createTeamItem,
  fetchTeams,
  requestTeamsJson,
  updateTeamDefaults,
  type TeamDefaultsPayload,
  updateTeamItem,
} from '../features/teams/api';
import { EditTeamItemModal, TeamConfigRepositoryModal, NewTeamItemModal, type NewTeamItemKind, type TeamItemEditPayload } from '../features/teams/TeamSettingsModals';
import { TeamsStatusPanel, TeamsWorkspace } from '../features/teams/TeamsWorkspace';
import { useDispatcherStatusSnapshot } from '../features/system/dispatcher/useDispatcherStatusSnapshot';
import { useTeamConfigRepositoryController } from '../features/teams/hooks/useTeamConfigRepositoryController';
import { useTeamOperationsSummary } from '../features/teams/hooks/useTeamOperationsSummary';
import { useTeamResourceCatalog } from '../features/teams/hooks/useTeamResourceCatalog';
import { getTeamCreateParentOptions, getTeamMoveParentOptions } from '../features/teams/model';
import {
  buildTeamPath,
  findTeamByURLValue,
  isAppTeam,
  normalizeTeamURLValue,
  teamDisplayName,
  teamPathForURL,
  type Team,
} from '../lib/teamModels';
import { extractTeamPathFromRoute, teamScopedRoute } from '../lib/teamRoutes';

type TeamItemPayload = {
  kind: 'team' | 'app';
  name: string;
  description: string;
  repoURL: string;
  parentID: number | null;
};

function isReservedRootTeamName(name: string) {
  const normalized = name.trim().replace(/^\/+|\/+$/g, '').toLowerCase();
  return normalized === 'root' || normalized === 'global' || normalized === 'general';
}

export default function TeamsPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const { currentUser } = useAuth();
  const [teams, setTeams] = useState<Team[]>([]);
  const [teamsLoaded, setTeamsLoaded] = useState(false);
  const [teamsLoading, setTeamsLoading] = useState(false);
  const [teamsError, setTeamsError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [createKind, setCreateKind] = useState<NewTeamItemKind>('team');
  const [createError, setCreateError] = useState<string | null>(null);
  const [createPending, setCreatePending] = useState(false);
  const [editTeam, setEditTeam] = useState<Team | null>(null);
  const [editError, setEditError] = useState<string | null>(null);
  const [editPending, setEditPending] = useState(false);
  const [operationsRefreshKey, setOperationsRefreshKey] = useState(0);

  const activeTeamValue = useMemo(
    () => normalizeTeamURLValue(extractTeamPathFromRoute(location.pathname, 'teams') || new URLSearchParams(location.search).get('team')),
    [location.pathname, location.search]
  );
  const activeTeam = useMemo(() => findTeamByURLValue(activeTeamValue, teams), [activeTeamValue, teams]);
  const activeTeamID = activeTeam?.id ?? null;
  const activeTeamPath = useMemo(() => buildTeamPath(activeTeamID, teams), [activeTeamID, teams]);
  const activeTeamLabel = activeTeam ? teamDisplayName(activeTeam) : 'Global';
  const activeTeamURLValue = useMemo(
    () => (activeTeam ? teamPathForURL(activeTeam, teams) : ''),
    [activeTeam, teams]
  );
  const resourceCatalogPath = useMemo(() => {
    if (!activeTeam || !isAppTeam(activeTeam)) return activeTeamURLValue;
    if (activeTeam.team_path) return activeTeam.team_path;
    const parent = activeTeam.parent_id == null ? null : teams.find(team => team.id === activeTeam.parent_id) || null;
    return parent ? teamPathForURL(parent, teams) : '';
  }, [activeTeam, activeTeamURLValue, teams]);

  const fetchJson = useCallback(
    async <T,>(path: string, options?: RequestInit): Promise<T> => requestTeamsJson<T>(path, options),
    []
  );

  const checkAccessPermission = useCallback(async (action: string, resourceType: string, resourceID: string) => {
    const params = new URLSearchParams({
      action,
      resource_type: resourceType,
      resource_id: resourceID,
    });
    try {
      const payload = await fetchJson<{ allowed?: boolean }>(`/v1/access/effective-permissions?${params.toString()}`);
      return Boolean(payload?.allowed);
    } catch {
      return false;
    }
  }, [fetchJson]);

  const config = useTeamConfigRepositoryController({ teams, fetchJson, checkAccessPermission });
  const operationsSummary = useTeamOperationsSummary({
    team: activeTeam,
    teams,
    fetchJson,
    checkAccessPermission,
    refreshKey: operationsRefreshKey,
  });
  const resourceCatalog = useTeamResourceCatalog({ teamPath: resourceCatalogPath });
  const runnerAssignments = useDispatcherStatusSnapshot({ enabled: teamsLoaded });

  const saveTeamDefaults = useCallback(async (teamPath: string, defaults: TeamDefaultsPayload) => {
    await updateTeamDefaults(teamPath, defaults);
    setOperationsRefreshKey(value => value + 1);
  }, []);

  const loadTeams = useCallback(async () => {
    setTeamsLoaded(false);
    setTeamsLoading(true);
    setTeamsError(null);
    try {
      const loadedTeams = await fetchTeams();
      setTeams(loadedTeams);
      return loadedTeams;
    } catch (error) {
      setTeamsError(error instanceof Error ? error.message : 'Unable to load teams');
      return [];
    } finally {
      setTeamsLoaded(true);
      setTeamsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadTeams();
  }, [loadTeams]);

  useEffect(() => {
    if (!activeTeamValue || !teamsLoaded) return;
    if (activeTeamURLValue) {
      const nextPath = teamScopedRoute('/teams', activeTeamURLValue);
      if (location.pathname === nextPath && !new URLSearchParams(location.search).has('team')) return;
      navigate(nextPath, { replace: true });
      return;
    }
    navigate('/teams', { replace: true });
  }, [activeTeamURLValue, activeTeamValue, location.pathname, location.search, navigate, teamsLoaded]);

  const selectTeam = useCallback(
    (id: number | null) => {
      const team = id == null ? null : teams.find(item => item.id === id) || null;
      navigate(teamScopedRoute('/teams', team ? teamPathForURL(team, teams) : ''), { replace: true });
    },
    [navigate, teams]
  );

  const openCreateModal = useCallback((kind: NewTeamItemKind = 'team') => {
    setCreateKind(kind);
    setCreateError(null);
    setCreateOpen(true);
  }, []);

  const openEditModal = useCallback((team: Team) => {
    setEditTeam(team);
    setEditError(null);
    setEditPending(false);
  }, []);

  const closeEditModal = useCallback(() => {
    setEditTeam(null);
    setEditError(null);
    setEditPending(false);
  }, []);

  const submitNewTeamItem = useCallback(
    async ({ kind, name, description, repoURL, parentID }: TeamItemPayload) => {
      const trimmedName = name.trim();
      const trimmedDescription = description.trim();
      const trimmedRepoURL = repoURL.trim();
      if (!trimmedName) {
        setCreateError(kind === 'app' ? 'Application name is required.' : 'Team name is required.');
        return;
      }
      if (kind === 'team' && isReservedRootTeamName(trimmedName)) {
        setCreateError('Global is reserved and cannot be used as a team name.');
        return;
      }
      if (kind === 'app' && !trimmedRepoURL) {
        setCreateError('Repository URL is required for applications.');
        return;
      }
      setCreatePending(true);
      setCreateError(null);
      try {
        await createTeamItem({
          kind,
          name: trimmedName,
          description: kind === 'team' ? trimmedDescription || undefined : undefined,
          repoURL: kind === 'app' ? trimmedRepoURL : undefined,
          parentID,
        });
        setCreateOpen(false);
        await loadTeams();
        window.dispatchEvent(new Event('nopsai-resource-teams-changed'));
      } catch (error) {
        setCreateError(error instanceof Error ? error.message : 'Unable to create team item');
      } finally {
        setCreatePending(false);
      }
    },
    [loadTeams]
  );

  const deleteTeamItem = useCallback(
    async (team: Team) => {
      const label = teamDisplayName(team);
      const noun = isAppTeam(team) ? 'application' : 'team';
      if (!window.confirm(`Delete ${noun} "${label}"? Runs remain in history.`)) return;
      try {
        const endpoint = isAppTeam(team)
          ? `/v1/teams/${encodeURIComponent(team.parent_id == null ? 'root' : String(team.parent_id))}/applications/${encodeURIComponent(String(team.id))}`
          : `/v1/teams/${encodeURIComponent(String(team.id))}`;
        await fetchJson<void>(endpoint, { method: 'DELETE' });
        if (activeTeamID === team.id) selectTeam(null);
        await loadTeams();
        window.dispatchEvent(new Event('nopsai-resource-teams-changed'));
      } catch (error) {
        alert(error instanceof Error ? error.message : `Unable to delete ${noun}`);
      }
    },
    [activeTeamID, fetchJson, loadTeams, selectTeam]
  );

  const submitEditedTeamItem = useCallback(
    async ({ name, description, repoURL, parentID }: TeamItemEditPayload) => {
      if (!editTeam) return;
      const trimmedName = name.trim();
      const trimmedDescription = description.trim();
      const trimmedRepoURL = repoURL.trim();
      const app = isAppTeam(editTeam);
      if (!trimmedName) {
        setEditError(app ? 'Application name is required.' : 'Team name is required.');
        return;
      }
      if (!app && isReservedRootTeamName(trimmedName)) {
        setEditError('Global is reserved and cannot be used as a team name.');
        return;
      }
      if (app && !trimmedRepoURL) {
        setEditError('Repository URL is required for applications.');
        return;
      }

      setEditPending(true);
      setEditError(null);
      try {
        await updateTeamItem(editTeam, {
          name: trimmedName,
          description: app ? undefined : trimmedDescription,
          repoURL: app ? trimmedRepoURL : undefined,
          parentID,
        });
        const loadedTeams = await loadTeams();
        const updatedTeam = loadedTeams.find(team => team.id === editTeam.id) || null;
        if (updatedTeam) {
          navigate(teamScopedRoute('/teams', teamPathForURL(updatedTeam, loadedTeams)), { replace: true });
        }
        setEditTeam(null);
        window.dispatchEvent(new Event('nopsai-resource-teams-changed'));
      } catch (error) {
        setEditError(error instanceof Error ? error.message : 'Unable to update team item');
      } finally {
        setEditPending(false);
      }
    },
    [editTeam, loadTeams, navigate]
  );

  const editParentOptions = useMemo(
    () => (editTeam ? getTeamMoveParentOptions(teams, editTeam) : []),
    [editTeam, teams]
  );
  const createParentOptions = useMemo(() => getTeamCreateParentOptions(teams), [teams]);
  const defaultCreateParentID = useMemo(
    () => {
      if (!activeTeam) return null;
      return isAppTeam(activeTeam) ? activeTeam.parent_id ?? null : activeTeam.id;
    },
    [activeTeam]
  );

  return (
    <div data-page="teams" className="active min-h-full">
      {teamsError ? (
        <div className="teams-page-shell">
          <TeamsStatusPanel
            tone="error"
            icon={<RefreshCw className="h-5 w-5" aria-hidden="true" />}
            title="Teams could not load"
            message={teamsError}
            actionLabel="Retry"
            onAction={() => void loadTeams()}
          />
        </div>
      ) : (!teamsLoaded || (teamsLoading && teams.length === 0)) ? (
        <div className="teams-page-shell">
          <TeamsStatusPanel
            icon={<RefreshCw className="h-5 w-5 animate-spin" aria-hidden="true" />}
            title="Loading teams"
            message="Fetching the team hierarchy and visible applications for your account."
          />
        </div>
      ) : teams.length === 0 ? (
        <div className="teams-page-shell">
          <TeamsStatusPanel
            icon={<UsersRound className="h-5 w-5" aria-hidden="true" />}
            title="No visible teams"
            message="Teams appear here after a team is created or when your account has access to existing teams."
            actionLabel="Create team"
            onAction={() => openCreateModal('team')}
          />
        </div>
      ) : (
        <TeamsWorkspace
          teams={teams}
          activeTeam={activeTeam}
          activeTeamPath={activeTeamPath}
          searchTerm={searchTerm}
          onSearchTermChange={setSearchTerm}
          onSelectTeam={selectTeam}
          onCreate={openCreateModal}
          onEditTeam={openEditModal}
          onDeleteTeam={team => void deleteTeamItem(team)}
          onOpenConfig={config.openTeamConfigRepository}
          onSaveTeamDefaults={saveTeamDefaults}
          operationsSummary={operationsSummary}
          resourceCatalog={resourceCatalog}
          runnerStatus={runnerAssignments.status}
          runnerStatusLoading={runnerAssignments.loading}
          runnerStatusError={runnerAssignments.error}
          currentUser={currentUser}
        />
      )}

      {createOpen && (
        <NewTeamItemModal
          open={createOpen}
          parentLabel={activeTeamLabel}
          parentOptions={createParentOptions}
          defaultParentID={defaultCreateParentID}
          initialKind={createKind}
          error={createError}
          pending={createPending}
          onClose={() => {
            setCreateOpen(false);
            setCreateError(null);
            setCreatePending(false);
          }}
          onSubmit={submitNewTeamItem}
        />
      )}

      {editTeam && (
        <EditTeamItemModal
          open={Boolean(editTeam)}
          team={editTeam}
          parentOptions={editParentOptions}
          error={editError}
          pending={editPending}
          onClose={closeEditModal}
          onSubmit={submitEditedTeamItem}
        />
      )}

      {config.configRepoTeam && (
        <TeamConfigRepositoryModal
          teamLabel={config.configRepoTeam.teamPath}
          repo={config.configRepo}
          form={config.configRepoForm}
          loading={config.configRepoLoading}
          saving={config.configRepoSaving}
          syncing={config.configRepoSyncing}
          error={config.configRepoError}
          driftLoading={config.configRepoDriftLoading}
          notificationRoute={config.notificationRoute}
          notificationForm={config.notificationRouteForm}
          notificationLoading={config.notificationRouteLoading}
          notificationSaving={config.notificationRouteSaving}
          notificationError={config.notificationRouteError}
          initialTab={config.configRepoInitialTab}
          canManage={config.configRepoManageAllowed}
          canSync={config.configRepoSyncAllowed}
          onChange={config.setConfigRepoForm}
          onNotificationChange={config.setNotificationRouteForm}
          onSave={config.saveTeamConfigRepository}
          onDelete={config.deleteTeamConfigRepository}
          onSync={config.syncTeamConfigRepository}
          onCancelSync={config.cancelTeamConfigRepositorySync}
          onCheckDrift={config.checkTeamConfigRepositoryDrift}
          onSaveNotification={config.saveTeamNotificationRoute}
          onDeleteNotification={config.deleteTeamNotificationRoute}
          onClose={config.closeTeamConfigRepository}
        />
      )}

      {config.configRepoTeam && config.configRepoDriftOpen && (
        <ConfigRepositoryDriftModal
          title={`${config.configRepoTeam.teamPath} config repository`}
          drift={config.configRepoDrift}
          loading={config.configRepoDriftLoading}
          error={config.configRepoDriftError}
          pushing={config.configRepoPushing}
          pushResult={config.configRepoPushResult}
          canPush={config.configRepoManageAllowed && Boolean(config.configRepoDrift?.can_push)}
          onClose={() => config.setConfigRepoDriftOpen(false)}
          onRefresh={config.checkTeamConfigRepositoryDrift}
          onPush={config.pushTeamConfigRepositoryDrift}
        />
      )}
    </div>
  );
}
