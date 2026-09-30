import React, { Suspense } from 'react';

const JSONSchemaRenderer = React.lazy(async () => {
  const module = await import('./JSONSchemaRenderer');
  return { default: module.JSONSchemaRenderer };
});

type JSONSchemaRendererProps = React.ComponentProps<typeof import('./JSONSchemaRenderer')['JSONSchemaRenderer']>;

export const LazyJSONSchemaRenderer: React.FC<JSONSchemaRendererProps> = (props) => (
  <Suspense
    fallback={
      <div
        style={{
          minHeight: 220,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          border: '1px dashed #d0d5dd',
          borderRadius: 12,
          color: '#667085',
          fontSize: 13,
          background: '#fcfcfd',
        }}
      >
        配置表单加载中...
      </div>
    }
  >
    <JSONSchemaRenderer {...props} />
  </Suspense>
);
