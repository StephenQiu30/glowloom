"use client";
import { useEffect, useId, useState, type ReactNode } from "react";
import Link from "next/link";
import { RefreshCw, SlidersHorizontal, Copy, Info } from "lucide-react";
import { useSession } from "@/components/auth/session";
import { PageToolbar } from "@/components/layout/page-toolbar";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
  SheetTrigger,
} from "@/components/ui/sheet";
import { toast } from "@/components/feedback/notification";
import { filterQuery, timestamp } from "./contracts";
import "./insights.css";
export function useInsightIdentity() {
  const s = useSession();
  return `${s.actor.id}:${s.session_id}:${s.actor.revision}:${s.actor.platform_role}`;
}
export function InsightsNav({
  active,
}: {
  active: "health" | "audit" | "analytics";
}) {
  const admin = useSession().actor.platform_role === "platform_admin";
  return (
    <nav className="insights-nav" aria-label="日志与分析">
      {(admin
        ? ([
            ["health", "运行诊断"],
            ["audit", "审计日志"],
            ["analytics", "数据分析"],
          ] as const)
        : ([["analytics", "我的分析"]] as const)
      ).map(([id, label]) => (
        <Button
          key={id}
          variant={id === active ? "secondary" : "ghost"}
          asChild
        >
          <Link
            href={`/${id}`}
            aria-current={active === id ? "page" : undefined}
          >
            {label}
          </Link>
        </Button>
      ))}
    </nav>
  );
}
export function InsightsHeading({
  active,
  title,
  description,
  pending,
  refresh,
  children,
}: {
  active: "health" | "audit" | "analytics";
  title: string;
  description: string;
  pending?: boolean;
  refresh: () => void;
  children?: ReactNode;
}) {
  return (
    <>
      <PageToolbar inHeader>
        <InsightsNav active={active} />
      </PageToolbar>
      <div className="insights-header">
        <div>
          <h1>{title}</h1>
          <p className="insights-subtitle">{description}</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {children}
          <Button variant="ghost" onClick={refresh} disabled={pending}>
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        </div>
      </div>
      {pending ? (
        <span className="insights-note" role="status">
          正在更新…
        </span>
      ) : null}
    </>
  );
}
export function Panel({
  title,
  description,
  children,
}: {
  title: string;
  description?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card variant="flat">
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}
export function Metric({
  label,
  value,
  context,
}: {
  label: string;
  value: string;
  context: string;
}) {
  return (
    <Card variant="flat" className="insights-metric">
      <CardHeader>
        <CardDescription>{label}</CardDescription>
      </CardHeader>
      <CardContent>
        <p className="insights-metric-value">{value}</p>
        <p className="insights-note">{context}</p>
      </CardContent>
    </Card>
  );
}
export function Notice({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <Alert>
      <Info />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{children}</AlertDescription>
    </Alert>
  );
}
export function Loading() {
  return (
    <div aria-label="正在读取" aria-busy="true" className="flex flex-col gap-5">
      <div className="insights-metrics">
        {[0, 1, 2, 3].map((n) => (
          <Skeleton key={n} className="h-32" />
        ))}
      </div>
      <Skeleton className="h-72" />
    </div>
  );
}
export function ReadState({
  error,
  retry,
}: {
  error: string;
  retry: () => void;
}) {
  return (
    <ContentState
      state="error"
      title={error === "forbidden" ? "无权读取此范围" : "暂时无法读取"}
      description={
        error === "forbidden"
          ? "权限已变化，请重新确认当前账号。"
          : "请稍后重试。"
      }
      actions={
        <Button variant="secondary" onClick={retry}>
          重试
        </Button>
      }
    />
  );
}
export function Snapshot({ at, pending }: { at: string; pending: boolean }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 60000);
    return () => clearInterval(timer);
  }, []);
  return (
    <p className="insights-note" role="status">
      快照 {timestamp(at)} · 上海时间
      {pending ? " · 正在刷新，保留上次结果" : ""}
      {now - Date.parse(at) > 15 * 60000 ? " · 快照较早，请刷新" : ""}
    </p>
  );
}
export type FilterField = {
  key: string;
  label: string;
  type?: string;
  options?: readonly (readonly [string, string])[];
};
export const dateFields: FilterField[] = [
  { key: "from", label: "开始时间（上海）", type: "datetime-local" },
  { key: "to", label: "结束时间（不含）", type: "datetime-local" },
];
type FilterControl =
  | {
      draft: Record<string, string>;
      onDraftChange: (draft: Record<string, string>) => void;
      applied: Record<string, string>;
    }
  | { draft?: never; onDraftChange?: never; applied?: never };
