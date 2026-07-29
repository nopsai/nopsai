import { SYSTEM_SETTINGS_SECTIONS, type SystemSettingsSectionId } from './settingsPresentation';

export function sectionByID(sectionID: SystemSettingsSectionId) {
  const section = SYSTEM_SETTINGS_SECTIONS.find(item => item.id === sectionID);
  if (!section) throw new Error(`Unknown settings section: ${sectionID}`);
  return section;
}

export function systemSettingsSectionDomID(sectionID: SystemSettingsSectionId) {
  return `system-settings-${sectionID}`;
}

export function formatTimestamp(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString();
}
