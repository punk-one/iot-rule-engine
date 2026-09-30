import React, { useMemo, useState, useCallback, useRef, useEffect } from 'react';
import Form from '@rjsf/core';
import { customizeValidator } from '@rjsf/validator-ajv8';
import { RJSFSchema, UiSchema, WidgetProps, ObjectFieldTemplateProps, ArrayFieldTemplateProps, FieldTemplateProps, FieldProps } from '@rjsf/utils';
import { Input, InputNumber, Switch, Select, Form as AntdForm, Button, Tooltip } from 'antd';
import { ErrorBoundary } from './ErrorBoundary';
import { LazyCodeEditor } from './LazyCodeEditor';
import { createS7PointWorkbook, parseS7PointWorkbook } from '../utils/s7PointWorkbook';

/* =============================================================================
 * 1. UI Options & Types
 * ============================================================================= */

interface UIOptions {
  collapsible?: boolean;
  collapsed?: boolean;
  title?: string;
  addable?: boolean;
  removable?: boolean;
  placeholder?: string;
  language?: string;
  height?: string | number;
  enableS7Workbook?: boolean;
}

interface JSONSchemaRendererProps {
  schema: string | RJSFSchema;
  formData: Record<string, any>;
  onChange: (formData: Record<string, any>) => void;
  onValidationChange?: (isValid: boolean, errors: any) => void;
  readonly?: boolean;
  pluginId?: string;
  pluginName?: string;
}

type S7PluginKind = 'source' | 'sink' | null;

const validator = customizeValidator({
  customFormats: {
    json: () => true,
    cel: () => true,
    code: () => true,
  },
});

const OPTIONAL_LOCAL_ENDPOINT_KEYS = new Set(['localhost', 'localport', 'local_host', 'local_port']);
const LEGACY_TRANSFORM_MAP_TEMPLATE_DESCRIPTION = 'Structured template object or JSON string rendered with Go templates';
const DEFAULT_TIMESCALE_WRITE_MODE = 'single';

const isPlainObject = (value: any): value is Record<string, any> =>
  value !== null && typeof value === 'object' && !Array.isArray(value);

const cloneValue = <T,>(value: T): T => JSON.parse(JSON.stringify(value));

const detectS7PluginKind = (pluginId?: string, pluginName?: string): S7PluginKind => {
  const fullName = `${pluginId || ''} ${pluginName || ''}`.toLowerCase();
  if (/(^|[\s_-])source[\s_-]?s7($|[\s_-])/.test(fullName)) return 'source';
  if (/(^|[\s_-])sink[\s_-]?s7($|[\s_-])/.test(fullName)) return 'sink';
  return null;
};

const isTransformMapPlugin = (pluginId?: string, pluginName?: string): boolean => {
  const fullName = `${pluginId || ''} ${pluginName || ''}`.toLowerCase();
  return fullName.includes('transform-map') || fullName.includes('transform map');
};

const isTimescaleSinkPlugin = (pluginId?: string, pluginName?: string): boolean => {
  const fullName = `${pluginId || ''} ${pluginName || ''}`.toLowerCase();
  return fullName.includes('sink-timeseries-timescaledb') || fullName.includes('sink timeseries timescaledb');
};

const dropRequiredKeys = (schema: RJSFSchema, keys: Set<string>) => {
  if (!Array.isArray(schema.required)) return;
  schema.required = schema.required.filter((field) => !keys.has(String(field)));
};

const setNumberDefault = (schema: RJSFSchema, field: string, defaultValue: number) => {
  if (!isPlainObject(schema.properties)) return;
  const prop = (schema.properties[field] as RJSFSchema | undefined);
  if (!prop) {
    schema.properties[field] = { type: 'integer', default: defaultValue };
    return;
  }
  prop.default = defaultValue;
};

