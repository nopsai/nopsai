import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

const sourceRoot = resolve(process.cwd(), 'src');

function typescriptReactFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) return typescriptReactFiles(path);
    return entry.isFile() && entry.name.endsWith('.tsx') ? [path] : [];
  });
}

test('keeps authenticated route h1 ownership in the app shell', () => {
  const shellSource = readFileSync(resolve(sourceRoot, 'app/AppShell.tsx'), 'utf8');
  assert.match(shellSource, /<h1 id="main-header"[\s\S]*?\{title\}<\/h1>/);

  const standaloneLoginPath = resolve(sourceRoot, 'pages/Login.tsx');
  const standaloneLoginSource = readFileSync(standaloneLoginPath, 'utf8');
  assert.match(standaloneLoginSource, /<h1\b/);

  const authenticatedRenderFiles = [
    ...typescriptReactFiles(resolve(sourceRoot, 'pages')).filter(path => path !== standaloneLoginPath),
    ...typescriptReactFiles(resolve(sourceRoot, 'features')),
  ];

  for (const path of authenticatedRenderFiles) {
    const source = readFileSync(path, 'utf8');
    const relativePath = path.slice(sourceRoot.length + 1);
    assert.doesNotMatch(source, /<h1\b/, `${relativePath} must defer its h1 to AppShell`);
  }
});

test('keeps authenticated route description ownership in the app shell', () => {
  const shellSource = readFileSync(resolve(sourceRoot, 'app/AppShell.tsx'), 'utf8');
  const navigationSource = readFileSync(resolve(sourceRoot, 'app/navigation.tsx'), 'utf8');

  assert.match(shellSource, /description=\{description\}/);
  assert.match(shellSource, /<p className="app-header-description"[\s\S]*?\{description\}<\/p>/);
  assert.match(navigationSource, /export const descriptionMap: Record<string, string>/);
  assert.match(navigationSource, /'knowledge-context': 'Manage run-time knowledge documents and provider connections.'/);
});
