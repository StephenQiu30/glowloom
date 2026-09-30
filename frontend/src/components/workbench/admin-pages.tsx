"use client";

import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { FieldGroup } from "@/components/ui/field";
import {
  PageHeading,
  Panel,
  DataTable,
  TextField,
  ChoiceField,
  DraftNotice,
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
  const { readOnly } = usePreviewQuery();
  const [model, setModel] = useState("视频模型（样例）");
  const [price, setPrice] = useState("1.20");
  const [duration, setDuration] = useState("5");
  if (section === "models")
    return (
      <div className="space-y-8">
        <PageHeading
          eyebrow="INTERNAL / CATALOG"
          title="模型能力与价格"
          description="按协议、输入、输出和计费单位表达能力。厂商品牌不能替代模型合同，样例不表示真实可用。"
        />
        <div className="grid gap-7 xl:grid-cols-2">
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
              <p className="text-sm">
                估算：
                {Number.isFinite(Number(price)) && Number(price) >= 0
                  ? `¥${(Number(price) * Number(duration)).toFixed(2)}`
                  : "请输入有效价格"}
                （样例）
              </p>
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
      description:
        "内部账号和项目角色预览。权限最终由 Go 服务检查，当前页面不提供真实授权保护。",
      columns: ["用户", "角色", "状态", "项目范围"],
      rows: [
        ["创作负责人（样例）", "owner", "启用", "雾港来信"],
        ["分镜编辑（样例）", "editor", "启用", "雾港来信"],
        ["审阅者（样例）", "viewer", "只读", "雾港来信"],
      ],
    },
    providers: {
      title: "供应商连接",
      description: "配置与连通性状态预览。页面不读取、显示或保存任何真实凭据。",
      columns: ["供应商", "协议", "区域", "状态"],
      rows: [
        ["图像供应商（样例）", "异步任务", "待确认", "尚未接入"],
        ["视频供应商（样例）", "提交 / 轮询 / 下载", "待确认", "尚未接入"],
        ["声音供应商（样例）", "语音合成", "待确认", "尚未接入"],
      ],
    },
    audit: {
      title: "审计记录",
      description:
        "展示对象、动作和结果；真实审计由服务记录。不展示剧本正文、凭据或签名 URL。",
      columns: ["时间", "操作", "对象", "结果"],
      rows: [
        ["今天 10:42", "候选登记", "shot-01 / v3", "样例成功"],
        ["今天 10:38", "生成失败", "task-02", "参数需核对"],
        ["今天 10:35", "提交不确定", "task-03", "等待核对"],
      ],
    },
    health: {
      title: "依赖状态",
      description:
        "连通性、技术检查和业务验收分开记录。样例状态不会被解释为真实服务健康。",
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
  return (
    <div className="space-y-8">
      <PageHeading
        eyebrow="INTERNAL / OPERATIONS"
        title={screen.title}
        description={screen.description}
        action={<Badge variant="secondary">内部管理</Badge>}
      />
      <Panel title={screen.title}>
        <DataTable
          caption={screen.title}
          columns={screen.columns}
          rows={screen.rows}
        />
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
    <div className="space-y-8">
      <PageHeading
        title="账号与偏好"
        description="内部演示账号。当前未接入登录会话或账号持久化。"
      />
      <Panel title="个人资料" className="max-w-2xl">
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
    </div>
  );
}
