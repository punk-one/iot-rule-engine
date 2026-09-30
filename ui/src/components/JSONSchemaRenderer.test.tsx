import React from 'react';
import { act, render, screen, waitFor } from '@testing-library/react';
import { JSONSchemaRenderer } from './JSONSchemaRenderer';

const mockEditorPropsSpy = jest.fn();

jest.mock('antd', () => {
  const React = require('react');

  return {
    Input: (props: any) => React.createElement('input', props),
    InputNumber: (props: any) => React.createElement('input', { ...props, type: 'number' }),
    Switch: (props: any) => React.createElement('input', { ...props, type: 'checkbox' }),
    Select: (props: any) => React.createElement('select', props),
    Button: ({ children, danger, block, ...props }: any) => React.createElement('button', props, children),
    Tooltip: ({ children, title }: any) => React.createElement('span', { 'data-testid': 'mock-tooltip', 'data-title': title || '' }, children),
    Form: {
      Item: ({ children }: any) => React.createElement('div', null, children),
    },
  };
});

jest.mock('./LazyCodeEditor', () => {
  const React = require('react');

  return {
    LazyCodeEditor: function MockCodeEditor(props: any) {
      mockEditorPropsSpy(props);

      return React.createElement('div', { 'data-testid': 'mock-code-editor' });
    },
  };
});

