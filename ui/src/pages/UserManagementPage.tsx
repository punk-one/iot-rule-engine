import React from 'react';
import { Alert, Button, Card, Input, Modal, Select, Space, Table, Tag, type TableProps } from 'antd';
import { PlusOutlined, ReloadOutlined, SafetyCertificateOutlined, UserOutlined } from '@ant-design/icons';
import { authAPI, AuthUser, usersAPI } from '../services/api';
import { useAuth } from '../auth/AuthContext';
import { ConsoleHeader, ConsolePage } from '../components/ConsolePage';
import { ActionButton, StatusTag } from '../components/ui/ConsoleUI';
import { useFeedback } from '../components/FeedbackProvider';

interface MessageState {
  type: 'success' | 'error';
  text: string;
}

const initialCreateForm = {
  username: '',
  password: '',
  phone: '',
  email: '',
  remark: '',
  status: 1,
};

export const UserManagementPage: React.FC = () => {
  const { user } = useAuth();
  const isAdmin = user?.username === 'admin';
  const feedback = useFeedback();
  const [users, setUsers] = React.useState<AuthUser[]>([]);
  const [loading, setLoading] = React.useState(false);
  const [loadError, setLoadError] = React.useState<string | null>(null);
  const [showCreateModal, setShowCreateModal] = React.useState(false);
  const [createForm, setCreateForm] = React.useState(initialCreateForm);
  const [creating, setCreating] = React.useState(false);
  const [changingPassword, setChangingPassword] = React.useState(false);
  const [pendingUserAction, setPendingUserAction] = React.useState<string | null>(null);
  const [passwordForm, setPasswordForm] = React.useState({ oldPassword: '', newPassword: '', confirmPassword: '' });
  const [message, setMessage] = React.useState<MessageState | null>(null);

  const loadUsers = React.useCallback(async () => {
    if (!isAdmin) return;
    setLoading(true);
    setLoadError(null);
    try {
      setUsers(await usersAPI.list());
    } catch (error: any) {
      setLoadError(error.message || '未知错误');
    } finally {
      setLoading(false);
    }
  }, [isAdmin]);

  React.useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  const handleCreateUser = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage(null);
    if (!createForm.username.trim() || !createForm.password.trim()) {
      setMessage({ type: 'error', text: '请输入用户名和初始密码。' });
      return;
    }

    setCreating(true);
    try {
      await usersAPI.create({
        username: createForm.username.trim(),
        password: createForm.password,
        phone: createForm.phone.trim(),
        email: createForm.email.trim(),
        remark: createForm.remark.trim(),
        status: createForm.status,
      });
      setCreateForm(initialCreateForm);
      setShowCreateModal(false);
      setMessage({ type: 'success', text: '用户创建成功。' });
      await loadUsers();
    } catch (error: any) {
      setMessage({ type: 'error', text: `创建用户失败：${error.message || '未知错误'}` });
    } finally {
      setCreating(false);
    }
  };

  const handleChangeOwnPassword = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMessage(null);
    if (!passwordForm.oldPassword || !passwordForm.newPassword) {
      setMessage({ type: 'error', text: '请输入旧密码和新密码。' });
      return;
    }
    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      setMessage({ type: 'error', text: '两次输入的新密码不一致。' });
      return;
    }

    setChangingPassword(true);
    try {
      await authAPI.changePassword(passwordForm.oldPassword, passwordForm.newPassword);
      setPasswordForm({ oldPassword: '', newPassword: '', confirmPassword: '' });
      setMessage({ type: 'success', text: '密码修改成功，下次登录请使用新密码。' });
    } catch (error: any) {
      setMessage({ type: 'error', text: `修改密码失败：${error.message || '未知错误'}` });
    } finally {
      setChangingPassword(false);
    }
  };

  const handleToggleStatus = async (targetUser: AuthUser) => {
    if (targetUser.id === user?.id) return;
    const nextStatus = targetUser.status === 1 ? 0 : 1;
    const actionText = nextStatus === 1 ? '启用' : '禁用';
    if (!await feedback.confirm({
      title: `确定要${actionText}用户 “${targetUser.username}” 吗？`,
      danger: nextStatus === 0,
    })) return;

    const actionKey = `status:${targetUser.id}`;
    setPendingUserAction(actionKey);
    setMessage(null);
    try {
      await usersAPI.updateStatus(targetUser.id, nextStatus);
      setMessage({ type: 'success', text: `用户 ${targetUser.username} 已${actionText}。` });
      await loadUsers();
    } catch (error: any) {
      setMessage({ type: 'error', text: `${actionText}失败：${error.message || '未知错误'}` });
    } finally {
      setPendingUserAction(null);
    }
  };

  const handleResetPassword = async (targetUser: AuthUser) => {
    const newPassword = await feedback.prompt({
      title: `重置用户 ${targetUser.username} 密码`,
      message: '请输入新的登录密码。',
      initialValue: '',
      placeholder: '请输入新密码',
      requiredMessage: '请输入新密码',
      okText: '重置密码',
    });
    if (!newPassword) return;

    const actionKey = `password:${targetUser.id}`;
    setPendingUserAction(actionKey);
    setMessage(null);
    try {
      await usersAPI.resetPassword(targetUser.id, newPassword);
      setMessage({ type: 'success', text: `用户 ${targetUser.username} 密码已重置。` });
    } catch (error: any) {
      setMessage({ type: 'error', text: `重置密码失败：${error.message || '未知错误'}` });
    } finally {
      setPendingUserAction(null);
    }
  };

  const columns: NonNullable<TableProps<AuthUser>['columns']> = [
    {
      title: '用户',
      dataIndex: 'username',
      render: (_, item) => (
        <div className="user-management__identity">
          <span className="user-management__avatar"><UserOutlined /></span>
          <div>
            <div className="user-management__username">
              {item.username}
              {item.id === user?.id ? <Tag color="blue">当前账号</Tag> : null}
            </div>
            <div className="user-management__muted">ID {item.id}</div>
          </div>
        </div>
      ),
    },
    { title: '邮箱', dataIndex: 'email', ellipsis: true, render: (value) => value || '-' },
    { title: '手机号', dataIndex: 'phone', width: 150, render: (value) => value || '-' },
    {
      title: '状态', dataIndex: 'status', width: 96, align: 'center',
      render: (value) => <StatusTag tone={value === 1 ? 'success' : 'neutral'}>{value === 1 ? '已启用' : '已禁用'}</StatusTag>,
    },
    {
      title: '最近登录', dataIndex: 'last_login_at', width: 190,
      render: (value) => value ? new Date(value).toLocaleString() : '尚未登录',
    },
    {
      title: '操作', dataIndex: '__actions__', width: 210, align: 'center',
      render: (_, item) => {
        const isCurrentUser = item.id === user?.id;
        return (
          <Space size={6}>
            <ActionButton
              size="small"
              danger={item.status === 1}
              disabled={isCurrentUser || pendingUserAction !== null}
              loading={pendingUserAction === `status:${item.id}`}
              title={isCurrentUser ? '不能在这里禁用当前登录账号' : undefined}
              onClick={() => void handleToggleStatus(item)}
            >
              {item.status === 1 ? '禁用' : '启用'}
            </ActionButton>
            <ActionButton
              size="small"
              disabled={pendingUserAction !== null}
              loading={pendingUserAction === `password:${item.id}`}
              onClick={() => void handleResetPassword(item)}
            >
              重置密码
            </ActionButton>
          </Space>
        );
      },
    },
  ];

  return (
    <ConsolePage maxWidth={1440}>
      <ConsoleHeader
        title="用户管理"
        description={isAdmin ? '集中管理系统账号、启停状态与密码安全。' : '查看当前账号并维护自己的登录密码。'}
        actions={isAdmin ? (
          <Space wrap>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadUsers()}>刷新</Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setShowCreateModal(true)}>创建用户</Button>
          </Space>
        ) : undefined}
      />

      {message ? <Alert className="user-management__notice" showIcon type={message.type} message={message.text} closable onClose={() => setMessage(null)} /> : null}

      <section className="user-management__overview">
        <Card className="user-management__profile-card" bordered={false}>
          <div className="user-management__profile-icon"><UserOutlined /></div>
          <div>
            <div className="user-management__eyebrow">当前登录账号</div>
            <div className="user-management__profile-name">{user?.username || '-'}</div>
            <Space wrap size={6}>
              <StatusTag tone="success">正常</StatusTag>
              {isAdmin ? <StatusTag tone="info">系统管理员</StatusTag> : <StatusTag>普通用户</StatusTag>}
            </Space>
          </div>
        </Card>

        <Card className="user-management__password-card" bordered={false} title={<span><SafetyCertificateOutlined /> 修改我的密码</span>}>
          <form className="user-management__password-form" onSubmit={handleChangeOwnPassword}>
            <Input.Password placeholder="旧密码" autoComplete="current-password" value={passwordForm.oldPassword} onChange={(event) => setPasswordForm((prev) => ({ ...prev, oldPassword: event.target.value }))} />
            <Input.Password placeholder="新密码" autoComplete="new-password" value={passwordForm.newPassword} onChange={(event) => setPasswordForm((prev) => ({ ...prev, newPassword: event.target.value }))} />
            <Input.Password placeholder="确认新密码" autoComplete="new-password" value={passwordForm.confirmPassword} onChange={(event) => setPasswordForm((prev) => ({ ...prev, confirmPassword: event.target.value }))} />
            <ActionButton type="primary" htmlType="submit" loading={changingPassword} pendingText="提交中...">修改密码</ActionButton>
          </form>
        </Card>
      </section>

      {isAdmin ? (
        <section className="console-surface user-management__table">
          <div className="user-management__section-header">
            <div>
              <h2>系统用户</h2>
              <p>共 {users.length} 个账号。禁用账号后将无法再次登录。</p>
            </div>
          </div>
          {loadError ? <Alert type="error" showIcon message="用户列表加载失败" description={loadError} action={<Button onClick={() => void loadUsers()}>重试</Button>} /> : null}
          <Table<AuthUser>
            columns={columns}
            dataSource={users}
            rowKey="id"
            loading={loading}
            pagination={{ defaultPageSize: 10, showSizeChanger: true, pageSizeOptions: [10, 20, 50] }}
            locale={{ emptyText: '暂无用户' }}
            scroll={{ x: 960 }}
          />
        </section>
      ) : null}

      <Modal
        title="创建用户"
        open={showCreateModal}
        footer={null}
        destroyOnHidden
        onCancel={() => { if (!creating) setShowCreateModal(false); }}
      >
        <form className="user-management__create-form" onSubmit={handleCreateUser}>
          <Field label="用户名" required><Input autoFocus value={createForm.username} onChange={(event) => setCreateForm((prev) => ({ ...prev, username: event.target.value }))} /></Field>
          <Field label="初始密码" required><Input.Password autoComplete="new-password" value={createForm.password} onChange={(event) => setCreateForm((prev) => ({ ...prev, password: event.target.value }))} /></Field>
          <div className="user-management__form-row">
            <Field label="手机号"><Input value={createForm.phone} onChange={(event) => setCreateForm((prev) => ({ ...prev, phone: event.target.value }))} /></Field>
            <Field label="邮箱"><Input value={createForm.email} onChange={(event) => setCreateForm((prev) => ({ ...prev, email: event.target.value }))} /></Field>
          </div>
          <Field label="备注"><Input.TextArea rows={3} value={createForm.remark} onChange={(event) => setCreateForm((prev) => ({ ...prev, remark: event.target.value }))} /></Field>
          <Field label="初始状态"><Select value={createForm.status} onChange={(value) => setCreateForm((prev) => ({ ...prev, status: value }))} options={[{ label: '启用', value: 1 }, { label: '禁用', value: 0 }]} /></Field>
          <div className="user-management__modal-actions">
            <Button disabled={creating} onClick={() => setShowCreateModal(false)}>取消</Button>
            <ActionButton type="primary" htmlType="submit" loading={creating} pendingText="创建中...">创建用户</ActionButton>
          </div>
        </form>
      </Modal>
    </ConsolePage>
  );
};

const Field: React.FC<{ label: string; required?: boolean; children: React.ReactNode }> = ({ label, required, children }) => (
  <label className="user-management__field">
    <span>{label}{required ? <em> *</em> : null}</span>
    {children}
  </label>
);
