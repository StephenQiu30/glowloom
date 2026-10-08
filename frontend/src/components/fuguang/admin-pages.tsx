"use client";

import { useState } from "react";
import Link from "next/link";
import {
  ArrowUpRight,
  Circle,
  Download,
  KeyRound,
  Plus,
  RefreshCw,
  ShieldAlert,
  TriangleAlert,
  X,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableCell,
  TableHead,
} from "@/components/ui/table";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Switch } from "@/components/ui/switch";
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  ProductShell,
  PageHeading,
  SearchInput,
  Choice,
  MoreMenu,
  LocalDialog,
  demoNotice,
  EmptySearch,
} from "./shared";
import { screenHref } from "./screens";

type User = { login: string; name: string; role: string; status: string };
const initialUsers: User[] = [
  { login: "chendao", name: "陈导", role: "管理员", status: "启用" },
  { login: "shen.wan", name: "沈晚", role: "制作者", status: "启用" },
  { login: "lizhou", name: "李舟", role: "制作者", status: "需改密" },
  { login: "xiaoman", name: "小满", role: "制作者", status: "锁定中" },
  { login: "oldpost", name: "周驻", role: "制作者", status: "已停用" },
  { login: "ali", name: "阿梨", role: "管理员", status: "启用" },
];
function Status({ value }: { value: string }) {
  return (
    <Badge
      variant={
        /失败|不可用|停用|错误|失效/.test(value)
          ? "destructive"
          : /待|锁定|改密|未发布/.test(value)
            ? "warning"
            : "muted"
      }
    >
      {/失败|不可用|停用/.test(value) ? (
        <X />
      ) : /待|改密/.test(value) ? (
        <TriangleAlert />
      ) : (
        <Circle />
      )}
      {value}
    </Badge>
  );
}
export function UsersPage() {
  const [users, setUsers] = useState(initialUsers);
  const [search, setSearch] = useState("");
  const [role, setRole] = useState("角色：全部");
  const [status, setStatus] = useState("状态：全部");
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [login, setLogin] = useState("");
  const [newRole, setNewRole] = useState("制作者");
  const visible = users.filter(
    (user) =>
      (user.name + user.login).includes(search) &&
      (role.endsWith("全部") || role.includes(user.role)) &&
      (status.endsWith("全部") || status.includes(user.status)),
  );
  const updateStatus = (target: string, value: string) => {
    const user = users.find((item) => item.login === target);
    if (
      value === "已停用" &&
      user?.role === "管理员" &&
      users.filter((item) => item.role === "管理员" && item.status === "启用")
        .length <= 1
    ) {
      toast.error("至少保留一个启用中的管理员");
      return;
    }
    setUsers((current) =>
      current.map((user) =>
        user.login === target ? { ...user, status: value } : user,
      ),
    );
    demoNotice("账号状态已更新");
  };
  return (
    <ProductShell screen="users" contextual>
      <PageHeading
        title="账号"
        description="账号不会被删除；禁用后其所有会话立即失效，历史记录保留。"
      >
        <Button onClick={() => setCreating(true)}>
          <Plus data-icon="inline-start" />
          创建账号
        </Button>
      </PageHeading>
      <div className="mb-5 flex flex-wrap gap-2">
        <SearchInput
          value={search}
          onChange={setSearch}
          placeholder="登录名或显示名"
          className="w-72"
        />
        <Choice
          value={role}
          onChange={setRole}
          label="角色筛选"
          options={["角色：全部", "角色：管理员", "角色：制作者"]}
        />
        <Choice
          value={status}
          onChange={setStatus}
          label="状态筛选"
          options={[
            "状态：全部",
            "状态：启用",
            "状态：需改密",
            "状态：锁定中",
            "状态：已停用",
          ]}
        />
      </div>
      <div className="overflow-hidden rounded-lg bg-surface-1">
        <Table>
          <TableHeader>
            <TableRow>
              {["登录名", "显示名", "角色", "状态", "最后登录", ""].map(
                (head, index) => (
                  <TableHead key={index}>{head}</TableHead>
                ),
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
            {visible.map((user) => (
              <TableRow key={user.login}>
                <TableCell className="py-5 font-mono text-xs">
                  {user.login}
                </TableCell>
                <TableCell>
                  <span className="flex items-center gap-2">
                    <Avatar size="sm">
                      <AvatarFallback>{user.name[0]}</AvatarFallback>
                    </Avatar>
                    {user.name}
                  </span>
                </TableCell>
                <TableCell>{user.role}</TableCell>
                <TableCell>
                  <Status value={user.status} />
                </TableCell>
                <TableCell>
                  <span className="text-xs text-muted-foreground">[时间]</span>
                </TableCell>
                <TableCell>
                  <MoreMenu
                    label={`${user.name}的操作`}
                    items={[
                      {
                        label:
                          user.status === "已停用" ? "启用账号" : "停用账号",
                        action: () =>
                          updateStatus(
                            user.login,
                            user.status === "已停用" ? "启用" : "已停用",
                          ),
                      },
                      {
                        label: "重置密码",
                        action: () => updateStatus(user.login, "需改密"),
                      },
                      {
                        label: "解除锁定",
                        action: () => updateStatus(user.login, "启用"),
                      },
                    ]}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {!visible.length ? <EmptySearch /> : null}
      </div>
      <p className="mt-4 text-xs text-muted-foreground">
        至少保留一个启用中的管理员。
      </p>
      <LocalDialog
        open={creating}
        onOpenChange={setCreating}
        title="创建账号"
        confirmLabel="创建账号"
        onConfirm={() => {
          if (!name.trim() || !login.trim()) {
            toast.error("请填写显示名与登录名");
            return false;
          }
          if (users.some((user) => user.login === login)) {
            toast.error("登录名已存在");
            return false;
          }
          setUsers([
            ...users,
            { name, login, role: newRole, status: "需改密" },
          ]);
          setName("");
          setLogin("");
          demoNotice("演示账号已创建");
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="new-name">显示名</FieldLabel>
            <Input
              id="new-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="new-login">登录名</FieldLabel>
            <Input
              id="new-login"
              value={login}
              onChange={(e) => setLogin(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>角色</FieldLabel>
            <Choice
              value={newRole}
              onChange={setNewRole}
              label="新账号角色"
              options={["制作者", "管理员"]}
            />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
export function ProvidersPage() {
  const [editing, setEditing] = useState<string | null>("MiniMax");
  const [key, setKey] = useState("");
  const [group, setGroup] = useState("");
  const [tested, setTested] = useState(false);
  return (
    <ProductShell screen="providers" contextual>
      <PageHeading
        title="供应商凭据"
        description="仅管理员可见；已保存密钥永不回显。通过应用代理访问供应商。"
      />
      <div className="grid gap-4 xl:grid-cols-2">
        {["火山方舟", "MiniMax", "待扩供应商"].map((provider, index) => (
          <Card key={provider} variant="panel">
            <CardHeader>
              <div className="mb-4 flex items-center justify-between">
                <span className="flex size-10 items-center justify-center rounded-md bg-surface-3">
                  <KeyRound className="size-5" />
                </span>
                <Status
                  value={
                    index === 0 || (tested && index === 1)
                      ? "可用"
                      : index === 1
                        ? "测试失败"
                        : "未配置"
                  }
                />
              </div>
              <CardTitle>{provider}</CardTitle>
              <CardDescription>
                {index === 0
                  ? "生图 · 生视频 · 文本 / Agent"
                  : index === 1
                    ? "配音 · 音色复刻"
                    : "接入更多模型与生成能力"}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-5">
              <dl className="flex flex-col gap-3 text-xs">
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">API Key</dt>
                  <dd className="font-mono">
                    {index < 2 ? "••••••••••••" : "未设置"}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">最近测试</dt>
                  <dd>{index < 2 ? "10-07 13:06" : "—"}</dd>
                </div>
                <div className="flex justify-between">
                  <dt className="text-muted-foreground">操作者</dt>
                  <dd>{index < 2 ? "陈导" : "—"}</dd>
                </div>
              </dl>
              {index === 1 && !tested ? (
                <Alert variant="danger">
                  <ShieldAlert />
                  <AlertDescription>
                    凭据无效，请更新密钥后重试。
                  </AlertDescription>
                </Alert>
              ) : null}
              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  onClick={() => {
                    setEditing(provider);
                    setKey("");
                  }}
                >
                  {index < 2 ? "更新密钥" : "配置凭据"}
                </Button>
                {index < 2 ? (
                  <Button
                    variant="ghost"
                    onClick={() => {
                      setTested(true);
                      demoNotice("连接测试通过");
                    }}
                  >
                    测试连接
                  </Button>
                ) : null}
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
      <LocalDialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) {
            setEditing(null);
            setKey("");
          }
        }}
        title={`更新 ${editing ?? ""} 密钥`}
        description="新密钥仅用于此次测试；测试通过后才替换旧密钥。此预览不会发送或保存输入。"
        confirmLabel="保存并测试"
        onConfirm={() => {
          setKey("");
          setGroup("");
          setTested(true);
          demoNotice("演示连接测试通过");
        }}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="provider-key">API Key</FieldLabel>
            <Input
              id="provider-key"
              type="password"
              autoComplete="off"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="输入新的 API Key"
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="provider-group">Group ID（可选）</FieldLabel>
            <Input
              id="provider-group"
              value={group}
              onChange={(e) => setGroup(e.target.value)}
              placeholder="Group ID"
            />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
const modelRows = [
  {
    name: "[图像模型]",
    id: "ark.image.auto",
    provider: "火山方舟",
    kind: "生图",
    capability: "生图 · 改图",
    status: "启用",
    version: "v3",
  },
  {
    name: "[视频模型 A]",
    id: "ark.video.a",
    provider: "火山方舟",
    kind: "生视频",
    capability: "生视频",
    status: "启用",
    version: "v3",
  },
  {
    name: "[视频模型 B]",
    id: "ark.video.b",
    provider: "火山方舟",
    kind: "生视频",
    capability: "生视频 · 全能参考",
    status: "启用",
    version: "v2",
  },
  {
    name: "[配音模型]",
    id: "minimax.tts",
    provider: "MiniMax",
    kind: "配音",
    capability: "配音",
    status: "凭据失效",
    version: "v2",
  },
  {
    name: "[音色复刻]",
    id: "minimax.voice",
    provider: "MiniMax",
    kind: "配音",
    capability: "音色复刻",
    status: "凭据失效",
    version: "v1",
  },
  {
    name: "[文本模型]",
    id: "ark.text",
    provider: "火山方舟",
    kind: "文本 / Agent",
    capability: "文本 · Agent",
    status: "启用",
    version: "v5",
  },
  {
    name: "[境外视频模型]",
    id: "intl.video",
    provider: "[境外供应商]",
    kind: "生视频",
    capability: "生视频",
    status: "停用",
    version: "v1",
  },
];
export function ModelsPage() {
  const [kind, setKind] = useState("全部 12");
  const [selected, setSelected] = useState(1);
  const [duration, setDuration] = useState("5s");
  const [resolution, setResolution] = useState("1080p");
  const [published, setPublished] = useState(false);
  const [enabled, setEnabled] = useState(true);
  const [creating, setCreating] = useState(false);
  const model = modelRows[selected];
  return (
    <ProductShell screen="models" contextual>
      <PageHeading
        title="模型注册表"
        description="能力、上限与参数均由注册配置决定；每次编辑产生新版本。"
      >
        <Button onClick={() => setCreating(true)}>
          <Plus data-icon="inline-start" />
          新增模型
        </Button>
      </PageHeading>
      <ToggleGroup
        type="single"
        value={kind}
        onValueChange={(value) => value && setKind(value)}
        className="mb-5 flex-wrap"
        aria-label="模型能力"
      >
        {["全部 12", "生图", "生视频", "配音", "文本 / Agent"].map((value) => (
          <ToggleGroupItem key={value} value={value}>
            {value}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="overflow-hidden rounded-lg bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                {["模型", "供应商", "能力", "区域", "状态", "版本"].map(
                  (head) => (
                    <TableHead key={head}>{head}</TableHead>
                  ),
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {modelRows.map((row, index) =>
                kind === "全部 12" || kind === row.kind ? (
                  <TableRow
                    key={row.id}
                    data-state={selected === index ? "selected" : undefined}
                  >
                    <TableCell className="py-2.5">
                      <Button
                        variant="link"
                        size="sm"
                        onClick={() => {
                          setSelected(index);
                          setPublished(false);
                          setEnabled(row.status !== "停用");
                        }}
                      >
                        {row.name}
                      </Button>
                      <p className="font-mono text-[10px] text-muted-foreground">
                        {row.id}
                      </p>
                    </TableCell>
                    <TableCell>{row.provider}</TableCell>
                    <TableCell>{row.capability}</TableCell>
                    <TableCell>{index === 6 ? "境外" : "境内"}</TableCell>
                    <TableCell>
                      <Status value={row.status} />
                    </TableCell>
                    <TableCell>{row.version}</TableCell>
                  </TableRow>
                ) : null,
              )}
            </TableBody>
          </Table>
        </div>
        <Card variant="panel">
          <CardHeader>
            <div className="flex flex-wrap justify-between gap-2">
              <CardTitle>{model.name}</CardTitle>
              <Status value={published ? "已发布" : "未发布修改"} />
            </div>
            <CardDescription>{model.id} · v4 草稿</CardDescription>
          </CardHeader>
          <CardContent>
            <Tabs defaultValue="form">
              <TabsList>
                <TabsTrigger value="form">表单预览</TabsTrigger>
                <TabsTrigger value="json">JSON</TabsTrigger>
                <TabsTrigger value="history">版本差异</TabsTrigger>
              </TabsList>
              <TabsContent value="form" className="pt-5">
                <FieldGroup className="gap-5">
                  <Field>
                    <FieldLabel>支持模式</FieldLabel>
                    <div className="flex flex-wrap gap-2">
                      <Badge variant="secondary">图生视频</Badge>
                      <Badge variant="secondary">首尾帧</Badge>
                      <Badge variant="muted">全能参考 · 不支持</Badge>
                    </div>
                  </Field>
                  <Field>
                    <FieldLabel>参考上限</FieldLabel>
                    <p className="text-xs text-muted-foreground">
                      图片 9 · 视频 3 · 音频 3
                    </p>
                  </Field>
                  <Field>
                    <FieldLabel>生成默认参数</FieldLabel>
                    <div className="grid grid-cols-2 gap-3">
                      <Choice
                        value={duration}
                        onChange={setDuration}
                        label="时长"
                        options={["5s", "10s", "15s"]}
                        className="w-full"
                      />
                      <Choice
                        value={resolution}
                        onChange={setResolution}
                        label="分辨率"
                        options={["720p", "1080p"]}
                        className="w-full"
                      />
                    </div>
                  </Field>
                  <Field orientation="horizontal">
                    <FieldLabel htmlFor="fixed-camera">固定镜头</FieldLabel>
                    <Switch id="fixed-camera" />
                  </Field>
                </FieldGroup>
              </TabsContent>
              <TabsContent value="json">
                <pre className="mt-4 overflow-auto rounded-lg bg-background p-3 text-xs">
                  {JSON.stringify(
                    { id: model.id, duration, resolution, enabled },
                    null,
                    2,
                  )}
                </pre>
              </TabsContent>
              <TabsContent value="history">
                <p className="py-5 text-xs text-muted-foreground">
                  v3 → v4：默认分辨率更新为 {resolution}，时长 {duration}。
                </p>
              </TabsContent>
            </Tabs>
            <div className="mt-6 flex gap-2">
              <Button
                className="flex-1"
                onClick={() => {
                  setPublished(true);
                  demoNotice("模型 v4 已发布");
                }}
              >
                发布 v4
              </Button>
              <Button
                variant="outline"
                onClick={() => {
                  setEnabled(!enabled);
                  demoNotice(enabled ? "模型已停用" : "模型已启用");
                }}
              >
                {enabled ? "停用模型" : "启用模型"}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
      <LocalDialog
        open={creating}
        onOpenChange={setCreating}
        title="新增模型"
        onConfirm={() => demoNotice("模型草稿已保存")}
      >
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="model-name">模型名称</FieldLabel>
            <Input id="model-name" placeholder="输入模型名称" />
          </Field>
          <Field>
            <FieldLabel htmlFor="model-id">模型 ID</FieldLabel>
            <Input id="model-id" placeholder="provider.model" />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
const auditRows = [
  ["10-07 14:31:52", "chendao", "登录", "会话 macOS", "成功"],
  ["10-07 14:22:08", "shen.wan", "改选候选", "镜头 S03-02-02", "成功"],
  ["10-07 13:06:43", "chendao", "更新凭据", "供应商 MiniMax", "测试失败"],
  ["10-07 13:41:17", "lizhou", "登录", "会话 Windows", "密码错误"],
  ["10-07 12:09:03", "shen.wan", "改定稿", "设定 林舟·雨夜", "成功"],
  ["10-07 11:47:55", "chendao", "发布模型版本", "[视频模型 B] v2", "成功"],
  ["10-07 10:30:12", "ali", "禁用账号", "账号 oldpost", "成功"],
  ["10-06 22:14:38", "xiaoman", "登录", "会话 iPad", "已锁定"],
  ["10-06 19:02:21", "chendao", "复制项目", "雾港来信 → 副本", "成功"],
];
export function AuditPage() {
  const [selected, setSelected] = useState(1);
  const [actor, setActor] = useState("操作人：全部");
  const [action, setAction] = useState("动作：全部");
  const [period, setPeriod] = useState("最近 7 天");
  const [project, setProject] = useState("项目：全部");
  const [object, setObject] = useState("对象：全部");
  const visible = auditRows.filter(
    (row) =>
      (actor.endsWith("全部") || actor.includes(row[1])) &&
      (action.endsWith("全部") || action.includes(row[2])) &&
      (object.endsWith("全部") || row[3].includes(object.split("：")[1])),
  );
  const row = auditRows[selected];
  function exportCsv() {
    const blob = new Blob(
      [
        "\uFEFF" +
          [["时间", "操作人", "动作", "对象", "结果"], ...visible]
            .map((row) =>
              row.map((value) => `"${value.replaceAll('"', '""')}"`).join(","),
            )
            .join("\n"),
      ],
      { type: "text/csv;charset=utf-8" },
    );
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "浮光-演示审计日志.csv";
    a.click();
    URL.revokeObjectURL(url);
    demoNotice("演示日志已导出");
  }
  return (
    <ProductShell screen="audit" contextual>
      <PageHeading
        title="审计日志"
        description="记录账号、凭据、模型、生成与选定等关键操作；只读，不可修改。"
      >
        <Button variant="secondary" onClick={exportCsv}>
          <Download data-icon="inline-start" />
          导出 CSV
        </Button>
      </PageHeading>
      <div className="mb-5 flex flex-wrap gap-2">
        <Choice
          value={period}
          onChange={setPeriod}
          options={["最近 7 天", "最近 14 天", "最近 30 天"]}
          label="时间范围"
        />
        <Choice
          value={actor}
          onChange={setActor}
          options={[
            "操作人：全部",
            "操作人：chendao",
            "操作人：shen.wan",
            "操作人：lizhou",
          ]}
          label="操作人"
        />
        <Choice
          value={project}
          onChange={setProject}
          options={["项目：全部", "项目：雾港来信"]}
          label="项目"
        />
        <Choice
          value={object}
          onChange={setObject}
          options={["对象：全部", "对象：会话", "对象：镜头", "对象：供应商"]}
          label="对象"
        />
        <Choice
          value={action}
          onChange={setAction}
          options={[
            "动作：全部",
            "动作：登录",
            "动作：改选候选",
            "动作：更新凭据",
          ]}
          label="动作"
        />
      </div>
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="overflow-hidden rounded-lg bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                {["时间", "操作人", "动作", "对象", "结果"].map((head) => (
                  <TableHead key={head}>{head}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((item) => (
                <TableRow
                  key={item[0]}
                  data-state={item === row ? "selected" : undefined}
                >
                  <TableCell className="py-4 font-mono text-xs">
                    {item[0]}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{item[1]}</TableCell>
                  <TableCell>
                    <Button
                      variant="link"
                      size="xs"
                      onClick={() => setSelected(auditRows.indexOf(item))}
                    >
                      {item[2]}
                    </Button>
                  </TableCell>
                  <TableCell>{item[3]}</TableCell>
                  <TableCell>
                    {item[4] === "成功" ? item[4] : <Status value={item[4]} />}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          {!visible.length ? <EmptySearch /> : null}
        </div>
        <Card variant="panel">
          <CardHeader>
            <CardTitle>{row[2]}</CardTitle>
            <CardDescription>
              {row[0]} · {row[1]}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <dl className="grid grid-cols-[50px_1fr] gap-y-3 text-xs">
              <dt className="text-muted-foreground">项目</dt>
              <dd>雾港来信</dd>
              <dt className="text-muted-foreground">对象</dt>
              <dd>{row[3]}</dd>
              <dt className="text-muted-foreground">请求</dt>
              <dd className="font-mono">req_7c1e_a98</dd>
              <dt className="text-muted-foreground">变更</dt>
              <dd>{row[4]}</dd>
            </dl>
            <div className="overflow-hidden rounded-md bg-background font-mono text-[11px]">
              <p className="bg-destructive/10 p-2 text-destructive">
                − selected_candidate: &quot;A&quot;
              </p>
              <p className="bg-surface-3 p-2">
                + selected_candidate: &quot;B&quot;
              </p>
              <p className="p-2 text-muted-foreground">revision: 11 → 12</p>
            </div>
            <Alert variant="warning">
              <TriangleAlert />
              <AlertDescription>
                上次选定的 2 个下游结果已过期。
              </AlertDescription>
            </Alert>
          </CardContent>
        </Card>
      </div>
    </ProductShell>
  );
}
export function HealthPage() {
  const [resolved, setResolved] = useState(false);
  return (
    <ProductShell screen="health" contextual>
      <PageHeading title="系统健康" description="每 30 秒刷新 · 以下为示意数据">
        <Button
          variant="secondary"
          onClick={() => demoNotice("Grafana 监控入口")}
        >
          Grafana
          <ArrowUpRight data-icon="inline-end" />
        </Button>
        <Button
          variant="secondary"
          onClick={() => demoNotice("Temporal 工作流入口")}
        >
          Temporal UI
          <ArrowUpRight data-icon="inline-end" />
        </Button>
      </PageHeading>
      <Alert variant={resolved ? "default" : "warning"}>
        <TriangleAlert />
        <AlertDescription>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <span>
              {resolved
                ? "演示检查已完成，当前服务运行正常。"
                : "部分降级：MiniMax 凭据失效，1 个任务待人工核对超过 24 小时"}
            </span>
            <Button variant="link" size="xs" asChild>
              <Link href={screenHref("providers")}>去处理</Link>
            </Button>
          </div>
        </AlertDescription>
      </Alert>
      <div className="my-5 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {[
          ["失败率 · 15 分钟", "3.0%", "阈值 5%"],
          ["结果未知", "0", "自动对账中"],
          ["待人工核对", resolved ? "0" : "1", "1 个超过 24 小时"],
          ["Outbox 积压", "4", "最旧 12 秒"],
        ].map(([title, value, note]) => (
          <Card variant="panel" key={title}>
            <CardHeader>
              <CardDescription>{title}</CardDescription>
              <CardTitle>
                <span className="font-mono text-[30px]">{value}</span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-xs text-muted-foreground">{note}</p>
            </CardContent>
          </Card>
        ))}
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Card variant="panel">
          <CardHeader>
            <CardTitle>供应商 · 近 1 小时</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  {["供应商", "成功率", "P95 延迟", "状态"].map((head) => (
                    <TableHead key={head}>{head}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ["火山方舟", "98.9%", "42.8s", "正常"],
                  [
                    "MiniMax",
                    resolved ? "99.0%" : "0.0%",
                    resolved ? "1.2s" : "—",
                    resolved ? "正常" : "不可用",
                  ],
                  ["[境外供应商]", "—", "—", "未启用"],
                ].map((row) => (
                  <TableRow key={row[0]}>
                    {row.map((value, i) => (
                      <TableCell key={i} className="py-4">
                        {i === 3 ? <Status value={value} /> : value}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        <Card variant="panel">
          <CardHeader>
            <CardTitle>任务队列</CardTitle>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  {["队列", "积压", "Worker", "负载"].map((head) => (
                    <TableHead key={head}>{head}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ["agent.ark", 12, 2, 60],
                  ["media.tool", 3, 2, 16],
                  ["tts.minimax", 28, 1, 99],
                  ["export.ffmpeg", 0, 1, 0],
                ].map((row) => (
                  <TableRow key={row[0]}>
                    <TableCell className="py-4 font-mono text-xs">
                      {row[0]}
                    </TableCell>
                    <TableCell>{row[1]}</TableCell>
                    <TableCell>{row[2]}</TableCell>
                    <TableCell>
                      <Progress
                        value={Number(row[3])}
                        aria-label={`${row[0]} 负载`}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>
      <div className="mt-5 flex justify-end">
        <Button
          variant="ghost"
          onClick={() => {
            setResolved(!resolved);
            demoNotice("系统状态已刷新");
          }}
        >
          <RefreshCw data-icon="inline-start" />
          刷新状态
        </Button>
      </div>
    </ProductShell>
  );
}
