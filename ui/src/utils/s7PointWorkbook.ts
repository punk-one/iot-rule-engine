import ExcelJS from 'exceljs';

export type S7PointRange = {
  minimum?: number;
  maximum?: number;
  policy?: 'use_default' | 'clamp' | 'drop' | 'error';
  default_value?: number;
};

export type S7PointDefinition = {
  name: string;
  address: string;
  value_type: string;
  read_write?: 'R' | 'RO' | 'RW';
  description?: string;
  max_length?: number;
  scale?: number;
  precision?: number;
  range?: S7PointRange;
};

const COLUMNS = [
  { header: 'name', key: 'name', width: 24 },
  { header: 'address', key: 'address', width: 22 },
  { header: 'value_type', key: 'value_type', width: 16 },
  { header: 'read_write', key: 'read_write', width: 14 },
  { header: 'description', key: 'description', width: 30 },
  { header: 'max_length', key: 'max_length', width: 14 },
  { header: 'scale', key: 'scale', width: 12 },
  { header: 'precision', key: 'precision', width: 12 },
  { header: 'range_min', key: 'range_min', width: 14 },
  { header: 'range_max', key: 'range_max', width: 14 },
  { header: 'out_of_range', key: 'out_of_range', width: 18 },
  { header: 'default_value', key: 'default_value', width: 16 },
] as const;

const HEADER_ALIASES: Record<string, string> = {
  name: 'name', '点位名称': 'name',
  address: 'address', nodename: 'address', '地址': 'address',
  value_type: 'value_type', valuetype: 'value_type', '数据类型': 'value_type',
  read_write: 'read_write', readwrite: 'read_write', '读写': 'read_write',
  description: 'description', '描述': 'description',
  max_length: 'max_length', maxlength: 'max_length', '最大长度': 'max_length',
  scale: 'scale', '缩放': 'scale',
  precision: 'precision', '精度': 'precision',
  range_min: 'range_min', minimum: 'range_min', min: 'range_min', '最小值': 'range_min',
  range_max: 'range_max', maximum: 'range_max', max: 'range_max', '最大值': 'range_max',
  out_of_range: 'out_of_range', policy: 'out_of_range', '越界策略': 'out_of_range',
  default_value: 'default_value', defaultvalue: 'default_value', '越界默认值': 'default_value',
};

const ALLOWED_VALUE_TYPES = new Set([
  'Bool', 'Int8', 'Uint8', 'Int16', 'Uint16', 'Int32', 'Uint32',
  'Float32', 'Float64', 'Byte', 'Word', 'DWord', 'String', 'WString',
]);
const ALLOWED_POLICIES = new Set(['use_default', 'clamp', 'drop', 'error']);
const NUMERIC_VALUE_TYPES = new Set(['Int8', 'Uint8', 'Int16', 'Uint16', 'Int32', 'Uint32', 'Float32', 'Float64', 'Byte', 'Word', 'DWord']);
const MAX_WORKBOOK_BYTES = 5 * 1024 * 1024;
const MAX_POINTS = 5000;

const textValue = (value: ExcelJS.CellValue): string => {
  if (value === null || value === undefined) return '';
  if (typeof value === 'object' && 'text' in value) return String(value.text).trim();
  if (typeof value === 'object' && 'result' in value) return String(value.result ?? '').trim();
  return String(value).trim();
};

const optionalNumber = (value: ExcelJS.CellValue, rowNumber: number, field: string): number | undefined => {
  const text = textValue(value);
  if (text === '') return undefined;
  const parsed = Number(text);
  if (!Number.isFinite(parsed)) {
    throw new Error(`第 ${rowNumber} 行 ${field} 必须是有限数字`);
  }
  return parsed;
};

const normalizeValueType = (raw: string): string => {
  const lower = raw.toLowerCase();
  const found = [...ALLOWED_VALUE_TYPES].find((item) => item.toLowerCase() === lower);
  return found || raw;
};

