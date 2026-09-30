import React from 'react';
import { App as AntdApp, Input } from 'antd';

type ConfirmOptions = {
  title: string;
  content?: React.ReactNode;
  okText?: string;
  cancelText?: string;
  danger?: boolean;
};

type PromptOptions = {
  title: string;
  message?: React.ReactNode;
  placeholder?: string;
  initialValue?: string;
  okText?: string;
  cancelText?: string;
  required?: boolean;
  requiredMessage?: string;
};

interface FeedbackContextValue {
  success: (content: React.ReactNode) => void;
  error: (content: React.ReactNode) => void;
  info: (content: React.ReactNode) => void;
  warning: (content: React.ReactNode) => void;
  confirm: (options: ConfirmOptions | string) => Promise<boolean>;
  prompt: (options: PromptOptions) => Promise<string | null>;
}

const FeedbackContext = React.createContext<FeedbackContextValue | null>(null);

const PromptContent: React.FC<{
  initialValue?: string;
  placeholder?: string;
  message?: React.ReactNode;
  onValueChange: (value: string) => void;
}> = ({ initialValue = '', placeholder, message, onValueChange }) => {
  const inputRef = React.useRef<any>(null);

  React.useEffect(() => {
    const timer = window.setTimeout(() => {
      inputRef.current?.focus?.();
    }, 50);
    return () => window.clearTimeout(timer);
  }, []);

  return (
    <div style={{ display: 'grid', gap: '12px' }}>
      {message ? <div style={{ color: 'var(--color-text-secondary)', lineHeight: 1.7 }}>{message}</div> : null}
      <Input
        ref={inputRef}
        defaultValue={initialValue}
        placeholder={placeholder}
        onChange={(event) => onValueChange(event.target.value)}
        onPressEnter={(event) => {
          (event.target as HTMLInputElement).blur();
        }}
      />
    </div>
  );
};

export const FeedbackProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const { message, modal } = AntdApp.useApp();

  const success = React.useCallback((content: React.ReactNode) => {
    void message.success({ content });
  }, [message]);

  const error = React.useCallback((content: React.ReactNode) => {
    void message.error({ content });
  }, [message]);

  const info = React.useCallback((content: React.ReactNode) => {
    void message.info({ content });
  }, [message]);

  const warning = React.useCallback((content: React.ReactNode) => {
    void message.warning({ content });
  }, [message]);

  const confirm = React.useCallback((options: ConfirmOptions | string) => {
    const config = typeof options === 'string' ? { title: options } : options;

    return new Promise<boolean>((resolve) => {
      modal.confirm({
        title: config.title,
        content: config.content,
        okText: config.okText || '确认',
        cancelText: config.cancelText || '取消',
        okButtonProps: config.danger ? { danger: true } : undefined,
        onOk: () => resolve(true),
        onCancel: () => resolve(false),
      });
    });
  }, [modal]);

  const prompt = React.useCallback((options: PromptOptions) => {
    let currentValue = options.initialValue || '';

    return new Promise<string | null>((resolve) => {
      modal.confirm({
        title: options.title,
        icon: null,
        okText: options.okText || '确认',
        cancelText: options.cancelText || '取消',
        content: (
          <PromptContent
            initialValue={options.initialValue}
            placeholder={options.placeholder}
            message={options.message}
            onValueChange={(value) => {
              currentValue = value;
            }}
          />
        ),
        onOk: async () => {
          const normalized = currentValue.trim();
          if (options.required !== false && !normalized) {
            void message.warning(options.requiredMessage || '请输入内容');
            return Promise.reject();
          }
          resolve(normalized);
          return Promise.resolve();
        },
        onCancel: () => resolve(null),
      });
    });
  }, [message, modal]);

  const value = React.useMemo<FeedbackContextValue>(() => ({
    success,
    error,
    info,
    warning,
    confirm,
    prompt,
  }), [confirm, error, info, prompt, success, warning]);

  return <FeedbackContext.Provider value={value}>{children}</FeedbackContext.Provider>;
};

export const useFeedback = () => {
  const context = React.useContext(FeedbackContext);
  if (!context) {
    throw new Error('useFeedback must be used within FeedbackProvider');
  }
  return context;
};
