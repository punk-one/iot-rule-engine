import React, { Suspense } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import './App.css';
import { AuthProvider } from './auth/AuthContext';
import { AppShell } from './components/AppShell';
import { RequireAuth } from './components/RequireAuth';
import { LoginPage } from './pages/LoginPage';

const lazyNamed = <T extends Record<string, React.ComponentType<any>>>(
  importer: () => Promise<T>,
  exportName: keyof T
) => React.lazy(async () => {
  const module = await importer();
  return { default: module[exportName] };
});

const DashboardPage = lazyNamed(() => import('./pages/DashboardPage'), 'DashboardPage');
const RuleListPage = lazyNamed(() => import('./pages/RuleListPage'), 'RuleListPage');
const RuleEditorPage = lazyNamed(() => import('./pages/RuleEditorPage'), 'RuleEditorPage');
const NewRulePage = lazyNamed(() => import('./pages/NewRulePage'), 'NewRulePage');
const PluginManagerPage = lazyNamed(() => import('./pages/PluginManagerPage'), 'PluginManagerPage');
const ConnectorsPage = lazyNamed(() => import('./pages/ConnectorsPage'), 'ConnectorsPage');
const ConnectorOverviewPage = lazyNamed(() => import('./pages/ConnectorOverviewPage'), 'ConnectorOverviewPage');
const UserManagementPage = lazyNamed(() => import('./pages/UserManagementPage'), 'UserManagementPage');
const ConfigPage = lazyNamed(() => import('./pages/ConfigPage'), 'ConfigPage');
const RuleOverviewPage = lazyNamed(() => import('./pages/RuleOverviewPage'), 'RuleOverviewPage');
const RuntimeDAGPage = lazyNamed(() => import('./pages/RuntimeDAGPage'), 'RuntimeDAGPage');
const NodeInspectorPage = lazyNamed(() => import('./pages/NodeInspectorPage'), 'NodeInspectorPage');
const EdgeMonitorPage = lazyNamed(() => import('./pages/EdgeMonitorPage'), 'EdgeMonitorPage');
const RuleLifecyclePage = lazyNamed(() => import('./pages/RuleLifecyclePage'), 'RuleLifecyclePage');

const routeFallback = (
  <div
    style={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      color: '#47655b',
      background: '#f4faf7',
      fontSize: '15px',
      fontWeight: 600,
    }}
  >
    页面加载中...
  </div>
);

const ProtectedLayout: React.FC = () => {
  return (
    <RequireAuth>
      <AppShell />
    </RequireAuth>
  );
};

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Suspense fallback={routeFallback}>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route element={<ProtectedLayout />}>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<DashboardPage />} />
              <Route path="/rules" element={<RuleListPage />} />
              <Route path="/rules/new" element={<NewRulePage />} />
              <Route path="/rules/:id/edit" element={<RuleEditorPage />} />
              <Route path="/rules/:id/lifecycle" element={<RuleLifecyclePage />} />
              <Route path="/plugins" element={<PluginManagerPage />} />
              <Route path="/connectors/overview" element={<ConnectorOverviewPage />} />
              <Route path="/connectors" element={<ConnectorsPage />} />
              <Route path="/users" element={<UserManagementPage />} />
              <Route path="/config" element={<ConfigPage />} />
              <Route path="/config/users" element={<Navigate to="/users" replace />} />
              <Route path="/configs/users" element={<Navigate to="/users" replace />} />
              <Route path="/runtime" element={<RuleOverviewPage />} />
              <Route path="/runtime/overview" element={<Navigate to="/runtime" replace />} />
              <Route path="/runtime/:instanceId/dag" element={<RuntimeDAGPage />} />
              <Route path="/runtime/:instanceId/nodes/:nodeId" element={<NodeInspectorPage />} />
              <Route path="/runtime/:instanceId/edges/:edgeId" element={<EdgeMonitorPage />} />
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;