const normalizeSchemaForS7 = (schema: RJSFSchema, pluginKind: S7PluginKind): RJSFSchema => {
  const normalized = cloneValue(schema);
  if (!pluginKind || !isPlainObject(normalized.properties)) {
    return normalized;
  }

  // 统一连接配置行为（source-s7 / sink-s7）
  const connectionSchema = normalized.properties.connection as RJSFSchema | undefined;
  if (connectionSchema && isPlainObject(connectionSchema.properties)) {
    dropRequiredKeys(connectionSchema, OPTIONAL_LOCAL_ENDPOINT_KEYS);
    setNumberDefault(connectionSchema, 'rack', 0);
    setNumberDefault(connectionSchema, 'timeout_sec', 5);
    setNumberDefault(connectionSchema, 'idle_timeout_sec', 30);
  }

  // 防御性处理：即使 schema 顶层误设 required，也强制 local host/port 可选
  dropRequiredKeys(normalized, OPTIONAL_LOCAL_ENDPOINT_KEYS);

  if (pluginKind === 'sink') {
    // 某些 sink-s7 版本 schema 里可能缺失 device_code，这里兜底补齐 UI 字段
    if (!normalized.properties.device_code) {
      normalized.properties.device_code = {
        type: 'string',
        title: 'device_code',
        description: '设备编码',
      };
    }
  }

  return normalized;
};

const normalizeSchemaForTransformMap = (schema: RJSFSchema, isTransformMap: boolean): RJSFSchema => {
  const normalized = cloneValue(schema);
  if (!isTransformMap || !isPlainObject(normalized.properties)) {
    return normalized;
  }

  const processSchema = normalized.properties.process as RJSFSchema | undefined;
  if (!processSchema || !isPlainObject(processSchema.properties)) {
    return normalized;
  }

  const templateSchema = processSchema.properties.template as RJSFSchema | undefined;
  if (!templateSchema || Array.isArray(templateSchema) || typeof templateSchema !== 'object') {
    return normalized;
  }

  if (templateSchema.type) {
    return normalized;
  }

  if (templateSchema.description !== LEGACY_TRANSFORM_MAP_TEMPLATE_DESCRIPTION) {
    return normalized;
  }

  processSchema.properties.template = {
    title: 'Template',
    type: 'string',
    format: 'json',
    ...templateSchema,
  };

  return normalized;
};

const resolveTimescaleWriteMode = (formData: Record<string, any>): string => {
  const rawMode = formData?.output?.write_mode;
  if (typeof rawMode !== 'string' || rawMode.trim() === '') {
    return DEFAULT_TIMESCALE_WRITE_MODE;
  }
  return rawMode.trim();
};

const normalizeSchemaForTimescaleSink = (
  schema: RJSFSchema,
  isTimescaleSink: boolean,
  formData: Record<string, any>
): RJSFSchema => {
  const normalized = cloneValue(schema);
  if (!isTimescaleSink || !isPlainObject(normalized.properties)) {
    return normalized;
  }

  const outputSchema = normalized.properties.output as RJSFSchema | undefined;
  if (!outputSchema || !isPlainObject(outputSchema.properties)) {
    return normalized;
  }

  if (resolveTimescaleWriteMode(formData) !== 'batch') {
    delete outputSchema.properties.batch;
  }

  return normalized;
};

const normalizeFormDataForTimescaleSink = (formData: Record<string, any>, isTimescaleSink: boolean): Record<string, any> => {
  const normalized = isPlainObject(formData) ? cloneValue(formData) : {};
  if (!isTimescaleSink || !isPlainObject(normalized.output)) {
    return normalized;
  }

  if (resolveTimescaleWriteMode(normalized) !== 'batch') {
    delete normalized.output.batch;
  }

  return normalized;
};

