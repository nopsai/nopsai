import { Fragment, type ReactNode } from 'react';

type TokenKind =
  | 'indent'
  | 'ws'
  | 'dash'
  | 'key'
  | 'punctuation'
  | 'string'
  | 'number'
  | 'boolean'
  | 'operator'
  | 'scalar'
  | 'comment';

type YamlToken = {
  kind: TokenKind;
  text: string;
};

const YAML_KEY_RE = /^[A-Za-z0-9_.-]+$/;
const YAML_NUMBER_RE = /^-?\d+(\.\d+)?$/;
const YAML_BOOL_RE = /^(true|false)$/i;

function token(kind: TokenKind, text: string): YamlToken {
  return { kind, text };
}

function findCommentStart(line: string): number {
  let inSingle = false;
  let inDouble = false;

  for (let i = 0; i < line.length; i += 1) {
    const ch = line[i];
    if (ch === "'" && !inDouble) {
      inSingle = !inSingle;
      continue;
    }
    if (ch === '"' && !inSingle) {
      inDouble = !inDouble;
      continue;
    }
    if (ch === '#' && !inSingle && !inDouble) {
      return i;
    }
  }
  return -1;
}

function tokenizeScalar(value: string): YamlToken[] {
  if (!value) return [];
  const trimmed = value.trim();
  if (!trimmed) return [token('ws', value)];

  const leadingWs = value.match(/^\s+/)?.[0] ?? '';
  const trailingWs = value.match(/\s+$/)?.[0] ?? '';
  const core = value.slice(leadingWs.length, value.length - trailingWs.length);
  const tokens: YamlToken[] = [];

  if (leadingWs) tokens.push(token('ws', leadingWs));

  if (core === '|' || core === '>') {
    tokens.push(token('operator', core));
  } else if ((core.startsWith('"') && core.endsWith('"')) || (core.startsWith("'") && core.endsWith("'"))) {
    tokens.push(token('string', core));
  } else if (YAML_BOOL_RE.test(core)) {
    tokens.push(token('boolean', core));
  } else if (YAML_NUMBER_RE.test(core)) {
    tokens.push(token('number', core));
  } else {
    tokens.push(token('scalar', core));
  }

  if (trailingWs) tokens.push(token('ws', trailingWs));
  return tokens;
}

export function tokenizeYamlLine(line: string): YamlToken[] {
  const commentStart = findCommentStart(line);
  const code = commentStart >= 0 ? line.slice(0, commentStart) : line;
  const comment = commentStart >= 0 ? line.slice(commentStart) : '';

  const tokens: YamlToken[] = [];

  const indent = code.match(/^\s*/)?.[0] ?? '';
  if (indent) tokens.push(token('indent', indent));
  let idx = indent.length;

  if (code.slice(idx).startsWith('-')) {
    tokens.push(token('dash', '-'));
    idx += 1;
    const ws = code.slice(idx).match(/^\s*/)?.[0] ?? '';
    if (ws) {
      tokens.push(token('ws', ws));
      idx += ws.length;
    }
  }

  const remainder = code.slice(idx);
  const colonIndex = remainder.indexOf(':');
  if (colonIndex > 0) {
    const possibleKey = remainder.slice(0, colonIndex).trimEnd();
    const keyTrailing = remainder.slice(possibleKey.length, colonIndex);
    const afterColon = remainder.slice(colonIndex + 1);
    if (YAML_KEY_RE.test(possibleKey)) {
      tokens.push(token('key', possibleKey));
      if (keyTrailing) tokens.push(token('ws', keyTrailing));
      tokens.push(token('punctuation', ':'));
      tokens.push(...tokenizeScalar(afterColon));
    } else {
      tokens.push(token('scalar', remainder));
    }
  } else if (remainder) {
    tokens.push(token('scalar', remainder));
  }

  if (comment) tokens.push(token('comment', comment));
  return tokens;
}

function classNameForToken(kind: TokenKind): string {
  return `yaml-token yaml-token--${kind}`;
}

export function renderYamlLineTokens(line: string, keyPrefix: string): ReactNode {
  const tokens = tokenizeYamlLine(line);
  if (tokens.length === 0) return null;
  return (
    <>
      {tokens.map((t, idx) => (
        <span key={`${keyPrefix}-${idx}`} className={classNameForToken(t.kind)}>
          {t.text}
        </span>
      ))}
    </>
  );
}

export function renderYamlLines(yamlText: string): ReactNode {
  const lines = (yamlText || '').split('\n');
  return (
    <>
      {lines.map((line, idx) => (
        <div key={`yaml-line-${idx}`} className="yaml-line">
          <span className="yaml-line-number">{idx + 1}</span>
          <span className="yaml-line-text">{renderYamlLineTokens(line, `view-${idx}`)}</span>
        </div>
      ))}
    </>
  );
}

export function renderYamlHighlight(yamlText: string): ReactNode {
  const lines = (yamlText || '').split('\n');
  return (
    <>
      {lines.map((line, idx) => (
        <Fragment key={`yaml-hl-${idx}`}>
          {renderYamlLineTokens(line, `hl-${idx}`)}
          {idx < lines.length - 1 ? '\n' : null}
        </Fragment>
      ))}
    </>
  );
}