describe('JSONSchemaRenderer code editor widget', () => {
  beforeAll(() => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: jest.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: jest.fn(),
        removeListener: jest.fn(),
        addEventListener: jest.fn(),
        removeEventListener: jest.fn(),
        dispatchEvent: jest.fn(),
      })),
    });
  });

  beforeEach(() => {
    mockEditorPropsSpy.mockClear();
  });

  it('does not warn about custom cel formats during validation', async () => {
    const warnSpy = jest.spyOn(console, 'warn').mockImplementation(() => {});

    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            expr: {
              type: 'string',
              title: 'Filter Expression',
              format: 'cel',
            },
          },
        }}
        formData={{}}
        onChange={jest.fn()}
      />
    );

    expect(await screen.findByTestId('mock-code-editor')).toBeInTheDocument();
    expect(warnSpy).not.toHaveBeenCalledWith(expect.stringContaining('unknown format "cel"'));

    warnSpy.mockRestore();
  });

  it('uses plain-text mode for cel fields', async () => {
    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            expr: {
              type: 'string',
              title: 'Filter Expression',
              format: 'cel',
            },
          },
        }}
        formData={{}}
        onChange={jest.fn()}
      />
    );

    expect(await screen.findByTestId('mock-code-editor')).toBeInTheDocument();

    await waitFor(() => {
      expect(mockEditorPropsSpy).toHaveBeenCalled();
    });

    const latestProps = mockEditorPropsSpy.mock.calls[mockEditorPropsSpy.mock.calls.length - 1][0];
    expect(latestProps.mode).toBe('text');
  });

  it('uses the large schema-defined height for JavaScript code fields', async () => {
    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            code: {
              type: 'string',
              title: 'JavaScript 代码',
              format: 'code',
              'x-ui-height': '520px',
              'x-ui-placeholder': 'function main(input) {}',
            } as any,
          },
        }}
        formData={{}}
        onChange={jest.fn()}
      />
    );

    expect(await screen.findByTestId('mock-code-editor')).toBeInTheDocument();
    const latestProps = mockEditorPropsSpy.mock.calls[mockEditorPropsSpy.mock.calls.length - 1][0];
    expect(latestProps.height).toBe('520px');
    expect(latestProps.mode).toBe('text');
    expect(latestProps.placeholder).toBe('function main(input) {}');
  });

  it('keeps json editor values as strings for string schema fields', async () => {
    const handleChange = jest.fn();

    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            template: {
              type: 'string',
              title: 'Template',
              format: 'json',
            },
          },
        }}
        formData={{}}
        onChange={handleChange}
      />
    );

    await waitFor(() => {
      expect(mockEditorPropsSpy).toHaveBeenCalled();
    });

    const latestProps = mockEditorPropsSpy.mock.calls[mockEditorPropsSpy.mock.calls.length - 1][0];
    await act(async () => {
      latestProps.onChange('{"device_code":"{{ .payload.device_code }}"}');
    });

    await waitFor(() => {
      expect(handleChange).toHaveBeenCalledWith({
        template: '{"device_code":"{{ .payload.device_code }}"}',
      });
    });
  });

  it('renders transform_map template as a json code editor', async () => {
    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            process: {
              type: 'object',
              properties: {
                template: {
                  type: 'string',
                  title: 'Template',
                  format: 'json',
                  description: 'JSON object template text rendered with Go templates',
                },
                iterate: {
                  type: 'string',
                },
              },
            },
          },
        }}
        formData={{ process: {} }}
        onChange={jest.fn()}
      />
    );

    expect(await screen.findByTestId('mock-code-editor')).toBeInTheDocument();

    const latestProps = mockEditorPropsSpy.mock.calls[mockEditorPropsSpy.mock.calls.length - 1][0];
    expect(latestProps.mode).toBe('json');
  });

  it('renders legacy transform_map template schema without type as a json code editor', async () => {
    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            process: {
              type: 'object',
              properties: {
                template: {
                  description: 'Structured template object or JSON string rendered with Go templates',
                },
              },
            },
          },
        }}
        formData={{
          process: {
            template: {
              device_code: '{{ .payload.device_code }}',
            },
          },
        }}
        onChange={jest.fn()}
        pluginId="transform-map"
        pluginName="Transform Map"
      />
    );

    expect(await screen.findByTestId('mock-code-editor')).toBeInTheDocument();

    const latestProps = mockEditorPropsSpy.mock.calls[mockEditorPropsSpy.mock.calls.length - 1][0];
    expect(latestProps.mode).toBe('json');
  });

  it('hides timescaledb batch settings and prunes batch config when write_mode is single', async () => {
    const handleChange = jest.fn();

    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            output: {
              type: 'object',
              properties: {
                table: {
                  type: 'string',
                  title: 'Table',
                },
                write_mode: {
                  type: 'string',
                  title: 'Write Mode',
                  enum: ['single', 'batch'],
                  default: 'single',
                },
                batch: {
                  type: 'object',
                  title: 'Batch Settings',
                  properties: {
                    max_rows: {
                      type: 'integer',
                      title: 'Max Rows',
                      default: 200,
                    },
                  },
                },
              },
            },
          },
        }}
        formData={{
          output: {
            table: 'metrics',
            write_mode: 'single',
            batch: {
              max_rows: 200,
            },
          },
        }}
        onChange={handleChange}
        pluginId="sink-timeseries-timescaledb"
        pluginName="Sink Timeseries TimescaleDB"
      />
    );

    expect(screen.queryByText('Batch Settings')).not.toBeInTheDocument();
    expect(screen.queryByText('Max Rows')).not.toBeInTheDocument();

    await waitFor(() => {
      expect(handleChange).toHaveBeenCalledWith({
        output: {
          table: 'metrics',
          write_mode: 'single',
        },
      });
    });
  });

  it('shows timescaledb batch settings when write_mode is batch', async () => {
    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            output: {
              type: 'object',
              properties: {
                table: {
                  type: 'string',
                  title: 'Table',
                },
                write_mode: {
                  type: 'string',
                  title: 'Write Mode',
                  enum: ['single', 'batch'],
                  default: 'single',
                },
                batch: {
                  type: 'object',
                  title: 'Batch Settings',
                  properties: {
                    max_rows: {
                      type: 'integer',
                      title: 'Max Rows',
                    },
                  },
                },
              },
            },
          },
        }}
        formData={{
          output: {
            table: 'metrics',
            write_mode: 'batch',
            batch: {
              max_rows: 500,
            },
          },
        }}
        onChange={jest.fn()}
        pluginId="sink-timeseries-timescaledb"
        pluginName="Sink Timeseries TimescaleDB"
      />
    );

    expect(await screen.findByText('Batch Settings')).toBeInTheDocument();
  });

  it('shows tooltip metadata for array field titles', async () => {
    const errorSpy = jest.spyOn(console, 'error').mockImplementation(() => {});

    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            state: {
              type: 'object',
              properties: {
                key_by: {
                  type: 'array',
                  title: 'Key By',
                  description: 'State partition keys used to isolate change detection per device or metric',
                  items: {
                    type: 'string',
                  },
                },
              },
            },
          },
        }}
        formData={{ state: { key_by: ['device_code'] } }}
        onChange={jest.fn()}
      />
    );

    expect(await screen.findByText('Key By')).toBeInTheDocument();

    const tooltip = screen.getAllByTestId('mock-tooltip').find((node) =>
      node.getAttribute('data-title') === 'State partition keys used to isolate change detection per device or metric'
    );
    expect(tooltip).toBeDefined();
    expect(
      errorSpy.mock.calls.some((args) => args.some((arg) => String(arg).includes('non-boolean attribute `danger`')))
    ).toBe(false);
    expect(
      errorSpy.mock.calls.some((args) => args.some((arg) => String(arg).includes('non-boolean attribute `block`')))
    ).toBe(false);

    errorSpy.mockRestore();
  });

  it('renders a visible label for boolean fields', async () => {
    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            output: {
              type: 'object',
              properties: {
                auto_create_columns: {
                  type: 'boolean',
                  title: 'Auto Create Columns',
                },
              },
            },
          },
        }}
        formData={{ output: { auto_create_columns: false } }}
        onChange={jest.fn()}
      />
    );

    expect(await screen.findByText('Auto Create Columns')).toBeInTheDocument();
  });

  it('renders titles for the reported toggle fields', async () => {
    render(
      <JSONSchemaRenderer
        schema={{
          type: 'object',
          properties: {
            sink_output: {
              type: 'object',
              title: 'Output',
              properties: {
                auto_create_columns: {
                  type: 'boolean',
                  title: 'Auto Create Columns',
                },
              },
            },
            encode: {
              type: 'object',
              title: 'Encode',
              properties: {
                include_metadata: {
                  type: 'boolean',
                  title: 'Include Metadata',
                },
                include_context: {
                  type: 'boolean',
                  title: 'Include Context',
                },
              },
            },
            source_output: {
              type: 'object',
              title: 'Output',
              properties: {
                include_meta: {
                  type: 'boolean',
                  title: 'Include Meta',
                },
              },
            },
          },
        }}
        formData={{
          sink_output: { auto_create_columns: false },
          encode: { include_metadata: false, include_context: false },
          source_output: { include_meta: false },
        }}
        onChange={jest.fn()}
      />
    );

    expect(await screen.findByText('Auto Create Columns')).toBeInTheDocument();
    expect(screen.getByText('Include Metadata')).toBeInTheDocument();
    expect(screen.getByText('Include Context')).toBeInTheDocument();
    expect(screen.getByText('Include Meta')).toBeInTheDocument();
  });
});