export const applySchemaDefaults = (schema: RJSFSchema, input: any): any => {
  if (schema.type !== 'object' || !isPlainObject(schema.properties)) {
    return input;
  }

  const current = isPlainObject(input) ? { ...input } : {};

  for (const [key, propSchema] of Object.entries(schema.properties)) {
    const childSchema = propSchema as RJSFSchema;
    const childValue = current[key];

    if (childSchema.type === 'object' && isPlainObject(childSchema.properties)) {
      const nestedValue = applySchemaDefaults(childSchema, isPlainObject(childValue) ? childValue : {});
      if (childValue !== undefined || Object.keys(nestedValue || {}).length > 0) {
        current[key] = nestedValue;
      }
      continue;
    }

    if (childValue === undefined && childSchema.default !== undefined) {
      current[key] = cloneValue(childSchema.default);
      continue;
    }
  }

  return current;
};

/* =============================================================================
 * 2. Custom Templates (Object, Array, Field)
 * ============================================================================= */

/**
 * ObjectFieldTemplate: Handles object rendering and collapsible sections.
 * Object behavior is defined by ui:options, not ui:widget.
 */
const CollapsibleObjectFieldTemplate = (props: ObjectFieldTemplateProps) => {
  const { title, description, properties, uiSchema, idSchema, required } = props;

  const options = (uiSchema?.['ui:options'] || {}) as UIOptions;
  const isCollapsible = options.collapsible === true;
  const isCollapsedInitial = options.collapsed === true;
  const displayTitle = options.title || title;

  const [collapsed, setCollapsed] = useState(isCollapsedInitial);

  // If no title and not collapsible, just render children
  if (!displayTitle && !isCollapsible) {
    return (
      <div id={idSchema.$id}>
        {properties.map((prop) => (
          <div key={prop.name} style={{ marginBottom: 12 }}>{prop.content}</div>
        ))}
      </div>
    );
  }

  return (
    <fieldset
      id={idSchema.$id}
      style={{
        border: '1px solid #f0f0f0',
        borderRadius: '8px',
        marginBottom: '16px',
        padding: '0',
        overflow: 'hidden',
        backgroundColor: 'var(--color-surface)'
      }}
    >
      <div
        onClick={() => isCollapsible && setCollapsed(!collapsed)}
        style={{
          padding: '10px 16px',
          backgroundColor: '#fafafa',
          borderBottom: collapsed ? 'none' : '1px solid #f0f0f0',
          cursor: isCollapsible ? 'pointer' : 'default',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          userSelect: 'none'
        }}
      >
        <span style={{ fontWeight: 600, fontSize: '14px', color: '#262626' }}>
          {displayTitle}
          {required && <span style={{ color: '#ff4d4f', marginLeft: '4px' }}>*</span>}
          {description && (
            <Tooltip title={description}>
              <span style={{ marginLeft: '8px', color: '#8c8c8c', fontSize: '12px', cursor: 'help' }}>ⓘ</span>
            </Tooltip>
          )}
        </span>
        {isCollapsible && (
          <span style={{ fontSize: '12px', color: '#8c8c8c' }}>
            {collapsed ? '▶ 展开' : '▼ 收起'}
          </span>
        )}
      </div>

      {!collapsed && (
        <div style={{ padding: '16px 16px 4px 16px' }}>
          {properties.map((prop) => (
            <div key={prop.name}>{prop.content}</div>
          ))}
        </div>
      )}
    </fieldset>
  );
};

/**
 * ArrayFieldTemplate: Handles array rendering with Antd buttons.
 */
