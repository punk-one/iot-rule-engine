import React from 'react';
import { ConfigProvider, theme as antdTheme } from 'antd';

export type ConsoleTheme = 'light' | 'dark';

type ThemeContextValue = {
  theme: ConsoleTheme;
  toggleTheme: () => void;
};

const STORAGE_KEY = 'iot-rule-theme';

const ThemeContext = React.createContext<ThemeContextValue | null>(null);

const readInitialTheme = (): ConsoleTheme => {
  if (typeof window === 'undefined') return 'light';
  return window.localStorage.getItem(STORAGE_KEY) === 'dark' ? 'dark' : 'light';
};

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [theme, setTheme] = React.useState<ConsoleTheme>(readInitialTheme);

  React.useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
    window.localStorage.setItem(STORAGE_KEY, theme);
  }, [theme]);

  const value = React.useMemo<ThemeContextValue>(() => ({
    theme,
    toggleTheme: () => setTheme((current) => current === 'light' ? 'dark' : 'light'),
  }), [theme]);

  const dark = theme === 'dark';

  return (
    <ThemeContext.Provider value={value}>
      <ConfigProvider
        theme={{
          algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
          token: {
            colorPrimary: '#168c6a',
            colorInfo: '#168c6a',
            colorSuccess: '#168c6a',
            borderRadius: 10,
            colorBgLayout: dark ? '#11161b' : '#e6e8eb',
            colorBgContainer: dark ? '#1a2128' : '#f4f5f6',
            colorBgElevated: dark ? '#222a32' : '#f8f9fa',
            colorBorder: dark ? '#35424c' : '#cfd3d8',
            colorText: dark ? '#e9efed' : '#252b30',
            colorTextSecondary: dark ? '#a8b4b0' : '#626a72',
          },
          components: {
            Layout: {
              bodyBg: dark ? '#11161b' : '#e6e8eb',
              headerBg: dark ? '#181f25' : '#eceef0',
              siderBg: dark ? '#141a20' : '#d9dde1',
            },
            Menu: {
              itemBg: 'transparent',
              subMenuItemBg: dark ? '#10161b' : '#d1d6da',
              itemSelectedBg: dark ? '#214d40' : '#c5ddd4',
              itemSelectedColor: dark ? '#effcf7' : '#173c31',
            },
            Table: {
              headerBg: dark ? '#202830' : '#e9ebed',
            },
          },
        }}
      >
        {children}
      </ConfigProvider>
    </ThemeContext.Provider>
  );
};

export const useConsoleTheme = (): ThemeContextValue => {
  const context = React.useContext(ThemeContext);
  if (!context) {
    throw new Error('useConsoleTheme must be used inside ThemeProvider');
  }
  return context;
};
