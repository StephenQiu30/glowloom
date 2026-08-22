import { HeartTwoTone, SmileTwoTone } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-components';
import { Alert, Card, Typography } from 'antd';
import React from 'react';

const Admin: React.FC = () => {
  return (
    <PageContainer
      content="仅管理员可以访问此页面"
    >
      <Card>
        <Alert
          title="Lanverse 管理端基础入口已准备就绪。"
          type="success"
          showIcon
          banner
          style={{
            margin: -12,
            marginBottom: 48,
          }}
        />
        <Typography.Title level={2} style={{ textAlign: 'center' }}>
          <SmileTwoTone /> Lanverse 管理端{' '}
          <HeartTwoTone twoToneColor="#eb2f96" />
        </Typography.Title>
      </Card>
      <p style={{ textAlign: 'center', marginTop: 24 }}>
        后续管理模块将在此入口下按业务域逐步接入。
      </p>
    </PageContainer>
  );
};

export default Admin;
