import { useEffect, useState } from 'react';
import type { AccessResourceCatalog } from './resourceCatalog';
import {
  AAA_ANY_SCOPE_VALUE,
  AAA_CUSTOM_VALUE,
  AAA_RESOURCE_TYPE_CONFIGS,
  buildAAANamedResourceSelector,
  buildAAAResourceSelector,
  buildAAAResourceTargetOptionGroups,
  customAAAActionPlaceholder,
  denormalizeAAAScopeOptionValue,
  flattenAAAOptionGroups,
  formatAAAActionValue,
  getAAAActionOptionGroups,
  getAAAResourceTypeConfig,
  normalizeAAAActionForResource,
  normalizeAAAScopeOptionValue,
  parseAAAActionValue,
  parseAAANamedResourceID,
  parseAAAResourceSelector,
  selectValueForAAAOptions,
  type AAAEffect,
  type AAANamedResourceDraft,
} from './policyRuleModel';

export function AccessPolicyRuleFields({
  policy,
  onChange,
  resourceCatalog,
}: {
  policy: { name: string; obj: string; act: string };
  onChange: (next: { name: string; obj: string; act: string }) => void;
  resourceCatalog: AccessResourceCatalog;
}) {
  const normalizedResource = (policy.obj || '').trim();
  const parsedResource = parseAAAResourceSelector(normalizedResource);
  const parsedAction = parseAAAActionValue(policy.act);
  const resourceTypeConfig = getAAAResourceTypeConfig(parsedResource.resourceType);
  const selectedResourceType = resourceTypeConfig ? resourceTypeConfig.value : AAA_CUSTOM_VALUE;
  const isNamedScopedResourceType = resourceTypeConfig?.value === 'secret' || resourceTypeConfig?.value === 'variable';
  const [forceCustomNamedScope, setForceCustomNamedScope] = useState(false);
  const namedResourceParts = isNamedScopedResourceType ? parseAAANamedResourceID(parsedResource.resourceID) : { repoName: '', scope: '', name: '', hasScope: false };
  const namedScopeOptions =
    resourceTypeConfig?.value === 'secret'
      ? resourceCatalog.secretScopeOptions
      : resourceTypeConfig?.value === 'variable'
        ? resourceCatalog.variableScopeOptions
        : [];
  const resourceTargetOptionGroups =
    resourceTypeConfig && !isNamedScopedResourceType ? buildAAAResourceTargetOptionGroups(resourceTypeConfig, resourceCatalog) : [];
  const resourceTargetOptions = flattenAAAOptionGroups(resourceTargetOptionGroups);
  const selectedResourceTarget =
    isNamedScopedResourceType
      ? ''
      : selectedResourceType === '*'
      ? '*'
      : selectedResourceType !== AAA_CUSTOM_VALUE && resourceTargetOptions.some(option => option.value === parsedResource.resourceID)
      ? parsedResource.resourceID
      : AAA_CUSTOM_VALUE;
  const normalizedNamedScope = normalizeAAAScopeOptionValue(namedResourceParts.scope);
  const derivedSelectedNamedScope =
    !isNamedScopedResourceType
      ? ''
      : !namedResourceParts.hasScope
        ? AAA_ANY_SCOPE_VALUE
        : namedScopeOptions.some(option => option.value === normalizedNamedScope)
          ? normalizedNamedScope
          : AAA_CUSTOM_VALUE;
  const selectedNamedScope = forceCustomNamedScope && isNamedScopedResourceType ? AAA_CUSTOM_VALUE : derivedSelectedNamedScope;
  const allowCustomTarget = resourceTypeConfig?.value !== '*';
  const selectedResourceTypeValue = resourceTypeConfig?.value || '';
  const selectedNamedScopeValue = namedResourceParts.scope;
  const selectedNamedScopeHasScope = namedResourceParts.hasScope;
  const actionOptions = getAAAActionOptionGroups(normalizedResource);
  const selectedAction = selectValueForAAAOptions(actionOptions, parsedAction.action);
  const customResourceDraft =
    selectedResourceType === AAA_CUSTOM_VALUE
      ? normalizedResource
      : normalizedResource.endsWith(':')
        ? ''
        : parsedResource.resourceID;
  const customNamedScopeDraft = selectedNamedScope === AAA_CUSTOM_VALUE ? selectedNamedScopeValue : '';
  const buildNamedResourceSelector = (next: Partial<AAANamedResourceDraft>) =>
    buildAAANamedResourceSelector(selectedResourceTypeValue, {
      repoName: '',
      scope: 'scope' in next ? next.scope ?? '' : selectedNamedScopeValue,
      name: '',
      hasScope: 'hasScope' in next ? Boolean(next.hasScope) : selectedNamedScopeHasScope,
    });
  const hasNamedResourceItemFilter = isNamedScopedResourceType && Boolean(namedResourceParts.repoName || namedResourceParts.name);

  useEffect(() => {
    if (!isNamedScopedResourceType) {
      const handle = window.setTimeout(() => setForceCustomNamedScope(false), 0);
      return () => window.clearTimeout(handle);
    }
    return undefined;
  }, [isNamedScopedResourceType]);

  useEffect(() => {
    if (!hasNamedResourceItemFilter || !resourceTypeConfig) return;
    const nextObj = buildAAANamedResourceSelector(selectedResourceTypeValue, {
      repoName: '',
      scope: selectedNamedScopeValue,
      name: '',
      hasScope: selectedNamedScopeHasScope,
    });
    if (nextObj === normalizedResource) return;
    onChange({
      name: policy.name,
      obj: nextObj,
      act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
    });
  }, [
    hasNamedResourceItemFilter,
    normalizedResource,
    onChange,
    parsedAction.action,
    parsedAction.effect,
    policy.name,
    resourceTypeConfig,
    selectedNamedScopeHasScope,
    selectedNamedScopeValue,
    selectedResourceTypeValue,
  ]);

  return (
    <>
      <label className="flex flex-col gap-1 text-sm">
        <span>Policy label</span>
        <input
          className="pipelines-input"
          value={policy.name}
          onChange={e => onChange({ name: e.target.value, obj: policy.obj, act: policy.act })}
          placeholder="Pipeline reader"
          required
        />
      </label>
      <div className="grid gap-3 md:grid-cols-[0.56fr_1fr]">
        <label className="flex flex-col gap-1 text-sm">
          <span>Resource type</span>
          <select
            className="pipelines-input"
            value={selectedResourceType}
            onChange={e => {
              const nextType = e.target.value;
              if (nextType === AAA_CUSTOM_VALUE) {
                onChange({ name: policy.name, obj: normalizedResource, act: policy.act });
                return;
              }
              const nextConfig = getAAAResourceTypeConfig(nextType);
              const nextTarget = nextConfig?.allowAll ? '*' : nextConfig?.presets?.[0]?.value || '';
              const nextObj = buildAAAResourceSelector(nextType, nextTarget, { preserveEmpty: nextTarget === '' });
              onChange({
                name: policy.name,
                obj: nextObj,
                act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
              });
            }}
          >
            {AAA_RESOURCE_TYPE_CONFIGS.map(option => (
              <option key={`resource-type-${option.value}`} value={option.value}>
                {option.label}
              </option>
            ))}
            {selectedResourceType === AAA_CUSTOM_VALUE && (
              <option value={AAA_CUSTOM_VALUE} disabled>
                Unsupported selector
              </option>
            )}
          </select>
        </label>
        {selectedResourceType === AAA_CUSTOM_VALUE ? (
          <label className="flex flex-col gap-1 text-sm">
            <span>Resource selector</span>
            <input
              className="pipelines-input"
              value={normalizedResource}
              onChange={e => onChange({ name: policy.name, obj: e.target.value, act: policy.act })}
              placeholder="pipeline:team/build"
              required
            />
          </label>
        ) : isNamedScopedResourceType ? (
          <div className="space-y-3">
            <label className="flex flex-col gap-1 text-sm">
              <span>{resourceTypeConfig?.targetLabel || 'Scope'}</span>
              <select
                className="pipelines-input"
                value={selectedNamedScope}
                onChange={e => {
                  const value = e.target.value;
                  if (!resourceTypeConfig) return;
                  if (value === AAA_ANY_SCOPE_VALUE) {
                    setForceCustomNamedScope(false);
                    const nextObj = buildNamedResourceSelector({ hasScope: false, scope: '' });
                    onChange({
                      name: policy.name,
                      obj: nextObj,
                      act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
                    });
                    return;
                  }
                  if (value === AAA_CUSTOM_VALUE) {
                    setForceCustomNamedScope(true);
                    return;
                  }
                  setForceCustomNamedScope(false);
                  const nextObj = buildNamedResourceSelector({
                    hasScope: true,
                    scope: denormalizeAAAScopeOptionValue(value),
                  });
                  onChange({
                    name: policy.name,
                    obj: nextObj,
                    act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
                  });
                }}
              >
                <option value={AAA_ANY_SCOPE_VALUE}>Any scope</option>
                {namedScopeOptions.map(option => (
                  <option key={`resource-scope-${resourceTypeConfig?.value}-${option.value}`} value={option.value}>
                    {option.label}
                  </option>
                ))}
                <option value={AAA_CUSTOM_VALUE}>Custom scope…</option>
              </select>
            </label>
            {selectedNamedScope === AAA_CUSTOM_VALUE && (
              <label className="flex flex-col gap-1 text-sm">
                <span>Custom scope</span>
                <input
                  className="pipelines-input"
                  value={customNamedScopeDraft}
                  onChange={e =>
                    onChange({
                      name: policy.name,
                      obj: buildNamedResourceSelector({
                        hasScope: true,
                        scope: e.target.value,
                      }),
                      act: policy.act,
                    })
                  }
                  placeholder="prod"
                />
              </label>
            )}
          </div>
        ) : (
          <label className="flex flex-col gap-1 text-sm">
            <span>{resourceTypeConfig?.targetLabel || 'Target'}</span>
            <select
              className="pipelines-input"
              value={selectedResourceTarget}
              onChange={e => {
                const value = e.target.value;
                if (!resourceTypeConfig) return;
                if (value === AAA_CUSTOM_VALUE) {
                  const nextObj = buildAAAResourceSelector(resourceTypeConfig.value, parsedResource.resourceID === '*' ? '' : parsedResource.resourceID, {
                    preserveEmpty: true,
                  });
                  onChange({ name: policy.name, obj: nextObj, act: policy.act });
                  return;
                }
                const nextObj = buildAAAResourceSelector(resourceTypeConfig.value, value);
                onChange({
                  name: policy.name,
                  obj: nextObj,
                  act: normalizeAAAActionForResource(nextObj, parsedAction.action, parsedAction.effect),
                });
              }}
            >
              {resourceTargetOptionGroups.map(group => (
                <optgroup key={`resource-target-group-${resourceTypeConfig?.value}-${group.label}`} label={group.label}>
                  {group.options.map(option => (
                    <option key={`resource-target-${resourceTypeConfig?.value}-${option.value}`} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </optgroup>
              ))}
              {allowCustomTarget && <option value={AAA_CUSTOM_VALUE}>Custom…</option>}
            </select>
            {allowCustomTarget && selectedResourceTarget === AAA_CUSTOM_VALUE && (
              <input
                className="pipelines-input"
                value={customResourceDraft}
                onChange={e =>
                  onChange({
                    name: policy.name,
                    obj: buildAAAResourceSelector(resourceTypeConfig?.value || '', e.target.value, { preserveEmpty: true }),
                    act: policy.act,
                  })
                }
                placeholder={resourceTypeConfig?.customPlaceholder || 'team/build'}
                required
              />
            )}
          </label>
        )}
      </div>
      <div className="grid gap-3 md:grid-cols-[0.42fr_1fr]">
        <label className="flex flex-col gap-1 text-sm">
          <span>Effect</span>
          <select
            className="pipelines-input"
            value={parsedAction.effect}
            onChange={e => onChange({ name: policy.name, obj: policy.obj, act: formatAAAActionValue(e.target.value as AAAEffect, parsedAction.action) })}
          >
            <option value="allow">Allow</option>
            <option value="deny">Deny</option>
          </select>
        </label>
        <label className="flex flex-col gap-1 text-sm">
          <span>Action</span>
          <select
            className="pipelines-input"
            value={selectedAction}
            onChange={e => {
              const value = e.target.value;
              if (value === AAA_CUSTOM_VALUE) {
                onChange({
                  name: policy.name,
                  obj: policy.obj,
                  act: formatAAAActionValue(parsedAction.effect, selectedAction === AAA_CUSTOM_VALUE ? parsedAction.action : ''),
                });
                return;
              }
              onChange({ name: policy.name, obj: policy.obj, act: formatAAAActionValue(parsedAction.effect, value) });
            }}
          >
            {actionOptions.map(group => (
              <optgroup key={`action-${group.label}`} label={group.label}>
                {group.options.map(option => (
                  <option key={`action-${group.label}-${option.value}`} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </optgroup>
            ))}
            <option value={AAA_CUSTOM_VALUE}>Custom action…</option>
          </select>
          {selectedAction === AAA_CUSTOM_VALUE && (
            <input
              className="pipelines-input"
              value={parsedAction.action}
              onChange={e => onChange({ name: policy.name, obj: policy.obj, act: formatAAAActionValue(parsedAction.effect, e.target.value) })}
              placeholder={customAAAActionPlaceholder(normalizedResource)}
              required
            />
          )}
        </label>
      </div>
    </>
  );
}

