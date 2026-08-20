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

  it('lays the footer out as one row of equal icon buttons', () => {
    assert.match(styles, /\.sidebar-footer\s*\{[\s\S]*display:\s*grid;[\s\S]*flex:\s*0 0 auto;/);
    assert.match(styles, /\.sidebar-footer-utilities\s*\{[\s\S]*display:\s*flex;[\s\S]*justify-content:\s*space-between;/);
    assert.match(styles, /\.sidebar-footer-action\s*\{[\s\S]*flex:\s*1 1 0;/);
  });

  it('wraps the footer icons two to a line on the collapsed rail', () => {
    assert.match(
      styles,
      /\.app-sidebar-shell--collapsed \.sidebar-footer-utilities\s*\{[\s\S]*grid-template-columns:\s*repeat\(2, minmax\(0, 1fr\)\);/
    );
  });

  it('gives every navigation category its own icon colour, in both themes', () => {
    const topics = ['observe', 'build-automate', 'lab', 'ai-knowledge', 'workspace', 'administration'];
    for (const topic of topics) {
      assert.match(
        styles,
        new RegExp(`\\.sidebar-nav-section\\[data-topic-id="${topic}"\\] \\.sidebar-nav-icon\\s*\\{\\s*color: var\\(--nav-topic-${topic}\\);`),
        `${topic} should colour its icons from its own token`
      );
      assert.match(styles, new RegExp(`^\\s*--nav-topic-${topic}: #`, 'm'), `${topic} needs a light value`);
    }

    const darkBlock = styles.slice(styles.indexOf('html.dark {'));
    for (const topic of topics) {
      assert.match(darkBlock, new RegExp(`--nav-topic-${topic}: #`), `${topic} needs a dark value`);
    }

    // Every category gets a hue of its own. Administration was slate, which is
    // the colour of an uncoloured icon, so its section read as unstyled.
    const values = topics.map(topic => {
      const match = styles.match(new RegExp(`^\\s*--nav-topic-${topic}: (#[0-9a-f]{6});`, 'm'));
      return match?.[1];
    });
    assert.equal(new Set(values).size, topics.length, 'each category needs a distinct colour');
    assert.ok(!values.includes('#64748b'), 'Administration should not fall back to slate');
  });

  it('keeps the category colour on the selected row', () => {
    // The active row used to repaint its icon in the text colour, which dropped
    // the hue exactly where the user is looking.
    assert.doesNotMatch(styles, /\.sidebar-nav-link\.active \.sidebar-nav-icon\s*\{[\s\S]{0,80}color:\s*currentColor;/);
    assert.doesNotMatch(styles, /\.sidebar-nav-icon\s*\{[^}]*opacity:/);
  });
});
