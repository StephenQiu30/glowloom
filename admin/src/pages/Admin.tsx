import { PageContainer } from '@ant-design/pro-components';
import { Alert, App, Card, Input, Select, Space, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import React, { useEffect, useMemo, useState } from 'react';
import { useModel } from '@umijs/max';
import { listMembers, updateMember, type MembershipStatus, type WorkspaceMember } from '@/services/admin';
import type { RoleCode } from '@/services/session';

const roleLabels: Record<RoleCode, string> = {
	admin: '管理员',
	user: '普通用户',
	ban: '已封禁',
};

const statusLabels: Record<MembershipStatus, string> = {
  active: '正常',
  suspended: '已停用',
  removed: '已移除',
};

const roleOptions = () =>
  (Object.keys(roleLabels) as RoleCode[])
    .map((role) => ({ label: roleLabels[role], value: role }));

const statusOptions = (status: MembershipStatus) =>
  (Object.keys(statusLabels) as MembershipStatus[])
    .filter((value) => !(status === 'removed' && value !== 'removed'))
    .map((value) => ({ label: statusLabels[value], value }));

const Admin: React.FC = () => {
  const { message } = App.useApp();
  const { initialState } = useModel('@@initialState');
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [updatingID, setUpdatingID] = useState('');
  const currentMembershipID = initialState?.currentUser?.membershipId;

  const loadMembers = async (keyword = search) => {
    setLoading(true);
    setError('');
    try {
      const result = await listMembers(keyword);
      setMembers(result.items);
    } catch {
      setError('成员列表加载失败，请确认当前账户具有管理员权限。');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadMembers('');
  }, []);

  const changeMember = async (member: WorkspaceMember, update: { role?: RoleCode; status?: MembershipStatus }) => {
    setUpdatingID(member.membership_id);
    try {
      const updated = await updateMember(member.membership_id, update);
      setMembers((items) => items.map((item) => (item.membership_id === updated.membership_id ? updated : item)));
      message.success('成员信息已更新');
    } catch {
      message.error('成员信息更新失败');
    } finally {
      setUpdatingID('');
    }
  };

  const columns = useMemo<ColumnsType<WorkspaceMember>>(
    () => [
      {
        title: '成员',
        key: 'member',
        render: (_, member) => (
          <Space direction="vertical" size={0}>
            <Typography.Text strong>{member.display_name}</Typography.Text>
            <Typography.Text type="secondary">{member.email}</Typography.Text>
          </Space>
        ),
      },
      {
        title: '角色',
        dataIndex: 'role',
        key: 'role',
        render: (role: RoleCode, member) => (
          <Select
            value={role}
            options={roleOptions()}
            disabled={member.membership_id === currentMembershipID || updatingID === member.membership_id}
            loading={updatingID === member.membership_id}
            onChange={(value: RoleCode) => void changeMember(member, { role: value })}
            style={{ minWidth: 120 }}
          />
        ),
      },
      {
        title: '状态',
        dataIndex: 'membership_status',
        key: 'membership_status',
        render: (status: MembershipStatus, member) => (
          <Select
            value={status}
            options={statusOptions(status)}
            disabled={member.membership_id === currentMembershipID || updatingID === member.membership_id}
            loading={updatingID === member.membership_id}
            onChange={(value: MembershipStatus) => void changeMember(member, { status: value })}
            style={{ minWidth: 110 }}
          />
        ),
      },
      {
        title: '账户状态',
        dataIndex: 'account_status',
        key: 'account_status',
        render: (status: WorkspaceMember['account_status']) => (
          <Tag color={status === 'active' ? 'green' : 'default'}>{status}</Tag>
        ),
      },
      {
        title: 'Membership ID',
        dataIndex: 'membership_id',
        key: 'membership_id',
        render: (id: string) => <Typography.Text copyable={{ text: id }}>{id.slice(0, 8)}…</Typography.Text>,
      },
    ],
    [currentMembershipID, updatingID],
  );

  return (
    <PageContainer title="用户管理" content="管理当前 Workspace 的成员、角色和访问状态">
      <Card>
        <Space direction="vertical" size="large" style={{ width: '100%' }}>
          <Input.Search
            allowClear
            placeholder="按邮箱或显示名搜索"
            onSearch={(value) => {
              setSearch(value);
              void loadMembers(value);
            }}
            style={{ maxWidth: 360 }}
          />
          {error && <Alert type="error" showIcon message={error} />}
          <Table
            rowKey="membership_id"
            loading={loading}
            columns={columns}
            dataSource={members}
            pagination={false}
            locale={{ emptyText: '当前 Workspace 暂无成员' }}
          />
        </Space>
      </Card>
    </PageContainer>
  );
};

export default Admin;
