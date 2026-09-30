"use client";

import { useState, type ReactNode } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from "@/components/ui/empty";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectGroup,
  SelectItem,
} from "@/components/ui/select";
import { Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableHeader,
  TableRow,
  TableHead,
  TableBody,
  TableCell,
} from "@/components/ui/table";
import { QuoteConfirmDialog } from "@/components/operation/quote-confirm-dialog";
import { updatePreviewQuery } from "./routes";

export function usePreviewQuery() {
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  return {
    params,
    readOnly: params.get("state") === "readonly",
    set: (key: string, value: string) => {
      const query = updatePreviewQuery(params.toString(), key, value);
      router.replace(`${pathname}${query ? `?${query}` : ""}`, {
        scroll: false,
      });
    },
  };
}
export function PageHeading({
  eyebrow = "CREATIVE WORKSPACE",
  title,
  description,
  action,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-5">
      <div>
        <p className="font-mono text-xs tracking-[0.18em] text-muted-foreground">
          {eyebrow}
        </p>
        <h1 className="mt-3 text-3xl font-semibold tracking-tight sm:text-4xl">
          {title}
        </h1>
        <p className="mt-3 max-w-2xl text-sm leading-7 text-muted-foreground">
          {description}
        </p>
      </div>
      {action}
    </div>
  );
}
export function Panel({
  title,
  description,
  children,
  className = "",
}: {
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={`border-0 bg-muted/40 shadow-none ring-0 ${className}`}>
      <CardHeader>
        <CardTitle>
          <h2>{title}</h2>
        </CardTitle>
        {description && <CardDescription>{description}</CardDescription>}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}
export function DataTable({
  caption,
  columns,
  rows,
}: {
  caption: string;
  columns: string[];
  rows: ReactNode[][];
}) {
  return (
    <Table>
      <caption className="sr-only">{caption}</caption>
      <TableHeader>
        <TableRow>
          {columns.map((c) => (
            <TableHead key={c} scope="col">
              {c}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, i) => (
          <TableRow key={i}>
            {row.map((cell, j) => (
              <TableCell key={j}>{cell}</TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
export function EmptyMessage({
  title = "暂时没有内容",
  description = "调整筛选条件，或返回项目继续准备内容。",
  action,
  page = false,
}: {
  title?: string;
  description?: string;
  action?: ReactNode;
  page?: boolean;
}) {
  return (
    <Empty className="border-0 py-16">
      <EmptyHeader>
        <EmptyTitle>{page ? <h1>{title}</h1> : <h2>{title}</h2>}</EmptyTitle>
        <EmptyDescription>{description}</EmptyDescription>
      </EmptyHeader>
      {action && <EmptyContent>{action}</EmptyContent>}
    </Empty>
  );
}
export function TextField({
  id,
  label,
  value,
  onChange,
  disabled = false,
  type = "text",
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  type?: string;
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        type={type}
      />
    </Field>
  );
}
export function ChoiceField({
  id,
  label,
  value,
  options,
  onChange,
  disabled = false,
}: {
  id: string;
  label: string;
  value: string;
  options: string[];
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            {options.map((o) => (
              <SelectItem key={o} value={o}>
                {o}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </Field>
  );
}
const states = {
  ready: "正常",
  loading: "加载",
  empty: "空",
  error: "读取错误",
  forbidden: "无权限",
  readonly: "只读",
};
export function PreviewBoundary({ children }: { children: ReactNode }) {
  const { params, set } = usePreviewQuery();
  const requestedState = params.get("state") ?? "ready";
  const state = Object.keys(states).includes(requestedState)
    ? requestedState
    : "ready";
  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg bg-muted/60 px-4 py-2">
        <p className="text-xs text-muted-foreground">
          演示模式 · 样例数据 · 服务尚未接入
        </p>
        <Select
          value={state in states ? state : "ready"}
          onValueChange={(v) => set("state", v === "ready" ? "" : v)}
        >
          <SelectTrigger aria-label="页面状态演示" className="h-8 w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {Object.entries(states).map(([v, l]) => (
                <SelectItem key={v} value={v}>
                  {l}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      {state === "loading" ? (
        <section
          aria-label="正在加载样例页面"
          aria-busy="true"
          className="space-y-6"
        >
          <h1 className="sr-only">正在加载页面</h1>
          <Skeleton className="h-10 w-56" />
          <Skeleton className="h-56 w-full" />
          <Skeleton className="h-32 w-full" />
        </section>
      ) : ["empty", "error", "forbidden"].includes(state) ? (
        <EmptyMessage
          page
          title={
            state === "empty"
              ? "当前页面还没有内容"
              : state === "error"
                ? "页面读取失败"
                : "你没有此页面的访问权限"
          }
          description={
            state === "empty"
              ? "准备剧本和素材后，可以在这里继续创作。"
              : state === "error"
                ? "这次读取没有改变任务的执行状态。可以重试查看。"
                : "请联系项目负责人；此处只演示无权限状态。"
          }
          action={
            <Button variant="secondary" onClick={() => set("state", "")}>
              {state === "error" ? "重试" : "返回正常样例"}
            </Button>
          }
        />
      ) : (
        <>
          {state === "readonly" && (
            <p role="status" className="text-sm text-muted-foreground">
              只读模式：可查看与筛选，编辑和变更预览已禁用。
            </p>
          )}
          {children}
        </>
      )}
    </div>
  );
}
export function QuotePreview({
  label = "预览生成报价",
  target = "镜头 01 · 视频",
  disabled = false,
  kind = "video",
}: {
  label?: string;
  target?: string;
  disabled?: boolean;
  kind?: "video" | "text" | "audio";
}) {
  const [quote, setQuote] = useState<null | { expires: string; id: string }>(
    null,
  );
  const open = () =>
    setQuote({
      expires: new Date(Date.now() + 300000).toISOString(),
      id: crypto.randomUUID(),
    });
  return (
    <>
      <Button onClick={open} disabled={disabled}>
        {label}
      </Button>
      {quote && (
        <QuoteConfirmDialog
          previewOnly
          open
          onOpenChange={(v) => {
            if (!v) setQuote(null);
          }}
          quote={{
            batch_id: quote.id,
            expires_at: quote.expires,
            items: [
              {
                operation_id: "00000000-0000-4000-8000-000000000001",
                target_label: target,
                model_key: `${kind === "video" ? "视频" : kind === "text" ? "文本" : "语音"}模型（样例）`,
                quote_micros: 6000000,
                region: "domestic",
                errors: [],
                quote_detail:
                  kind === "video"
                    ? {
                        unit: "per_second",
                        quantity: 5,
                        unit_price_micros: 1200000,
                        outputs: 1,
                      }
                    : {
                        unit: "per_request",
                        quantity: 1,
                        unit_price_micros: 6000000,
                        outputs: 1,
                      },
              },
            ],
            total_micros: 6000000,
            available_micros: 240000000,
            confirmable: true,
          }}
          onConfirm={() => {
            throw new Error("Preview does not submit operations");
          }}
          onRequote={open}
        />
      )}
    </>
  );
}
export function DraftNotice({ changed }: { changed: boolean }) {
  return (
    <p role="status" className="text-xs leading-6 text-muted-foreground">
      {changed
        ? "草稿已修改 · 仅本页预览，刷新后丢弃"
        : "样例版本 · 编辑仅在本页保留，刷新后丢弃"}
    </p>
  );
}
