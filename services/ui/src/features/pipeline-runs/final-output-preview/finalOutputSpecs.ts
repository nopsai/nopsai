export type DocumentSpec = {
  version: '1';
  title: string;
  subtitle?: string;
  metadata?: Array<{ label: string; value: string }>;
  sections: Array<{
    title: string;
    blocks: DocumentBlock[];
  }>;
};

export type DocumentBlock =
  | { type: 'paragraph'; text: string }
  | { type: 'bullet_list' | 'numbered_list'; items: string[] }
  | { type: 'table'; table: { columns: string[]; rows: string[][] } }
  | { type: 'callout'; title?: string; tone?: 'info' | 'success' | 'warning' | 'critical'; text: string };

export type SpreadsheetSpec = {
  version: '1';
  title?: string;
  sheets: Array<{
    name: string;
    columns: Array<{
      key: string;
      header: string;
      width?: number;
      number_format?: string;
    }>;
    rows: Array<Record<string, string | number | boolean | null>>;
    freeze_header?: boolean;
    auto_filter?: boolean;
  }>;
};

export function parseDocumentSpec(content: string): DocumentSpec | null {
  const value = parseObject(content);
  if (!value || value.version !== '1' || typeof value.title !== 'string' || !Array.isArray(value.sections)) return null;
  return value as DocumentSpec;
}

export function parseSpreadsheetSpec(content: string): SpreadsheetSpec | null {
  const value = parseObject(content);
  if (!value || value.version !== '1' || !Array.isArray(value.sheets)) return null;
  return value as SpreadsheetSpec;
}

export function documentSpecToText(spec: DocumentSpec) {
  const lines = [spec.title];
  if (spec.subtitle) lines.push(spec.subtitle);
  for (const item of spec.metadata || []) lines.push(`${item.label}: ${item.value}`);
  for (const section of spec.sections) {
    lines.push('', section.title);
    for (const block of section.blocks) {
      if (block.type === 'paragraph' || block.type === 'callout') lines.push(block.text);
      if (block.type === 'bullet_list') lines.push(...block.items.map(item => `- ${item}`));
      if (block.type === 'numbered_list') lines.push(...block.items.map((item, index) => `${index + 1}. ${item}`));
      if (block.type === 'table') {
        lines.push(block.table.columns.join('\t'));
        lines.push(...block.table.rows.map(row => row.join('\t')));
      }
    }
  }
  return lines.join('\n').trim();
}

export function spreadsheetSpecToText(spec: SpreadsheetSpec) {
  const sheets = spec.sheets.map(sheet => {
    const rows = [sheet.columns.map(column => column.header).join('\t')];
    for (const row of sheet.rows) {
      rows.push(sheet.columns.map(column => formatCell(row[column.key])).join('\t'));
    }
    return [`[${sheet.name}]`, ...rows].join('\n');
  });
  return sheets.join('\n\n');
}

export function formatCell(value: unknown) {
  if (value === null || value === undefined) return '';
  if (typeof value === 'boolean') return value ? 'Yes' : 'No';
  return String(value);
}

function parseObject(content: string): Record<string, unknown> | null {
  try {
    const value: unknown = JSON.parse(content);
    return value !== null && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}
