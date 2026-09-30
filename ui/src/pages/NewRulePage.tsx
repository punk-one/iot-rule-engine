import React, { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useRuleStore } from '../store/ruleStore';
import { RuleFlow } from '../types';
import { ConsolePage } from '../components/ConsolePage';

export const NewRulePage: React.FC = () => {
  const navigate = useNavigate();
  const { setCurrentRule } = useRuleStore();

  useEffect(() => {
    // Immediate creation logic
    const createAndRedirect = () => {
        const newId = `rule_${Date.now()}`;

        const newRule: RuleFlow = {
            id: newId,
            name: '新建规则 ' + new Date().toLocaleTimeString(),
            version: 1,
            enabled: false,
            debug_mode: false,
            description: '',
            nodes: [],
        };

        // Set to store (Client-side only initially)
        setCurrentRule(newRule);

        // Navigate to Editor
        navigate(`/rules/${newId}/edit`);
    };

    createAndRedirect();
  }, [navigate, setCurrentRule]);

  return (
    <ConsolePage maxWidth={520}>
      <div
        className="card"
        style={{
          padding: '40px',
          borderRadius: '24px',
          textAlign: 'center',
          marginTop: '80px',
          boxShadow: '0 20px 40px rgba(15, 23, 42, 0.08)',
        }}
      >
        <div style={{ fontSize: '18px', fontWeight: 700, color: 'var(--color-text)', marginBottom: '12px' }}>
          正在初始化规则画布...
        </div>
        <div style={{ fontSize: '14px', color: 'var(--color-text-secondary)', lineHeight: 1.7 }}>
          系统正在创建草稿规则并跳转到编辑器，请稍候。
        </div>
      </div>
    </ConsolePage>
  );
};
