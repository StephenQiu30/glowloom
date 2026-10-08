"use client";
import { cn } from "cn";

import Link from "next/link";
import { useState } from "react";
import { Check, TriangleAlert, Waves } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Field,
  FieldGroup,
  FieldLabel,
  FieldError,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Brand, demoNotice } from "./shared";

const colors = [
  ["background", "#0A0A0A", "画布 / 页面"],
  ["surface-1", "#111111", "导航 / 侧栏"],
  ["surface-2", "#171717", "卡片 / 节点"],
  ["surface-3", "#1F1F1F", "浮层 / 悬停"],
  ["surface-4", "#262626", "选中背景"],
  ["border", "#333333", "只用于控件"],
  ["primary", "#EDEDED", "正文 / 主按钮"],
  ["muted-foreground", "#A1A1A1", "次要文本"],
  ["subtle-foreground", "#8F8F8F", "说明 / 占位"],
  ["ring", "#737373", "焦点环 / 连线"],
  ["warning", "#FFB224", "生成中 / 过期"],
  ["destructive", "#E5484D", "失败 / 危险"],
];
export function DesignSystemPage() {
  const [view, setView] = useState("故事板");
  return (
    <main className="mx-auto flex w-full max-w-[1440px] min-w-0 flex-col gap-10 px-6 py-12 lg:px-20 lg:py-16">
      <header>
        <Brand wordmark />
        <h1 className="mt-3 text-[40px] font-semibold">暗色设计语言</h1>
        <div className="mt-3 flex flex-wrap justify-between gap-5">
          <p className="max-w-3xl text-sm leading-7 text-muted-foreground">
            默认暗色，遵循 Vercel / Next.js
            的黑白质感：主操作用前景白，选中用前景白与更深背景。不用蓝紫渐变、内容大面积描边。留白与对齐分组，不制造边框；控件保留描边、轻阴影与焦点环。
          </p>
          <div className="flex items-center gap-2">
            {["Geist Sans / Mono", "shadcn/ui · Radix", "Lucide"].map(
              (value) => (
                <Badge key={value} variant="secondary">
                  {value}
                </Badge>
              ),
            )}
          </div>
        </div>
      </header>
      <section>
        <h2 className="mb-5 text-lg font-medium">
          颜色{" "}
          <span className="ml-3 text-xs font-normal text-muted-foreground">
            Vercel / Geist 中性色阶，无品牌色；只保留警示与错误两个状态色
          </span>
        </h2>
        <div className="grid grid-cols-2 gap-x-3 gap-y-5 sm:grid-cols-3 lg:grid-cols-6">
          {colors.map(([token, hex, description]) => (
            <div key={token}>
              <div
                className="mb-2 h-20 rounded-lg"
                style={{ backgroundColor: `var(--${token})` }}
              />
              <p className="text-xs">{token}</p>
              <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                {hex} · {description}
              </p>
            </div>
          ))}
        </div>
      </section>
      <div className="grid gap-5 lg:grid-cols-2">
        <Card variant="panel">
          <CardHeader>
            <CardTitle>字体层级</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {[
              [32, 600, "雾港来信"],
              [20, 600, "第 3 集 · 分镜"],
              [15, 500, "页面标题与节点标题"],
              [14, 400, "正文、表单与说明，中文回退到 PingFang SC。"],
              [12, 500, "徽标、元信息、标签"],
            ].map(([size, weight, text]) => (
              <div key={text} className="flex items-center gap-6">
                <span className="w-16 shrink-0 font-mono text-[10px] text-muted-foreground">
                  {size} / {weight}
                </span>
                <span
                  style={{ fontSize: Number(size), fontWeight: Number(weight) }}
                >
                  {text}
                </span>
              </div>
            ))}
            <div className="flex gap-6 font-mono text-xs">
              <span className="w-16 text-muted-foreground">Mono 12</span>
              S03-02-04 · 4.0s · 9:16 · 1080p
            </div>
          </CardContent>
        </Card>
        <Card variant="panel">
          <CardHeader>
            <CardTitle>层级与圆角</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-3">
              {["卡片", "浮层", "画布选中"].map((label, index) => (
                <div
                  key={label}
                  className="flex h-28 flex-col justify-between rounded-xl p-3"
                  style={{
                    backgroundColor: `var(--surface-${index + 2})`,
                    outline:
                      index === 2 ? "1px solid var(--primary)" : undefined,
                  }}
                >
                  <span className="text-xs">{label}</span>
                  <span className="text-[10px] text-muted-foreground">
                    {index === 0
                      ? "surface-2 / 12px 无描边"
                      : index === 1
                        ? "surface-3 / 16px 轻阴影"
                        : "用白色强调选中"}
                  </span>
                </div>
              ))}
            </div>
            <div className="mt-5 flex gap-3">
              {[6, 8, 12, 16, 24].map((radius) => (
                <div key={radius}>
                  <div
                    className="size-11 bg-surface-4"
                    style={{ borderRadius: radius }}
                  />
                  <p className="mt-2 text-[10px] text-muted-foreground">
                    {radius}px
                  </p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
        <Card variant="panel">
          <CardHeader>
            <CardTitle>控件</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => demoNotice("主操作")}>主要操作</Button>
              <Button variant="secondary">次要</Button>
              <Button variant="ghost">幽灵</Button>
              <Button variant="destructive">删除</Button>
              <Button variant="outline">键盘焦点</Button>
              <Button disabled>禁用</Button>
            </div>
            <FieldGroup>
              <div className="grid grid-cols-2 gap-3">
                <Field>
                  <FieldLabel htmlFor="guide-project">项目名称</FieldLabel>
                  <Input id="guide-project" defaultValue="雾港来信" />
                </Field>
                <Field data-invalid>
                  <FieldLabel htmlFor="guide-duration">镜头时长</FieldLabel>
                  <Input id="guide-duration" aria-invalid defaultValue="0" />
                  <FieldError>时长需要 1–5 秒之间</FieldError>
                </Field>
              </div>
            </FieldGroup>
            <div className="flex flex-wrap gap-4">
              <ToggleGroup
                type="single"
                value={view}
                onValueChange={(value) => value && setView(value)}
              >
                <ToggleGroupItem value="镜头表">镜头表</ToggleGroupItem>
                <ToggleGroupItem value="故事板">故事板</ToggleGroupItem>
              </ToggleGroup>
              <Field orientation="horizontal">
                <Switch id="guide-switch" defaultChecked />
                <FieldLabel htmlFor="guide-switch">自动适配画幅</FieldLabel>
              </Field>
            </div>
          </CardContent>
        </Card>
        <Card variant="panel">
          <CardHeader>
            <CardTitle>状态徽标</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <p className="text-xs text-muted-foreground">
              状态图标 + 文字，不只依赖颜色
            </p>
            <div className="flex flex-wrap gap-2">
              <Badge variant="muted">未生成</Badge>
              <Badge variant="secondary">排队中</Badge>
              <Badge variant="warning">生成中 0:42</Badge>
              <Badge variant="secondary">候选 2</Badge>
              <Badge>
                <Check />
                已选定
              </Badge>
              <Badge variant="destructive">× 失败</Badge>
              <Badge variant="warning">
                <TriangleAlert />
                过期
              </Badge>
            </div>
            <p className="text-xs text-muted-foreground">
              生成入口 · 选定与生产
            </p>
            <div className="flex flex-wrap gap-2">
              <Button>↑ 生成</Button>
              <Button variant="secondary">批量生成 12 项</Button>
              <Button variant="outline">复用已有结果</Button>
            </div>
          </CardContent>
        </Card>
      </div>
      <Button asChild variant="link" className="self-start">
        <Link href="/">返回首页</Link>
      </Button>
    </main>
  );
}
function LayoutDiagram({ collapsed = false }: { collapsed?: boolean }) {
  return (
    <div className="flex h-[380px] overflow-hidden rounded-lg bg-background">
      <div className="flex w-[72px] shrink-0 flex-col items-center gap-5 bg-surface-1 py-4">
        <span className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <Waves className="size-4" />
        </span>
        {["项目", "素材", "任务", "管理"].map((label) => (
          <span className="text-[10px] text-muted-foreground" key={label}>
            {label}
          </span>
        ))}
      </div>
      {!collapsed ? (
        <div className="hidden w-[220px] shrink-0 flex-col gap-4 bg-surface-1 px-4 py-5 text-xs sm:flex">
          <strong>雾港来信</strong>
          {[
            "概览",
            "剧本",
            "设定集",
            "第 3 集",
            "资产定稿",
            "分镜",
            "配音",
            "画布",
          ].map((label) => (
            <span
              key={label}
              className={cn(
                label === "分镜"
                  ? "rounded-md bg-surface-3 p-2"
                  : "p-2 text-muted-foreground",
              )}
            >
              {label}
            </span>
          ))}
        </div>
      ) : null}
      <div className="flex min-w-0 flex-1 flex-col p-3 sm:p-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-medium">页面内容 24px</h3>
            <p className="mt-2 text-xs text-muted-foreground">一页一主标题</p>
          </div>
          <Button size="sm">主操作</Button>
        </div>
        <span className="m-auto text-xs text-muted-foreground">
          内容 max-width 1200–1440px
        </span>
      </div>
    </div>
  );
}
export function LayoutGuidePage() {
  return (
    <main className="mx-auto flex w-full max-w-[1440px] min-w-0 flex-col gap-12 px-6 py-16 lg:px-20">
      <header>
        <p className="font-mono text-xs text-muted-foreground">
          LAYOUT · v2 · 布局规范
        </p>
        <h1 className="mt-3 text-[40px] font-semibold">页框与导航</h1>
        <p className="mt-4 max-w-4xl text-sm leading-7 text-muted-foreground">
          全站统一使用左侧主导航，无顶部栏。主导航宽 72px
          保持全局入口；进入有上下文的页面后，右侧展开 220px
          的二级导航。分清内容和导航的职责，内容从标题开始。
        </p>
      </header>
      <section>
        <h2 className="mb-5 text-lg font-medium">页面结构</h2>
        <Card variant="panel">
          <CardContent>
            <LayoutDiagram />
            <p className="mt-4 text-xs leading-6 text-muted-foreground">
              ① 主导航 72px：品牌与全局创建、项目、素材、任务、管理。② 二级导航
              220px：项目或管理上下文。③
              内容区：页面主标题、工具栏、主操作与页面内容。
            </p>
          </CardContent>
        </Card>
      </section>
      <section>
        <h2 className="mb-5 text-lg font-medium">每个页面的侧栏组合</h2>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>页面</TableHead>
              <TableHead>主导航</TableHead>
              <TableHead>二级导航</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {[
              ["首页与项目库", "项目", "无"],
              ["素材库", "素材", "个人 / 项目与文件夹"],
              ["概览、设定集、分镜、配音", "项目", "项目导航"],
              ["画布", "无常驻", "浮动工具栏与面包屑"],
              ["镜头详情", "项目", "无"],
              ["管理页面", "管理", "管理导航"],
              ["登录、注册、改密", "无", "无"],
            ].map((row) => (
              <TableRow key={row[0]}>
                {row.map((value, index) => (
                  <TableCell key={index} className="py-4">
                    {value}
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </section>
      <section>
        <h2 className="mb-5 text-lg font-medium">收起侧栏</h2>
        <div className="grid gap-5 lg:grid-cols-2">
          <Card variant="panel">
            <CardHeader>
              <CardTitle>展开 · 默认</CardTitle>
            </CardHeader>
            <CardContent>
              <LayoutDiagram />
              <p className="mt-4 text-xs text-muted-foreground">
                主轨 72px，二级展开 220px。
              </p>
            </CardContent>
          </Card>
          <Card variant="panel">
            <CardHeader>
              <CardTitle>收起</CardTitle>
            </CardHeader>
            <CardContent>
              <LayoutDiagram collapsed />
              <p className="mt-4 text-xs text-muted-foreground">
                主轨始终可见；保留上下文并扩大内容区。
              </p>
            </CardContent>
          </Card>
        </div>
      </section>
      <section>
        <h2 className="mb-4 text-lg font-medium">窄屏</h2>
        <p className="text-sm leading-7 text-muted-foreground">
          窄屏下主导航和上下文导航收入可关闭抽屉。内容区卡片按宽度重排，保留主操作与当前页面标题。画布保持独立可滚动视口。
        </p>
      </section>
      <Button asChild variant="link" className="self-start">
        <Link href="/">返回首页</Link>
      </Button>
    </main>
  );
}
