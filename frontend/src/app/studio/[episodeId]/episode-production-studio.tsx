"use client";

import {
  AlertCircle,
  CheckCircle2,
  Clapperboard,
  Download,
  FileText,
  LoaderCircle,
} from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";

import { LayoutContainer } from "@/components/layout/layout-container";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useAuthSessionState } from "@/hooks/use-auth-session";
import request, { ApiClientError } from "@/lib/request";

import type { EpisodePanel } from "./episode-studio-model";

type Envelope<T> = { data: T };
type Episode = {
  id: string;
  project_id: string;
  name: string;
  position: number;
  target_duration_ms: number;
  current_script_version_id: string | null;
};
type Project = { id: string; name: string; aspect_ratio: string };
type Task = { id: string; kind: string; label: string; status: string; required: boolean };
type NarrativeUnit = { id: string; kind: string; text: string };
type Dialogue = { id: string; speaker: string; text: string };
type Scene = {
  id: string;
  heading: string;
  position: number;
  narrative_units: NarrativeUnit[];
  dialogues: Dialogue[];
  tasks: Task[];
};
type Structure = {
  id: string;
  status: "needs_review" | "confirmed" | "superseded";
  revision: number;
  script_version_id: string;
  scenes: Scene[];
};
type DraftShot = {
  proposal_key: string;
  position: number;
  title: string;
  narrative_unit_version_ids: string[];
  spec: { duration_ms?: number; visual?: { shot_size?: string; camera_movement?: string } };
  risk_codes: string[];
};
type DraftBatch = {
  id: string;
  status: "queued" | "running" | "needs_review" | "approved" | "applied" | "failed" | "unknown";
  revision: number;
  candidate: { shots: DraftShot[] };
  decisions: Record<string, string>;
  result_hash: string | null;
};
type Shot = {
  id: string;
  position: number;
  title: string;
  content_hash: string;
  spec: DraftShot["spec"];
};
type ApplyPreflight = {
  batch_id: string;
  batch_revision: number;
  order_hash: string;
  impact_hash: string;
  created: number;
};
type ExportPreflight = {
  order_hash: string;
  allowed: boolean;
  shot_count: number;
  blockers: Array<{ code: string; summary: string }>;
};
type StoryboardExport = {
  id: string;
  content_hash: string;
  download_url: string;
  files: Array<{ Name?: string; name?: string }>;
};

const panels: Array<{ id: EpisodePanel; label: string }> = [
  { id: "script", label: "剧本结构" },
  { id: "assets", label: "项目事实" },
  { id: "storyboard", label: "分镜设计" },
  { id: "media", label: "媒体" },
  { id: "tasks", label: "制作任务" },
];

function key(prefix: string) {
  return `${prefix}-${crypto.randomUUID()}`;
}

async function optional<T>(operation: Promise<Envelope<T>>): Promise<T | undefined> {
  try {
    return (await operation).data;
  } catch (error) {
    if (error instanceof ApiClientError && error.code === "not_found") return undefined;
    throw error;
  }
}

