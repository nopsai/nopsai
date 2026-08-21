import type { ReactNode } from 'react';

type AssistantRichContentProps = {
  content: string;
};

type InlineSegment =
  | { type: 'text'; value: string }
  | { type: 'code'; value: string }
  | { type: 'link'; label: string; href: string };

export function AssistantRichContent({ content }: AssistantRichContentProps) {
  const blocks = parseAssistantBlocks(content);
  if (blocks.length === 0) return null;

  return (
    <div className="space-y-2 text-[var(--text-primary)]">
      {blocks.map((block, index) => {
        if (block.type === 'heading') {
          const Heading = block.level === 1 ? 'h3' : 'h4';
          return (
            <Heading key={index} className={`font-semibold leading-snug ${block.level === 1 ? 'text-sm' : 'text-[13px]'}`}>
              {renderInlineText(block.text)}
            </Heading>
          );
        }
        if (block.type === 'list') {
          const List = block.ordered ? 'ol' : 'ul';
          return (
            <List key={index} className={`space-y-1 pl-5 leading-relaxed ${block.ordered ? 'list-decimal' : 'list-disc'}`}>
              {block.items.map((item, itemIndex) => <li key={itemIndex}>{renderInlineText(item)}</li>)}
            </List>
          );
        }
        if (block.type === 'code') {
          return (
            <pre key={index} className="max-h-72 overflow-auto rounded-md bg-[var(--bg-tertiary)] p-3 text-xs leading-relaxed text-[var(--text-primary)]">
              <code>{block.text}</code>
            </pre>
          );
        }
        return (
          <p key={index} className="leading-relaxed">
            {renderInlineText(block.text)}
          </p>
        );
      })}
    </div>
  );
}

type AssistantBlock =
  | { type: 'heading'; level: 1 | 2; text: string }
  | { type: 'paragraph'; text: string }
  | { type: 'list'; ordered: boolean; items: string[] }
  | { type: 'code'; text: string };

function parseAssistantBlocks(content: string): AssistantBlock[] {
  const lines = content.replace(/\r\n/g, '\n').split('\n');
  const blocks: AssistantBlock[] = [];
  let paragraph: string[] = [];
  let listItems: string[] = [];
  let listOrdered = false;
  let codeLines: string[] = [];
  let inCode = false;

  const flushParagraph = () => {
    if (paragraph.length === 0) return;
    blocks.push({ type: 'paragraph', text: paragraph.join(' ').trim() });
    paragraph = [];
  };
  const flushList = () => {
    if (listItems.length === 0) return;
    blocks.push({ type: 'list', ordered: listOrdered, items: listItems });
    listItems = [];
    listOrdered = false;
  };

  lines.forEach(rawLine => {
    const line = rawLine.trimEnd();
    if (line.trim().startsWith('```')) {
      if (inCode) {
        blocks.push({ type: 'code', text: codeLines.join('\n') });
        codeLines = [];
        inCode = false;
      } else {
        flushParagraph();
        flushList();
        inCode = true;
      }
      return;
    }
    if (inCode) {
      codeLines.push(rawLine);
      return;
    }
    if (!line.trim()) {
      flushParagraph();
      flushList();
      return;
    }

    const heading = /^(#{1,3})\s+(.+)$/.exec(line.trim());
    if (heading) {
      flushParagraph();
      flushList();
      blocks.push({ type: 'heading', level: heading[1].length === 1 ? 1 : 2, text: heading[2].trim() });
      return;
    }

    const bullet = /^[-*]\s+(.+)$/.exec(line.trim());
    const ordered = /^\d+\.\s+(.+)$/.exec(line.trim());
    if (bullet || ordered) {
      flushParagraph();
      const item = (bullet?.[1] || ordered?.[1] || '').trim();
      const itemOrdered = Boolean(ordered);
      if (listItems.length > 0 && listOrdered !== itemOrdered) flushList();
      listOrdered = itemOrdered;
      listItems.push(item);
      return;
    }

    flushList();
    paragraph.push(line.trim());
  });

  if (inCode) blocks.push({ type: 'code', text: codeLines.join('\n') });
  flushParagraph();
  flushList();
  return blocks;
}

function renderInlineText(text: string): ReactNode[] {
  return parseInlineSegments(text).map((segment, index) => {
    if (segment.type === 'code') {
      return (
        <code key={index} className="rounded bg-[var(--bg-tertiary)] px-1 py-0.5 text-[0.92em] text-[var(--text-primary)]">
          {segment.value}
        </code>
      );
    }
    if (segment.type === 'link') {
      return (
        <a key={index} className="text-[var(--text-accent)] underline underline-offset-2" href={segment.href} target="_blank" rel="noreferrer">
          {segment.label}
        </a>
      );
    }
    return segment.value;
  });
}

function parseInlineSegments(text: string): InlineSegment[] {
  const segments: InlineSegment[] = [];
  const pattern = /(`[^`]+`|\[[^\]]+\]\(https?:\/\/[^)\s]+\))/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) segments.push({ type: 'text', value: text.slice(lastIndex, match.index) });
    const value = match[0];
    if (value.startsWith('`')) {
      segments.push({ type: 'code', value: value.slice(1, -1) });
    } else {
      const link = /^\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)$/.exec(value);
      if (link) segments.push({ type: 'link', label: link[1], href: link[2] });
    }
    lastIndex = pattern.lastIndex;
  }
  if (lastIndex < text.length) segments.push({ type: 'text', value: text.slice(lastIndex) });
  return segments;
}
