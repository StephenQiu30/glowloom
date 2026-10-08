"use client";

import { ProductShell } from "@/components/layout/product-shell";
import { PageHeading } from "@/components/layout/page-heading";
import { StatusBadge } from "@/components/feedback/status-badge";
import { demoNotice } from "@/components/feedback/demo-notice";
import { LocalDialog } from "@/components/controls/local-dialog";
import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

export function ProvidersPage() {
  const [editing, setEditing] = useState<string | null>("MiniMax");
  const [key, setKey] = useState("");
  const [group, setGroup] = useState("");
  const [tested, setTested] = useState(false);
  return (
    <ProductShell screen="providers" contextual>
      <PageHeading
        title="供应商凭据"
        description="仅管理员可见，保存后不可再次查看；浏览器只显示末 4 位与测试结果。"
      >
        <Button onClick={() => setEditing("待扩供应商")}>
          <Plus data-icon="inline-start" />
          添加供应商
        </Button>
      </PageHeading>
      <div className="grid gap-4 xl:grid-cols-2">
        {["火山方舟", "MiniMax", "[境外供应商]"].map((provider, index) => (
          <Card key={provider} variant="panel">
            <CardHeader>
              <div className="flex items-start justify-between gap-3">
                <div className="flex items-center gap-3">
                  <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-surface-2 text-muted-foreground">
                    {index === 0 ? "火" : index === 1 ? "M" : "?"}
                  </span>
                  <div>
                    <CardTitle>{provider}</CardTitle>
                    <CardDescription>
                      {index === 0
                        ? "境内 · 7 个模型"
                        : index === 1
                          ? "境内 · 3 个模型"
                          : "境外 · 2 个模型"}
                    </CardDescription>
                  </div>
                </div>
                <StatusBadge
                  value={
                    index === 0 || (tested && index === 1)
                      ? "可用"
                      : index === 1
                        ? "测试失败"
                        : "未配置"
                  }
                />
              </div>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {index < 2 ? (
                <dl className="flex flex-col gap-2 text-xs">
                  <div className="grid grid-cols-[72px_1fr] gap-3">
                    <dt className="text-muted-foreground">密钥</dt>
                    <dd className="font-mono">
                      {index === 0 ? "•••• •••• 7f3a" : "•••• •••• 02bc"}
                    </dd>
                  </div>
                  <div className="grid grid-cols-[72px_1fr] gap-3">
                    <dt className="text-muted-foreground">最近测试</dt>
                    <dd
                      className={
                        index === 1 && !tested
                          ? "text-destructive"
                          : "text-muted-foreground"
                      }
                    >
                      {index === 1 && !tested
                        ? "401 鉴权失败 · [时间]"
                        : "通过 · [时间]"}
                    </dd>
                  </div>
                </dl>
              ) : (
                <p className="py-2 text-xs text-muted-foreground">
                  配置前，该供应商的模型在生成面板中禁用。
                </p>
              )}
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  onClick={() => {
                    setEditing(provider);
                    setKey("");
                  }}
                >
                  {index === 0
                    ? "轮换密钥"
                    : index === 1
                      ? "更新密钥"
                      : "配置密钥"}
                </Button>
                {index < 2 ? (
                  <Button
                    variant="ghost"
                    onClick={() => {
                      setTested(true);
                      demoNotice("连接测试通过");
                    }}
                  >
                    {index === 0 ? "测试连接" : "重新测试"}
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
        position="upper"
        description="保存后立即测试；旧密钥在测试通过后才被替换。"
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
              placeholder=""
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="provider-group">Group ID（可选）</FieldLabel>
            <Input
              id="provider-group"
              value={group}
              onChange={(e) => setGroup(e.target.value)}
              placeholder=""
            />
          </Field>
        </FieldGroup>
      </LocalDialog>
    </ProductShell>
  );
}