const CustomArrayFieldTemplate = (props: ArrayFieldTemplateProps) => {
  const { title, items, canAdd, onAddClick, uiSchema, required, schema } = props;
  const options = (uiSchema?.['ui:options'] || {}) as UIOptions;
  const addable = options.addable !== false;
  const removable = options.removable !== false;
  const arrayDescription = (schema as any)?.description || '';

  return (
    <div style={{ marginBottom: '16px' }}>
      {title && (
        <div style={{ marginBottom: '8px', fontWeight: 500, fontSize: '14px' }}>
          {title} {required && <span style={{ color: '#ff4d4f' }}>*</span>}
          {arrayDescription && (
            <Tooltip title={arrayDescription}>
              <span style={{ marginLeft: '8px', color: '#8c8c8c', fontSize: '12px', cursor: 'help' }}>ⓘ</span>
            </Tooltip>
          )}
        </div>
      )}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
        {items.map((item) => (
          <div key={item.key} style={{ display: 'flex', gap: '8px', alignItems: 'flex-start' }}>
            <div style={{ flex: 1 }}>{item.children}</div>
            {removable && item.hasRemove && (
              <Button
                type="text"
                danger
                onClick={item.onDropIndexClick(item.index)}
                style={{ marginTop: '32px' }} // Align with field
              >
                删除
              </Button>
            )}
          </div>
        ))}
      </div>
      {addable && canAdd && (
        <Button
          type="dashed"
          onClick={onAddClick}
          block
          style={{ marginTop: '8px' }}
        >
          + 添加项目
        </Button>
      )}
    </div>
  );
};

/**
 * FieldTemplate: Standard field wrapper using Antd Form.Item.
 * Description is shown as Tooltip in the label.
 */
const CustomFieldTemplate = (props: FieldTemplateProps) => {
  const { id, label, children, errors, help, description, hidden, required, displayLabel, schema } = props;
  if (hidden) return null;

  // RJSF 的 description 可能来自 props.description 或 schema.description
  // 优先使用 props.description，如果没有则从 schema 中获取
  const fieldDescription = description || (schema as any)?.description || '';

  // 调试：输出字段信息以便排查
  if (label && (label.includes('device_code') || label.includes('product_type') || label.includes('Device Code') || label.includes('Product Type'))) {
    console.log('[CustomFieldTemplate] Field info:', {
      label,
      id,
      descriptionProp: description,
      schemaDescription: (schema as any)?.description,
      fieldDescription,
      fullProps: props,
    });
  }

  // 构建带 tooltip 的 label
  const labelElement = displayLabel && label ? (
    <span>
      {label}
      {required && <span style={{ color: '#ff4d4f', marginLeft: '4px' }}>*</span>}
      {fieldDescription && (
        <Tooltip title={fieldDescription}>
          <span style={{ marginLeft: '8px', color: '#8c8c8c', fontSize: '12px', cursor: 'help' }}>ⓘ</span>
        </Tooltip>
      )}
    </span>
  ) : null;

  return (
    <AntdForm.Item
      label={labelElement}
      required={false} // required 标记已经在 label 中显示
      validateStatus={errors ? 'error' : ''}
      help={errors || help} // description 不再显示在 help 中，而是显示为 tooltip
      labelCol={{ span: 24 }} // 垂直布局：标签占满整行
      wrapperCol={{ span: 24 }} // 垂直布局：输入框占满整行
      style={{ marginBottom: '12px' }}
    >
      {children}
    </AntdForm.Item>
  );
};

/* =============================================================================
 * 3. Custom Widgets (Code editor, Input, etc.)
 * ============================================================================= */

const CodeEditorWidget = (props: WidgetProps) => {
  const { value, onChange, schema, options } = props;
  const format = schema.format;
  const storesJSONString = format === 'json' && schema.type === 'string';
  const mode = format === 'json' ? 'json' : 'text';
  const widgetOptions = (options || {}) as UIOptions;
  const height = widgetOptions.height || '200px';
  const placeholder = widgetOptions.placeholder || props.placeholder || '';

  const handleChange = (val: string | undefined) => {
    if (format === 'json') {
      if (storesJSONString) {
        onChange(val || '');
        return;
      }
      try {
        const parsed = JSON.parse(val || '{}');
        onChange(parsed);
      } catch {
        onChange(val);
      }
    } else {
      onChange(val);
    }
  };

  const displayValue = format === 'json' && typeof value !== 'string'
    ? JSON.stringify(value, null, 2)
    : (value || '');

  return (
    <div style={{ border: '1px solid #d9d9d9', borderRadius: '4px', overflow: 'hidden' }}>
      <LazyCodeEditor
        height={height}
        mode={mode}
        value={displayValue}
        onChange={(nextValue) => handleChange(nextValue)}
        readOnly={props.disabled || props.readonly}
        placeholder={placeholder}
      />
    </div>
  );
};

