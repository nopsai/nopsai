/** Attachments are inlined into the prompt, so they stay small enough to stay in context. */
export const assistantAttachmentMaxCharacters = 20000;
export const assistantAttachmentMaxBytes = 2 * 1024 * 1024;

export function assistantAttachmentBlock(fileName: string, content: string): string {
  const normalized = content.replace(/\r\n/g, '\n').trimEnd();
  const truncated = normalized.length > assistantAttachmentMaxCharacters
    ? `${normalized.slice(0, assistantAttachmentMaxCharacters)}\n... truncated ${normalized.length - assistantAttachmentMaxCharacters} more characters`
    : normalized;
  const fence = truncated.includes('```') ? '````' : '```';
  return `Attached file ${fileName}:\n${fence}\n${truncated}\n${fence}`;
}

export function appendAssistantAttachment(draft: string, fileName: string, content: string): string {
  const block = assistantAttachmentBlock(fileName, content);
  const existing = draft.trimEnd();
  return existing ? `${existing}\n\n${block}` : block;
}

export async function readAssistantAttachmentText(file: File): Promise<string> {
  if (file.size > assistantAttachmentMaxBytes) {
    throw new Error(`${file.name} is larger than 2 MB and was not attached`);
  }
  return file.text();
}
