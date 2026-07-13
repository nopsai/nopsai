import { useEffect, useState } from 'react';
import { checkTriggerPermission } from './api';

const TRIGGER_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildPermissionProbeRepository(owner: string) {
  const cleaned = owner.trim().replace(/^\/+|\/+$/g, '');
  return cleaned ? `${cleaned}/${TRIGGER_PERMISSION_PROBE_NAME}` : TRIGGER_PERMISSION_PROBE_NAME;
}

export function useTriggerPermissions(permissionOwner: string, selectedSlug: string | null) {
  const [createPermission, setCreatePermission] = useState<{ owner: string; allowed: boolean } | null>(null);
  const [updatePermission, setUpdatePermission] = useState<{ slug: string; allowed: boolean } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void checkTriggerPermission('trigger.update', buildPermissionProbeRepository(permissionOwner))
      .then(allowed => {
        if (!cancelled) setCreatePermission({ owner: permissionOwner, allowed });
      })
      .catch(() => {
        if (!cancelled) setCreatePermission({ owner: permissionOwner, allowed: false });
      });
    return () => {
      cancelled = true;
    };
  }, [permissionOwner]);

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
      createPermission?.owner === permissionOwner && createPermission.allowed
    ),
    canUpdateSelectedTrigger: Boolean(
      updatePermission?.slug === selectedSlug && updatePermission.allowed
    ),
  };
}
