import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ThemeProvider, useConsoleTheme } from './ThemeContext';

const ThemeProbe: React.FC = () => {
  const { theme, toggleTheme } = useConsoleTheme();
  return <button onClick={toggleTheme}>theme:{theme}</button>;
};

describe('ThemeProvider', () => {
  beforeEach(() => {
    window.localStorage.clear();
    delete document.documentElement.dataset.theme;
  });

  test('defaults to light and persists a dark selection', async () => {
    render(
      <ThemeProvider>
        <ThemeProbe />
      </ThemeProvider>,
    );

    expect(screen.getByRole('button', { name: 'theme:light' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'theme:light' }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'theme:dark' })).toBeInTheDocument();
      expect(document.documentElement.dataset.theme).toBe('dark');
      expect(window.localStorage.getItem('iot-rule-theme')).toBe('dark');
    });
  });
});
