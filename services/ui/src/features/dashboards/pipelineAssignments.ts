import {
  createSourceForm,
  normalizeRunScope,
  titleFromKey,
  type DashboardFormState,
  type DashboardSectionFormState,
  type DashboardSectionSeed,
  type DashboardSource,
  type DashboardSourceFormState,
} from './model.js';
import type { DashboardPipelineCatalogItem, DashboardPipelineOutputOption } from './sourceOptions.js';

export type DashboardOutputBinding = {
  pipelineID: string;
  runScope: string;
  output: DashboardPipelineOutputOption;
};

export function dashboardRefFromForm(form: DashboardFormState): string {
  const teamPath = form.teamPath.trim().replace(/^\/+|\/+$/g, '');
  const slug = form.slug.trim().replace(/^\/+|\/+$/g, '');
  return teamPath && slug ? `${teamPath}/${slug}` : '';
}

export function dashboardOutputBindingsFromForm(
  form: DashboardFormState,
  catalog: DashboardPipelineCatalogItem[]
): DashboardOutputBinding[] {
  const dashboardRef = dashboardRefFromForm(form);
  if (!dashboardRef || form.pipelineIDs.length === 0) return [];
  const selectedPipelineIDs = new Set(form.pipelineIDs);
  return catalog
    .filter(pipeline => selectedPipelineIDs.has(pipeline.id))
    .flatMap(pipeline => pipeline.outputs
      .filter(output => output.dashboardRef === dashboardRef && Boolean(output.sectionKey.trim()) && Boolean(output.name.trim()))
      .map(output => ({
        pipelineID: pipeline.id,
        runScope: normalizeRunScope(form.pipelineScopes[pipeline.id]),
        output: normalizeOutput(output),
      })))
    .sort(compareBindings);
}

export function sectionSeedsFromBindings(bindings: DashboardOutputBinding[]): DashboardSectionSeed[] {
  const seen = new Set<string>();
  const sections: DashboardSectionSeed[] = [];
  for (const binding of bindings) {
    const sectionKey = binding.output.sectionKey.trim();
    if (!sectionKey || seen.has(sectionKey)) continue;
    seen.add(sectionKey);
    sections.push({
      sectionKey,
      title: titleFromKey(sectionKey),
      description: '',
      displayOrder: sections.length * 10,
    });
  }
  return sections;
}

export function sourceBindingExists(sources: DashboardSource[], binding: DashboardOutputBinding): boolean {
  const entryKey = binding.output.entryKey.trim();
  return sources.some(source =>
    source.section_key === binding.output.sectionKey &&
    source.pipeline_id === binding.pipelineID &&
    source.output_name === binding.output.name &&
    (source.entry_key || '') === entryKey &&
    normalizeRunScope(source.run_scope) === binding.runScope
  );
}

export function unselectedDashboardOutputSources(
  form: DashboardFormState,
  catalog: DashboardPipelineCatalogItem[],
  sources: DashboardSource[]
): DashboardSource[] {
  const dashboardRef = dashboardRefFromForm(form);
  const selectedBindingKeys = new Set(dashboardOutputBindingsFromForm(form, catalog).map(bindingKey));
  const outputsByPipelineID = new Map(
    catalog.map(pipeline => [
      pipeline.id,
      pipeline.outputs.filter(output => output.dashboardRef === dashboardRef && Boolean(output.sectionKey.trim())),
    ])
  );
  return sources.filter(source => {
    return (outputsByPipelineID.get(source.pipeline_id) || []).some(output =>
      source.section_key === output.sectionKey.trim() &&
      source.output_name === output.name.trim() &&
      (source.entry_key || '') === output.entryKey.trim() &&
      !selectedBindingKeys.has(sourceKey(source))
    );
  });
}

export function sectionFormFromSeed(section: DashboardSectionSeed): DashboardSectionFormState {
  return {
    sectionKey: section.sectionKey,
    title: section.title || titleFromKey(section.sectionKey),
    description: section.description || '',
    displayOrder: String(section.displayOrder ?? 0),
  };
}

export function sourceFormFromBinding(binding: DashboardOutputBinding, index: number): DashboardSourceFormState {
  return {
    ...createSourceForm(binding.output.sectionKey),
    pipelineID: binding.pipelineID,
    outputName: binding.output.name,
    entryKey: binding.output.entryKey,
    runScope: binding.runScope,
    refreshOrder: String(index * 10),
  };
}

function normalizeOutput(output: DashboardPipelineOutputOption): DashboardPipelineOutputOption {
  return {
    ...output,
    name: output.name.trim(),
    sectionKey: output.sectionKey.trim(),
    entryKey: output.entryKey.trim(),
  };
}

function compareBindings(a: DashboardOutputBinding, b: DashboardOutputBinding): number {
  const sectionCompare = a.output.sectionKey.localeCompare(b.output.sectionKey);
  if (sectionCompare !== 0) return sectionCompare;
  const pipelineCompare = a.pipelineID.localeCompare(b.pipelineID);
  if (pipelineCompare !== 0) return pipelineCompare;
  const outputCompare = a.output.name.localeCompare(b.output.name);
  if (outputCompare !== 0) return outputCompare;
  return a.runScope.localeCompare(b.runScope);
}

function bindingKey(binding: DashboardOutputBinding): string {
  return [
    binding.output.sectionKey,
    binding.pipelineID,
    binding.output.name,
    binding.output.entryKey,
    binding.runScope,
  ].join('\u001f');
}

function sourceKey(source: DashboardSource): string {
  return [
    source.section_key,
    source.pipeline_id,
    source.output_name,
    source.entry_key || '',
    normalizeRunScope(source.run_scope),
  ].join('\u001f');
}
