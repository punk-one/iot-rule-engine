import React from 'react';
import { EyeInvisibleOutlined, EyeOutlined } from '@ant-design/icons';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { BrandLogo } from '../components/BrandLogo';
import styles from './LoginPage.module.css';

interface LocationState {
  from?: {
    pathname?: string;
  };
}

export const LoginPage: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { login, loading, isAuthenticated } = useAuth();
  const [username, setUsername] = React.useState('');
  const [password, setPassword] = React.useState('');
  const [showPassword, setShowPassword] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [error, setError] = React.useState('');

  const redirectTo = (location.state as LocationState | null)?.from?.pathname || '/dashboard';

  if (!loading && isAuthenticated) {
    return <Navigate to={redirectTo} replace />;
  }

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setSubmitting(true);
    setError('');

    try {
      await login({ username, password });
      navigate(redirectTo, { replace: true });
    } catch (err: any) {
      setError(err?.message || '登录失败，请检查账号或密码。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className={styles.page}>
      <div className={styles.shell}>
        <section className={styles.hero}>
          <div>
            <BrandLogo size={78} theme="dark" />
            <div className={styles.eyebrow}>INDUSTRIAL IoT RULE ENGINE</div>
            <h1 className={styles.heroTitle}>让工业数据沿着可靠的规则流持续运转</h1>
            <div className={styles.heroAccent} aria-hidden="true">
              <span className={styles.heroAccentLine} />
              <span className={styles.heroAccentDot} />
            </div>
            <p className={styles.heroDescription}>
              连接 PLC、消息系统与业务服务，在统一控制台中完成规则设计、运行追踪、插件协同
              和系统配置。
            </p>
          </div>

          <div className={styles.bulletList}>
            <div className={styles.bulletItem}>
              <span className={styles.bulletMarker} aria-hidden="true" />
              <span>集中查看规则资产、运行实例与异常信号。</span>
            </div>
            <div className={styles.bulletItem}>
              <span className={styles.bulletMarker} aria-hidden="true" />
              <span>统一进入规则、插件、Connector 与配置入口。</span>
            </div>
            <div className={styles.bulletItem}>
              <span className={styles.bulletMarker} aria-hidden="true" />
              <span>默认工作台聚合值守信息，减少日常切页成本。</span>
            </div>
          </div>
        </section>

        <div className={styles.formPanel}>
          <div className={styles.formCard}>
            <h2 className={styles.formTitle}>欢迎登录</h2>

            <form onSubmit={handleSubmit} autoComplete="off">
              <label style={{ display: 'block', marginBottom: '14px' }}>
                <div style={{ marginBottom: '8px', color: 'var(--color-text)', fontWeight: 600 }}>账号</div>
                <input
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  autoComplete="off"
                  name="login-username"
                  placeholder="请输入账号"
                  style={inputStyle}
                />
              </label>

              <label style={{ display: 'block', marginBottom: '12px' }}>
                <div style={{ marginBottom: '8px', color: 'var(--color-text)', fontWeight: 600 }}>密码</div>
                <div className={styles.passwordField}>
                  <input
                    type={showPassword ? 'text' : 'password'}
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    autoComplete="new-password"
                    name="login-password"
                    placeholder="请输入密码"
                    style={passwordInputStyle}
                  />
                  <button
                    type="button"
                    className={styles.passwordToggle}
                    aria-label={showPassword ? '隐藏密码' : '显示密码'}
                    onClick={() => setShowPassword((visible) => !visible)}
                  >
                    {showPassword ? <EyeInvisibleOutlined /> : <EyeOutlined />}
                  </button>
                </div>
              </label>

              {error && (
                <div
                  style={{
                    marginBottom: '16px',
                    padding: '12px 14px',
                    borderRadius: '10px',
                    backgroundColor: '#fef2f2',
                    color: '#dc2626',
                    fontSize: '13px',
                  }}
                >
                  {error}
                </div>
              )}

              <button
                type="submit"
                disabled={submitting}
                style={{
                  width: '100%',
                  padding: '11px 16px',
                  border: 'none',
                  borderRadius: '10px',
                  background: submitting ? '#86d0a5' : 'linear-gradient(135deg, #2f855a 0%, #168c6a 100%)',
                  color: '#fff',
                  fontSize: '15px',
                  fontWeight: 700,
                  cursor: submitting ? 'not-allowed' : 'pointer',
                  boxShadow: '0 12px 24px rgba(22, 140, 106, 0.18)',
                }}
              >
                {submitting ? '登录中...' : '登录'}
              </button>
            </form>
          </div>
        </div>
      </div>
    </div>
  );
};

const inputStyle: React.CSSProperties = {
  width: '100%',
  boxSizing: 'border-box',
  padding: '12px 14px',
  borderRadius: '10px',
  border: '1px solid var(--color-border)',
  outline: 'none',
  fontSize: '14px',
  color: 'var(--color-text)',
  backgroundColor: 'var(--color-surface)',
};

const passwordInputStyle: React.CSSProperties = {
  ...inputStyle,
  border: 'none',
  paddingRight: '42px',
};
