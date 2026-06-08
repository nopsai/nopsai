import { useEffect, useMemo, useState } from 'react';
import { checkStepPermission } from './api';
import { splitIdentifier } from './model';

const STEP_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildPermissionProbeIdentifier(folder: string) {
  const cleaned = folder.trim().replace(/^\/+|\/+$/g, '');
  return cleaned ? `${cleaned}/${STEP_PERMISSION_PROBE_NAME}` : STEP_PERMISSION_PROBE_NAME;
}

export function useStepPermissions(selectedID: string | null, activeFolder: string) {
  const [createPermission, setCreatePermission] = useState<{ folder: string; allowed: boolean } | null>(null);
  const [updatePermission, setUpdatePermission] = useState<{ id: string; allowed: boolean } | null>(null);
  const permissionFolder = useMemo(
    () => (selectedID ? splitIdentifier(selectedID).path : activeFolder),
    [activeFolder, selectedID]
  );

  useEffect(() => {
    let cancelled = false;
    void checkStepPermission('step.create', buildPermissionProbeIdentifier(permissionFolder))
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
    permissionFolder,
    canCreateStepHere: Boolean(
      createPermission?.folder === permissionFolder && createPermission.allowed
    ),
    canUpdateSelectedStep: Boolean(
      updatePermission?.id === selectedID && updatePermission.allowed
    ),
  };
}
