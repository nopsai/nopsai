import { describe, expect, it } from 'vitest';
import {
  createEmptyNotificationRouteForm,
  defaultNotificationEventState,
  defaultNotificationRouteDefinition,
  defaultNotificationRouteRule,
  normalizeNotificationRouteDefinition,
  normalizeNotificationRouteRecord,
  normalizeNotificationRouteRule,
  notificationRouteFormAddRoute,
  notificationRouteFormFromDefinition,
  notificationRouteFormRemoveSelectedRoute,
  notificationRouteFormSelectRoute,
  notificationRoutePayloadFromForm,
} from './notificationRoutes';

describe('notification routes', () => {
  it('creates enterprise-safe defaults', () => {
    expect(defaultNotificationEventState()).toMatchObject({
      failure: true,
      waiting_approval: true,
      approval_rejected: true,
      success: false,
    });
    expect(defaultNotificationRouteRule('security').name).toBe('security');
    expect(defaultNotificationRouteDefinition().routes).toHaveLength(1);
    expect(createEmptyNotificationRouteForm()).toMatchObject({
      routeName: 'default',
      includeSameTeam: true,
      pipelineInclude: '*',
      maxPerRun: '5',
    });
  });

  it('normalizes legacy and malformed definitions into route rules', () => {
    const record = normalizeNotificationRouteRecord({
      id: '4',
      team_id: 7,
      definition: {
        enabled: true,
        recipients: { include: { teams: ['same_team'], users: ['ops@example.test'] } },
        events: { failure: true, success: true },
        filters: { pipelines: { include: ['deploy/*'] } },
        delivery: { channels: ['mail'], throttle: { dedupe_window: '15m', max_per_run: 3 } },
      },
    });

    expect(record.id).toBe(4);
    expect(record.definition.routes).toHaveLength(1);
    expect(record.definition.routes?.[0].events.success).toBe(true);
    expect(record.definition.routes?.[0].filters.pipelines?.include).toEqual(['deploy/*']);
    expect(normalizeNotificationRouteRecord(null)).toMatchObject({ source: 'database', managed_by_config_repo: false });
    expect(normalizeNotificationRouteDefinition(null).routes).toHaveLength(1);
    expect(normalizeNotificationRouteRule({ name: ' ', delivery: { channels: [] } }, 'fallback')).toMatchObject({
      name: 'fallback',
      enabled: true,
      delivery: { channels: ['mail'], throttle: { dedupe_window: '10m', max_per_run: 5 } },
    });
  });

  it('round-trips and edits multi-route forms', () => {
    let form = createEmptyNotificationRouteForm();
    form = {
      ...form,
      includeUsers: 'Ops@example.test, ops@example.test\nowner@example.test',
      maxPerRun: '8',
    };
    form = notificationRouteFormAddRoute(form);
    expect(form.routes).toHaveLength(2);
    expect(form.selectedRouteIndex).toBe(1);

    form = { ...form, routeName: 'security', enabled: false, branchInclude: 'main\nrelease/*' };
    form = notificationRouteFormSelectRoute(form, 0);
    const payload = notificationRoutePayloadFromForm(form);
    expect(payload.routes?.[1].name).toBe('security');
    expect(payload.routes?.[1].enabled).toBe(false);
    expect(payload.routes?.[0].recipients.include?.users).toEqual(['Ops@example.test', 'owner@example.test']);
    expect(payload.routes?.[0].delivery.throttle?.max_per_run).toBe(8);

    const restored = notificationRouteFormFromDefinition(payload);
    expect(restored.routes).toHaveLength(2);
    const reduced = notificationRouteFormRemoveSelectedRoute(notificationRouteFormSelectRoute(restored, 1));
    expect(reduced.routes).toHaveLength(1);
    expect(notificationRouteFormRemoveSelectedRoute(reduced)).toEqual(reduced);
  });
});