const CustomTextWidget = (props: WidgetProps) => {
  return <Input value={props.value || ''} onChange={(e) => props.onChange(e.target.value)} placeholder={props.placeholder} disabled={props.disabled || props.readonly} />;
};

const CustomNumberWidget = (props: WidgetProps) => {
  return <InputNumber style={{ width: '100%' }} value={props.value} onChange={props.onChange} disabled={props.disabled || props.readonly} />;
};

const CustomSelectWidget = (props: WidgetProps) => {
  const { options, value, onChange, disabled, readonly } = props;
  return (
    <Select
      style={{ width: '100%' }}
      value={value}
      onChange={(val) => onChange(val)}
      disabled={disabled || readonly}
      options={(options.enumOptions as any[])?.map(opt => ({ label: opt.label, value: opt.value }))}
    />
  );
};

const CustomCheckboxWidget = (props: WidgetProps) => {
  const inlineLabel = props.label || props.schema.title || '';

  return (
    <label
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: '8px',
        cursor: props.disabled || props.readonly ? 'not-allowed' : 'pointer',
      }}
    >
      <Switch checked={!!props.value} onChange={(checked) => props.onChange(checked)} disabled={props.disabled || props.readonly} />
      {inlineLabel && <span>{inlineLabel}</span>}
    </label>
  );
};

const CustomTextareaWidget = (props: WidgetProps) => {
  return <Input.TextArea rows={4} value={props.value || ''} onChange={(e) => props.onChange(e.target.value)} placeholder={props.placeholder} disabled={props.disabled || props.readonly} />;
};

const JSON_ARRAY_HINT = '请输入 JSON 数组，支持多行滚动编辑';

