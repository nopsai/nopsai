import { useCallback, useMemo } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import LLMProfilesPanel from '../features/system/LLMProfilesPanel';
import { aiResourceRoute, decodeAIResourceRouteID } from '../features/system/aiResourceTeams';

export default function LLMProfilesPage({ canManage }: { canManage: boolean }) {
  const location = useLocation();
  const navigate = useNavigate();
  const selectedProfileName = useMemo(
    () => decodeAIResourceRouteID(location.pathname, 'models'),
    [location.pathname]
  );
  const setSelectedProfileName = useCallback((profileName: string) => {
    navigate(aiResourceRoute('/models', profileName, new URLSearchParams(location.search)), { preventScrollReset: true });
  }, [location.search, navigate]);

  return (
    <div data-page="models" className="active h-full flex flex-col">
      <LLMProfilesPanel
        canManage={canManage}
        selectedProfileName={selectedProfileName}
        onSelectedProfileNameChange={setSelectedProfileName}
      />
    </div>
  );
}