export function Filters({
  fields,
  maxDays,
  onApply,
  defaults = {},
  draft: controlledDraft,
  onDraftChange,
  applied: controlledApplied,
}: {
  fields: FilterField[];
  maxDays: number;
  onApply: (query: Record<string, string>) => void;
  defaults?: Record<string, string>;
} & FilterControl) {
  const [localDraft, setLocalDraft] =
    useState<Record<string, string>>(defaults);
  const [open, setOpen] = useState(false);
  const [localApplied, setLocalApplied] = useState(defaults);
  const draft = controlledDraft ?? localDraft;
  const applied = controlledApplied ?? localApplied;
  function setDraft(value: Record<string, string>) {
    if (onDraftChange) onDraftChange(value);
    else setLocalDraft(value);
  }
  function setApplied(value: Record<string, string>) {
    if (controlledApplied === undefined) setLocalApplied(value);
  }
  const appliedCount = Object.entries(applied).filter(
    ([key, value]) => value && !(key === "scope" && value === "personal"),
  ).length;
  const id = useId();
  function submit() {
    try {
      const query = filterQuery(draft, maxDays);
      onApply(query);
      setApplied(query);
      setOpen(false);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "请检查筛选条件");
    }
  }
  function reset() {
    setDraft(defaults);
    setApplied(defaults);
    onApply(defaults);
    setOpen(false);
  }
  const controls = (suffix: string) => (
    <>
      <FieldGroup className="insights-fields">
        {fields.map((f) => (
          <Field key={f.key}>
            <FieldLabel htmlFor={`${id}-${suffix}-${f.key}`}>
              {f.label}
            </FieldLabel>
            {f.options ? (
              <NativeSelect
                id={`${id}-${suffix}-${f.key}`}
                value={draft[f.key] ?? ""}
                onChange={(e) =>
                  setDraft({ ...draft, [f.key]: e.target.value })
                }
              >
                {f.options.map(([value, label]) => (
                  <NativeSelectOption value={value} key={value}>
                    {label}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            ) : (
              <Input
                id={`${id}-${suffix}-${f.key}`}
                type={f.type ?? "text"}
                value={draft[f.key] ?? ""}
                onChange={(e) =>
                  setDraft({ ...draft, [f.key]: e.target.value })
                }
              />
            )}
          </Field>
        ))}
      </FieldGroup>
      <div className="flex gap-2">
        <Button type="submit" variant="secondary">
          筛选
        </Button>
        <Button type="button" variant="ghost" onClick={reset}>
          重置
        </Button>
      </div>
    </>
  );
  return (
    <div className="insights-filters">
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger asChild>
          <Button variant="secondary">
            <SlidersHorizontal data-icon="inline-start" />
            筛选条件
          </Button>
        </SheetTrigger>
        <SheetContent className="insights-detail">
          <SheetHeader>
            <SheetTitle>筛选条件</SheetTitle>
            <SheetDescription>
              上海时间，最多{maxDays}天；填写后点击筛选。
            </SheetDescription>
          </SheetHeader>
          <form
            className="insights-detail-content"
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            {controls("sheet")}
          </form>
        </SheetContent>
      </Sheet>
      <span className="insights-note">
        {appliedCount ? `已应用 ${appliedCount} 项条件` : "默认范围 · 上海时间"}
      </span>
      <Button variant="ghost" onClick={reset}>
        重置
      </Button>
    </div>
  );
}
export function Pager({
  back,
  next,
  onBack,
  onNext,
}: {
  back: boolean;
  next: boolean;
  onBack: () => void;
  onNext: () => void;
}) {
  return (
    <div className="insights-pagination">
      <p className="insights-note">按当前快照分页</p>
      <div className="flex gap-2">
        <Button variant="ghost" disabled={!back} onClick={onBack}>
          上一页
        </Button>
        <Button variant="secondary" disabled={!next} onClick={onNext}>
          下一页
        </Button>
      </div>
    </div>
  );
}
export function CopyValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="insights-definition">
      <div className="min-w-0">
        <dt>{label}</dt>
        <dd className="insights-code">{value || "—"}</dd>
      </div>
      {value ? (
        <Button
          size="icon"
          variant="ghost"
          aria-label={`复制${label}`}
          onClick={() =>
            navigator.clipboard
              .writeText(value)
              .then(() => toast.success("已复制"))
              .catch(() => toast.error("复制失败，请手动选择文本"))
          }
        >
          <Copy />
        </Button>
      ) : null}
    </div>
  );
}
