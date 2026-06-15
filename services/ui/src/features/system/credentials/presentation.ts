export function formatCredentialDate(value?: string): string {
  if (!value) return '-';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function formatCredentialLabel(value: string): string {
  const knownLabels: Record<string, string> = {
    api: 'API',
    github: 'GitHub',
    llm: 'LLM',
    mcp: 'MCP',
    oidc: 'OIDC',
    openai: 'OpenAI',
    smtp: 'SMTP',
  };
  return value
    .split(/[-_.]/)
    .filter(Boolean)
    .map(part => knownLabels[part.toLowerCase()] || part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}
