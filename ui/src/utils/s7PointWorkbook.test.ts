import fs from 'fs';
import path from 'path';
import { createS7PointWorkbook, parseS7PointWorkbook, S7PointDefinition } from './s7PointWorkbook';

describe('S7 point workbook', () => {
  it('round-trips point configuration without losing precision or range policy', async () => {
    const points: S7PointDefinition[] = [
      {
        name: 'total_output', address: 'DB24.DBD440', value_type: 'Uint32', read_write: 'RW',
        description: '总产量', precision: 0,
        range: { minimum: 0, maximum: 1000000, policy: 'use_default', default_value: 0 },
      },
      { name: 'stop_alarm', address: 'DB21.DBX25.1', value_type: 'Bool', read_write: 'RW' },
    ];

    const buffer = await createS7PointWorkbook(points);
    const imported = await parseS7PointWorkbook(buffer);
    expect(imported).toEqual(points);
  });

  it('rejects duplicate names during import', async () => {
    const buffer = await createS7PointWorkbook([
      { name: 'same', address: 'DB1.DBW0', value_type: 'Int16' },
      { name: 'same', address: 'DB1.DBW2', value_type: 'Int16' },
    ]);
    await expect(parseS7PointWorkbook(buffer)).rejects.toThrow('重复');
  });

  it('imports the delivered cnc18 workbook template', async () => {
  const template = fs.readFileSync(path.resolve(__dirname, '../../../docs/cnc18-s7-points.xlsx'));
  const imported = await parseS7PointWorkbook(template as any);
  expect(imported).toHaveLength(19);
  expect(imported[0]).toMatchObject({ name: 'total_output', address: 'DB24.DBD440', value_type: 'Uint32' });
  expect(imported[18]).toMatchObject({ name: 'beat_two', address: 'DB23.DBW112', value_type: 'Uint16' });
  });

  it('rejects an out-of-range fallback value', async () => {
  const buffer = await createS7PointWorkbook([{
    name: 'temperature', address: 'DB1.DBD0', value_type: 'Float32',
    range: { minimum: 0, maximum: 100, policy: 'use_default', default_value: 101 },
  }]);
  await expect(parseS7PointWorkbook(buffer)).rejects.toThrow('default_value');
  });
});
