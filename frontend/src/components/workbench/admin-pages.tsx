"use client";

import { useState } from "react";
import { Search, UserRound, ShieldCheck } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { FieldGroup } from "@/components/ui/field";
import {
  PageHeading,
  Panel,
  DataTable,
  TextField,
  ChoiceField,
  DraftNotice,
  EmptyMessage,
  usePreviewQuery,
} from "./workbench-components";

export const adminPages = [
  "users",
  "providers",
  "models",
  "audit",
  "health",
] as const;
export type AdminPage = (typeof adminPages)[number];
export function AdminScreen({ section }: { section: AdminPage }) {
  const { readOnly, params, set } = usePreviewQuery();
  const [model, setModel] = useState("视频模型（样例）");
  const [price, setPrice] = useState("1.20");
  const [duration, setDuration] = useState("5");
  if (section === "models")
    return (
      <div className="space-y-6">
        <PageHeading
          eyebrow="服务管理"
          title="模型能力与价格"
          description="查看模型输入、输出与计费方式。当前为配置预览，模型服务尚未接入。"
          action={<Badge variant="secondary">配置预览</Badge>}
        />
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(280px,1fr)]">
          <Panel title="模型注册表">
            <DataTable
              caption="模型注册表"
              columns={["模型", "模式", "计费", "状态"]}
              rows={[
                ["图像模型（样例）", "文本 / 图像参考", "¥1.20 / 图", "待接入"],
                ["视频模型（样例）", "首帧 / 多引用", "¥1.20 / 秒", "待接入"],
                ["语音模型（样例）", "台词 / 声线", "按字符", "授权待核对"],
              ]}
            />
          </Panel>
          <Panel
            title="参数与计价预览"
            description="仅修改本页草稿，不发布模型或价格版本。"
          >
            <FieldGroup>
              <ChoiceField
                id="admin-model"
                label="模型"
                value={model}
                options={["视频模型（样例）", "图像模型（样例）"]}
                onChange={setModel}
                disabled={readOnly}
              />
              <TextField
                id="admin-price"
                label="单位价格（元，样例）"
                value={price}
                onChange={setPrice}
                disabled={readOnly}
              />
              <ChoiceField
                id="admin-duration"
                label={model.startsWith("视频") ? "时长（秒）" : "输出图数"}
                value={duration}
                options={["1", "4", "5", "6"]}
                onChange={setDuration}
                disabled={readOnly}
              />
              <div className="rounded-xl bg-muted/50 p-4">
                <p className="text-xs text-muted-foreground">费用估算 · 样例</p>
                <p className="mt-2 text-xl font-semibold tabular-nums">
                  {Number.isFinite(Number(price)) && Number(price) >= 0
                    ? `¥${(Number(price) * Number(duration)).toFixed(2)}`
                    : "请输入有效价格"}
                </p>
              </div>
              <DraftNotice
                changed={
                  price !== "1.20" ||
                  duration !== "5" ||
                  model !== "视频模型（样例）"
                }
              />
              <Button disabled>发布服务待接入</Button>
            </FieldGroup>
          </Panel>
        </div>
        <Panel title="输入限制">
          <DataTable
            caption="模型输入限制"
            columns={["约束", "样例合同"]}
            rows={[
              ["引用容量", "最多 3 个结构化图像引用"],
              ["可用时长", "4 / 5 / 6 秒"],
              ["输出画幅", "16:9 / 9:16"],
              ["提交不确定", "先核对，不自动重复付费提交"],
            ]}
          />
        </Panel>
      </div>
    );
  const screens = {
    users: {
      title: "用户与权限",
      description: "查看内部成员与项目角色。当前为样例账号与权限。",
      listTitle: "项目成员",
      columns: ["用户", "角色", "状态", "项目范围"],
      rows: [
        ["创作负责人（样例）", "owner", "启用", "雾港来信"],
        ["分镜编辑（样例）", "editor", "启用", "雾港来信"],
        ["审阅者（样例）", "viewer", "只读", "雾港来信"],
      ],
    },
    providers: {
      title: "供应商连接",
      description: "查看连接协议与服务状态，当前供应商均未接入。",
      listTitle: "连接列表",
      columns: ["供应商", "协议", "区域", "状态"],
      rows: [
        ["图像供应商（样例）", "异步任务", "待确认", "尚未接入"],
        ["视频供应商（样例）", "提交 / 轮询 / 下载", "待确认", "尚未接入"],
        ["声音供应商（样例）", "语音合成", "待确认", "尚未接入"],
      ],
    },
    audit: {
      title: "审计记录",
      description: "按时间追溯操作、对象与结果。当前为样例记录。",
      listTitle: "操作记录",
      columns: ["时间", "操作", "对象", "结果"],
      rows: [
        ["今天 10:42", "候选登记", "shot-01 / v3", "样例成功"],
        ["今天 10:38", "生成失败", "task-02", "参数需核对"],
        ["今天 10:35", "提交不确定", "task-03", "等待核对"],
      ],
    },
    health: {
      title: "依赖状态",
      description: "查看工作台依赖。当前页面尚未执行实时检查。",
      listTitle: "服务依赖",
      columns: ["依赖", "职责", "当前证据", "状态"],
      rows: [
        ["Go API", "业务命令与查询", "未从本页检测", "待验证"],
        ["PostgreSQL", "业务事实", "未从本页检测", "待验证"],
        ["Temporal", "持久化工作流", "未从本页检测", "待验证"],
        ["对象存储", "私有素材与产物", "未从本页检测", "待验证"],
        ["模型供应商", "真实生成", "未发起调用", "待接入"],
        ["Agent", "可选服务", "允许必要时取消", "Go 整合待评估"],
      ],
    },
  };
  const screen = screens[section];
  const q = params.get("q") ?? "";
  const search = q.trim().toLocaleLowerCase();
  const rows = screen.rows.filter((row) =>
    row.join(" ").toLocaleLowerCase().includes(search),
  );
  return (
    <div className="space-y-6">
      <PageHeading
        eyebrow="服务管理"
        title={screen.title}
        description={screen.description}
        action={<Badge variant="secondary">内部管理</Badge>}
      />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {rows.length} 条样例记录
        </p>
        <div className="relative w-full sm:max-w-xs">
          <Search
            aria-hidden="true"
            className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
          />
          <Input
            aria-label={`搜索${screen.title}`}
            placeholder="搜索名称、状态或范围"
            value={q}
            onChange={(event) => set("q", event.target.value)}
            className="h-9 rounded-full border-0 bg-muted/50 pl-9"
          />
        </div>
      </div>
      <Panel title={screen.listTitle}>
        {rows.length ? (
          <DataTable
            caption={screen.title}
            columns={screen.columns}
            rows={rows}
          />
        ) : (
          <EmptyMessage title="没有匹配的记录" description="尝试其他搜索词。" />
        )}
      </Panel>
      <p className="text-xs text-muted-foreground">
        管理写入、权限与实时检查将在服务补齐阶段实现。
      </p>
    </div>
  );
}
export function AccountPage() {
  const { readOnly } = usePreviewQuery();
  const [name, setName] = useState("创作负责人");
  const [language, setLanguage] = useState("简体中文");
  return (
    <div className="space-y-6">
      <PageHeading
        title="账号与偏好"
        description="内部演示账号。当前未接入登录会话或账号持久化。"
      />
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(250px,1fr)]">
        <Panel title="个人资料">
          <div className="mb-6 flex items-center gap-3">
            <span className="flex size-12 items-center justify-center rounded-full bg-primary/10 text-primary">
              <UserRound aria-hidden="true" className="size-5" />
            </span>
            <div>
              <p className="text-sm font-medium">{name || "未填写名称"}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                演示账号 · 资料仅在本页预览
              </p>
            </div>
          </div>
          <FieldGroup>
            <TextField
              id="profile-name"
              label="显示名称"
              value={name}
              onChange={setName}
              disabled={readOnly}
            />
            <ChoiceField
              id="profile-language"
              label="工作台语言"
              value={language}
              options={["简体中文"]}
              onChange={setLanguage}
              disabled={readOnly}
            />
            <DraftNotice changed={name !== "创作负责人"} />
            <Button disabled>保存服务待接入</Button>
          </FieldGroup>
        </Panel>
        <Panel title="访问与会话">
          <ShieldCheck
            aria-hidden="true"
            className="mb-4 size-6 text-muted-foreground"
          />
          <p className="text-sm font-medium">登录会话待接入</p>
          <p className="mt-2 text-xs leading-6 text-muted-foreground">
            当前资料来自演示页面。实际身份、项目权限和账号保存由账号服务提供。
          </p>
        </Panel>
      </div>
    </div>
  );
}
