import { KeyOutlined, LockOutlined, MailOutlined } from '@ant-design/icons';
import { LoginForm, ProFormCheckbox, ProFormText } from '@ant-design/pro-components';
import { Helmet, history, Link, useModel } from '@umijs/max';
import { Alert, App } from 'antd';
import { createStyles } from 'antd-style';
import React, { startTransition, useState } from 'react';
import { Footer } from '@/components';
import { login } from '@/services/ant-design-pro/login';
import { getWorkspaceId } from '@/services/session';
import Settings from '../../../../config/defaultSettings';

const getSafeRedirectUrl = (redirect: string | null): string => {
  if (!redirect?.startsWith('/') || redirect.startsWith('//')) return '/admin';
  try {
    const parsed = new URL(redirect, window.location.origin);
    if (parsed.origin !== window.location.origin) return '/admin';
    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return '/admin';
  }
};

const useStyles = createStyles(() => ({
  container: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100vh',
    overflow: 'auto',
    background: '#f5f7fa',
  },
  content: {
    flex: 1,
    padding: '48px 0',
  },
}));

const Login: React.FC = () => {
  const { styles } = useStyles();
  const { message } = App.useApp();
  const { initialState, setInitialState } = useModel('@@initialState');
  const [error, setError] = useState('');

  const handleSubmit = async (values: API.LoginParams) => {
    setError('');
    try {
      await login(values);
      const userInfo = await initialState?.fetchUserInfo?.();
      if (userInfo) {
        startTransition(() => {
          setInitialState((state) => ({ ...state, currentUser: userInfo }));
        });
      }
      if (userInfo?.role !== 'admin') {
        history.replace('/exception/403');
        return;
      }
      message.success('登录成功');
      const redirect = new URL(window.location.href).searchParams.get('redirect');
      history.replace(getSafeRedirectUrl(redirect));
    } catch (reason) {
      const messageText = reason instanceof Error ? reason.message : '邮箱或密码错误';
      setError(messageText);
    }
  };

  return (
    <div className={styles.container}>
      <Helmet>
        <title>登录{Settings.title ? ` - ${Settings.title}` : ''}</title>
      </Helmet>
      <div className={styles.content}>
        <LoginForm
          contentStyle={{ minWidth: 280, maxWidth: 420 }}
          logo={<img alt="Lanverse" src="/logo.svg" />}
          title="Lanverse 管理端"
          subTitle="仅 Admin 可访问后台管理内容"
          initialValues={{ workspaceId: getWorkspaceId() }}
          onFinish={(values) => handleSubmit(values as API.LoginParams)}
        >
          {error && <Alert title={error} type="error" showIcon />}
          <ProFormText
            name="email"
            fieldProps={{ size: 'large', prefix: <MailOutlined /> }}
            placeholder="邮箱"
            rules={[{ required: true, type: 'email', message: '请输入有效邮箱' }]}
          />
          <ProFormText.Password
            name="password"
            fieldProps={{ size: 'large', prefix: <LockOutlined /> }}
            placeholder="密码"
            rules={[{ required: true, message: '请输入密码' }]}
          />
          <ProFormText
            name="workspaceId"
            fieldProps={{ size: 'large', prefix: <KeyOutlined /> }}
            placeholder="Workspace ID（注册后可查看）"
            rules={[{ required: true, message: '请输入 Workspace ID' }]}
          />
          <ProFormCheckbox noStyle name="autoLogin">保持本次会话</ProFormCheckbox>
          <div style={{ marginTop: 24, textAlign: 'center' }}>
            <Link to="/user/register" prefetch>还没有账户？立即注册</Link>
          </div>
        </LoginForm>
      </div>
      <Footer />
    </div>
  );
};

export default Login;
