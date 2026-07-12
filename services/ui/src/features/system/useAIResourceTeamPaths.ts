import { useCallback, useEffect, useState } from 'react';
import { fetchResourceTeamPaths } from '../../lib/resourceTeams';

export function useAIResourceTeamPaths() {
  const [teamPaths, setTeamPaths] = useState<string[]>([]);
  const [teamPathsLoading, setTeamPathsLoading] = useState(false);

  const loadTeamPaths = useCallback(async () => {
    setTeamPathsLoading(true);
    try {
      setTeamPaths(await fetchResourceTeamPaths());
    } finally {
      setTeamPathsLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadTeamPaths();
  }, [loadTeamPaths]);

  return {
    teamPaths,
    teamPathsLoading,
    reloadTeamPaths: loadTeamPaths,
  };
}
