import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, it } from 'node:test';

const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8');

describe('page display contract', () => {
  it('keeps active flex and grid pages from being flattened to block', () => {
    const blockFallback = '[data-page].active { display: block !important; }';
    const flexOverride = '[data-page].active.flex { display: flex !important; }';
    const gridOverride = '[data-page].active.grid { display: grid !important; }';

    assert.match(styles, /\[data-page\]\s*\{\s*display:\s*none !important;\s*\}/);
    assert.ok(styles.includes(blockFallback), 'active page fallback should keep simple pages visible');
    assert.ok(styles.includes(flexOverride), 'active flex page roots should preserve flex layout');
    assert.ok(styles.includes(gridOverride), 'active grid page roots should preserve grid layout');
    assert.ok(
      styles.indexOf(blockFallback) < styles.indexOf(flexOverride),
      'flex override must come after the block fallback'
    );
    assert.ok(
      styles.indexOf(blockFallback) < styles.indexOf(gridOverride),
      'grid override must come after the block fallback'
    );
  });
});