export function EpisodeProductionStudio({
  episodeId,
  initialPanel,
}: {
  episodeId: string;
  initialPanel: EpisodePanel;
}) {
  const authState = useAuthSessionState();
  const [episode, setEpisode] = useState<Episode>();
  const [project, setProject] = useState<Project>();
  const [episodes, setEpisodes] = useState<Episode[]>([]);
  const [structure, setStructure] = useState<Structure>();
  const [batch, setBatch] = useState<DraftBatch>();
  const [shots, setShots] = useState<Shot[]>([]);
  const [applyPreflight, setApplyPreflight] = useState<ApplyPreflight>();
  const [exportPreflight, setExportPreflight] = useState<ExportPreflight>();
  const [storyboardExport, setStoryboardExport] = useState<StoryboardExport>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  const [notice, setNotice] = useState<string>();

  const loadBatch = useCallback(async () => {
    const value = await optional(
      request<Envelope<DraftBatch>>(`/api/episodes/${episodeId}/storyboard-draft`),
    );
    setBatch(value);
    return value;
  }, [episodeId]);

  useEffect(() => {
    if (authState !== "authenticated") return;
    let active = true;
    void (async () => {
      try {
        setLoading(true);
        const episodeResponse = await request<Envelope<Episode>>(`/api/episodes/${episodeId}`);
        const currentEpisode = episodeResponse.data;
        const [projectResponse, episodeList, currentStructure, currentBatch, currentExport] = await Promise.all([
          request<Envelope<Project>>(`/api/projects/${currentEpisode.project_id}`),
          request<Envelope<Episode[]>>(`/api/projects/${currentEpisode.project_id}/episodes`),
          optional(request<Envelope<Structure>>(`/api/episodes/${episodeId}/structure`)),
          optional(request<Envelope<DraftBatch>>(`/api/episodes/${episodeId}/storyboard-draft`)),
          optional(request<Envelope<StoryboardExport>>(`/api/episodes/${episodeId}/storyboard-export`)),
        ]);
        const shotResponse = await request<Envelope<Shot[]>>(`/api/episodes/${episodeId}/shots`);
        if (!active) return;
        setEpisode(currentEpisode);
        setProject(projectResponse.data);
        setEpisodes(episodeList.data);
        setStructure(currentStructure);
        setBatch(currentBatch);
        setStoryboardExport(currentExport);
        setShots(shotResponse.data);
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : "无法加载剧集工作台");
      } finally {
        if (active) setLoading(false);
      }
    })();
    return () => { active = false; };
  }, [authState, episodeId]);

  useEffect(() => {
    if (batch?.status !== "queued" && batch?.status !== "running") return;
    const timer = window.setInterval(() => {
      void loadBatch().catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "候选状态刷新失败"));
    }, 2_000);
    return () => window.clearInterval(timer);
  }, [batch?.status, loadBatch]);

  const tasks = useMemo(
    () => structure?.scenes.flatMap((scene) => scene.tasks) ?? [],
    [structure],
  );
  const allTasksAccepted = tasks.length > 0 && tasks.every((task) => !task.required || task.status === "accepted");
  const acceptedDrafts = batch?.candidate.shots.filter((shot) => batch.decisions[shot.proposal_key] === "accepted").length ?? 0;

  async function run(operation: () => Promise<void>) {
    setBusy(true);
    setError(undefined);
    setNotice(undefined);
    try { await operation(); } catch (cause) { setError(cause instanceof Error ? cause.message : "操作失败"); } finally { setBusy(false); }
  }

  async function acceptTask(task: Task) {
    if (!structure) return;
    await run(async () => {
      const response = await request<Envelope<Structure>>(`/api/episode-structures/${structure.id}/tasks/${task.id}/accept`, { method: "POST", data: { expected_revision: structure.revision, idempotency_key: key("accept-task") } });
      setStructure(response.data);
      setNotice(`已接受制作任务：${task.label}`);
    });
  }

  async function confirmStructure() {
    if (!structure) return;
    await run(async () => {
      const response = await request<Envelope<Structure>>(`/api/episode-structures/${structure.id}/confirm`, { method: "POST", data: { expected_revision: structure.revision, idempotency_key: key("confirm-structure") } });
      setStructure(response.data);
      setNotice(`结构已确认，生成剧本 v${response.data.revision} 的稳定叙事单元`);
    });
  }

  async function createDraft() {
    await run(async () => {
      const response = await request<Envelope<DraftBatch>>(`/api/episodes/${episodeId}/storyboard-drafts`, { method: "POST", data: { idempotency_key: key("storyboard-draft") } });
      setBatch(response.data);
      setApplyPreflight(undefined);
      setNotice("分镜候选任务已进入队列");
    });
  }

  async function acceptDraft(shot: DraftShot) {
    if (!batch) return;
    await run(async () => {
      const response = await request<Envelope<DraftBatch>>(`/api/storyboard-draft-batches/${batch.id}/decisions`, { method: "POST", data: { proposal_key: shot.proposal_key, action: "accepted", expected_revision: batch.revision, idempotency_key: key("accept-shot") } });
      setBatch(response.data);
      setNotice(`已接受此镜：${shot.title}`);
    });
  }

  async function approveBatch() {
    if (!batch) return;
    await run(async () => {
      const response = await request<Envelope<DraftBatch>>(`/api/storyboard-draft-batches/${batch.id}/approve`, { method: "POST", data: { expected_revision: batch.revision, idempotency_key: key("approve-storyboard") } });
      setBatch(response.data);
      setNotice("整批分镜草案已批准");
    });
  }

  async function preflightApply() {
    if (!batch) return;
    await run(async () => {
      const response = await request<Envelope<ApplyPreflight>>(`/api/storyboard-draft-batches/${batch.id}/apply-preflight`, { method: "POST", data: { expected_revision: batch.revision } });
      setApplyPreflight(response.data);
      setNotice(`预检完成：将创建 ${response.data.created} 个正式镜头`);
    });
  }

  async function applyDraft() {
    if (!batch || !applyPreflight) return;
    await run(async () => {
      const response = await request<Envelope<{ batch: DraftBatch; shots: Shot[] }>>(`/api/storyboard-draft-batches/${batch.id}/apply`, { method: "POST", data: { expected_revision: applyPreflight.batch_revision, expected_order_hash: applyPreflight.order_hash, impact_hash: applyPreflight.impact_hash, idempotency_key: key("apply-storyboard") } });
      setBatch(response.data.batch);
      setShots(response.data.shots);
      setExportPreflight(undefined);
      setNotice(`已原子写入 ${response.data.shots.length} 个正式镜头`);
    });
  }

  async function preflightExportPackage() {
    await run(async () => {
      const response = await request<Envelope<ExportPreflight>>(`/api/episodes/${episodeId}/storyboard-exports/preflight`, { method: "POST", data: {} });
      setExportPreflight(response.data);
      setNotice(response.data.allowed ? "分镜包导出条件已满足" : "分镜包仍有阻塞项");
    });
  }

  async function createExportPackage() {
    if (!exportPreflight?.allowed) return;
    await run(async () => {
      const response = await request<Envelope<StoryboardExport>>(`/api/episodes/${episodeId}/storyboard-exports`, { method: "POST", data: { expected_order_hash: exportPreflight.order_hash, idempotency_key: key("storyboard-export") } });
      setStoryboardExport(response.data);
      setNotice(`分镜包已生成，内容哈希 ${response.data.content_hash.slice(0, 12)}…`);
    });
  }

  async function downloadExportPackage() {
    if (!storyboardExport) return;
    await run(async () => {
      const contents = await request<Blob>(storyboardExport.download_url, {
        responseType: "blob",
      });
      const objectURL = URL.createObjectURL(contents);
      const anchor = document.createElement("a");
      anchor.href = objectURL;
      anchor.download = `storyboard-${episodeId}.zip`;
      anchor.click();
      URL.revokeObjectURL(objectURL);
      setNotice("分镜包下载已开始");
    });
  }

  if (authState === "checking" || loading) {
    return <main className="grid min-h-[60vh] place-items-center"><LoaderCircle className="size-6 animate-spin" aria-label="正在加载剧集工作台" /></main>;
  }
  if (authState !== "authenticated") {
    return <main className="grid min-h-[60vh] place-items-center"><Link className="underline" href="/login">请先登录</Link></main>;
  }

  return (
    <main className="py-8">
      <LayoutContainer>
        <div className="flex flex-wrap items-end justify-between gap-4 border-b pb-6">
          <div>
            <Link className="text-sm text-muted-foreground hover:underline" href={episode ? `/projects/${episode.project_id}` : "/projects"}>{project?.name ?? "项目"}</Link>
            <h1 className="mt-2 text-3xl font-semibold">第 {episode?.position} 集 · {episode?.name}</h1>
            <p className="mt-2 text-sm text-muted-foreground">确认结构事实后生成候选分镜，再经人工决议原子写入与导出。</p>
          </div>
          <select aria-label="切换当前剧集" className="h-10 rounded-md border bg-background px-3 text-sm" onChange={(event) => { window.location.href = `/studio/${event.target.value}/${initialPanel}`; }} value={episodeId}>
            {episodes.map((item) => <option key={item.id} value={item.id}>第 {item.position} 集 · {item.name}</option>)}
          </select>
        </div>

        <nav aria-label="剧集制作流程" className="my-6 flex flex-wrap gap-2">
          {panels.map((panel) => <Link aria-current={panel.id === initialPanel ? "page" : undefined} className={panel.id === initialPanel ? "rounded-full bg-foreground px-4 py-2 text-sm text-background" : "rounded-full border px-4 py-2 text-sm hover:bg-muted"} href={`/studio/${episodeId}/${panel.id}`} key={panel.id}>{panel.label}</Link>)}
        </nav>

        {error ? <Alert className="mb-5" variant="destructive"><AlertCircle className="size-4" /><AlertTitle>操作未完成</AlertTitle><AlertDescription>{error}</AlertDescription></Alert> : null}
        {notice ? <div className="mb-5 rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status"><CheckCircle2 className="mr-2 inline size-4" />{notice}</div> : null}

        {initialPanel === "script" || initialPanel === "tasks" ? (
          <StructurePanel allTasksAccepted={allTasksAccepted} busy={busy} confirmStructure={confirmStructure} onAcceptTask={acceptTask} structure={structure} />
        ) : null}
        {initialPanel === "storyboard" ? (
          <StoryboardPanel acceptedDrafts={acceptedDrafts} applyDraft={applyDraft} applyPreflight={applyPreflight} approveBatch={approveBatch} batch={batch} busy={busy} createDraft={createDraft} createExportPackage={createExportPackage} downloadExportPackage={downloadExportPackage} exportPreflight={exportPreflight} onAcceptDraft={acceptDraft} preflightApply={preflightApply} preflightExportPackage={preflightExportPackage} shots={shots} storyboardExport={storyboardExport} structure={structure} />
        ) : null}
        {initialPanel === "assets" ? <BoundaryPanel title="项目事实" description="MVP 使用已确认的制作圣经作为角色与世界观事实，不在剧集层复制资产写模型。" /> : null}
        {initialPanel === "media" ? <BoundaryPanel title="媒体" description="媒体上传与版本由 Backend 的对象存储模块统一管理；分镜 MVP 不生成或复制媒体文件。" /> : null}
      </LayoutContainer>
    </main>
  );
}

