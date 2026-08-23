import { PageContainer } from '@ant-design/pro-components';
import { useModel } from '@umijs/max';
import {
  Alert,
  App,
  Button,
  Card,
  DatePicker,
  Input,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { Dayjs } from 'dayjs';
import dayjs from 'dayjs';
import React, { useEffect, useMemo, useState } from 'react';
import {
  type AccessAuditEvent,
  listAccessAudit,
  listMembers,
  type MembershipStatus,
  updateMember,
  type WorkspaceMember,
} from '@/services/admin';
import type { RoleCode } from '@/services/session';

const roleLabels: Record<RoleCode, string> = {
  admin: '管理员',
  user: '普通用户',
  ban: '已封禁',
};

const statusLabels: Record<API.MembershipStatus, string> = {
  active: '正常',
  suspended: '已停用',
  removed: '已移除',
};

const auditResultLabels: Record<API.AccessAuditResult, string> = {
  succeeded: '成功',
  denied: '拒绝',
  failed: '失败',
};

const auditResultColors: Record<API.AccessAuditResult, string> = {
  succeeded: 'success',
  denied: 'warning',
  failed: 'error',
};

const auditStateText = (state: Record<string, unknown>) => {
  const entries = Object.entries(state);
  if (entries.length === 0) return '无';
  return entries
    .map(([key, value]) => {
      if (key === 'role' && typeof value === 'string' && value in roleLabels) {
        return `角色：${roleLabels[value as RoleCode]}`;
      }
      if (
        key === 'status' &&
        typeof value === 'string' &&
        value in statusLabels
      ) {
        return `状态：${statusLabels[value as API.MembershipStatus]}`;
      }
      return `${key}：${String(value)}`;
    })
    .join('；');
};

const roleOptions = () =>
  (Object.keys(roleLabels) as RoleCode[]).map((role) => ({
    label: roleLabels[role],
    value: role,
  }));

const statusOptions = (status: API.MembershipStatus) =>
  (Object.keys(statusLabels) as API.MembershipStatus[])
    .filter((value) => !(status === 'removed' && value !== 'removed'))
    .map((value) => ({
      label: statusLabels[value],
      value,
    }));

type PendingMemberChange = {
  member: WorkspaceMember;
  update: { role?: RoleCode; status?: MembershipStatus };
};

const Admin: React.FC = () => {
  const { message } = App.useApp();
  const { initialState } = useModel('@@initialState');
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [updatingID, setUpdatingID] = useState('');
  const [pendingChange, setPendingChange] = useState<PendingMemberChange>();
  const [changeReason, setChangeReason] = useState('');
  const [auditEvents, setAuditEvents] = useState<AccessAuditEvent[]>([]);
  const [auditLoading, setAuditLoading] = useState(true);
  const [auditError, setAuditError] = useState('');
  const [auditSearch, setAuditSearch] = useState('');
  const [auditActor, setAuditActor] = useState('');
  const [auditObject, setAuditObject] = useState('');
  const [auditResult, setAuditResult] = useState<API.AccessAuditResult>();
  const [auditRange, setAuditRange] = useState<
    [Dayjs | null, Dayjs | null] | null
  >(null);
  const [auditQuery, setAuditQuery] = useState<API.adminListAccessAuditParams>({
    page: 1,
    page_size: 20,
  });
  const [auditTotal, setAuditTotal] = useState(0);
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

  const loadAudit = async (query: API.adminListAccessAuditParams) => {
    setAuditLoading(true);
    setAuditError('');
    try {
      const result = await listAccessAudit(query);
      setAuditEvents(result.items);
      setAuditTotal(result.total);
      setAuditQuery({
        ...query,
        page: result.page,
        page_size: result.page_size,
      });
    } catch {
      setAuditError('访问审计加载失败，请稍后重试。');
    } finally {
      setAuditLoading(false);
    }
  };

  const currentAuditFilters = (): API.adminListAccessAuditParams => ({
    search: auditSearch.trim() || undefined,
    actor: auditActor.trim() || undefined,
    object: auditObject.trim() || undefined,
    result: auditResult,
    occurred_from: auditRange?.[0]?.startOf('day').toISOString(),
    occurred_to: auditRange?.[1]?.endOf('day').toISOString(),
    page: 1,
    page_size: auditQuery.page_size ?? 20,
  });

  useEffect(() => {
    void loadMembers('');
    void loadAudit({ page: 1, page_size: 20 });
  }, []);

  const requestMemberChange = (
    member: WorkspaceMember,
    update: { role?: RoleCode; status?: MembershipStatus },
  ) => {
    setPendingChange({ member, update });
    setChangeReason('');
  };

  const changeMember = async () => {
    if (!pendingChange || !changeReason.trim()) return;

    const { member, update } = pendingChange;
    setUpdatingID(member.membership_id);
    try {
      const updated = await updateMember(member.membership_id, {
        ...update,
        reason: changeReason.trim(),
      });
      setMembers((items) =>
        items.map((item) =>
          item.membership_id === updated.membership_id ? updated : item,
        ),
      );
      message.success('成员信息已更新');
      setPendingChange(undefined);
      setChangeReason('');
      await loadAudit({ ...auditQuery, page: 1 });
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
          <Space orientation="vertical" size={0}>
            <Typography.Text strong>{member.display_name}</Typography.Text>
            <Typography.Text>{member.email}</Typography.Text>
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
            disabled={
              member.membership_id === currentMembershipID ||
              updatingID === member.membership_id
            }
            loading={updatingID === member.membership_id}
            aria-label={`修改 ${member.display_name} 的角色`}
            onChange={(value: RoleCode) =>
              requestMemberChange(member, { role: value })
            }
            style={{ minWidth: 120 }}
          />
        ),
      },
      {
        title: '状态',
        dataIndex: 'membership_status',
        key: 'membership_status',
        render: (status: API.MembershipStatus, member) => (
          <Select
            value={status}
            options={statusOptions(status)}
            disabled={
              member.membership_id === currentMembershipID ||
              updatingID === member.membership_id
            }
            loading={updatingID === member.membership_id}
            aria-label={`修改 ${member.display_name} 的状态`}
            onChange={(value: API.MembershipStatus) =>
              requestMemberChange(member, { status: value })
            }
            style={{ minWidth: 110 }}
          />
        ),
      },
      {
        title: '账户状态',
        dataIndex: 'account_status',
        key: 'account_status',
        render: (status: WorkspaceMember['account_status']) => (
          <Tag>{status}</Tag>
        ),
      },
      {
        title: 'Membership ID',
        dataIndex: 'membership_id',
        key: 'membership_id',
        render: (id: string) => (
          <Typography.Text copyable={{ text: id }}>
            {id.slice(0, 8)}…
          </Typography.Text>
        ),
      },
    ],
    [currentMembershipID, updatingID],
  );

  const auditColumns = useMemo<ColumnsType<AccessAuditEvent>>(
    () => [
      {
        title: '时间',
        dataIndex: 'occurred_at',
        key: 'occurred_at',
        width: 170,
        render: (occurredAt: string) =>
          dayjs(occurredAt).format('YYYY-MM-DD HH:mm:ss'),
      },
      {
        title: '操作者',
        key: 'actor',
        render: (_, event) => (
          <Space orientation="vertical" size={0}>
            <Typography.Text strong>
              {event.actor_display_name || event.actor_type}
            </Typography.Text>
            {event.actor_email && (
              <Typography.Text>{event.actor_email}</Typography.Text>
            )}
            <Typography.Text
              type="secondary"
              copyable={{ text: event.actor_id }}
            >
              {event.actor_id.slice(0, 12)}…
            </Typography.Text>
          </Space>
        ),
      },
      {
        title: '动作与对象',
        key: 'object',
        render: (_, event) => (
          <Space orientation="vertical" size={0}>
            <Typography.Text code>{event.action}</Typography.Text>
            <Typography.Text strong>
              {event.object_display_name || event.object_type}
            </Typography.Text>
            {event.object_email && (
              <Typography.Text>{event.object_email}</Typography.Text>
            )}
            <Typography.Text
              type="secondary"
              copyable={{ text: event.object_id }}
            >
              {event.object_id.slice(0, 8)}…
            </Typography.Text>
          </Space>
        ),
      },
      {
        title: '变更基线',
        key: 'state',
        render: (_, event) => (
          <Space orientation="vertical" size={0}>
            <Typography.Text>
              之前：{auditStateText(event.before_state)}
            </Typography.Text>
            <Typography.Text>
              当前：{auditStateText(event.after_state)}
            </Typography.Text>
          </Space>
        ),
      },
      {
        title: '理由与结果',
        key: 'reason',
        render: (_, event) => (
          <Space orientation="vertical" size={0}>
            <Typography.Text>{event.reason}</Typography.Text>
            <Tag color={auditResultColors[event.result]}>
              {auditResultLabels[event.result]}
            </Tag>
          </Space>
        ),
      },
      {
        title: 'Request ID',
        dataIndex: 'request_id',
        key: 'request_id',
        render: (requestID: string) => (
          <Typography.Text copyable={{ text: requestID }}>
            {requestID.slice(0, 12)}…
          </Typography.Text>
        ),
      },
    ],
    [],
  );

  return (
    <PageContainer
      title="用户管理"
      content="管理当前 Workspace 的成员、角色和访问状态"
    >
      <Card>
        <Space orientation="vertical" size="large" style={{ width: '100%' }}>
          <Input.Search
            allowClear
            placeholder="按邮箱或显示名搜索"
            onSearch={(value) => {
              setSearch(value);
              void loadMembers(value);
            }}
            style={{ maxWidth: 360 }}
          />
          {error && <Alert type="error" showIcon title={error} />}
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
      <Card title="访问审计" style={{ marginTop: 24 }}>
        <Space orientation="vertical" size="large" style={{ width: '100%' }}>
          <Space wrap>
            <Input
              allowClear
              aria-label="搜索审计记录"
              placeholder="动作、理由或 Request ID"
              value={auditSearch}
              onChange={(event) => setAuditSearch(event.target.value)}
              style={{ width: 240 }}
            />
            <Input
              allowClear
              aria-label="按审计主体筛选"
              placeholder="主体名称、邮箱或 ID"
              value={auditActor}
              onChange={(event) => setAuditActor(event.target.value)}
              style={{ width: 220 }}
            />
            <Input
              allowClear
              aria-label="按审计对象筛选"
              placeholder="对象名称、邮箱或 ID"
              value={auditObject}
              onChange={(event) => setAuditObject(event.target.value)}
              style={{ width: 220 }}
            />
            <Select
              allowClear
              aria-label="按审计结果筛选"
              placeholder="全部结果"
              value={auditResult}
              options={(
                Object.keys(auditResultLabels) as API.AccessAuditResult[]
              ).map((result) => ({
                label: auditResultLabels[result],
                value: result,
              }))}
              onChange={(value) => setAuditResult(value)}
              style={{ width: 130 }}
            />
            <DatePicker.RangePicker
              aria-label="按审计时间筛选"
              value={auditRange}
              onChange={(value) => setAuditRange(value)}
            />
            <Button
              type="primary"
              onClick={() => void loadAudit(currentAuditFilters())}
            >
              查询
            </Button>
            <Button
              onClick={() => {
                setAuditSearch('');
                setAuditActor('');
                setAuditObject('');
                setAuditResult(undefined);
                setAuditRange(null);
                void loadAudit({ page: 1, page_size: 20 });
              }}
            >
              重置
            </Button>
          </Space>
          {auditError && <Alert type="error" showIcon title={auditError} />}
          <Table
            rowKey="id"
            loading={auditLoading}
            columns={auditColumns}
            dataSource={auditEvents}
            scroll={{ x: 1200 }}
            locale={{ emptyText: '当前 Workspace 暂无访问审计' }}
            pagination={{
              current: auditQuery.page ?? 1,
              pageSize: auditQuery.page_size ?? 20,
              total: auditTotal,
              showSizeChanger: false,
              showTotal: (total) => `共 ${total} 条`,
              onChange: (page) => {
                const nextQuery = { ...auditQuery, page };
                setAuditQuery(nextQuery);
                void loadAudit(nextQuery);
              },
            }}
          />
        </Space>
      </Card>
      <Modal
        title="确认成员权限变更"
        open={Boolean(pendingChange)}
        okText="确认变更"
        cancelText="取消"
        confirmLoading={Boolean(updatingID)}
        okButtonProps={{ disabled: !changeReason.trim() }}
        destroyOnHidden
        onCancel={() => {
          if (updatingID) return;
          setPendingChange(undefined);
          setChangeReason('');
        }}
        onOk={() => void changeMember()}
      >
        {pendingChange && (
          <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
            <Typography.Text>
              {pendingChange.update.role
                ? `将 ${pendingChange.member.display_name} 的角色从“${roleLabels[pendingChange.member.role]}”改为“${roleLabels[pendingChange.update.role]}”。`
                : `将 ${pendingChange.member.display_name} 的状态从“${statusLabels[pendingChange.member.membership_status]}”改为“${statusLabels[pendingChange.update.status as MembershipStatus]}”。`}
            </Typography.Text>
            <Input.TextArea
              aria-label="成员变更理由"
              autoFocus
              autoSize={{ minRows: 3, maxRows: 6 }}
              maxLength={500}
              showCount
              placeholder="填写可供审计复核的明确理由"
              value={changeReason}
              onChange={(event) => setChangeReason(event.target.value)}
            />
          </Space>
        )}
      </Modal>
    </PageContainer>
  );
};

export default Admin;
