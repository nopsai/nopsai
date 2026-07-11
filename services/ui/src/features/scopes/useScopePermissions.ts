import { useEffect, useState } from 'react';
import { checkScopePermission } from './api';
import { normalizeScopeLabel } from './model';

const SCOPE_PERMISSION_PROBE_NAME = '__nopsai_permission_probe__';

function buildScopePermissionProbe(team: string) {
  const cleaned = normalizeScopeLabel(team);
  return cleaned ? `${cleaned}/${SCOPE_PERMISSION_PROBE_NAME}` : SCOPE_PERMISSION_PROBE_NAME;
}

export function buildNamedResourceID(repoName: string, scope: string, name: string) {
  const params = new URLSearchParams();
  if (repoName.trim()) params.set('repo', repoName.trim());
  if (normalizeScopeLabel(scope)) params.set('scope', normalizeScopeLabel(scope));
  params.set('name', name.trim());
  return params.toString();
}

export function useScopePermissions(activeTeam: string, selectedScope: string | null) {
  const [createPermission, setCreatePermission] = useState<{ team: string; allowed: boolean } | null>(null);
  const [valuePermissions, setValuePermissions] = useState<{
    scope: string;
    canWriteVariables: boolean;
    canWriteSecrets: boolean;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void checkScopePermission('scope.update', 'scope', buildScopePermissionProbe(activeTeam))
      .then(allowed => {
        if (!cancelled) setCreatePermission({ team: activeTeam, allowed });
      })
      .catch(() => {
        if (!cancelled) setCreatePermission({ team: activeTeam, allowed: false });
      });
    return () => {
      cancelled = true;
    };
  }, [activeTeam]);

  useEffect(() => {
    let cancelled = false;
    if (selectedScope == null) {
      return () => {
        cancelled = true;
      };
    }

    const scope = normalizeScopeLabel(selectedScope);
    void Promise.all([
      checkScopePermission(
        'variable.write_value',
        'variable',
        buildNamedResourceID('', scope, SCOPE_PERMISSION_PROBE_NAME)
      ),
      checkScopePermission(
        'secret.write_value',
        'secret',
        buildNamedResourceID('', scope, SCOPE_PERMISSION_PROBE_NAME)
      ),
    ])
      .then(([variableAllowed, secretAllowed]) => {
        if (cancelled) return;
        setValuePermissions({
          scope,
          canWriteVariables: variableAllowed,
          canWriteSecrets: secretAllowed,
        });
      })
      .catch(() => {
        if (cancelled) return;
        setValuePermissions({ scope, canWriteVariables: false, canWriteSecrets: false });
      });
    return () => {
      cancelled = true;
    };
  }, [selectedScope]);

  return {
    canCreateScopeHere: Boolean(
      createPermission?.team === activeTeam && createPermission.allowed
    ),
    canWriteVariablesInSelectedScope: Boolean(
      selectedScope != null &&
      valuePermissions?.scope === normalizeScopeLabel(selectedScope) &&
      valuePermissions.canWriteVariables
    ),
    canWriteSecretsInSelectedScope: Boolean(
      selectedScope != null &&
      valuePermissions?.scope === normalizeScopeLabel(selectedScope) &&
      valuePermissions.canWriteSecrets
    ),
  };
}