export async function createS7PointWorkbook(points: S7PointDefinition[]): Promise<ExcelJS.Buffer> {
  if (points.length > MAX_POINTS) throw new Error(`点位数量不能超过 ${MAX_POINTS}`);
  const workbook = new ExcelJS.Workbook();
  workbook.creator = 'iot-rule-engine';
  workbook.created = new Date();
  const sheet = workbook.addWorksheet('S7点表', { views: [{ state: 'frozen', ySplit: 1 }] });
  sheet.columns = COLUMNS.map((column) => ({ ...column }));

  for (const point of points) {
    sheet.addRow({
      name: point.name,
      address: point.address,
      value_type: point.value_type,
      read_write: point.read_write || 'R',
      description: point.description || '',
      max_length: point.max_length,
      scale: point.scale,
      precision: point.precision,
      range_min: point.range?.minimum,
      range_max: point.range?.maximum,
      out_of_range: point.range?.policy,
      default_value: point.range?.default_value,
    });
  }

  const header = sheet.getRow(1);
  header.font = { bold: true, color: { argb: 'FFFFFFFF' } };
  header.fill = { type: 'pattern', pattern: 'solid', fgColor: { argb: 'FF1F4E78' } };
  header.alignment = { vertical: 'middle', horizontal: 'center' };
  header.height = 24;
  sheet.autoFilter = { from: 'A1', to: 'L1' };

  for (let row = 2; row <= Math.max(points.length + 1, 501); row += 1) {
    sheet.getCell(`C${row}`).dataValidation = {
      type: 'list', allowBlank: false,
      formulae: [`"${[...ALLOWED_VALUE_TYPES].join(',')}"`],
    };
    sheet.getCell(`D${row}`).dataValidation = { type: 'list', allowBlank: true, formulae: ['"R,RO,RW"'] };
    sheet.getCell(`K${row}`).dataValidation = { type: 'list', allowBlank: true, formulae: ['"use_default,clamp,drop,error"'] };
  }

  sheet.addTable({
    name: 'S7Points', ref: 'A1', headerRow: true,
    rows: points.map((point) => [
      point.name, point.address, point.value_type, point.read_write || 'R', point.description || '',
      point.max_length ?? null, point.scale ?? null, point.precision ?? null,
      point.range?.minimum ?? null, point.range?.maximum ?? null,
      point.range?.policy ?? null, point.range?.default_value ?? null,
    ]),
    columns: COLUMNS.map((column) => ({ name: column.header })),
    style: { theme: 'TableStyleMedium2', showRowStripes: true },
  });

  const help = workbook.addWorksheet('填写说明');
  help.columns = [{ width: 22 }, { width: 90 }];
  help.addRows([
    ['字段', '说明'],
    ['name / address / value_type', '必填；name 在一个 source-s7 节点内必须唯一。'],
    ['scale / precision', '先缩放，再按 precision 保留小数位；precision=0 表示取整。'],
    ['range_min / range_max', '工程值范围，可只填一侧；范围判断发生在缩放与精度处理之后。'],
    ['out_of_range', 'use_default 使用 default_value；clamp 截断到边界；drop 丢弃该点；error 使本轮采集失败。'],
    ['兼容表头', '导入同时识别 nodeName、valueType、readWrite、minimum、maximum、policy、defaultValue 及中文表头。'],
  ]);
  help.getRow(1).font = { bold: true, color: { argb: 'FFFFFFFF' } };
  help.getRow(1).fill = { type: 'pattern', pattern: 'solid', fgColor: { argb: 'FF1F4E78' } };
  help.getColumn(2).alignment = { wrapText: true, vertical: 'top' };

  return workbook.xlsx.writeBuffer();
}

