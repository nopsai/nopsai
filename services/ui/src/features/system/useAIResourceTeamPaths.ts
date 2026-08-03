import { useCallback, useEffect, useState } from 'react';
import { fetchResourceTeamPaths, isGlobalResourceTeamPath } from '../../lib/resourceTeams';

export function useAIResourceTeamPaths() {
  const [teamPaths, setTeamPaths] = useState<string[]>([]);
  const [teamPathsLoading, setTeamPathsLoading] = useState(false);

  const loadTeamPaths = useCallback(async () => {
    setTeamPathsLoading(true);
    try {
      const paths = await fetchResourceTeamPaths();
      setTeamPaths(paths.filter(path => !isGlobalResourceTeamPath(path)));
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
