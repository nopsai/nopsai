import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  dashboardOutputBindingsFromForm,
  sectionFormFromSeed,
  sectionSeedsFromBindings,
  sourceBindingExists,
  sourceFormFromBinding,
  unselectedDashboardOutputSources,
} from './pipelineAssignments.js';
import { createDashboardForm, type DashboardSource } from './model.js';
import type { DashboardPipelineCatalogItem } from './sourceOptions.js';

describe('dashboard pipeline assignments', () => {
  it('filters selected pipelines to dashboard outputs targeting the form dashboard ref', () => {
    const form = {
      ...createDashboardForm('team-1'),
      slug: 'ops',
      title: 'Ops',
      pipelineIDs: ['team-1/service-dashboard', 'team-1/deployments-dashboard'],
    };

    const bindings = dashboardOutputBindingsFromForm(form, catalog());

    assert.deepEqual(bindings.map(binding => `${binding.pipelineID}:${binding.output.sectionKey}:${binding.output.name}`), [
      'team-1/deployments-dashboard:deployments:Deployment Summary',
      'team-1/service-dashboard:service-health:Service Health',
    ]);
  });

  it('derives unique section seeds and source forms from output bindings', () => {
    const form = {
      ...createDashboardForm('team-1'),
      slug: 'ops',
      title: 'Ops',
      pipelineIDs: ['team-1/service-dashboard', 'team-1/deployments-dashboard'],
    };
    const bindings = dashboardOutputBindingsFromForm(form, catalog());

    assert.deepEqual(sectionSeedsFromBindings(bindings), [
      { sectionKey: 'deployments', title: 'Deployments', description: '', displayOrder: 0 },
      { sectionKey: 'service-health', title: 'Service Health', description: '', displayOrder: 10 },
    ]);
    assert.deepEqual(sectionFormFromSeed({ sectionKey: 'service-health', displayOrder: 20 }), {
      sectionKey: 'service-health',
      title: 'Service Health',
      description: '',
      displayOrder: '20',
    });
    assert.deepEqual(sourceFormFromBinding(bindings[0], 1), {
      sectionKey: 'deployments',
      pipelineID: 'team-1/deployments-dashboard',
      outputName: 'Deployment Summary',
      entryKey: 'deployments',
      enabled: true,
      requiredForRefresh: true,
      refreshOrder: '10',
    });
  });

  it('detects existing bindings and removes only unselected dashboard-output sources', () => {
    const form = {
      ...createDashboardForm('team-1'),
      slug: 'ops',
      title: 'Ops',
      pipelineIDs: ['team-1/service-dashboard'],
    };
    const bindings = dashboardOutputBindingsFromForm(form, catalog());
    const sources: DashboardSource[] = [
      {
        id: 'source-1',
        section_key: 'service-health',
        pipeline_id: 'team-1/service-dashboard',
        output_name: 'Service Health',
        entry_key: 'health',
        enabled: true,
        required_for_refresh: true,
        refresh_order: 0,
      },
      {
        id: 'source-2',
        section_key: 'deployments',
        pipeline_id: 'team-1/deployments-dashboard',
        output_name: 'Deployment Summary',
        entry_key: 'deployments',
        enabled: true,
        required_for_refresh: true,
        refresh_order: 10,
      },
      {
        id: 'source-3',
        section_key: 'deployments',
        pipeline_id: 'team-1/manual-report',
        output_name: 'Deployment Summary',
        entry_key: 'deployments',
        enabled: true,
        required_for_refresh: true,
        refresh_order: 20,
      },
    ];

    assert.equal(sourceBindingExists(sources, bindings[0]), true);
    assert.deepEqual(unselectedDashboardOutputSources(form, catalog(), sources).map(source => source.id), ['source-2']);
  });
});

function catalog(): DashboardPipelineCatalogItem[] {
  return [
    {
      id: 'team-1/service-dashboard',
      outputs: [
        {
          name: 'Service Health',
          type: 'dashboard',
          when: 'success',
          dashboardRef: 'team-1/ops',
          sectionKey: 'service-health',
          entryKey: 'health',
          mode: 'replace',
          preset: 'status',
          ttl: '24h',
        },
        {
          name: 'Other Dashboard',
          type: 'dashboard',
          when: 'success',
          dashboardRef: 'team-1/other',
          sectionKey: 'service-health',
          entryKey: 'other',
          mode: 'replace',
          preset: 'status',
          ttl: '',
        },
      ],
    },
    {
      id: 'team-1/deployments-dashboard',
      outputs: [
        {
          name: 'Deployment Summary',
          type: 'dashboard',
          when: 'success',
          dashboardRef: 'team-1/ops',
          sectionKey: 'deployments',
          entryKey: 'deployments',
          mode: 'replace',
          preset: 'status',
          ttl: '',
        },
      ],
    },
    {
      id: 'team-1/unselected-dashboard',
      outputs: [
        {
          name: 'Unselected',
          type: 'dashboard',
          when: 'success',
          dashboardRef: 'team-1/ops',
          sectionKey: 'unused',
          entryKey: 'unused',
          mode: 'replace',
          preset: 'status',
          ttl: '',
        },
      ],
    },
  ];
}