export async function parseS7PointWorkbook(data: ExcelJS.Buffer | ArrayBuffer): Promise<S7PointDefinition[]> {
  if (data.byteLength > MAX_WORKBOOK_BYTES) throw new Error('Excel 文件不能超过 5 MB');
  const workbook = new ExcelJS.Workbook();
  await workbook.xlsx.load(data as ExcelJS.Buffer);
  const sheet = workbook.getWorksheet('S7点表') || workbook.worksheets[0];
  if (!sheet || sheet.rowCount < 1) throw new Error('工作簿中没有点表数据');
  if (sheet.rowCount > MAX_POINTS + 1) throw new Error(`点位数量不能超过 ${MAX_POINTS}`);

  const headerMap = new Map<number, string>();
  sheet.getRow(1).eachCell((cell, columnNumber) => {
    const normalized = textValue(cell.value).toLowerCase().replace(/\s+/g, '');
    const key = HEADER_ALIASES[normalized];
    if (key) headerMap.set(columnNumber, key);
  });
  for (const required of ['name', 'address', 'value_type']) {
    if (![...headerMap.values()].includes(required)) throw new Error(`缺少必填列 ${required}`);
  }

  const points: S7PointDefinition[] = [];
  const names = new Set<string>();
  for (let rowNumber = 2; rowNumber <= sheet.rowCount; rowNumber += 1) {
    const row = sheet.getRow(rowNumber);
    const record: Record<string, ExcelJS.CellValue> = {};
    headerMap.forEach((key, columnNumber) => { record[key] = row.getCell(columnNumber).value; });
    if ([...headerMap.keys()].every((columnNumber) => textValue(row.getCell(columnNumber).value) === '')) continue;

    const name = textValue(record.name);
    const address = textValue(record.address);
    const valueType = normalizeValueType(textValue(record.value_type));
    if (!name || !address || !valueType) throw new Error(`第 ${rowNumber} 行 name、address、value_type 均为必填`);
    if (names.has(name)) throw new Error(`第 ${rowNumber} 行点位名称 ${name} 重复`);
    if (!ALLOWED_VALUE_TYPES.has(valueType)) throw new Error(`第 ${rowNumber} 行 value_type ${valueType} 不受支持`);
    names.add(name);

    const readWrite = (textValue(record.read_write).toUpperCase() || 'R') as S7PointDefinition['read_write'];
    if (!['R', 'RO', 'RW'].includes(readWrite || '')) throw new Error(`第 ${rowNumber} 行 read_write 必须是 R、RO 或 RW`);
    const maxLength = optionalNumber(record.max_length, rowNumber, 'max_length');
    const scale = optionalNumber(record.scale, rowNumber, 'scale');
    const precision = optionalNumber(record.precision, rowNumber, 'precision');
    if (maxLength !== undefined && (!Number.isInteger(maxLength) || maxLength < 1 || maxLength > 254)) throw new Error(`第 ${rowNumber} 行 max_length 必须是 1-254 的整数`);
    if (scale !== undefined && scale <= 0) throw new Error(`第 ${rowNumber} 行 scale 必须大于 0`);
    if (precision !== undefined && (!Number.isInteger(precision) || precision < 0 || precision > 15)) throw new Error(`第 ${rowNumber} 行 precision 必须是 0-15 的整数`);
  if (!NUMERIC_VALUE_TYPES.has(valueType) && ((scale !== undefined && scale !== 1) || precision !== undefined)) {
    throw new Error(`第 ${rowNumber} 行非数值类型不能配置 scale 或 precision`);
  }

    const minimum = optionalNumber(record.range_min, rowNumber, 'range_min');
    const maximum = optionalNumber(record.range_max, rowNumber, 'range_max');
    const policyText = textValue(record.out_of_range).toLowerCase();
    const defaultValue = optionalNumber(record.default_value, rowNumber, 'default_value');
    let range: S7PointRange | undefined;
    if (minimum !== undefined || maximum !== undefined || policyText || defaultValue !== undefined) {
      if (minimum === undefined && maximum === undefined) throw new Error(`第 ${rowNumber} 行范围至少填写 range_min 或 range_max`);
    if (!NUMERIC_VALUE_TYPES.has(valueType)) throw new Error(`第 ${rowNumber} 行非数值类型不能配置范围`);
      if (minimum !== undefined && maximum !== undefined && minimum > maximum) throw new Error(`第 ${rowNumber} 行 range_min 不能大于 range_max`);
      const policy = (policyText || 'use_default') as S7PointRange['policy'];
      if (!ALLOWED_POLICIES.has(policy || '')) throw new Error(`第 ${rowNumber} 行 out_of_range 策略无效`);
      if (policy === 'use_default' && defaultValue === undefined) throw new Error(`第 ${rowNumber} 行 use_default 必须填写 default_value`);
    if (defaultValue !== undefined && ((minimum !== undefined && defaultValue < minimum) || (maximum !== undefined && defaultValue > maximum))) {
    throw new Error(`第 ${rowNumber} 行 default_value 必须位于配置范围内`);
    }
      range = { minimum, maximum, policy, default_value: defaultValue };
      Object.keys(range).forEach((key) => range?.[key as keyof S7PointRange] === undefined && delete range?.[key as keyof S7PointRange]);
    }

    const point: S7PointDefinition = {
      name, address, value_type: valueType, read_write: readWrite,
      description: textValue(record.description) || undefined,
      max_length: maxLength, scale, precision, range,
    };
    Object.keys(point).forEach((key) => point[key as keyof S7PointDefinition] === undefined && delete point[key as keyof S7PointDefinition]);
    points.push(point);
  }
  if (points.length === 0) throw new Error('点表中没有可导入的数据行');
  return points;
}
