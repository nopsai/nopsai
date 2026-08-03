import { useEffect, useMemo, useState } from 'react';
import { checkStepPermission } from './api';
import { normalizeRootPath, splitIdentifier } from './model';

const STEP_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildPermissionProbeIdentifier(team: string) {
  const cleaned = team.trim().replace(/^\/+|\/+$/g, '');
  return cleaned ? `${cleaned}/${STEP_PERMISSION_PROBE_NAME}` : STEP_PERMISSION_PROBE_NAME;
}

export function useStepPermissions(selectedID: string | null, activeTeam: string) {
  const [createPermission, setCreatePermission] = useState<{ team: string; allowed: boolean } | null>(null);
  const [updatePermission, setUpdatePermission] = useState<{ id: string; allowed: boolean } | null>(null);
  const permissionTeam = useMemo(
    () => normalizeRootPath(selectedID ? splitIdentifier(selectedID).path : activeTeam),
    [activeTeam, selectedID]
  );

  useEffect(() => {
    let cancelled = false;
    void checkStepPermission('step.create', buildPermissionProbeIdentifier(permissionTeam))
      .then(allowed => {
        if (!cancelled) setCreatePermission({ team: permissionTeam, allowed });
      })
      .catch(() => {
        if (!cancelled) setCreatePermission({ team: permissionTeam, allowed: false });
      });
    return () => {
      cancelled = true;
    };
  }, [permissionTeam]);

  useEffect(() => {
    let cancelled = false;
    if (!selectedID) {
      return () => {
        cancelled = true;
      };
    }

    void checkStepPermission('step.update', selectedID)
      .then(allowed => {
        if (!cancelled) setUpdatePermission({ id: selectedID, allowed });
      })
      .catch(() => {
        if (!cancelled) setUpdatePermission({ id: selectedID, allowed: false });
      });
    return () => {
      cancelled = true;
    };
  }, [selectedID]);

  return {
    permissionTeam,
    canCreateStepHere: Boolean(
      createPermission?.team === permissionTeam && createPermission.allowed
    ),
    canUpdateSelectedStep: Boolean(
      updatePermission?.id === selectedID && updatePermission.allowed
    ),
  };
}
