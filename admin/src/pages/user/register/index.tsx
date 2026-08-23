import { LockOutlined, MailOutlined, TeamOutlined, UserOutlined } from '@ant-design/icons';
import { LoginForm, ProFormText } from '@ant-design/pro-components';
import { history, Link, useModel } from '@umijs/max';
import { Alert, App } from 'antd';
import React, { useState } from 'react';
import { register } from './service';

const Register: React.FC = () => {
  const { message } = App.useApp();
  const { initialState, setInitialState } = useModel('@@initialState');
  const [error, setError] = useState('');

  const handleSubmit = async (values: Record<string, string>) => {
    setError('');
    try {
      const auth = await register({
        email: values.email,
        password: values.password,
        displayName: values.displayName,
        workspaceName: values.workspaceName,
      });
      const userInfo = await initialState?.fetchUserInfo?.();
      if (userInfo) {
        setInitialState((state) => ({ ...state, currentUser: userInfo }));
      }
      message.success('注册成功，已自动登录');
      history.replace(`/user/register-result?account=${encodeURIComponent(auth.user.email)}`);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '注册失败，请重试');
    }
  };

  return (
    <LoginForm
      logo={<img alt="Lanverse" src="/logo.svg" />}
      title="创建 Lanverse 账户"
      subTitle="注册后将自动创建一个 Workspace 并成为 Admin"
      onFinish={(values) => handleSubmit(values as Record<string, string>)}
    >
      {error && <Alert title={error} type="error" showIcon />}
      <ProFormText
        name="email"
        fieldProps={{ size: 'large', prefix: <MailOutlined /> }}
        placeholder="邮箱"
        rules={[{ required: true, type: 'email', message: '请输入有效邮箱' }]}
      />
      <ProFormText
        name="displayName"
        fieldProps={{ size: 'large', prefix: <UserOutlined /> }}
        placeholder="显示名（可选）"
      />
      <ProFormText
        name="workspaceName"
        fieldProps={{ size: 'large', prefix: <TeamOutlined /> }}
        placeholder="Workspace 名称"
        rules={[{ required: true, message: '请输入 Workspace 名称' }]}
      />
      <ProFormText.Password
        name="password"
        fieldProps={{ size: 'large', prefix: <LockOutlined /> }}
        placeholder="密码（12—72 字节）"
        rules={[{ required: true, min: 12, max: 72, message: '密码长度必须为 12—72 字节' }]}
      />
      <ProFormText.Password
        name="confirmPassword"
        fieldProps={{ size: 'large', prefix: <LockOutlined /> }}
        placeholder="确认密码"
        dependencies={['password']}
        rules={[
          { required: true, message: '请确认密码' },
          ({ getFieldValue }) => ({
            validator(_, value) {
              return !value || getFieldValue('password') === value
                ? Promise.resolve()
                : Promise.reject(new Error('两次输入的密码不一致'));
            },
          }),
        ]}
      />
      <div style={{ marginTop: 24, textAlign: 'center' }}>
        <Link to="/user/login" prefetch>已有账户？返回登录</Link>
      </div>
    </LoginForm>
  );
};

export default Register;
