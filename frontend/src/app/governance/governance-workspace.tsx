"use client";

import {
  AlertCircle,
  CalendarRange,
  Check,
  ChevronRight,
  Clock3,
  FileCheck2,
  History,
  LoaderCircle,
  PencilLine,
  Plus,
  ShieldCheck,
  ShieldX,
} from "lucide-react";
import Link from "next/link";
import { type FormEvent, useRef, useState } from "react";

import {
  ConsentFormDialog,
  type ConsentFormValue,
} from "@/app/governance/consent-form-dialog";
import {
  AuditTrail,
  type AuditFilters,
} from "@/app/governance/audit-trail";
import { LayoutContainer } from "@/components/layout/layout-container";
import { StudioShell } from "@/components/studio/studio-shell";
import { MetricGroup } from "@/components/studio/metric-group";
import { PageHeader } from "@/components/studio/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
  useAuditEventsQuery,
  useConsentQuery,
  useConsentsQuery,
  useCreateConsentMutation,
  useMeQuery,
  useMediaVersionsQuery,
  useReviseConsentMutation,
  useRevokeConsentMutation,
} from "@/lib/server-state";

const statusLabels: Record<API.ConsentStatus, string> = {
  active: "有效",
  expired: "已到期",
  revoked: "已撤销",
};

const actionLabels: Record<API.ConsentRevisionResponse["action"], string> = {
  register: "首次登记",
  update: "范围修订",
  revoke: "撤销授权",
};

const subjectTypeLabels: Record<API.SubjectType, string> = {
  SCRIPT_VERSION: "剧本版本",
  ASSET_VERSION: "资产版本",
  SHOT_SPEC_VERSION: "分镜规格版本",
  CANDIDATE: "生成候选",
  MEDIA_VERSION: "媒体版本",
  TIMELINE_VERSION: "时间线版本",
  DELIVERY: "交付版本",
};

const termLabels: Record<string, string> = {
  copyright: "著作权",
  image: "形象",
  voice: "声音",
  ai_short_drama_generation: "AI 漫剧生成",
  public_distribution: "公开分发",
  internal_demo: "内部演示",
  lanverse_preview: "平台预览",
  lanverse_download: "受控下载",
  public_export: "公开导出",
  CN: "中国大陆",
};

function statusBadge(status: API.ConsentStatus) {
  if (status === "active") {
    return "border-border bg-muted text-foreground";
  }
  if (status === "revoked") {
    return "border-destructive/30 bg-destructive/10 text-destructive";
  }
  return "border-border bg-muted text-muted-foreground";
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "short",
    day: "numeric",
    timeZone: "UTC",
  }).format(new Date(value));
}

function shortId(value: string): string {
  return `${value.slice(0, 8)}…${value.slice(-4)}`;
}

function ScopeTerms({ values }: { values: string[] }) {
  return (
    <div className="flex flex-wrap gap-2">
      {values.map((value) => (
        <Badge key={value} variant="outline">
          {termLabels[value] ?? value}
        </Badge>
      ))}
    </div>
  );
}

function LoadingWorkspace() {
  return (
    <div className="grid min-h-[520px] place-items-center">
      <div className="text-center text-sm text-muted-foreground">
        <LoaderCircle className="mx-auto mb-3 size-6 animate-spin text-foreground" aria-hidden="true" />
        正在读取授权事实…
      </div>
    </div>
  );
}

function EmptyConsent({ onCreate }: { onCreate: () => void }) {
  return (
    <section
      aria-labelledby="governance-empty-title"
      className="grid min-h-80 place-items-center bg-muted/30 px-6 py-16 text-center"
    >
      <div className="max-w-sm">
        <span className="mx-auto grid size-11 place-items-center rounded-full bg-background text-muted-foreground">
          <ShieldCheck className="size-5" aria-hidden="true" />
        </span>
        <h2 className="mt-5 text-lg font-semibold tracking-tight" id="governance-empty-title">还没有授权记录</h2>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          从一个固定媒体或剧本版本开始，登记用途、地域、有效期与证明。
        </p>
        <Button className="mt-5" onClick={onCreate}>
          <Plus aria-hidden="true" />新建授权
        </Button>
      </div>
    </section>
  );
}

