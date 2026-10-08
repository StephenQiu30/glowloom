"use client";

import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { demoNotice } from "@/components/feedback/demo-notice";
import { StatusBadge } from "@/components/feedback/status-badge";
import { useState } from "react";
import Link from "next/link";
import { ArrowUpRight, RefreshCw, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
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
import { Progress } from "@/components/ui/progress";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { screenHref } from "@/components/layout/routes";

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
      <div className="my-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {[
          ["失败率 · 15 分钟", "3.0%", "阈值 5%"],
          ["结果未知", "0", "自动对账中"],
          ["待人工核对", resolved ? "0" : "1", "1 个超过 24 小时"],
          ["Outbox 积压", "4", "最旧 12 秒"],
        ].map(([title, value, note]) => (
          <Card
            variant={title === "待人工核对" && !resolved ? "danger" : "panel"}
            key={title}
          >
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
                  <TableRow
                    key={row[0]}
                    className={
                      row[0] === "MiniMax" && !resolved
                        ? "bg-destructive/10 text-destructive"
                        : undefined
                    }
                  >
                    {row.map((value, i) => (
                      <TableCell key={i} className="py-2">
                        {i === 3 ? <StatusBadge value={value} /> : value}
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
                    <TableCell className="py-3 font-mono text-xs">
                      {row[0]}
                    </TableCell>
                    <TableCell>{row[1]}</TableCell>
                    <TableCell>{row[2]}</TableCell>
                    <TableCell>
                      <Progress
                        value={Number(row[3])}
                        variant={
                          row[0] === "tts.minimax" ? "warning" : "default"
                        }
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
