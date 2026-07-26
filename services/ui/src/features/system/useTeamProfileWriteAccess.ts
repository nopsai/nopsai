import { useEffect, useState } from 'react';
import { normalizeAIResourceTeamPath } from './aiResourceTeams';
import { requestTeamsJson } from './teamProfileApi';

export function useTeamProfileWriteAccess(teamPath: string) {
  const [allowed, setAllowed] = useState(false);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const normalizedTeamPath = normalizeAIResourceTeamPath(teamPath);
    if (!normalizedTeamPath) {
      setAllowed(false);
      setLoading(false);
      return () => {
        cancelled = true;
      };
    }

    const params = new URLSearchParams({
      action: 'team.update',
      resource_type: 'team',
      resource_id: normalizedTeamPath,
    });
    setLoading(true);
    requestTeamsJson<{ allowed?: boolean }>(`/v1/access/effective-permissions?${params.toString()}`)
      .then(payload => {
        if (!cancelled) setAllowed(Boolean(payload?.allowed));
      })
      .catch(() => {
        if (!cancelled) setAllowed(false);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [teamPath]);

  return { allowed, loading };
}
