import fs from 'fs';
import path from 'path';

const targets = [
  'pages/RuleListPage.tsx',
  'pages/RuleOverviewPage.tsx',
  'pages/PluginManagerPage.tsx',
  'pages/ConnectorsPage.tsx',
];

describe('ProComponents migration', () => {
  it('removes direct @ant-design/pro-components imports from migrated pages', () => {
    for (const relativePath of targets) {
      const source = fs.readFileSync(path.join(__dirname, relativePath), 'utf8');
      expect(source).not.toContain('@ant-design/pro-components');
    }
  });
});