function StructurePanel({ allTasksAccepted, busy, confirmStructure, onAcceptTask, structure }: { allTasksAccepted: boolean; busy: boolean; confirmStructure: () => void; onAcceptTask: (task: Task) => void; structure?: Structure }) {
  if (!structure) return <BoundaryPanel title="剧本结构" description="尚未发布剧集剧本。请先从项目页完成分集计划、原子创建剧集并发布剧本。" />;
  const unitCount = structure.scenes.reduce((sum, scene) => sum + scene.narrative_units.length + scene.dialogues.length + 1, 0);
  return <div className="grid gap-5">
    <section className="rounded-2xl border bg-card p-6">
      <div className="flex flex-wrap items-center justify-between gap-3"><div><p className="text-sm text-muted-foreground">{structure.scenes.length} 项建议 · {structure.status === "confirmed" ? "已完成" : "待确认"}</p><h2 className="mt-1 text-xl font-semibold">场景与制作任务</h2></div>{structure.status === "needs_review" ? <Button disabled={busy || !allTasksAccepted} onClick={confirmStructure}>确认剧本结构</Button> : <span className="rounded-full bg-emerald-100 px-3 py-1 text-sm text-emerald-800">结构已确认</span>}</div>
      <div className="mt-6 grid gap-4">{structure.scenes.map((scene) => <section aria-label={`${scene.heading} 制作任务`} className="rounded-xl border p-4" key={scene.id}><div className="flex items-center justify-between gap-3"><h3 className="font-semibold">场景 {scene.position} · {scene.heading}</h3><span className="text-xs text-muted-foreground">{scene.narrative_units.length} 个叙事段 · {scene.dialogues.length} 段对白</span></div><div className="mt-3 grid gap-2">{scene.tasks.map((task) => <article className="flex items-center justify-between gap-3 rounded-lg bg-muted/60 px-3 py-2" key={task.id}><p className="text-sm"><span className="font-medium">{task.label}</span><span className="ml-2 text-xs text-muted-foreground">{task.required ? "必需" : "可选"} · {task.status}</span></p>{task.status === "pending" ? <Button disabled={busy} onClick={() => onAcceptTask(task)} size="sm">接受</Button> : <CheckCircle2 className="size-4 text-emerald-600" aria-label="已接受" />}</article>)}</div></section>)}</div>
    </section>
    <section className="rounded-2xl border bg-card p-6"><p className="text-sm text-muted-foreground">稳定叙事单元</p><p className="mt-2 text-3xl font-semibold">{unitCount}</p><p className="mt-2 text-sm text-muted-foreground">场景标题、动作/叙述与对白均以来源 UUID 固定，作为分镜覆盖输入。</p></section>
  </div>;
}

