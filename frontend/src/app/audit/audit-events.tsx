"use client";
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
          <Button variant="secondary" onClick={() => setAttempt((n) => n + 1)}>
            刷新
          </Button>
        }
      />
      {error ? (
        <p role="alert">无法读取审计日志，请检查权限后重试。</p>
      ) : !data ? (
        <p role="status">正在读取…</p>
      ) : !data.items.length ? (
        <p className="py-16 text-center text-muted-foreground">暂无记录</p>
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
