import { applySchemaDefaults } from './JSONSchemaRenderer';

jest.mock('./LazyCodeEditor', () => ({
  LazyCodeEditor: () => null,
}));

describe('applySchemaDefaults', () => {
  it('fills nested defaults for missing S7 connection fields', () => {
    const schema = {
      type: 'object',
      properties: {
        connection: {
          type: 'object',
          properties: {
            host: { type: 'string' },
            rack: { type: 'integer', default: 0 },
            slot: { type: 'integer', default: 1 },
          },
        },
      },
    };

    expect(applySchemaDefaults(schema as any, { connection: { host: '127.0.0.1' } })).toEqual({
      connection: {
        host: '127.0.0.1',
        rack: 0,
        slot: 1,
      },
    });
  });

  it('creates nested object defaults when object is missing', () => {
    const schema = {
      type: 'object',
      properties: {
        connection: {
          type: 'object',
          properties: {
            rack: { type: 'integer', default: 0 },
            slot: { type: 'integer', default: 1 },
          },
        },
      },
    };

    expect(applySchemaDefaults(schema as any, {})).toEqual({
      connection: {
        rack: 0,
        slot: 1,
      },
    });
  });

  it('preserves explicit zero values', () => {
    const schema = {
      type: 'object',
      properties: {
        connection: {
          type: 'object',
          properties: {
            rack: { type: 'integer', default: 1 },
          },
        },
      },
    };

    expect(applySchemaDefaults(schema as any, { connection: { rack: 0 } })).toEqual({
      connection: {
        rack: 0,
      },
    });
  });
});
