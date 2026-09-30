import fs from 'fs';
import path from 'path';

describe('resize observer pre-bundle guard', () => {
  it('installs the guard in index.html before the app root so CRA runtime overlay sees it first', () => {
    const htmlPath = path.resolve(__dirname, '../public/index.html');
    const html = fs.readFileSync(htmlPath, 'utf8');

    const guardMarker = 'id="resize-observer-prebundle-guard"';
    const guardIndex = html.indexOf(guardMarker);
    const rootIndex = html.indexOf('<div id="root"></div>');

    expect(guardIndex).toBeGreaterThan(-1);
    expect(rootIndex).toBeGreaterThan(-1);
    expect(guardIndex).toBeLessThan(rootIndex);
    expect(html).toContain('ResizeObserver loop completed with undelivered notifications');
    expect(html).toContain('window.addEventListener(\'error\'');
    expect(html).toContain('stopImmediatePropagation');
  });
});
