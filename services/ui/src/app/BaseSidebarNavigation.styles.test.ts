import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, it } from 'node:test';

const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8');

describe('BaseSidebarNavigation styles', () => {
  it('uses light-blue collapsible category labels', () => {
    assert.match(styles, /\.sidebar-nav-label\s*\{[\s\S]*color:\s*#0369a1;/);
    assert.match(styles, /html\.dark \.sidebar-nav-label\s*\{[\s\S]*color:\s*#7dd3fc;/);
    assert.match(styles, /\.sidebar-nav-toggle\s*\{[\s\S]*cursor:\s*pointer;/);
  });

  it('rotates the chevron when a category is collapsed', () => {
    assert.match(
      styles,
      /\.sidebar-nav-toggle\[aria-expanded="false"\] \.sidebar-nav-toggle-chevron\s*\{[\s\S]*transform:\s*rotate\(-90deg\);/
    );
  });

  it('keeps sidebar footer utilities compact when the rail is collapsed', () => {
    assert.match(styles, /\.sidebar-footer\s*\{[\s\S]*display:\s*grid;[\s\S]*flex:\s*0 0 auto;/);
    assert.match(styles, /\.sidebar-footer-section \+ \.sidebar-footer-section\s*\{[\s\S]*border-top:\s*1px solid/);
    assert.match(styles, /\.sidebar-footer-section--version\s*\{[\s\S]*justify-content:\s*space-between;/);
    assert.match(styles, /\.app-sidebar-shell--collapsed \.sidebar-footer-section--version\s*\{[\s\S]*display:\s*none;/);
    assert.match(styles, /\.app-sidebar-shell--collapsed \.sidebar-footer-action\s*\{[\s\S]*width:\s*2\.5rem;/);
  });
});