function ConsentList({
  consents,
  onSelect,
  selectedId,
}: {
  consents: API.ConsentSummaryResponse[];
  onSelect: (id: string) => void;
  selectedId?: string;
}) {
  return (
    <Card className="min-w-0 gap-0 bg-muted/30 py-0">
      <CardHeader className="py-4">
        <CardTitle>授权记录</CardTitle>
        <CardDescription>{consents.length} 项 Workspace 事实</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-1 p-2">
        {consents.map((consent) => (
          <button
            aria-pressed={selectedId === consent.id}
            className={`grid min-w-0 gap-2 overflow-hidden rounded-md border-0 p-3 text-left transition ${
              selectedId === consent.id
                ? "bg-background"
                : "hover:bg-background/70"
            }`}
            key={consent.id}
            onClick={() => onSelect(consent.id)}
            type="button"
          >
            <span className="flex min-w-0 items-start justify-between gap-3">
              <span className="min-w-0 truncate text-sm font-medium">
                {consent.subject_identity.reference}
              </span>
              <Badge className={`${statusBadge(consent.status)} shrink-0`} variant="outline">
                {statusLabels[consent.status]}
              </Badge>
            </span>
            <span className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
              <span>{subjectTypeLabels[consent.current_revision.scope.subject_type]}</span>
              <span className="flex items-center gap-1">
                r{consent.revision}<ChevronRight className="size-3" aria-hidden="true" />
              </span>
            </span>
          </button>
        ))}
      </CardContent>
    </Card>
  );
}