const JsonArrayField: React.FC<FieldProps> = (props) => {
  const { formData, onChange, disabled, readonly, uiSchema, rawErrors } = props;
  const uiOptions = ((uiSchema?.['ui:options'] || {}) as UIOptions) as UIOptions & { rows?: number };
  const rows = uiOptions.rows || 12;
  const height = uiOptions.height || 260;

  const formatArray = useCallback((value: any) => {
    return JSON.stringify(Array.isArray(value) ? value : [], null, 2);
  }, []);

  const [editorText, setEditorText] = useState<string>(() => formatArray(formData));
  const [parseError, setParseError] = useState<string>('');
  const lastSyncedRef = useRef<string>(formatArray(formData));
  const jsonFileRef = useRef<HTMLInputElement>(null);
  const excelFileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const next = formatArray(formData);
    if (next !== lastSyncedRef.current) {
      setEditorText(next);
      setParseError('');
      lastSyncedRef.current = next;
    }
  }, [formData, formatArray]);

  const handleChange = (event: React.ChangeEvent<HTMLTextAreaElement>) => {
    const nextText = event.target.value;
    setEditorText(nextText);

    if (nextText.trim() === '') {
      onChange([]);
      setParseError('');
      lastSyncedRef.current = '[]';
      return;
    }

    try {
      const parsed = JSON.parse(nextText);
      if (!Array.isArray(parsed)) {
        setParseError('必须是 JSON 数组（例如: []）');
        return;
      }
      onChange(parsed);
      setParseError('');
      lastSyncedRef.current = formatArray(parsed);
    } catch {
      setParseError('JSON 格式错误，请检查括号、逗号与引号');
    }
  };

  const hasError = parseError || (rawErrors && rawErrors.length > 0);
  const replaceArray = useCallback((parsed: any[]) => {
    onChange(parsed);
    const formatted = formatArray(parsed);
    setEditorText(formatted);
    setParseError('');
    lastSyncedRef.current = formatted;
  }, [formatArray, onChange]);

  const download = (blob: Blob, fileName: string) => {
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = fileName;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const exportJSON = () => {
    download(new Blob([formatArray(formData)], { type: 'application/json;charset=utf-8' }), 's7-points.json');
  };

  const importJSON = async (file?: File) => {
    if (!file) return;
    try {
      const parsed = JSON.parse(await file.text());
      if (!Array.isArray(parsed)) throw new Error('文件根节点必须是 JSON 数组');
      replaceArray(parsed);
    } catch (error) {
      setParseError(error instanceof Error ? error.message : 'JSON 导入失败');
    } finally {
      if (jsonFileRef.current) jsonFileRef.current.value = '';
    }
  };

  const exportExcel = async () => {
    try {
      const buffer = await createS7PointWorkbook(Array.isArray(formData) ? formData : []);
      download(new Blob([buffer as BlobPart], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' }), 's7-points.xlsx');
    } catch (error) {
      setParseError(error instanceof Error ? error.message : 'Excel 导出失败');
    }
  };

  const importExcel = async (file?: File) => {
    if (!file) return;
    try {
      if (file.size > 5 * 1024 * 1024) throw new Error('Excel 文件不能超过 5 MB');
      replaceArray(await parseS7PointWorkbook(await file.arrayBuffer()));
    } catch (error) {
      setParseError(error instanceof Error ? error.message : 'Excel 导入失败');
    } finally {
      if (excelFileRef.current) excelFileRef.current.value = '';
    }
  };

  return (
    <div>
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 8 }}>
    <Button size="small" onClick={() => jsonFileRef.current?.click()} disabled={disabled || readonly}>导入 JSON</Button>
    <Button size="small" onClick={exportJSON} disabled={disabled || readonly}>导出 JSON</Button>
    {uiOptions.enableS7Workbook && (
      <>
      <Button size="small" onClick={() => excelFileRef.current?.click()} disabled={disabled || readonly}>导入 Excel</Button>
      <Button size="small" onClick={exportExcel} disabled={disabled || readonly}>导出 Excel</Button>
      </>
    )}
    <input ref={jsonFileRef} type="file" accept="application/json,.json" hidden onChange={(event) => importJSON(event.target.files?.[0])} />
    <input ref={excelFileRef} type="file" accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" hidden onChange={(event) => importExcel(event.target.files?.[0])} />
    </div>
      <Input.TextArea
        value={editorText}
        onChange={handleChange}
        rows={rows}
        autoSize={false}
        disabled={disabled || readonly}
        placeholder={JSON_ARRAY_HINT}
        style={{
          fontFamily: 'SFMono-Regular, Consolas, Menlo, monospace',
          lineHeight: 1.45,
          height: typeof height === 'number' ? `${height}px` : height,
          overflowY: 'auto',
          resize: 'vertical',
        }}
      />
      {parseError && (
        <div style={{ color: '#ff4d4f', fontSize: '12px', marginTop: '4px' }}>
          {parseError}
        </div>
      )}
      {!parseError && rawErrors && rawErrors.length > 0 && (
        <div style={{ color: '#ff4d4f', fontSize: '12px', marginTop: '4px' }}>
          {rawErrors.join('；')}
        </div>
      )}
      {!hasError && (
        <div style={{ color: '#8c8c8c', fontSize: '12px', marginTop: '4px' }}>
          {JSON_ARRAY_HINT}
        </div>
      )}
    </div>
  );
};

const widgets = {
  TextWidget: CustomTextWidget,
  TextareaWidget: CustomTextareaWidget,
  UpdownWidget: CustomNumberWidget,
  SelectWidget: CustomSelectWidget,
  CheckboxWidget: CustomCheckboxWidget,
  codeEditor: CodeEditorWidget,
};

