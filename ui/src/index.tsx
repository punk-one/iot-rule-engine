import React from 'react';
import ReactDOM from 'react-dom/client';
import 'antd/dist/reset.css';
import { App as AntdApp } from 'antd';
import './index.css';
import App from './App';
import reportWebVitals from './reportWebVitals';
import { ErrorBoundary } from './components/ErrorBoundary';
import { FeedbackProvider } from './components/FeedbackProvider';
import { ThemeProvider } from './theme/ThemeContext';

const root = ReactDOM.createRoot(
  document.getElementById('root') as HTMLElement
);
root.render(
  <React.StrictMode>
    <ThemeProvider>
      <AntdApp>
        <FeedbackProvider>
          <ErrorBoundary>
            <App />
          </ErrorBoundary>
        </FeedbackProvider>
      </AntdApp>
    </ThemeProvider>
  </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
