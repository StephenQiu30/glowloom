"use client";

import { Brand } from "@/components/layout/brand";
import { demoNotice } from "@/components/feedback/demo-notice";
import { Choice } from "@/components/forms/choice";
import { useState } from "react";
import { Check, TriangleAlert } from "lucide-react";
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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

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
  const [ratio, setRatio] = useState("画幅 9:16");
  return (
    <main className="mx-auto flex w-full max-w-[1440px] min-w-0 flex-col gap-14 px-6 py-12 lg:px-20 lg:py-16">
      <header>
        <Brand wordmark />
        <h1 className="mt-3 text-[40px] leading-[48px] font-semibold">
          暗色设计语言
        </h1>
        <div className="mt-3 flex flex-wrap justify-between gap-5">
          <p className="max-w-[640px] text-sm leading-6 text-muted-foreground">
            默认暗色，遵循 Vercel / Next.js
            的黑白质感：主操作用前景白，选中用前景白与更深背景。不用蓝紫强调。内容以表面层级、留白与对齐分组，不画闭合边框；控件保留描边、轻阴影与焦点环。画布沿用同一套
            token，节点与浮层按职责分组。
          </p>
          <div className="flex items-end gap-2 pb-1">
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
                className="mb-2 h-[88px] rounded-lg"
                style={{ backgroundColor: `var(--${token})` }}
              />
              <p className="text-xs">
                {token === "border"
                  ? "control-border"
                  : token === "primary"
                    ? "foreground / primary"
                    : token}
              </p>
              <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                {hex} · {description}
              </p>
            </div>
          ))}
        </div>
      </section>
      <div className="grid gap-5 lg:grid-cols-2 [&_[data-slot=card-title]]:text-lg [&_[data-slot=card]]:[--card-spacing:--spacing(6)]">
        <Card variant="panel">
          <CardHeader>
            <CardTitle>字体层级</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
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
              <Choice
                label="规范画幅"
                value={ratio}
                onChange={setRatio}
                options={["画幅 9:16", "画幅 16:9"]}
              />
              <Field orientation="horizontal" className="w-auto">
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
    </main>
  );
}