function StoryboardPanel({ acceptedDrafts, applyDraft, applyPreflight, approveBatch, batch, busy, createDraft, createExportPackage, downloadExportPackage, exportPreflight, onAcceptDraft, preflightApply, preflightExportPackage, shots, storyboardExport, structure }: { acceptedDrafts: number; applyDraft: () => void; applyPreflight?: ApplyPreflight; approveBatch: () => void; batch?: DraftBatch; busy: boolean; createDraft: () => void; createExportPackage: () => void; downloadExportPackage: () => void; exportPreflight?: ExportPreflight; onAcceptDraft: (shot: DraftShot) => void; preflightApply: () => void; preflightExportPackage: () => void; shots: Shot[]; storyboardExport?: StoryboardExport; structure?: Structure }) {
  return <div className="grid gap-5">
    {structure?.status !== "confirmed" ? <Alert><AlertCircle className="size-4" /><AlertTitle>需先确认剧本结构</AlertTitle><AlertDescription>分镜候选只能引用已确认的稳定叙事单元。</AlertDescription></Alert> : null}
    <section className="rounded-2xl border bg-card p-6">
      <div className="flex flex-wrap items-center justify-between gap-3"><div><p className="text-sm text-muted-foreground">Agent 仅生成候选，不写正式业务数据</p><h2 className="mt-1 text-xl font-semibold">待审核分镜草案</h2></div><Button disabled={busy || structure?.status !== "confirmed" || batch?.status === "queued" || batch?.status === "running"} onClick={createDraft}><Clapperboard className="mr-2 size-4" />生成待审核草案</Button></div>
      {batch?.status === "queued" || batch?.status === "running" ? <div className="mt-5 flex items-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />候选生成中，页面会自动刷新</div> : null}
      {batch?.status === "failed" || batch?.status === "unknown" ? <p className="mt-5 text-sm text-destructive">候选生成失败，可检查私有 Agent 后创建新批次。</p> : null}
      {batch?.candidate.shots.length ? <div className="mt-6 grid gap-3">{batch.candidate.shots.map((shot) => { const accepted = batch.decisions[shot.proposal_key] === "accepted"; return <article className="rounded-xl border p-4" key={shot.proposal_key}><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-xs text-muted-foreground">镜头 {shot.position} · {shot.spec.duration_ms ?? "—"} ms</p><h3 className="mt-1 font-semibold">{shot.title}</h3><p className="mt-2 text-sm text-muted-foreground">覆盖 {shot.narrative_unit_version_ids.length} 个叙事单元</p></div>{accepted ? <span className="rounded-full bg-emerald-100 px-3 py-1 text-sm text-emerald-800">accepted</span> : <Button disabled={busy || batch.status !== "needs_review"} onClick={() => onAcceptDraft(shot)} size="sm">接受此镜</Button>}</div></article>; })}</div> : <p className="mt-5 text-sm text-muted-foreground">尚无分镜候选。</p>}
      {batch?.status === "needs_review" && batch.candidate.shots.length > 0 ? <div className="mt-5 flex items-center justify-between gap-3 border-t pt-5"><span className="text-sm text-muted-foreground">已接受 {acceptedDrafts}/{batch.candidate.shots.length}</span><Button disabled={busy || acceptedDrafts !== batch.candidate.shots.length} onClick={approveBatch}>批准整批草案</Button></div> : null}
      {batch?.status === "approved" ? <div className="mt-5 flex flex-wrap gap-3 border-t pt-5"><Button disabled={busy} onClick={preflightApply} variant="outline">预检写入影响</Button><Button disabled={busy || !applyPreflight} onClick={applyDraft}>原子写入正式分镜</Button>{applyPreflight ? <span className="self-center text-sm text-muted-foreground">将创建 {applyPreflight.created} 个镜头</span> : null}</div> : null}
    </section>

    <section aria-label="分镜准备度摘要" className="rounded-2xl border bg-card p-6"><div className="flex items-center justify-between gap-3"><div><p className="text-sm text-muted-foreground">正式分镜</p><h2 className="mt-1 text-xl font-semibold">{shots.length} 个镜头</h2></div>{shots.length > 0 ? <CheckCircle2 className="size-6 text-emerald-600" /> : <AlertCircle className="size-6 text-muted-foreground" />}</div>{shots.length > 0 ? <ol className="mt-4 grid gap-2">{shots.map((shot) => <li className="flex items-center justify-between rounded-lg bg-muted/60 px-3 py-2 text-sm" key={shot.id}><span>{shot.position}. {shot.title}</span><code className="text-xs text-muted-foreground">{shot.content_hash.slice(0, 8)}</code></li>)}</ol> : null}</section>

    <section className="rounded-2xl border bg-card p-6"><div className="flex flex-wrap items-center justify-between gap-3"><div><p className="text-sm text-muted-foreground">确定性 ZIP · manifest + JSON + CSV + HTML</p><h2 className="mt-1 text-xl font-semibold">分镜包导出</h2></div><Button disabled={busy} onClick={preflightExportPackage} variant="outline">检查导出条件</Button></div>{exportPreflight ? <div aria-label="分镜包预检结果" className="mt-4 rounded-lg bg-muted p-4 text-sm" role="region">{exportPreflight.allowed ? `允许导出 · ${exportPreflight.shot_count} 个镜头` : exportPreflight.blockers.map((blocker) => blocker.summary).join("；")}</div> : null}<div className="mt-4 flex flex-wrap items-center gap-3"><Button disabled={busy || !exportPreflight?.allowed} onClick={createExportPackage}><FileText className="mr-2 size-4" />生成分镜包</Button>{storyboardExport ? <Button disabled={busy} onClick={downloadExportPackage} variant="outline"><Download className="mr-2 size-4" />下载分镜包</Button> : null}{storyboardExport ? <code className="text-xs text-muted-foreground">SHA-256 {storyboardExport.content_hash}</code> : null}</div></section>
  </div>;
}

function BoundaryPanel({ description, title }: { description: string; title: string }) {
  return <section className="rounded-2xl border bg-card p-8"><h2 className="text-xl font-semibold">{title}</h2><p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground">{description}</p></section>;
}
