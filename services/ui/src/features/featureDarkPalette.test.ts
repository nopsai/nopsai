import assert from 'node:assert/strict';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { test } from 'node:test';

/*
 * Feature stylesheets may keep their own vocabulary, but not their own dark
 * theme. Credentials and Teams each shipped a hand-picked navy palette — a
 * near-black page with blue page glows — which read as "not following dark
 * mode" next to the sidebar and every other page. Their dark blocks are now
 * aliases for the tokens in styles.css, and these tests keep them that way.
 */

const sourceRoot = resolve(process.cwd(), 'src');
const appStyles = readFileSync(resolve(sourceRoot, 'styles.css'), 'utf8');

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

/*
 * The run and pipeline graph had the same problem in a different shape: instead
 * of a dark override it carried a whole navy theme as its default, with a
 * light-mode override on top. One token-driven block now serves both themes.
 */
test('the graph palette is one token-driven block, not a theme of its own', () => {
  const block = darkBlock(appStyles, '\n.run-graph-redesign {');

  for (const variable of [
    '--rg-bg',
    '--rg-panel',
    '--rg-canvas',
    '--rg-border',
    '--rg-text',
    '--rg-muted',
    '--rg-accent',
    '--rg-node-fill',
    '--rg-node-stroke',
    '--rg-grid-dot',
  ]) {
    assert.match(
      declaration(block, variable),
      /var\(--|color-mix\(/,
      `${variable} must come from a token, not a hand-picked colour`
    );
  }

  assert.ok(
    !appStyles.includes('html:not(.dark) .run-graph-redesign'),
    'a light-mode twin means the graph is carrying two palettes again'
  );
});

test('the graph keeps no hand-picked dark surfaces or off-theme accent', () => {
  const block = darkBlock(appStyles, '\n.run-graph-redesign {');
  const literals = block.match(/#[0-9a-fA-F]{6}\b/g) || [];

  assert.deepEqual(literals, [], 'graph surfaces must be tokens, not literals');
  assert.ok(
    !/rgba\(139, 92, 246/.test(block),
    'the violet accent belonged to the graph\'s own theme; use var(--accent)'
  );
});

test('the primary button follows the theme accent', () => {
  const block = darkBlock(appStyles, '\n.glass-button-primary {');

  assert.match(declaration(block, 'background'), /var\(--accent\)/);
});

/*
 * The same competing palette can arrive through a Tailwind class as easily as
 * through a stylesheet: the run detail hero was a `dark:bg-gradient-to-br
 * from-[#0b0c15]` navy card sitting on the app's greys. A component paints a
 * surface with a token or not at all.
 */
function componentFiles(directory: string): string[] {
  return readdirSync(directory).flatMap(entry => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return componentFiles(path);
    return path.endsWith('.tsx') && !path.includes('.test.') ? [path] : [];
  });
}

test('no component paints a surface with a hand-picked hex', () => {
  const offenders = componentFiles(sourceRoot)
    .map(path => ({ path, source: readFileSync(path, 'utf8') }))
    .flatMap(({ path, source }) => {
      const matches = source.match(/(?:bg|from|via|to)-\[#[0-9a-fA-F]{6}\]/g) || [];
      return matches.map(match => `${relative(sourceRoot, path)}: ${match}`);
    });

  assert.deepEqual(offenders, [], 'use a theme token, such as bg-[var(--bg-panel)]');
});
