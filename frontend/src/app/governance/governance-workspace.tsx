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
  X,
} from "lucide-react";
import Link from "next/link";
import { type FormEvent, useRef, useState } from "react";
import { Dialog } from "radix-ui";

import {
  ConsentFormDialog,
  type ConsentFormValue,
} from "@/app/governance/consent-form-dialog";
import { StudioShell } from "@/components/studio/studio-shell";
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
import { Label } from "@/components/ui/label";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import {
  appApiErrorMessage,
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
    return "border-emerald-200 bg-emerald-50 text-emerald-700";
  }
  if (status === "revoked") {
    return "border-rose-200 bg-rose-50 text-rose-700";
  }
  return "border-amber-200 bg-amber-50 text-amber-700";
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
      <div className="text-center text-sm text-slate-500">
        <LoaderCircle className="mx-auto mb-3 size-6 animate-spin text-[#079db3]" aria-hidden="true" />
        正在读取授权事实…
      </div>
    </div>
  );
}

function EmptyConsent({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="grid min-h-[460px] place-items-center rounded-2xl border border-dashed border-slate-300 bg-white p-8 text-center">
      <div className="max-w-sm">
        <span className="mx-auto grid size-14 place-items-center rounded-2xl bg-cyan-50 text-[#079db3]">
          <ShieldCheck className="size-6" aria-hidden="true" />
        </span>
        <h2 className="mt-5 text-lg font-semibold">还没有授权记录</h2>
        <p className="mt-2 text-sm leading-6 text-slate-500">
          从一个固定媒体或剧本版本开始，登记用途、地域、有效期与证明。
        </p>
        <Button className="mt-5 bg-[#079db3] text-white hover:bg-[#078da0]" onClick={onCreate}>
          <Plus aria-hidden="true" />新建授权
        </Button>
      </div>
    </div>
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
    <Card className="min-w-0 gap-0 py-0">
      <CardHeader className="border-b py-4">
        <CardTitle>授权记录</CardTitle>
        <CardDescription>{consents.length} 项 Workspace 事实</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-2 p-3">
        {consents.map((consent) => (
          <button
            aria-pressed={selectedId === consent.id}
            className={`grid min-w-0 gap-2 overflow-hidden rounded-xl border p-3 text-left transition ${
              selectedId === consent.id
                ? "border-cyan-300 bg-cyan-50/70 shadow-sm"
                : "border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-50"
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
            <span className="flex items-center justify-between gap-3 text-xs text-slate-500">
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
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <Dialog.Trigger asChild>
        <Button disabled={consent.status === "revoked"} variant="destructive">
          <ShieldX aria-hidden="true" />撤销授权
        </Button>
      </Dialog.Trigger>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-slate-950/25 backdrop-blur-[2px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-slate-200 bg-white p-6 shadow-2xl shadow-slate-950/10">
          <div className="flex items-start justify-between gap-4">
            <div>
              <Dialog.Title className="text-xl font-semibold">撤销当前授权</Dialog.Title>
              <Dialog.Description className="mt-1 text-sm leading-6 text-slate-500">
                撤销会追加 revision r{consent.revision + 1}，并立即阻止新的生成与交付。
              </Dialog.Description>
            </div>
            <Dialog.Close asChild>
              <Button aria-label="关闭" size="icon" variant="ghost"><X aria-hidden="true" /></Button>
            </Dialog.Close>
          </div>
          <form className="mt-6 grid gap-5" onSubmit={submit}>
            <div className="grid gap-2">
              <Label htmlFor="revokeReason">撤销原因</Label>
              <textarea
                className="min-h-24 resize-none rounded-xl border border-slate-200 px-3 py-3 text-sm outline-none focus:border-rose-400 focus:ring-3 focus:ring-rose-500/10"
                id="revokeReason"
                name="reason"
                required
              />
            </div>
            <Alert className="border-rose-100 bg-rose-50 px-4 py-3 text-rose-700">
              <AlertCircle aria-hidden="true" />
              <AlertTitle>历史不会被删除</AlertTitle>
              <AlertDescription className="text-rose-700/80">
                已有任务、费用和历史交付继续保留，供后续风险处置追溯。
              </AlertDescription>
            </Alert>
            <div className="flex justify-end gap-2">
              <Dialog.Close asChild><Button type="button" variant="outline">取消</Button></Dialog.Close>
              <Button disabled={isSubmitting} type="submit" variant="destructive">
                {isSubmitting ? "撤销中…" : "确认撤销"}
              </Button>
            </div>
          </form>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
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
      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-5">
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
              <p className="text-xs font-medium tracking-wide text-slate-400 uppercase">固定对象</p>
              <p className="mt-2 text-sm font-medium">{subjectTypeLabels[scope.subject_type]}</p>
              <p className="mt-1 font-mono text-xs text-slate-500">{scope.subject_id}</p>
            </div>
            <div>
              <p className="text-xs font-medium tracking-wide text-slate-400 uppercase">权利类型</p>
              <div className="mt-2"><ScopeTerms values={scope.rights_types} /></div>
            </div>
            <div>
              <p className="text-xs font-medium tracking-wide text-slate-400 uppercase">用途与渠道</p>
              <div className="mt-2"><ScopeTerms values={[...scope.authorized_purposes, ...scope.channels]} /></div>
            </div>
          </div>
          <div className="grid content-start gap-4 rounded-xl border border-slate-200 bg-slate-50/70 p-4">
            <div className="flex items-start gap-3">
              <CalendarRange className="mt-0.5 size-4 text-[#079db3]" aria-hidden="true" />
              <div><p className="text-sm font-medium">有效期</p><p className="mt-1 text-xs text-slate-500">{formatDate(scope.valid_from)} — {formatDate(scope.valid_to)}</p></div>
            </div>
            <div className="flex items-start gap-3">
              <FileCheck2 className="mt-0.5 size-4 text-[#079db3]" aria-hidden="true" />
              <div><p className="text-sm font-medium">证明媒体</p><p className="mt-1 font-mono text-xs text-slate-500">{consent.current_revision.proof_media_version_ids.map(shortId).join("、")}</p></div>
            </div>
            <div className="flex items-start gap-3">
              <Clock3 className="mt-0.5 size-4 text-[#079db3]" aria-hidden="true" />
              <div><p className="text-sm font-medium">地域</p><div className="mt-2"><ScopeTerms values={scope.regions} /></div></div>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card className="gap-0 py-0">
        <CardHeader className="border-b py-4">
          <div className="flex items-center gap-2"><History className="size-4 text-[#079db3]" aria-hidden="true" /><CardTitle>修订历史</CardTitle></div>
          <CardDescription>所有事实只追加，旧范围与撤销原因始终可追溯。</CardDescription>
        </CardHeader>
        <CardContent className="divide-y divide-slate-100 p-0">
          {[...consent.revisions].reverse().map((revision) => (
            <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-4 px-5 py-4" key={revision.id}>
              <span className={`mt-0.5 grid size-8 place-items-center rounded-full ${revision.action === "revoke" ? "bg-rose-50 text-rose-600" : "bg-cyan-50 text-[#079db3]"}`}>
                {revision.action === "revoke" ? <ShieldX className="size-4" aria-hidden="true" /> : <Check className="size-4" aria-hidden="true" />}
              </span>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center justify-between gap-2"><p className="text-sm font-medium">r{revision.revision_no} · {actionLabels[revision.action]}</p><time className="text-xs text-slate-400" dateTime={revision.created_at}>{formatDate(revision.created_at)}</time></div>
                <p className="mt-1 text-sm text-slate-600">{revision.reason}</p>
                <p className="mt-2 text-xs text-slate-400">{revision.scope.authorized_purposes.length} 项用途 · {revision.proof_media_version_ids.length} 份证明</p>
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
  const consents = useConsentsQuery(workspaceId ?? "", { skip: !workspaceId });
  const media = useMediaVersionsQuery(workspaceId ?? "", { skip: !workspaceId });
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
      topAction={
        authenticated ? (
          <Button className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]" onClick={() => setCreateOpen(true)}>
            <Plus aria-hidden="true" />新建授权
          </Button>
        ) : (
          <Button asChild className="h-10 bg-[#079db3] px-4 text-white hover:bg-[#078da0]"><Link href="/login">登录后管理</Link></Button>
        )
      }
    >
      {notice ? <div className="pointer-events-none fixed top-24 right-6 z-50 flex items-center gap-2 rounded-xl border border-emerald-200 bg-white px-4 py-3 text-sm shadow-lg shadow-slate-950/10" role="status"><Check className="size-4 text-emerald-600" aria-hidden="true" />{notice}</div> : null}
      <div className="mx-auto max-w-[1320px] px-5 py-8 md:px-8">
        <div className="flex flex-wrap items-end justify-between gap-5">
          <div>
            <Badge className="border-cyan-100 bg-cyan-50 text-[#087f91]" variant="outline">合规事实层</Badge>
            <h1 className="mt-3 text-3xl font-semibold tracking-[-0.035em]">授权治理</h1>
            <p className="mt-2 text-sm text-slate-500">登记固定版本的用途、地域、期限与证明，让生成和交付门禁基于可追溯事实。</p>
          </div>
          {workspaceId ? <p className="text-xs text-slate-400">Workspace {shortId(workspaceId)}</p> : null}
        </div>

        {sessionState === "checking" || (authenticated && me.isLoading) ? <LoadingWorkspace /> : null}
        {sessionState === "anonymous" ? (
          <Alert className="mt-7 border-amber-200 bg-amber-50 p-5 text-amber-800"><AlertCircle aria-hidden="true" /><AlertTitle>需要登录</AlertTitle><AlertDescription className="text-amber-700">授权记录受 Workspace 隔离保护。<Link className="ml-1 font-medium underline" href="/login">前往登录</Link></AlertDescription></Alert>
        ) : null}
        {authenticated && (me.error || consents.error || media.error) ? (
          <Alert className="mt-7 border-rose-200 bg-rose-50 p-5 text-rose-800" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>授权事实暂时无法读取</AlertTitle><AlertDescription>{appApiErrorMessage(me.error ?? consents.error ?? media.error)}</AlertDescription></Alert>
        ) : null}

        {workspaceId && !me.isLoading ? (
          <>
            <section className="mt-7 grid gap-4 sm:grid-cols-3">
              <Card><CardHeader><CardDescription>授权记录</CardDescription><CardTitle className="text-2xl">{consents.data?.total ?? 0}</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>当前有效</CardDescription><CardTitle className="text-2xl text-emerald-700">{activeCount}</CardTitle></CardHeader></Card>
              <Card><CardHeader><CardDescription>已阻断 / 到期</CardDescription><CardTitle className="text-2xl text-amber-700">{blockedCount}</CardTitle></CardHeader></Card>
            </section>

            {actionError ? <Alert className="mt-5 border-rose-200 bg-rose-50 p-4 text-rose-800" variant="destructive"><AlertCircle aria-hidden="true" /><AlertTitle>操作未完成</AlertTitle><AlertDescription>{actionError}</AlertDescription></Alert> : null}
            {mediaVersions.length === 0 ? <Alert className="mt-5 border-amber-200 bg-amber-50 p-4 text-amber-800"><FileCheck2 aria-hidden="true" /><AlertTitle>缺少可用证明媒体</AlertTitle><AlertDescription className="text-amber-700">先在资产或媒体流程完成一次私有上传，才能登记授权证明。</AlertDescription></Alert> : null}

            <div className="mt-6 grid items-start gap-5 lg:grid-cols-[300px_minmax(0,1fr)]">
              {consents.isLoading ? <Card><CardContent className="p-6 text-sm text-slate-500">正在读取授权列表…</CardContent></Card> : <ConsentList consents={items} onSelect={setSelectedId} selectedId={effectiveSelectedId} />}
              {items.length === 0 && !consents.isLoading ? <EmptyConsent onCreate={() => setCreateOpen(true)} /> : detail.isLoading ? <LoadingWorkspace /> : detail.data ? <ConsentDetail consent={detail.data} isRevoking={revokeState.isLoading} onEdit={() => setReviseOpen(true)} onRevoke={submitRevoke} /> : null}
            </div>
          </>
        ) : null}
      </div>

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
