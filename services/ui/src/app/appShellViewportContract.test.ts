import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

const sourceRoot = resolve(process.cwd(), 'src');

test('keeps document scrolling owned by app shell scroll containers', () => {
  const shellSource = readFileSync(resolve(sourceRoot, 'app/AppShell.tsx'), 'utf8');
  const indexStyles = readFileSync(resolve(sourceRoot, 'index.css'), 'utf8');

  assert.match(indexStyles, /html,\s*body,\s*#root\s*\{[\s\S]*?height:\s*100%/);
  assert.match(indexStyles, /body\s*\{[\s\S]*?overflow:\s*hidden/);
  assert.match(indexStyles, /body\s*\{[\s\S]*?overscroll-behavior:\s*none/);
  assert.match(shellSource, /<div className="h-screen overflow-auto">[\s\S]*?<LoginPage/);
  assert.match(
    shellSource,
    /id="page-content-wrapper"\s+className="[^"]*\bflex-1\b[^"]*\bmin-h-0\b[^"]*\boverflow-auto\b[^"]*\boverscroll-contain\b/
  );
});
