import { useCallback, useMemo } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import AgentProfilesPanel from '../features/system/AgentProfilesPanel';
import { aiResourceRoute, decodeAIResourceRouteID } from '../features/system/aiResourceTeams';

export default function AgentProfilesPage({ canManage }: { canManage: boolean }) {
  const location = useLocation();
  const navigate = useNavigate();
  const selectedProfileID = useMemo(
    () => decodeAIResourceRouteID(location.pathname, 'agent-profiles'),
    [location.pathname]
  );
  const setSelectedProfileID = useCallback((profileID: string) => {
    navigate(aiResourceRoute('/agent-profiles', profileID, new URLSearchParams(location.search)), { preventScrollReset: true });
  }, [location.search, navigate]);

  return (
    <div data-page="agent-profiles" className="active h-full flex flex-col">
      <AgentProfilesPanel
        canManage={canManage}
        selectedProfileID={selectedProfileID}
        onSelectedProfileIDChange={setSelectedProfileID}
      />
    </div>
  );
}
