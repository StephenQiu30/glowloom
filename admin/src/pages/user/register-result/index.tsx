import { Link, useSearchParams } from '@umijs/max';
import { Button, Result } from 'antd';
import React from 'react';
import useStyles from './style.style';

const RegisterResult: React.FC<Record<string, unknown>> = () => {
  const { styles } = useStyles();
  const [params] = useSearchParams();

  const actions = (
    <div className={styles.actions}>
      <Link to="/admin" prefetch>
        <Button size="large" type="primary">进入管理端</Button>
      </Link>
      <Link to="/" prefetch>
        <Button size="large">返回首页</Button>
      </Link>
    </div>
  );

  const email = params?.get('account') || '当前邮箱';
  const workspaceID = params?.get('workspace');
  return (
    <Result
      className={styles.registerResult}
      status="success"
      title={
        <div className={styles.title}>
          <span>你的账户：{email} 注册成功</span>
        </div>
      }
      subTitle={
        workspaceID
          ? `Workspace 已创建，Workspace ID：${workspaceID}。请保存该 ID，后续登录需要使用。`
          : '账户已创建并自动登录。'
      }
      extra={actions}
    />
  );
};
export default RegisterResult;