const fields = {
  jsonArray: JsonArrayField,
};

/* =============================================================================
 * 4. Helper: Automatic uiSchema Builder
 * ============================================================================= */

function buildUISchema(schema: RJSFSchema, basePath = '', ui: UiSchema = {}, jsonArrayFields: Set<string> = new Set(), s7WorkbookFields: Set<string> = new Set()): UiSchema {
  if (!schema.properties) return ui;

  for (const [key, prop] of Object.entries(schema.properties)) {
    const currentProp = prop as RJSFSchema;

    // Object type handling. NO ui:widget for objects.
    if (currentProp.type === 'object') {
      const isTopLevel = ['input', 'process', 'state', 'output'].includes(key);
      ui[key] = {
        'ui:options': {
          collapsible: isTopLevel || !!currentProp.properties,
          collapsed: false,
          title: currentProp.title || key.charAt(0).toUpperCase() + key.slice(1),
        },
      };

      if (currentProp.properties) {
    buildUISchema(currentProp, key, ui[key], jsonArrayFields, s7WorkbookFields);
      }
      continue;
    }

    // Array type handling
    if (currentProp.type === 'array') {
      if (jsonArrayFields.has(key)) {
        ui[key] = {
          'ui:field': 'jsonArray',
          'ui:options': {
            rows: 14,
            height: 280,
      enableS7Workbook: s7WorkbookFields.has(key),
          },
        };
        continue;
      }

      ui[key] = {
        'ui:options': {
          addable: true,
          removable: true,
        },
      };
      if (currentProp.items && typeof currentProp.items === 'object') {
    ui[key].items = buildUISchema(currentProp.items as RJSFSchema, 'items', {}, jsonArrayFields, s7WorkbookFields);
      }
      continue;
    }

    // Shared code editor activation for string with specific formats
    if (currentProp.type === 'string') {
      if (['json', 'cel', 'code'].includes(currentProp.format || '')) {
        const requestedHeight = (currentProp as any)['x-ui-height'];
        const requestedPlaceholder = (currentProp as any)['x-ui-placeholder'];
        ui[key] = {
          'ui:widget': 'codeEditor',
          'ui:options': {
            height: requestedHeight || (currentProp.format === 'code' ? '480px' : '240px'),
            placeholder: requestedPlaceholder || '',
          }
        };
      } else if (currentProp.format === 'textarea') {
        ui[key] = { 'ui:widget': 'textarea' };
      }
    }
  }

  return ui;
}

/* =============================================================================
 * 5. Main Component: JSONSchemaRenderer
 * ============================================================================= */

