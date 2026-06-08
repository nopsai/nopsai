import { useEffect, useState } from 'react';
import { checkTriggerPermission } from './api';

const TRIGGER_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildPermissionProbeRepository(folder: string) {
  const cleaned = folder.trim().replace(/^\/+|\/+$/g, '');
  return cleaned ? `${cleaned}/${TRIGGER_PERMISSION_PROBE_NAME}` : TRIGGER_PERMISSION_PROBE_NAME;
}

export function useTriggerPermissions(permissionFolder: string, selectedSlug: string | null) {
  const [createPermission, setCreatePermission] = useState<{ folder: string; allowed: boolean } | null>(null);
  const [updatePermission, setUpdatePermission] = useState<{ slug: string; allowed: boolean } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void checkTriggerPermission('trigger.update', buildPermissionProbeRepository(permissionFolder))
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
    if (!selectedSlug) {
      return () => {
        cancelled = true;
      };
    }

    void checkTriggerPermission('trigger.update', selectedSlug)
      .then(allowed => {
        if (!cancelled) setUpdatePermission({ slug: selectedSlug, allowed });
      })
      .catch(() => {
        if (!cancelled) setUpdatePermission({ slug: selectedSlug, allowed: false });
      });
    return () => {
      cancelled = true;
    };
  }, [selectedSlug]);

  return {
    canCreateTriggerHere: Boolean(
      createPermission?.folder === permissionFolder && createPermission.allowed
    ),
    canUpdateSelectedTrigger: Boolean(
      updatePermission?.slug === selectedSlug && updatePermission.allowed
    ),
  };
}
