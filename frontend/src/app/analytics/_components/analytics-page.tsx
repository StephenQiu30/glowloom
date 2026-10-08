"use client";

import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { Choice } from "@/components/forms/choice";
import { demoNotice } from "@/components/feedback/demo-notice";
import Link from "next/link";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart";
import { useState } from "react";
import { Check, TriangleAlert, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Progress } from "@/components/ui/progress";
import { screenHref } from "@/components/layout/routes";

const bars = [35, 46, 32, 61, 70, 52, 42, 80, 94, 74, 68, 86, 75, 61];

const progress = [
  [100, 100, 100, 100, 100, 100],
  [100, 100, 100, 100, 92, 100],
  [100, 100, 100, 74, 38, 28],
  [100, 82, 40, 12, 2, 0],
  [100, 41, 2, 0, 0, 0],
  [100, 0, 2, 0, 0, 0],
  [58, 0, 0, 0, 2, 0],
];

export function AnalyticsPage() {
  const [range, setRange] = useState("14 天");
  const [episode, setEpisode] = useState("分集：全部");
  const datedBars = bars.map((tasks, index) => ({
    tasks,
    date: index < 7 ? `9/${24 + index}` : `10/${index - 6}`,
  }));
  const shownBars = range === "7 天" ? datedBars.slice(-7) : datedBars;
  return (
    <ProductShell screen="analytics" contextual>
      <PageHeading
        title="概览"
        description="生产进度、生成质量与待处理事项，以下为示意数据。"
      >
        <ToggleGroup
          type="single"
          value={range}
          onValueChange={(value) => value && setRange(value)}
          aria-label="分析时间范围"
        >
          {["7 天", "14 天", "全部"].map((value) => (
            <ToggleGroupItem key={value} value={value}>
              {value}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        <Choice
          value={episode}
          onChange={setEpisode}
          options={["分集：全部", "分集：第 1 集", "分集：第 3 集"]}
          label="分析分集"
        />
      </PageHeading>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {[
          ["镜头完成", "86", "/ 142"],
          ["生成任务 · 14 天", range === "7 天" ? "426" : "821", ""],
          ["生成成功率", "92.4%", ""],
          ["视频平均耗时", "1:48", ""],
        ].map(([title, value, suffix], index) => (
          <Card variant="panel" key={title}>
            <CardHeader>
              <CardDescription>{title}</CardDescription>
              <CardTitle>
                <span className="font-mono text-[32px] font-medium">
                  {value}
                </span>
                <span className="ml-2 text-sm font-normal text-muted-foreground">
                  {suffix}
                </span>
              </CardTitle>
            </CardHeader>
            <CardContent>
              {index === 0 ? (
                <Progress
                  value={60.5}
                  variant="chart"
                  aria-label="镜头完成进度"
                />
              ) : (
                <p className="text-xs text-muted-foreground">
                  {index === 1
                    ? "较前 14 天 +18%"
                    : index === 2
                      ? "失败 51 · 待核对 11"
                      : "P95 3:12"}
                </p>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
      <div className="mt-4 grid gap-4 xl:grid-cols-2">
        <Card variant="panel">
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>每日生成任务</CardTitle>
              <Button
                variant="link"
                size="xs"
                onClick={() => demoNotice("每日任务数据已导出")}
              >
                数量趋势
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <ChartContainer
              config={{ tasks: { label: "生成任务", color: "var(--chart-1)" } }}
              className="h-[200px] w-full"
            >
              <BarChart accessibilityLayer data={shownBars} barCategoryGap={2}>
                <CartesianGrid vertical={false} />
                <YAxis
                  ticks={[0, 50, 100]}
                  domain={[0, 100]}
                  tickLine={false}
                  axisLine={false}
                  width={30}
                />
                <XAxis
                  dataKey="date"
                  tickLine={false}
                  axisLine={false}
                  ticks={
                    shownBars.length === 14
                      ? ["9/24", "9/30", "10/7"]
                      : undefined
                  }
                  minTickGap={20}
                />
                <ChartTooltip content={<ChartTooltipContent />} />
                <Bar
                  dataKey="tasks"
                  fill="var(--color-tasks)"
                  radius={[3, 3, 0, 0]}
                  isAnimationActive={false}
                />
              </BarChart>
            </ChartContainer>
          </CardContent>
        </Card>
        <Card variant="panel">
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>按模型的生成结果</CardTitle>
              <span className="flex items-center gap-3 text-[10px] text-muted-foreground">
                <span className="text-chart-1">■ 成功</span>
                <span className="text-warning">■ 待核对</span>
                <span className="text-destructive">■ 失败</span>
              </span>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {[
              ["[图像模型]", 94, 4],
              ["[视频模型 A]", 88, 9],
              ["[视频模型 B]", 91, 8],
              ["[配音模型]", 98, 2],
            ].map(([name, success, failed]) => (
              <div key={name}>
                <div className="mb-2 flex justify-between text-xs">
                  <span>{name}</span>
                  <span className="text-[10px] text-muted-foreground">
                    成功 {success}% · 失败 {failed}%
                  </span>
                </div>
                <div
                  className="flex h-3 overflow-hidden rounded-sm"
                  role="img"
                  aria-label={`${name} 成功 ${success}% 失败 ${failed}%`}
                >
                  <span
                    className="bg-chart-1"
                    style={{ width: `${success}%` }}
                  />
                  <span className="flex-1 bg-warning" />
                  <span
                    className="bg-destructive"
                    style={{ width: `${failed}%` }}
                  />
                </div>
              </div>
            ))}
          </CardContent>
        </Card>
        <Card variant="panel">
          <CardHeader>
            <div className="flex justify-between">
              <CardTitle>分集 × 阶段进度</CardTitle>
              <span className="text-[10px] text-muted-foreground">
                0% ░ ▒ ▓ 100%
              </span>
            </div>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-[50px_repeat(6,minmax(0,1fr))] gap-1 text-center text-[10px]">
              <span />
              {["剧本", "资产", "分镜", "关键帧", "配音", "视频"].map(
                (label) => (
                  <span key={label} className="pb-2 text-muted-foreground">
                    {label}
                  </span>
                ),
              )}
              {progress.map((row, index) => (
                <ProgressRow key={index} index={index} values={row} />
              ))}
            </div>
          </CardContent>
        </Card>
        <Card variant="panel">
          <CardHeader>
            <div className="flex justify-between">
              <CardTitle>待处理</CardTitle>
              <Button variant="link" size="xs" asChild>
                <Link href={screenHref("storyboard")}>全部 23 项</Link>
              </Button>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            {[
              ["失败", "S03-02-06 · 视频生成超时", "重试", "shot"],
              [
                "过期",
                "第 3 集 · 7 个镜头受“林舟”造型更新影响",
                "查看",
                "bible",
              ],
              ["待账", "S02-04-02 · 结果未知，等待对账", "核对", "health"],
              ["待选 4", "设定集 · 林舟全身参考待选定", "选定", "bible"],
            ].map(([status, text, action, target]) => (
              <div
                key={text}
                className="flex items-center gap-2 rounded-md bg-surface-2 p-2 text-xs"
              >
                <Badge
                  variant={
                    status === "失败"
                      ? "destructive"
                      : status === "过期"
                        ? "warning"
                        : "muted"
                  }
                >
                  {status === "失败" ? (
                    <X />
                  ) : status === "过期" ? (
                    <TriangleAlert />
                  ) : null}
                  {status}
                </Badge>
                <span className="flex-1 leading-5">{text}</span>
                <Button variant="ghost" size="xs" asChild>
                  <Link href={screenHref(target)}>{action}</Link>
                </Button>
              </div>
            ))}
          </CardContent>
        </Card>
      </div>
    </ProductShell>
  );
}

function ProgressRow({ index, values }: { index: number; values: number[] }) {
  return (
    <>
      <span className="py-1.5 text-muted-foreground">第 {index + 1} 集</span>
      {values.map((value, col) => (
        <Link
          key={col}
          href={screenHref(
            col === 1 ? "bible" : col === 5 ? "shot" : "storyboard",
          )}
          className="flex min-h-7 items-center justify-center rounded-sm"
          style={{
            backgroundColor: `color-mix(in srgb, var(--chart-1) ${value}%, var(--surface-2))`,
            color: value > 65 ? "var(--background)" : "var(--muted-foreground)",
          }}
          aria-label={`第 ${index + 1} 集阶段 ${col + 1}，完成 ${value}%`}
        >
          {value === 100 ? <Check className="size-3" /> : `${value}%`}
        </Link>
      ))}
    </>
  );
}
