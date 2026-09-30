import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import ExcelJS from 'exceljs';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const engineDir = path.resolve(scriptDir, '../..');
const rule = JSON.parse(await fs.readFile(path.join(engineDir, 'docs/cnc18-profile.rule.json'), 'utf8'));
const source = rule.nodes.find((node) => node.plugin === 'source-s7');
if (!source) throw new Error('source-s7 node not found');

const columns = [
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
];

const workbook = new ExcelJS.Workbook();
workbook.creator = 'iot-rule-engine';
const sheet = workbook.addWorksheet('S7点表', { views: [{ state: 'frozen', ySplit: 1 }] });
sheet.columns = columns;
for (const point of source.config.points) {
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
for (let row = 2; row <= 501; row += 1) {
  sheet.getCell(`C${row}`).dataValidation = { type: 'list', allowBlank: false, formulae: ['"Bool,Int8,Uint8,Int16,Uint16,Int32,Uint32,Float32,Float64,Byte,Word,DWord,String,WString"'] };
  sheet.getCell(`D${row}`).dataValidation = { type: 'list', allowBlank: true, formulae: ['"R,RO,RW"'] };
  sheet.getCell(`K${row}`).dataValidation = { type: 'list', allowBlank: true, formulae: ['"use_default,clamp,drop,error"'] };
}

const help = workbook.addWorksheet('填写说明');
help.columns = [{ width: 22 }, { width: 90 }];
help.addRows([
  ['字段', '说明'],
  ['name / address / value_type', '必填；name 在一个 source-s7 节点内必须唯一。'],
  ['scale / precision', '先缩放，再按 precision 保留小数位；precision=0 表示取整。'],
  ['range_min / range_max', '工程值范围，可只填一侧；范围判断发生在缩放与精度处理之后。'],
  ['out_of_range', 'use_default 使用 default_value；clamp 截断到边界；drop 丢弃该点；error 使本轮采集失败。'],
  ['导入兼容', '系统同时识别 nodeName、valueType、readWrite、minimum、maximum、policy、defaultValue 及中文表头。'],
]);
help.getRow(1).font = { bold: true, color: { argb: 'FFFFFFFF' } };
help.getRow(1).fill = { type: 'pattern', pattern: 'solid', fgColor: { argb: 'FF1F4E78' } };
help.getColumn(2).alignment = { wrapText: true, vertical: 'top' };

await workbook.xlsx.writeFile(path.join(engineDir, 'docs/cnc18-s7-points.xlsx'));
