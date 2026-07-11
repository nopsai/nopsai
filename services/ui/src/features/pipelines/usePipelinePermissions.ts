import { useEffect, useMemo, useState } from 'react';
import { checkPipelinePermission } from './api';
import { splitIdentifier } from './model';

const PIPELINE_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildPermissionProbeIdentifier(team: string) {
  const cleaned = team.trim().replace(/^\/+|\/+$/g, '');
  return cleaned ? `${cleaned}/${PIPELINE_PERMISSION_PROBE_NAME}` : PIPELINE_PERMISSION_PROBE_NAME;
}

export function usePipelinePermissions(selectedID: string | null, activeTeam: string) {
  const [createPermission, setCreatePermission] = useState<{ team: string; allowed: boolean } | null>(null);
  const [selectedPermissions, setSelectedPermissions] = useState<{
    id: string;
    canUpdate: boolean;
    canExecute: boolean;
  } | null>(null);
  const permissionTeam = useMemo(
    () => (selectedID ? splitIdentifier(selectedID).path : activeTeam),
    [activeTeam, selectedID]
  );

  useEffect(() => {
    let cancelled = false;
    void checkPipelinePermission('pipeline.create', buildPermissionProbeIdentifier(permissionTeam))
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

    void Promise.all([
      checkPipelinePermission('pipeline.update', selectedID),
      checkPipelinePermission('pipeline.execute', selectedID),
    ])
      .then(([updateAllowed, executeAllowed]) => {
        if (cancelled) return;
        setSelectedPermissions({
          id: selectedID,
          canUpdate: updateAllowed,
          canExecute: executeAllowed,
        });
      })
      .catch(() => {
        if (cancelled) return;
        setSelectedPermissions({ id: selectedID, canUpdate: false, canExecute: false });
      });
    return () => {
      cancelled = true;
    };
  }, [selectedID]);

  return {
    permissionTeam,
    canCreatePipelineHere: Boolean(
      createPermission?.team === permissionTeam && createPermission.allowed
    ),
    canUpdateSelectedPipeline: Boolean(
      selectedPermissions?.id === selectedID && selectedPermissions.canUpdate
    ),
    canExecuteSelectedPipeline: Boolean(
      selectedPermissions?.id === selectedID && selectedPermissions.canExecute
    ),
  };
}
