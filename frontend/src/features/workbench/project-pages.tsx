"use client";

import Link from "next/link";
import { useState } from "react";
import { ArrowUpRight, Plus, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { FieldGroup } from "@/components/ui/field";
import { BudgetCard } from "@/features/project/budget-card";
import { projects, episodes, tasks, budget } from "./data";
import {
  PageHeading,
  Panel,
  DataTable,
  PreviewBoundary,
  EmptyMessage,
  TextField,
  ChoiceField,
  DraftNotice,
  usePreviewQuery,
} from "./poc-components";

export function ProjectBrowser() {
  const { params, set, readOnly } = usePreviewQuery();
  const q = params.get("q") ?? "";
  const view = params.get("view") === "table" ? "table" : "cards";
  const list = projects.filter(
    (p) => p.name.includes(q) || p.style.includes(q),
  );
  const [name, setName] = useState("");
  const [aspect, setAspect] = useState("16:9");
  const [style, setStyle] = useState("电影写实");
  const [preview, setPreview] = useState(false);
  return (
    <PreviewBoundary>
      <div className="space-y-9">
        <PageHeading
          eyebrow="LANVERSE / WORKSPACE"
          title="把故事，变成画面。"
          description="每个故事都有自己的世界。从剧本、设定到镜头和声音，在一个工作台中完成。"
          action={
            <Dialog onOpenChange={() => setPreview(false)}>
              <DialogTrigger asChild>
                <Button disabled={readOnly}>
                  <Plus data-icon="inline-start" />
                  新建项目预览
                </Button>
              </DialogTrigger>
              <DialogContent className="sm:max-w-lg">
                <DialogHeader>
                  <DialogTitle>开始一个新故事</DialogTitle>
                  <DialogDescription>
                    仅预览项目配置，项目创建服务尚未接入。
                  </DialogDescription>
                </DialogHeader>
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (name.trim()) setPreview(true);
                  }}
                  className="space-y-5"
                >
                  <FieldGroup>
                    <TextField
                      id="project-name"
                      label="项目名称"
                      value={name}
                      onChange={setName}
                    />
                    <ChoiceField
                      id="project-aspect"
                      label="画幅"
                      value={aspect}
                      options={["16:9", "9:16", "1:1"]}
                      onChange={setAspect}
                    />
                    <ChoiceField
                      id="project-style"
                      label="视觉风格"
                      value={style}
                      options={["电影写实", "清新动画", "科幻写实"]}
                      onChange={setStyle}
                    />
                  </FieldGroup>
                  <Button type="submit" disabled={!name.trim()}>
                    预览配置
                  </Button>
                  {preview && (
                    <p role="status" className="rounded-lg bg-muted p-4">
                      {name} · {aspect} · {style}。配置已预览，尚未创建项目。
                    </p>
                  )}
                </form>
              </DialogContent>
            </Dialog>
          }
        />
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="relative w-full sm:max-w-sm">
            <Search
              aria-hidden="true"
              className="absolute top-2.5 left-3 size-4 text-muted-foreground"
            />
            <Input
              aria-label="搜索项目"
              placeholder="搜索项目或风格…"
              value={q}
              onChange={(e) => set("q", e.target.value)}
              className="pl-9"
            />
          </div>
          <ToggleGroup
            type="single"
            value={view}
            onValueChange={(v) => {
              if (v) set("view", v);
            }}
            aria-label="项目列表视图"
          >
            <ToggleGroupItem value="cards">卡片</ToggleGroupItem>
            <ToggleGroupItem value="table">表格</ToggleGroupItem>
          </ToggleGroup>
        </div>
        {list.length === 0 ? (
          <EmptyMessage
            title="没有匹配的项目"
            action={
              <Button variant="secondary" onClick={() => set("q", "")}>
                清除搜索
              </Button>
            }
          />
        ) : view === "table" ? (
          <DataTable
            caption="项目列表"
            columns={["项目", "状态", "画幅 / 风格", "单集", "进度"]}
            rows={list.map((p) => [
              <Link
                key={p.id}
                className="font-medium hover:underline"
                href={`/projects/${p.id}`}
              >
                {p.name}
              </Link>,
              <Badge key="status" variant="secondary">
                {p.status}
              </Badge>,
              `${p.aspect} · ${p.style}`,
              `${p.episodes} 集`,
              `${p.progress}%`,
            ])}
          />
        ) : (
          <div className="grid gap-7 md:grid-cols-2 xl:grid-cols-3">
            {list.map((p) => (
              <Card
                key={p.id}
                className="gap-5 border-0 bg-transparent p-0 shadow-none ring-0"
              >
                <Link
                  href={`/projects/${p.id}`}
                  aria-label={`打开项目 ${p.name}`}
                  className="group relative flex aspect-[16/10] items-end overflow-hidden rounded-2xl bg-muted p-7 focus-visible:ring-2 focus-visible:ring-blue-500"
                >
                  <span
                    aria-hidden="true"
                    className="absolute top-5 right-6 font-mono text-6xl font-light text-muted-foreground"
                  >
                    {p.cover}
                  </span>
                  <div>
                    <p className="text-xs tracking-widest text-muted-foreground">
                      LANVERSE ORIGINAL
                    </p>
                    <p className="mt-3 text-3xl font-semibold tracking-tight transition-transform group-hover:translate-x-1">
                      {p.name}
                    </p>
                    <p className="mt-3 text-xs text-muted-foreground">
                      {p.aspect} / {p.style}
                    </p>
                  </div>
                  <ArrowUpRight
                    aria-hidden="true"
                    className="absolute right-6 bottom-7 size-5 text-muted-foreground"
                  />
                </Link>
                <CardHeader className="px-0">
                  <CardTitle>
                    <h2>
                      <Link href={`/projects/${p.id}`}>{p.name}</Link>
                    </h2>
                  </CardTitle>
                  <CardDescription>{p.subtitle}</CardDescription>
                </CardHeader>
                <CardContent className="flex items-center justify-between px-0">
                  <Badge variant="secondary">{p.status}</Badge>
                  <span className="text-xs text-muted-foreground">
                    {p.episodes} 集 · 样例进度 {p.progress}%
                  </span>
                </CardContent>
                <CardFooter className="bg-transparent px-0 py-0">
                  <div className="h-1 w-full rounded-full bg-muted">
                    <div
                      className="h-1 rounded-full bg-foreground/60"
                      style={{ width: `${p.progress}%` }}
                    />
                  </div>
                </CardFooter>
              </Card>
            ))}
          </div>
        )}
        <p className="text-xs text-muted-foreground">
          这些项目共同使用同一套创作链路样例。配置、素材和费用仅用于前端验证。
        </p>
      </div>
    </PreviewBoundary>
  );
}
export function OverviewPage({ projectId }: { projectId: string }) {
  const project = projects.find((p) => p.id === projectId)!;
  const root = `/projects/${projectId}`;
  return (
    <div className="space-y-8">
      <PageHeading
        title={project.name}
        description="故事正在成形。看看每一集的进度，再继续下一步创作。"
        action={
          <Button asChild>
            <Link href={`${root}/script`}>
              继续创作
              <ArrowUpRight data-icon="inline-end" />
            </Link>
          </Button>
        }
      />
      <div className="grid gap-6 sm:grid-cols-3">
        {[
          ["单集", "3", "已解析 2 集"],
          ["镜头", "3", "1 个镜头已有候选"],
          ["待处理", "2", "素材缺失与生成失败"],
        ].map(([title, value, note]) => (
          <Panel key={title} title={title}>
            <p className="text-4xl font-semibold tabular-nums">{value}</p>
            <p className="mt-3 text-xs text-muted-foreground">{note}</p>
          </Panel>
        ))}
      </div>
      <Panel
        title="单集制作进度"
        description="每一列对应一项创作能力。样例阶段，不代表真实制作完成度。"
      >
        <DataTable
          caption="单集阶段矩阵"
          columns={["单集", "剧本", "资产", "分镜", "视频", "配音"]}
          rows={episodes.map((e) => [
            <Link
              key={e.id}
              className="font-medium hover:underline"
              href={`${root}/episodes/${e.id}/parse`}
            >
              {e.name}
            </Link>,
            ...e.stages,
          ])}
        />
      </Panel>
      <div className="grid gap-7 xl:grid-cols-[1fr_1fr]">
        <BudgetCard budget={budget} />
        <Panel
          title="最近任务"
          description="失败和提交不确定的任务需要不同的恢复动作。"
        >
          <ul className="space-y-5">
            {tasks.slice(0, 3).map((t) => (
              <li
                key={t.id}
                className="flex items-center justify-between gap-3"
              >
                <Link
                  href={`/tasks/${t.id}`}
                  className="text-sm font-medium hover:underline"
                >
                  {t.title}
                </Link>
                <Badge
                  variant={t.state === "failed" ? "destructive" : "secondary"}
                >
                  {t.label}
                </Badge>
              </li>
            ))}
          </ul>
          <Button variant="ghost" className="mt-5" asChild>
            <Link href="/tasks">
              查看全部任务
              <ArrowUpRight data-icon="inline-end" />
            </Link>
          </Button>
        </Panel>
      </div>
    </div>
  );
}
export function CostsPage() {
  return (
    <div className="space-y-8">
      <PageHeading
        title="成本与预算"
        description="报价、预留和结算分开记录。所有金额都是样例，费用未决的任务不推定为零费用。"
      />
      <div className="grid gap-7 xl:grid-cols-2">
        <BudgetCard budget={budget} />
        <Panel
          title="阶段成本"
          description="样例预算上限 ¥300；已结算 ¥42；预留 ¥18。"
        >
          <DataTable
            caption="阶段成本"
            columns={["阶段", "已结算", "已预留"]}
            rows={[
              ["剧本解析", "¥6.00", "¥0.00"],
              ["图像与视频", "¥30.00", "¥18.00"],
              ["配音", "¥6.00", "¥0.00"],
            ]}
          />
        </Panel>
      </div>
      <Panel title="费用未决项">
        <DataTable
          caption="费用未决"
          columns={["任务", "原因", "下一步"]}
          rows={tasks
            .filter((t) => ["unknown", "failed"].includes(t.state))
            .map((t) => [
              <Link
                key={t.id}
                href={`/tasks/${t.id}`}
                className="hover:underline"
              >
                {t.title}
              </Link>,
              t.label,
              "核对供应商事实后结算",
            ])}
        />
      </Panel>
    </div>
  );
}
export function SettingsPage({ projectId }: { projectId: string }) {
  const project = projects.find((p) => p.id === projectId)!;
  const [name, setName] = useState<string>(project.name);
  const [aspect, setAspect] = useState<string>(project.aspect);
  const { readOnly } = usePreviewQuery();
  return (
    <div className="space-y-8">
      <PageHeading
        title="项目设置"
        description="画幅与风格影响后续生成。此处只编辑本页草稿，服务保存与影响分析将在后续接入。"
      />
      <Panel title="基本信息" className="max-w-2xl">
        <FieldGroup>
          <TextField
            id="settings-name"
            label="项目名称"
            value={name}
            onChange={setName}
            disabled={readOnly}
          />
          <ChoiceField
            id="settings-aspect"
            label="画幅"
            value={aspect}
            options={["16:9", "9:16", "1:1"]}
            onChange={setAspect}
            disabled={readOnly}
          />
          <DraftNotice
            changed={name !== project.name || aspect !== project.aspect}
          />
          <Button disabled>保存服务待接入</Button>
        </FieldGroup>
      </Panel>
      <Panel
        title="项目生命周期"
        description="归档、删除和恢复均需服务端权限检查。"
      >
        <Button variant="outline" disabled>
          归档服务待接入
        </Button>
      </Panel>
    </div>
  );
}

export function ImpactPage() {
  return (
    <div className="space-y-8">
      <PageHeading
        title="看清改动会影响什么。"
        description="先比较，再决定。已有产物保留，新生成单独报价，不将布局移动当作业务选定。"
      />
      <Panel title="样例变更：角色林夏 v3 → v4">
        <DataTable
          caption="变更影响"
          columns={["影响对象", "影响", "后续动作"]}
          rows={[
            ["第一集 / 镜头 01", "角色引用版本变化", "核对关键帧候选"],
            ["第一集 / 镜头 02", "角色引用版本变化", "重新核对输入"],
            ["故事板", "缩略图可能过期", "重新选定后刷新"],
            ["已生成媒体", "保留现有结果", "不自动重新收费生成"],
          ]}
        />
      </Panel>
      <Panel
        title="预计成本"
        description="受影响的生成须重新报价。当前没有真实价格与服务端影响分析。"
      >
        <Button disabled>应用变更服务待接入</Button>
      </Panel>
    </div>
  );
}
