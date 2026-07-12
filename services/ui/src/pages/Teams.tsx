import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { RefreshCw, UsersRound } from 'lucide-react';
import { useLocation, useNavigate } from 'react-router-dom';
import { ConfigRepositoryDriftModal } from '../components/ConfigRepositoryDriftModal';
import { useAuth } from '../auth/AuthContext';
import { fetchTeams, requestTeamsJson } from '../features/teams/api';
import { TeamConfigRepositoryModal, NewTeamItemModal } from '../features/teams/TeamSettingsModals';
import { TeamsStatusPanel, TeamsWorkspace } from '../features/teams/TeamsWorkspace';
import { useTeamConfigRepositoryController } from '../features/teams/hooks/useTeamConfigRepositoryController';
import { useTeamOperationsSummary } from '../features/teams/hooks/useTeamOperationsSummary';
import { useTeamResourceCatalog } from '../features/teams/hooks/useTeamResourceCatalog';
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
};

function isReservedRootTeamName(name: string) {
  const normalized = name.trim().replace(/^\/+|\/+$/g, '').toLowerCase();
  return normalized === 'root' || normalized === '__general__';
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
  const [createError, setCreateError] = useState<string | null>(null);
  const [createPending, setCreatePending] = useState(false);
  const selectedBeforeCreateRef = useRef<number | null>(null);

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
  });
  const resourceCatalog = useTeamResourceCatalog({ teamPath: activeTeamURLValue });

  const loadTeams = useCallback(async () => {
    setTeamsLoaded(false);
    setTeamsLoading(true);
    setTeamsError(null);
    try {
      setTeams(await fetchTeams());
    } catch (error) {
      setTeamsError(error instanceof Error ? error.message : 'Unable to load teams');
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

  const openCreateModal = useCallback(() => {
    selectedBeforeCreateRef.current = activeTeamID;
    setCreateError(null);
    setCreateOpen(true);
  }, [activeTeamID]);

  const submitNewTeamItem = useCallback(
    async ({ kind, name, description, repoURL }: TeamItemPayload) => {
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

      const parentID = selectedBeforeCreateRef.current;
      setCreatePending(true);
      setCreateError(null);
      try {
        const endpoint = kind === 'team'
          ? '/v1/teams'
          : `/v1/teams/${encodeURIComponent(parentID == null ? 'root' : String(parentID))}/applications`;
        await fetchJson(endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: trimmedName,
            description: kind === 'team' ? trimmedDescription || undefined : undefined,
            repo_url: kind === 'app' ? trimmedRepoURL : undefined,
            parent_team_id: kind === 'team' ? parentID : undefined,
          }),
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
    [fetchJson, loadTeams]
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
            onAction={openCreateModal}
          />
        </div>
      ) : (
        <TeamsWorkspace
          teams={teams}
          activeTeam={activeTeam}
          activeTeamPath={activeTeamPath}
          searchTerm={searchTerm}
          teamsLoading={teamsLoading}
          onSearchTermChange={setSearchTerm}
          onSelectTeam={selectTeam}
          onRefresh={() => void loadTeams()}
          onCreate={openCreateModal}
          onDeleteTeam={team => void deleteTeamItem(team)}
          onOpenConfig={config.openTeamConfigRepository}
          operationsSummary={operationsSummary}
          resourceCatalog={resourceCatalog}
          currentUser={currentUser}
        />
      )}

      {createOpen && (
        <NewTeamItemModal
          open={createOpen}
          parentLabel={activeTeamLabel}
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