export const JSONSchemaRenderer: React.FC<JSONSchemaRendererProps> = ({
  schema,
  formData,
  onChange,
  onValidationChange,
  readonly = false,
  pluginId,
  pluginName,
}) => {
  // 1. Parse Schema
  const parsedSchema = useMemo<RJSFSchema>(() => {
    if (typeof schema === 'object') return schema;
    try {
      // Cleanup special characters that often occur in copy-paste
      const cleaned = schema.replace(/[\u00a0\u1680\u180e\u2000-\u200b\u202f\u205f\u3000\ufeff]/g, " ");
      return JSON.parse(cleaned);
    } catch (e) {
      console.error('Failed to parse schema JSON', e);
      return { type: 'object', properties: {} };
    }
  }, [schema]);

  const s7PluginKind = useMemo(() => detectS7PluginKind(pluginId, pluginName), [pluginId, pluginName]);
  const transformMapPlugin = useMemo(() => isTransformMapPlugin(pluginId, pluginName), [pluginId, pluginName]);
  const timescaleSinkPlugin = useMemo(() => isTimescaleSinkPlugin(pluginId, pluginName), [pluginId, pluginName]);
  const normalizedFormData = useMemo(
    () => normalizeFormDataForTimescaleSink(formData || {}, timescaleSinkPlugin),
    [formData, timescaleSinkPlugin]
  );

  // 2. 按插件类型归一化 schema（仅对 source-s7 / sink-s7 生效）
  const normalizedSchema = useMemo(() => {
    const s7Normalized = normalizeSchemaForS7(parsedSchema, s7PluginKind);
    const transformMapNormalized = normalizeSchemaForTransformMap(s7Normalized, transformMapPlugin);
    return normalizeSchemaForTimescaleSink(transformMapNormalized, timescaleSinkPlugin, normalizedFormData);
  }, [parsedSchema, s7PluginKind, transformMapPlugin, timescaleSinkPlugin, normalizedFormData]);

  // 3. 选择需要走 JSON 数组编辑器的字段
  const jsonArrayFields = useMemo(() => {
    const fields = new Set<string>();
    if (s7PluginKind === 'source') fields.add('points');
    if (s7PluginKind === 'sink') fields.add('allowed_writes');
    return fields;
  }, [s7PluginKind]);
  const s7WorkbookFields = useMemo(() => {
    const fields = new Set<string>();
    if (s7PluginKind === 'source') fields.add('points');
    return fields;
  }, [s7PluginKind]);

  // 4. Build uiSchema automatically based on current rules
  const uiSchema = useMemo(
    () => buildUISchema(normalizedSchema, '', {}, jsonArrayFields, s7WorkbookFields),
    [normalizedSchema, jsonArrayFields, s7WorkbookFields]
  );

  // 5. 将 schema 默认值写入 formData（例如 S7 的 rack/timeout 默认值）
  const formDataWithDefaults = useMemo(
    () => applySchemaDefaults(normalizedSchema, normalizedFormData),
    [normalizedSchema, normalizedFormData]
  );

  useEffect(() => {
    const current = JSON.stringify(formData || {});
    const merged = JSON.stringify(formDataWithDefaults || {});
    if (current !== merged) {
      onChange(formDataWithDefaults);
    }
  }, [formData, formDataWithDefaults, onChange]);

  // 6. Handle data changes and validation
  const handleChange = useCallback((data: any) => {
    onChange(data.formData);
      if (onValidationChange) {
      const isValid = !data.errors || data.errors.length === 0;
      onValidationChange(isValid, data.errors);
    }
  }, [onChange, onValidationChange]);

  // If empty schema
  if (!normalizedSchema.properties || Object.keys(normalizedSchema.properties).length === 0) {
    return (
      <div style={{ padding: '24px', textAlign: 'center', color: '#8c8c8c', backgroundColor: '#fafafa', borderRadius: '8px' }}>
        此节点无需配置
      </div>
    );
  }

  return (
    <div className="json-schema-renderer">
      <ErrorBoundary
        fallback={
          <div style={{ padding: '20px', color: '#ff4d4f', backgroundColor: '#fff2f0', border: '1px solid #ffccc7', borderRadius: '8px' }}>
            <h4 style={{ color: '#ff4d4f', marginTop: 0 }}>配置渲染错误</h4>
            <p style={{ margin: '8px 0', fontSize: '13px' }}>请检查插件配置 Schema 是否符合当前规范。具体错误请检查控制台。</p>
          </div>
        }
      >
        <Form
          schema={normalizedSchema}
          uiSchema={uiSchema}
          formData={formDataWithDefaults || {}}
          validator={validator}
          fields={fields}
          widgets={widgets}
          templates={{
            ObjectFieldTemplate: CollapsibleObjectFieldTemplate,
            ArrayFieldTemplate: CustomArrayFieldTemplate,
            FieldTemplate: CustomFieldTemplate,
          }}
          onChange={handleChange}
          readonly={readonly}
          liveValidate
          showErrorList={false}
          omitExtraData={true}
        >
          {/* Hide default submit button */}
          <div style={{ display: 'none' }} />
        </Form>
      </ErrorBoundary>
    </div>
  );
};

export default JSONSchemaRenderer;
