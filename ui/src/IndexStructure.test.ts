import fs from 'fs';
import path from 'path';

describe('index bundle structure', () => {
  it('does not depend on Monaco-specific runtime setup in the entry module', () => {
    const source = fs.readFileSync(path.join(__dirname, 'index.tsx'), 'utf8');

    expect(source).not.toContain('./monaco/');
    expect(source).not.toContain('installGlobalErrorGuards');
  });
});
