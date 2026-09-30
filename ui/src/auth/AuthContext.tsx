import React from 'react';
import { authAPI, AuthUser, setAPIToken } from '../services/api';

const AUTH_TOKEN_KEY = 'iot_rule_engine_auth_token';
const AUTH_USER_KEY = 'iot_rule_engine_auth_user';

interface LoginPayload {
  username: string;
  password: string;
}

interface AuthContextValue {
  user: AuthUser | null;
  token: string | null;
  loading: boolean;
  isAuthenticated: boolean;
  login: (payload: LoginPayload) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = React.createContext<AuthContextValue | undefined>(undefined);

const readStoredUser = (): AuthUser | null => {
  const rawUser = window.localStorage.getItem(AUTH_USER_KEY);
  if (!rawUser) {
    return null;
  }

  try {
    return JSON.parse(rawUser) as AuthUser;
  } catch {
    window.localStorage.removeItem(AUTH_USER_KEY);
    return null;
  }
};

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [token, setToken] = React.useState<string | null>(() => window.localStorage.getItem(AUTH_TOKEN_KEY));
  const [user, setUser] = React.useState<AuthUser | null>(() => readStoredUser());
  const [loading, setLoading] = React.useState<boolean>(true);

  const clearAuthState = React.useCallback(() => {
    setToken(null);
    setUser(null);
    window.localStorage.removeItem(AUTH_TOKEN_KEY);
    window.localStorage.removeItem(AUTH_USER_KEY);
    setAPIToken(null);
  }, []);

  React.useEffect(() => {
    const currentToken = window.localStorage.getItem(AUTH_TOKEN_KEY);
    setAPIToken(currentToken);

    if (!currentToken) {
      setLoading(false);
      return;
    }

    authAPI.me()
      .then((response) => {
        setUser(response.user);
        window.localStorage.setItem(AUTH_USER_KEY, JSON.stringify(response.user));
      })
      .catch(() => {
        clearAuthState();
      })
      .finally(() => {
        setLoading(false);
      });
  }, [clearAuthState]);

  React.useEffect(() => {
    const handleUnauthorized = () => {
      clearAuthState();
    };

    window.addEventListener('auth:unauthorized', handleUnauthorized);
    return () => {
      window.removeEventListener('auth:unauthorized', handleUnauthorized);
    };
  }, [clearAuthState]);

  const login = React.useCallback(async ({ username, password }: LoginPayload) => {
    const response = await authAPI.login(username, password);
    setToken(response.token);
    setUser(response.user);
    setAPIToken(response.token);
    window.localStorage.setItem(AUTH_TOKEN_KEY, response.token);
    window.localStorage.setItem(AUTH_USER_KEY, JSON.stringify(response.user));
  }, []);

  const logout = React.useCallback(async () => {
    try {
      await authAPI.logout();
    } catch (error) {
      // 忽略注销阶段的网络/鉴权错误，保证前端状态能清理干净
    } finally {
      clearAuthState();
    }
  }, [clearAuthState]);

  const value = React.useMemo<AuthContextValue>(() => ({
    user,
    token,
    loading,
    isAuthenticated: Boolean(token),
    login,
    logout,
  }), [loading, login, logout, token, user]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

export const useAuth = (): AuthContextValue => {
  const context = React.useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within AuthProvider');
  }
  return context;
};
