import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test } from 'node:test';

/*
 * Feature stylesheets may keep their own vocabulary, but not their own dark
 * theme. Credentials and Teams each shipped a hand-picked navy palette — a
 * near-black page with blue page glows — which read as "not following dark
 * mode" next to the sidebar and every other page. Their dark blocks are now
 * aliases for the tokens in styles.css, and these tests keep them that way.
 */

const sourceRoot = resolve(process.cwd(), 'src');

function darkBlock(source: string, selector: string): string {
  const index = source.indexOf(selector);
  assert.ok(index >= 0, `expected ${selector} in the stylesheet`);
  const open = source.indexOf('{', index);
  const close = source.indexOf('}', open);
  return source.slice(open + 1, close);
}

function declaration(block: string, property: string): string {
  const match = block.match(new RegExp(`${property}\\s*:\\s*([^;]+);`));
  assert.ok(match, `expected ${property} in the dark block`);
  return (match[1] || '').trim();
}

const surfaces: Array<{ file: string; selector: string; variables: string[] }> = [
  {
    file: 'features/system/credentials/credentials.css',
    selector: 'html.dark .credential-registry,',
    variables: [
      '--credential-bg',
      '--credential-panel',
      '--credential-panel-strong',
      '--credential-border',
      '--credential-text',
      '--credential-muted',
      '--credential-accent',
      '--credential-field-bg',
      '--credential-table-bg',
      '--credential-drawer-bg',
      '--credential-modal-bg',
    ],
  },
  {
    file: 'features/teams/teams.css',
    selector: 'html.dark .teams-page-shell {',
    variables: [
      '--teams-bg',
      '--teams-card',
      '--teams-elevated',
      '--teams-hover',
      '--teams-input',
      '--teams-text',
      '--teams-muted',
      '--teams-line',
      '--teams-primary',
    ],
  },
];

for (const surface of surfaces) {
  test(`${surface.file} takes its dark surfaces from the app tokens`, () => {
    const styles = readFileSync(resolve(sourceRoot, surface.file), 'utf8');
    const block = darkBlock(styles, surface.selector);

    for (const variable of surface.variables) {
      const value = declaration(block, variable);
      assert.match(
        value,
        /var\(--/,
        `${variable} must alias a token from styles.css, got "${value}"`
      );
    }
  });

  test(`${surface.file} keeps no competing near-black dark palette`, () => {
    const styles = readFileSync(resolve(sourceRoot, surface.file), 'utf8');
    const block = darkBlock(styles, surface.selector);
    const literals = block.match(/#[0-9a-fA-F]{6}\b/g) || [];

    for (const literal of literals) {
      const red = Number.parseInt(literal.slice(1, 3), 16);
      const green = Number.parseInt(literal.slice(3, 5), 16);
      const blue = Number.parseInt(literal.slice(5, 7), 16);
      const isDarkSurface = red < 0x40 && green < 0x40 && blue < 0x40;
      assert.ok(
        !isDarkSurface,
        `${literal} is a hand-picked dark surface; use a token from styles.css instead`
      );
    }
  });
}

test('dark pages stay flat, with no page-level radial glow', () => {
  const teams = readFileSync(resolve(sourceRoot, 'features/teams/teams.css'), 'utf8');
  const credentials = readFileSync(
    resolve(sourceRoot, 'features/system/credentials/credentials.css'),
    'utf8'
  );

  assert.ok(
    !darkBlock(teams, 'html.dark .teams-page-shell {').includes('radial-gradient'),
    'the Teams page must not paint a radial glow in dark mode'
  );
  assert.equal(
    declaration(
      darkBlock(credentials, 'html.dark .credential-registry,'),
      '--credential-page-radial-primary'
    ),
    'transparent'
  );
});
