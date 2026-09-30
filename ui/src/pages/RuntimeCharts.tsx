import React from 'react';
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from 'recharts';

export const NodeMetricsChart: React.FC<{ metricsHistory: any[] }> = ({ metricsHistory }) => (
  <ResponsiveContainer width="100%" height="100%">
    <AreaChart data={metricsHistory}>
      <defs>
        <linearGradient id="colorIn" x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.1} />
          <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
        </linearGradient>
        <linearGradient id="colorOut" x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor="#10b981" stopOpacity={0.1} />
          <stop offset="95%" stopColor="#10b981" stopOpacity={0} />
        </linearGradient>
      </defs>
      <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#f1f5f9" />
      <XAxis
        dataKey="time"
        axisLine={false}
        tickLine={false}
        tick={{ fontSize: 10, fill: '#94a3b8' }}
        minTickGap={30}
      />
      <YAxis axisLine={false} tickLine={false} tick={{ fontSize: 10, fill: '#94a3b8' }} />
      <Tooltip
        contentStyle={{
          borderRadius: '8px',
          border: 'none',
          boxShadow: '0 10px 15px -3px rgba(0,0,0,0.1)',
          fontSize: '12px',
        }}
      />
      <Area type="monotone" dataKey="in" name="输入" stroke="#3b82f6" fillOpacity={1} fill="url(#colorIn)" strokeWidth={2} />
      <Area type="monotone" dataKey="out" name="输出" stroke="#10b981" fillOpacity={1} fill="url(#colorOut)" strokeWidth={2} />
    </AreaChart>
  </ResponsiveContainer>
);

export const EdgeMetricsChart: React.FC<{ metricsHistory: any[] }> = ({ metricsHistory }) => (
  <ResponsiveContainer width="100%" height="100%">
    <AreaChart data={metricsHistory}>
      <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#f1f5f9" />
      <XAxis
        dataKey="time"
        axisLine={false}
        tickLine={false}
        tick={{ fontSize: 10, fill: '#94a3b8' }}
        minTickGap={30}
      />
      <YAxis axisLine={false} tickLine={false} tick={{ fontSize: 10, fill: '#94a3b8' }} />
      <Tooltip
        contentStyle={{
          borderRadius: '8px',
          border: 'none',
          boxShadow: '0 10px 15px -3px rgba(0,0,0,0.1)',
          fontSize: '12px',
        }}
      />
      <Area type="monotone" dataKey="published" name="已发布" stroke="#3b82f6" fill="#e0f2fe" strokeWidth={2} />
      <Area type="monotone" dataKey="consumed" name="已消费" stroke="#10b981" fill="#d1fae5" strokeWidth={2} />
      <Area type="monotone" dataKey="lag" name="积压 (Lag)" stroke="#f59e0b" fill="#fef3c7" strokeWidth={2} />
    </AreaChart>
  </ResponsiveContainer>
);
