import React, { useMemo } from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { json as jsonLanguage } from '@codemirror/lang-json';
import { EditorView } from '@codemirror/view';

export type CodeEditorMode = 'json' | 'text';

export interface LazyCodeEditorProps {
  value?: string;
  onChange?: (value: string) => void;
  height?: string | number;
  mode?: CodeEditorMode;
  readOnly?: boolean;
  placeholder?: string;
}

const toCssSize = (value?: string | number): string => {
  if (typeof value === 'number') {
    return `${value}px`;
  }

  return value || '200px';
};

export const LazyCodeEditor: React.FC<LazyCodeEditorProps> = ({
  value = '',
  onChange,
  height = '200px',
  mode = 'text',
  readOnly = false,
  placeholder,
}) => {
  const extensions = useMemo(() => {
    const baseExtensions = [EditorView.lineWrapping];

    if (mode === 'json') {
      return [...baseExtensions, jsonLanguage()];
    }

    return baseExtensions;
  }, [mode]);

  return (
    <CodeMirror
      value={value}
      height={toCssSize(height)}
      extensions={extensions}
      onChange={(nextValue) => onChange?.(nextValue)}
      readOnly={readOnly}
      editable={!readOnly}
      placeholder={placeholder}
      basicSetup={{
        lineNumbers: true,
        foldGutter: false,
        highlightActiveLine: false,
        highlightActiveLineGutter: false,
      }}
    />
  );
};
