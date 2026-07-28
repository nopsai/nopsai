import { useEffect, useState } from 'react';
import { checkTriggerPermission } from './api';

const TRIGGER_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildPermissionProbeRepository(owner: string) {
  // The owner segment here is only a synthetic repository id for the AAA probe.
  // Team-scoped trigger creation is authorized by the `team_path` query param.
  const cleaned = owner.trim().replace(/^\/+|\/+$/g, '');
  return cleaned ? `${cleaned}/${TRIGGER_PERMISSION_PROBE_NAME}` : TRIGGER_PERMISSION_PROBE_NAME;
}

export function useTriggerPermissions(permissionOwner: string, selectedSlug: string | null, permissionTeamPath = '') {
  const [createPermission, setCreatePermission] = useState<{ probeKey: string; allowed: boolean } | null>(null);
  const [updatePermission, setUpdatePermission] = useState<{ slug: string; allowed: boolean } | null>(null);

  useEffect(() => {
    let cancelled = false;
    const probeKey = `${permissionOwner}:${permissionTeamPath}`;
    void checkTriggerPermission('trigger.update', buildPermissionProbeRepository(permissionOwner), permissionTeamPath)
      .then(allowed => {
        if (!cancelled) setCreatePermission({ probeKey, allowed });
      })
      .catch(() => {
        if (!cancelled) setCreatePermission({ probeKey, allowed: false });
      });
    return () => {
      cancelled = true;
    };
  }, [permissionOwner, permissionTeamPath]);

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
      createPermission?.probeKey === `${permissionOwner}:${permissionTeamPath}` && createPermission.allowed
    ),
    canUpdateSelectedTrigger: Boolean(
      updatePermission?.slug === selectedSlug && updatePermission.allowed
    ),
  };
}
