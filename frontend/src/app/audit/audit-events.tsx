"use client";
import { ContentState } from "@/components/layout/content-state";
import { useEffect, useState } from "react";
import { getPlatformSecurityEvents } from "@/api/identity";
import { SessionGate } from "@/components/auth/session";
import { PageLayout } from "@/components/layout/page-layout";
import { PageHeading } from "@/components/layout/page-heading";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";

function Events() {
  const [data, setData] = useState<API.SecurityEventPageView | null>(null);
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    getPlatformSecurityEvents({ signal: controller.signal })
      .then((value) => {
        if (!Array.isArray(value.items)) throw new Error("invalid_receipt");
        if (!controller.signal.aborted) {
          setData(value);
          setError(false);
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) setError(true);
      });
    return () => controller.abort();
  }, [attempt]);
  return (
    <PageLayout screen="audit">
      <PageHeading
        title="审计日志"
        actions={
          <Button
            variant="secondary"
            onClick={() => {
              setError(false);
              setData(null);
              setAttempt((n) => n + 1);
            }}
          >
            刷新
          </Button>
        }
      />
      {error ? (
        <ContentState
          state="error"
          title="审计日志暂时无法读取"
          description="请检查访问权限后重试。"
          actions={
            <Button
              onClick={() => {
                setError(false);
                setData(null);
                setAttempt((n) => n + 1);
              }}
            >
              重试
            </Button>
          }
        />
      ) : !data ? (
        <ContentState state="loading" title="正在读取审计日志" />
      ) : !data.items.length ? (
        <ContentState title="暂无审计记录" />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>时间</TableHead>
              <TableHead>操作</TableHead>
              <TableHead>结果</TableHead>
              <TableHead>说明</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.items.map((item) => (
              <TableRow key={item.id}>
                <TableCell className="whitespace-nowrap">
                  {new Date(item.created_at).toLocaleString("zh-CN")}
                </TableCell>
                <TableCell>{item.action}</TableCell>
                <TableCell>{item.result}</TableCell>
                <TableCell>{item.reason || "—"}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </PageLayout>
  );
}
export function AuditEvents() {
  return (
    <SessionGate admin>
      <Events />
    </SessionGate>
  );
}
