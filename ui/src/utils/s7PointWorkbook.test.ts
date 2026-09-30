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

  it('imports a representative 19-point CNC workbook', async () => {
    const points: S7PointDefinition[] = [
      { name: 'total_output', address: 'DB24.DBD440', value_type: 'Uint32', read_write: 'RW' },
      ...Array.from({ length: 17 }, (_, index) => ({
        name: `point_${index + 1}`,
        address: `DB23.DBW${72 + index * 2}`,
        value_type: 'Int16' as const,
        read_write: 'RW' as const,
      })),
      { name: 'beat_two', address: 'DB23.DBW112', value_type: 'Uint16', read_write: 'RW' },
    ];

    const template = await createS7PointWorkbook(points);
    const imported = await parseS7PointWorkbook(template);
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