function RevokeDialog({
  consent,
  isSubmitting,
  onSubmit,
}: {
  consent: API.ConsentDetailResponse;
  isSubmitting: boolean;
  onSubmit: (reason: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    try {
      await onSubmit(String(form.get("reason")));
      setOpen(false);
    } catch {
      // The parent keeps the actionable API error visible on the workspace.
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button disabled={consent.status === "revoked"} variant="destructive">
          <ShieldX aria-hidden="true" />撤销授权
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>撤销当前授权</DialogTitle>
          <DialogDescription>
            撤销会追加 revision r{consent.revision + 1}，并立即阻止新的生成与交付。
          </DialogDescription>
        </DialogHeader>
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="revokeReason">撤销原因</Label>
              <Textarea className="min-h-24 resize-none" id="revokeReason" name="reason" required />
            </div>
            <Alert variant="destructive">
              <AlertCircle aria-hidden="true" />
              <AlertTitle>历史不会被删除</AlertTitle>
              <AlertDescription>
                已有任务、费用和历史交付继续保留，供后续风险处置追溯。
              </AlertDescription>
            </Alert>
            <DialogFooter>
              <DialogClose asChild><Button type="button" variant="outline">取消</Button></DialogClose>
              <Button disabled={isSubmitting} type="submit" variant="destructive">
                {isSubmitting ? "撤销中…" : "确认撤销"}
              </Button>
            </DialogFooter>
          </form>
      </DialogContent>
    </Dialog>
  );
}

function ConsentDetail({
  consent,
  isRevoking,
  onEdit,
  onRevoke,
}: {
  consent: API.ConsentDetailResponse;
  isRevoking: boolean;
  onEdit: () => void;
  onRevoke: (reason: string) => Promise<void>;
}) {
  const scope = consent.current_revision.scope;
  return (
    <div className="grid gap-5">
      <Card className="gap-0 bg-muted/30 py-0">
        <CardHeader className="py-5">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <Badge className={statusBadge(consent.status)} variant="outline">
                  {statusLabels[consent.status]}
                </Badge>
                <Badge variant="outline">revision {consent.revision}</Badge>
              </div>
              <CardTitle className="mt-3 text-xl">当前授权范围</CardTitle>
              <CardDescription className="mt-1">
                主体：{consent.subject_identity.reference} · Consent {shortId(consent.id)}
              </CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button disabled={consent.status === "revoked"} onClick={onEdit} variant="outline">
                <PencilLine aria-hidden="true" />修改范围
              </Button>
              <RevokeDialog
                consent={consent}
                isSubmitting={isRevoking}
                onSubmit={onRevoke}
              />
            </div>
          </div>
        </CardHeader>
        <CardContent className="grid gap-6 p-5 sm:grid-cols-2">
          <div className="grid content-start gap-4">
            <div>
            <p className="text-xs font-medium tracking-wide text-muted-foreground">固定对象</p>
              <p className="mt-2 text-sm font-medium">{subjectTypeLabels[scope.subject_type]}</p>
              <p className="mt-1 font-mono text-xs text-muted-foreground">{scope.subject_id}</p>
            </div>
            <div>
            <p className="text-xs font-medium tracking-wide text-muted-foreground">权利类型</p>
              <div className="mt-2"><ScopeTerms values={scope.rights_types} /></div>
            </div>
            <div>
            <p className="text-xs font-medium tracking-wide text-muted-foreground">用途与渠道</p>
              <div className="mt-2"><ScopeTerms values={[...scope.authorized_purposes, ...scope.channels]} /></div>
            </div>
          </div>
          <div className="grid content-start gap-4 bg-background p-4">
            <div className="flex items-start gap-3">
              <CalendarRange className="mt-0.5 size-4 text-foreground" aria-hidden="true" />
              <div><p className="text-sm font-medium">有效期</p><p className="mt-1 text-xs text-muted-foreground">{formatDate(scope.valid_from)} — {formatDate(scope.valid_to)}</p></div>
            </div>
            <div className="flex items-start gap-3">
              <FileCheck2 className="mt-0.5 size-4 text-foreground" aria-hidden="true" />
              <div><p className="text-sm font-medium">证明媒体</p><p className="mt-1 font-mono text-xs text-muted-foreground">{consent.current_revision.proof_media_version_ids.map(shortId).join("、")}</p></div>
            </div>
            <div className="flex items-start gap-3">
              <Clock3 className="mt-0.5 size-4 text-foreground" aria-hidden="true" />
              <div><p className="text-sm font-medium">地域</p><div className="mt-2"><ScopeTerms values={scope.regions} /></div></div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="gap-0 bg-muted/30 py-0">
        <CardHeader className="py-4">
          <div className="flex items-center gap-2"><History className="size-4 text-foreground" aria-hidden="true" /><CardTitle>修订历史</CardTitle></div>
          <CardDescription>所有事实只追加，旧范围与撤销原因始终可追溯。</CardDescription>
        </CardHeader>
        <CardContent className="divide-y divide-border p-0">
          {[...consent.revisions].reverse().map((revision) => (
            <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-4 px-5 py-4" key={revision.id}>
              <span className={`mt-0.5 grid size-8 place-items-center ${revision.action === "revoke" ? "bg-destructive/10 text-destructive" : "bg-muted text-foreground"}`}>
                {revision.action === "revoke" ? <ShieldX className="size-4" aria-hidden="true" /> : <Check className="size-4" aria-hidden="true" />}
              </span>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center justify-between gap-2"><p className="text-sm font-medium">r{revision.revision_no} · {actionLabels[revision.action]}</p><time className="text-xs text-muted-foreground" dateTime={revision.created_at}>{formatDate(revision.created_at)}</time></div>
                <p className="mt-1 text-sm text-muted-foreground">{revision.reason}</p>
                <p className="mt-2 text-xs text-muted-foreground">{revision.scope.authorized_purposes.length} 项用途 · {revision.proof_media_version_ids.length} 份证明</p>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

export type GovernancePrefill = {
  proofMediaVersionId?: string;
  subjectId: string;
  subjectType: "ASSET_VERSION";
};

export function GovernanceWorkspace({
  prefill,
}: {
  prefill?: GovernancePrefill;
} = {}) {
  const sessionState = useAuthSessionState();
  const authenticated = sessionState === "authenticated";
  const me = useMeQuery(undefined, { skip: !authenticated });
  const workspaceId = me.data?.workspace.id;
  const auditVisible = me.data?.workspace.role === "owner";
  const consents = useConsentsQuery(workspaceId ?? "", { skip: !workspaceId });
  const media = useMediaVersionsQuery(workspaceId ?? "", { skip: !workspaceId });
  const [auditFilters, setAuditFilters] = useState<AuditFilters>({});
  const audit = useAuditEventsQuery(
    {
      workspaceId: workspaceId ?? "",
      ...auditFilters,
    },
    { skip: !workspaceId || !auditVisible },
  );
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const items = consents.data?.items ?? [];
  const effectiveSelectedId = items.some((item) => item.id === selectedId)
    ? selectedId ?? undefined
    : items[0]?.id;
  const detail = useConsentQuery(effectiveSelectedId ?? "", {
    skip: !effectiveSelectedId,
  });
  const [createConsent, createState] = useCreateConsentMutation();
  const [reviseConsent, reviseState] = useReviseConsentMutation();
  const [revokeConsent, revokeState] = useRevokeConsentMutation();
  const [createOpen, setCreateOpen] = useState(Boolean(prefill));
  const [reviseOpen, setReviseOpen] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const createKey = useRef<string | null>(null);

  function showNotice(message: string) {
    setNotice(message);
    window.setTimeout(() => setNotice(null), 3200);
  }

  function nextIdempotencyKey(): string {
    if (createKey.current) return createKey.current;
    createKey.current =
      typeof crypto.randomUUID === "function"
        ? crypto.randomUUID()
        : `consent-${Date.now()}`;
    return createKey.current;
  }

  async function submitCreate(value: ConsentFormValue) {
    if (!workspaceId) return;
    setActionError(null);
    try {
      const created = await createConsent({
        workspace_id: workspaceId,
        subject_identity: value.subjectIdentity,
        scope: value.scope,
        proof_media_version_ids: value.proofMediaVersionIds,
        reason: value.reason,
        idempotency_key: nextIdempotencyKey(),
      }).unwrap();
      createKey.current = null;
      setSelectedId(created.id);
      setCreateOpen(false);
      showNotice("授权已登记，revision 1 已保存。");
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function submitRevision(value: ConsentFormValue) {
    if (!detail.data) return;
    setActionError(null);
    try {
      await reviseConsent({
        consentId: detail.data.id,
        body: {
          expected_revision: detail.data.revision,
          scope: value.scope,
          proof_media_version_ids: value.proofMediaVersionIds,
          reason: value.reason,
        },
      }).unwrap();
      setReviseOpen(false);
      showNotice(`授权新修订 r${detail.data.revision + 1} 已保存。`);
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
    }
  }

  async function submitRevoke(reason: string) {
    if (!detail.data) return;
    setActionError(null);
    try {
      await revokeConsent({
        consentId: detail.data.id,
        body: { expected_revision: detail.data.revision, reason },
      }).unwrap();
      showNotice("授权已撤销，新的生成与交付已被阻止。");
    } catch (error: unknown) {
      setActionError(appApiErrorMessage(error));
      throw error;
    }
  }

  const activeCount = items.filter((item) => item.status === "active").length;
  const blockedCount = items.filter((item) => item.status !== "active").length;
  const mediaVersions = (media.data?.items ?? []).filter(
    (item) => item.probe_status !== "quarantined",
  );

  return (
    <StudioShell
      active="governance"
    >
      {notice ? <div className="pointer-events-none fixed top-24 right-6 z-50 flex items-center gap-2 bg-foreground px-4 py-3 text-sm text-background" role="status"><Check className="size-4" aria-hidden="true" />{notice}</div> : null}
      <LayoutContainer className="py-12 md:py-14">
        <PageHeader
          actions={authenticated ? (
            <Button onClick={() => setCreateOpen(true)}>
              <Plus aria-hidden="true" />新建授权
            </Button>
          ) : (
            <Button asChild><Link href="/login">登录后管理</Link></Button>
          )}
          description="登记固定版本的用途、地域、期限与证明，让生成和交付门禁基于可追溯事实。"
          eyebrow={me.data?.workspace.name ?? "授权与审计"}
          title="授权治理"
        />

        {sessionState === "checking" || (authenticated && me.isLoading) ? <LoadingWorkspace /> : null}
        {sessionState === "anonymous" ? (
          <Alert className="mt-8 border-0 bg-muted/50"><AlertCircle aria-hidden="true" /><AlertTitle>需要登录</AlertTitle><AlertDescription>授权记录受 Workspace 隔离保护。<Link className="ml-1 font-medium underline" href="/login">前往登录</Link></AlertDescription></Alert>
        ) : null}
        {authenticated && (me.error || consents.error || media.error) ? (
          <Alert className="mt-8 border-0 bg-destructive/10" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>授权事实暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(me.error ?? consents.error ?? media.error)}</AlertDescription></Alert>
        ) : null}

        {workspaceId && !me.isLoading ? (
          <>
            <MetricGroup
              className="mt-8"
              columns={3}
              items={[
                { label: "授权记录", value: consents.data?.total ?? 0 },
                { label: "当前有效", value: activeCount },
                { label: "已阻断 / 到期", value: blockedCount },
              ]}
              label="授权状态摘要"
            />

            {actionError ? <Alert className="mt-6 border-0 bg-destructive/10" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>操作未完成</AlertTitle><AlertDescription>{actionError}</AlertDescription></Alert> : null}
            {mediaVersions.length === 0 ? <Alert className="mt-6 border-0 bg-muted/50"><FileCheck2 aria-hidden="true" /><AlertTitle>缺少可用证明媒体</AlertTitle><AlertDescription>先在资产或媒体流程完成一次私有上传，才能登记授权证明。</AlertDescription></Alert> : null}

            {consents.isLoading ? (
              <div className="mt-8 grid min-h-64 place-items-center bg-muted/30 text-sm text-muted-foreground">
                <div className="text-center"><LoaderCircle className="mx-auto mb-3 size-5 animate-spin" aria-hidden="true" />正在读取授权列表…</div>
              </div>
            ) : items.length === 0 ? (
              <div className="mt-8"><EmptyConsent onCreate={() => setCreateOpen(true)} /></div>
            ) : (
              <div className="mt-8 grid items-start gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
                <ConsentList consents={items} onSelect={setSelectedId} selectedId={effectiveSelectedId} />
                {detail.isLoading ? <LoadingWorkspace /> : detail.data ? <ConsentDetail consent={detail.data} isRevoking={revokeState.isLoading} onEdit={() => setReviseOpen(true)} onRevoke={submitRevoke} /> : null}
              </div>
            )}
            {auditVisible ? (
              audit.error ? (
                <Alert className="mt-8 border-0 bg-destructive/10" variant="destructive">
                  <AlertCircle aria-hidden="true" />
                  <AlertTitle>审计事实暂时无法读取</AlertTitle>
                  <AlertDescription>{appApiErrorMessage(audit.error)}</AlertDescription>
                </Alert>
              ) : (
                <AuditTrail
                  events={audit.data?.items ?? []}
                  loading={audit.isLoading || audit.isFetching}
                  total={audit.data?.total ?? 0}
                  onFilter={setAuditFilters}
                />
              )
            ) : null}
          </>
        ) : null}
      </LayoutContainer>

      {workspaceId ? (
        <ConsentFormDialog
          initialProofMediaVersionId={prefill?.proofMediaVersionId}
          initialSubjectId={prefill?.subjectId}
          initialSubjectType={prefill?.subjectType}
          isSubmitting={createState.isLoading}
          mediaVersions={mediaVersions}
          mode="create"
          onDirty={() => { createKey.current = null; }}
          onOpenChange={setCreateOpen}
          onSubmit={submitCreate}
          open={createOpen}
        />
      ) : null}
      {detail.data ? (
        <ConsentFormDialog
          key={`revise-${detail.data.current_revision_id}`}
          initialConsent={detail.data}
          isSubmitting={reviseState.isLoading}
          mediaVersions={mediaVersions}
          mode="revise"
          onOpenChange={setReviseOpen}
          onSubmit={submitRevision}
          open={reviseOpen}
        />
      ) : null}
    </StudioShell>
  );
}
