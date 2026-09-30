import fs from 'fs';
import path from 'path';

describe('App bundle structure', () => {
  it('defines lazy-loaded routes behind a suspense fallback', () => {
    const source = fs.readFileSync(path.join(__dirname, 'App.tsx'), 'utf8');

    expect(source).toContain('React.lazy');
    expect(source).toContain('Suspense');
    expect(source).toContain('页面加载中...');
  });
});
