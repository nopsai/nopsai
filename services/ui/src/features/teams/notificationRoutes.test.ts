import assert from 'node:assert/strict';
import test from 'node:test';
import {
  createEmptyNotificationRouteForm,
  teamNotificationGitOpsTarget,
  normalizeNotificationRouteRecord,
  notificationRouteFormAddRoute,
  notificationRouteFormFromDefinition,
  notificationRouteFormRemoveSelectedRoute,
  notificationRouteFormSelectRoute,
  notificationRoutePayloadFromForm,
} from './notificationRoutes.js';

test('uses the delegated team repository root for notification GitOps', () => {
  assert.equal(teamNotificationGitOpsTarget(''), 'notifications.yaml');
  assert.equal(teamNotificationGitOpsTarget('/configs/team-1/'), 'configs/team-1/notifications.yaml');
  assert.equal(teamNotificationGitOpsTarget('configs\\team-1'), 'configs/team-1/notifications.yaml');
});

test('normalizes legacy notification definitions into route rules', () => {
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

  assert.equal(record.id, 4);
  assert.equal(record.definition.routes?.length, 1);
  assert.equal(record.definition.routes?.[0].events.success, true);
  assert.deepEqual(record.definition.routes?.[0].filters.pipelines?.include, ['deploy/*']);
});

test('round-trips and edits multi-route notification forms', () => {
  let form = createEmptyNotificationRouteForm();
  form = {
    ...form,
    includeUsers: 'Ops@example.test, ops@example.test\nowner@example.test',
    maxPerRun: '8',
  };
  form = notificationRouteFormAddRoute(form);
  assert.equal(form.routes.length, 2);
  assert.equal(form.selectedRouteIndex, 1);

  form = { ...form, routeName: 'security', enabled: false, branchInclude: 'main\nrelease/*' };
  form = notificationRouteFormSelectRoute(form, 0);
  const payload = notificationRoutePayloadFromForm(form);
  assert.equal(payload.routes?.[1].name, 'security');
  assert.equal(payload.routes?.[1].enabled, false);
  assert.deepEqual(payload.routes?.[0].recipients.include?.users, ['Ops@example.test', 'owner@example.test']);
  assert.equal(payload.routes?.[0].delivery.throttle?.max_per_run, 8);

  const restored = notificationRouteFormFromDefinition(payload);
  assert.equal(restored.routes.length, 2);
  const reduced = notificationRouteFormRemoveSelectedRoute(notificationRouteFormSelectRoute(restored, 1));
  assert.equal(reduced.routes.length, 1);
});
