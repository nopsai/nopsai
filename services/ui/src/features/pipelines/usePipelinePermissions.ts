import { useEffect, useMemo, useState } from 'react';
import { checkPipelinePermission } from './api';
import { splitIdentifier } from './model';

const PIPELINE_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildPermissionProbeIdentifier(folder: string) {
  const cleaned = folder.trim().replace(/^\/+|\/+$/g, '');
  return cleaned ? `${cleaned}/${PIPELINE_PERMISSION_PROBE_NAME}` : PIPELINE_PERMISSION_PROBE_NAME;
}

export function usePipelinePermissions(selectedID: string | null, activeFolder: string) {
  const [createPermission, setCreatePermission] = useState<{ folder: string; allowed: boolean } | null>(null);
  const [selectedPermissions, setSelectedPermissions] = useState<{
    id: string;
    canUpdate: boolean;
    canExecute: boolean;
  } | null>(null);
  const permissionFolder = useMemo(
    () => (selectedID ? splitIdentifier(selectedID).path : activeFolder),
    [activeFolder, selectedID]
  );

  useEffect(() => {
    let cancelled = false;
    void checkPipelinePermission('pipeline.create', buildPermissionProbeIdentifier(permissionFolder))
      .then(allowed => {
        if (!cancelled) setCreatePermission({ folder: permissionFolder, allowed });
      })
      .catch(() => {
        if (!cancelled) setCreatePermission({ folder: permissionFolder, allowed: false });
      });
    return () => {
      cancelled = true;
    };
  }, [permissionFolder]);

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
    permissionFolder,
    canCreatePipelineHere: Boolean(
      createPermission?.folder === permissionFolder && createPermission.allowed
    ),
    canUpdateSelectedPipeline: Boolean(
      selectedPermissions?.id === selectedID && selectedPermissions.canUpdate
    ),
    canExecuteSelectedPipeline: Boolean(
      selectedPermissions?.id === selectedID && selectedPermissions.canExecute
    ),
  };
}
