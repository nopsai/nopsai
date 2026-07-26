import { useEffect, useState } from 'react';
import { normalizeAIResourceTeamPath } from './aiResourceTeams';
import { requestTeamsJson } from './teamProfileApi';

type TeamProfileWriteAccessState = {
  teamPath: string;
  allowed: boolean;
  loaded: boolean;
};

export function useTeamProfileWriteAccess(teamPath: string) {
  const normalizedTeamPath = normalizeAIResourceTeamPath(teamPath);
  const [access, setAccess] = useState<TeamProfileWriteAccessState>(() => ({
    teamPath: normalizedTeamPath,
    allowed: false,
    loaded: false,
  }));

  useEffect(() => {
    if (!normalizedTeamPath) {
      return undefined;
    }

    let cancelled = false;
    const params = new URLSearchParams({
      action: 'team.update',
      resource_type: 'team',
      resource_id: normalizedTeamPath,
    });
    requestTeamsJson<{ allowed?: boolean }>(`/v1/access/effective-permissions?${params.toString()}`)
      .then(payload => {
        if (!cancelled) setAccess({ teamPath: normalizedTeamPath, allowed: Boolean(payload?.allowed), loaded: true });
      })
      .catch(() => {
        if (!cancelled) setAccess({ teamPath: normalizedTeamPath, allowed: false, loaded: true });
      });

    return () => {
      cancelled = true;
    };
  }, [normalizedTeamPath]);

  if (!normalizedTeamPath) {
    return { allowed: false, loading: false };
  }
  if (access.teamPath !== normalizedTeamPath) {
    return { allowed: false, loading: true };
  }

  return { allowed: access.allowed, loading: !access.loaded };
}
