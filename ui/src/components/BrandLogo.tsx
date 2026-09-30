import React from 'react';
import brandLogo from '../assets/brand/iot-rule-engine-mark.svg';

interface BrandLogoProps {
  size?: number;
  showText?: boolean;
  theme?: 'light' | 'dark';
  title?: string;
}

export const BrandLogo: React.FC<BrandLogoProps> = ({
  size = 40,
  showText = true,
  theme = 'light',
  title = 'IoT Rule Engine',
}) => {
  const primaryText = theme === 'dark' ? '#effcf3' : '#102a21';
  const secondaryText = theme === 'dark' ? 'rgba(239,252,243,0.78)' : '#53746a';

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
      <img
        src={brandLogo}
        alt={title}
        style={{
          width: `${size}px`,
          height: `${size}px`,
          flexShrink: 0,
          display: 'block',
        }}
      />
      {showText ? (
        <div style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
          <div style={{ color: primaryText, fontSize: Math.max(16, size * 0.4), fontWeight: 800, lineHeight: 1.1 }}>
            IoT Rule Engine
          </div>
          <div style={{ color: secondaryText, fontSize: Math.max(11, size * 0.2), lineHeight: 1.25, letterSpacing: '0.04em' }}>
            Rules, Runtime and Control
          </div>
        </div>
      ) : null}
    </div>
  );
};
