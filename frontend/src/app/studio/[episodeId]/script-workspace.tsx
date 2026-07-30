"use client";

import {
  CheckCircle2,
  FileInput,
  GitCompareArrows,
  LoaderCircle,
  Play,
  Save,
  ShieldAlert,
} from "lucide-react";
import { type FormEvent, useState } from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import {
  assetKindLabels,
  candidateKindLabels,
  proposalDescription,
  proposalTitle,
  scriptStatusLabels,
  taskStatusLabels,
} from "./episode-studio-model";

type CandidateDecision = API.CandidateDecisionRequest["decision"];

export function ScriptWorkspace({
  episode,
  snapshot,
  source,
  editableVersion,
  versions,
  batch,
  candidates,
  assets,
  busy,
  onImport,
  onPublish,
  onStartExtraction,
  onDecide,
  onConfirm,
  onSetCurrent,
}: {
  episode: API.EpisodeResponse;
  snapshot: API.EpisodeProductionSnapshot;
  source?: API.ScriptSourceResponse;
  editableVersion?: API.ScriptVersionResponse;
  versions: API.ScriptVersionResponse[];
  batch?: API.ExtractionBatchResponse;
  candidates: API.ExtractionCandidateResponse[];
  assets: API.AssetResponse[];
  busy: boolean;
  onImport: (request: API.ScriptImportRequest) => Promise<void>;
  onPublish: (body: string) => Promise<void>;
  onStartExtraction: () => Promise<void>;
  onDecide: (
    candidate: API.ExtractionCandidateResponse,
    decision: CandidateDecision,
  ) => Promise<void>;
  onConfirm: () => Promise<void>;
  onSetCurrent: () => Promise<void>;
}) {
  const [editorBody, setEditorBody] = useState(editableVersion?.body ?? "");
  const [linkedAssets, setLinkedAssets] = useState<Record<string, string>>({});

  async function submitImport(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    await onImport({
      input_type: "text",
      title: String(form.get("title") ?? "").trim(),
      rights_declaration: String(form.get("rights") ?? "").trim(),
      body: String(form.get("body") ?? ""),
      idempotency_key: `studio-import:${episode.id}:${crypto.randomUUID()}`,
    });
  }

  if (!source && !editableVersion) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>导入第一份剧本</CardTitle>
          <CardDescription>
            文本先保存为不可变草稿，确认内容后再发布为当前版本。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-5" onSubmit={submitImport}>
            <div className="grid gap-2 md:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="scriptTitle">剧本标题</Label>
                <Input id="scriptTitle" name="title" required maxLength={120} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="scriptRights">权利声明</Label>
                <Input
                  id="scriptRights"
                  name="rights"
                  placeholder="例如：原创文本，允许用于本项目制作"
                  required
                  maxLength={1000}
                />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="scriptBody">剧本文本</Label>
              <textarea
                className="min-h-[360px] w-full resize-y rounded-xl border border-slate-200 bg-white px-4 py-3 font-mono text-sm leading-7 outline-none transition focus:border-cyan-400 focus:ring-3 focus:ring-cyan-100"
                id="scriptBody"
                name="body"
                required
                maxLength={20_000}
              />
            </div>
            <div className="flex items-center justify-between gap-4">
              <p className="text-xs text-slate-500">正文不会进入日志或消息载荷。</p>
              <Button disabled={busy} type="submit">
                {busy ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : <FileInput aria-hidden="true" />}
                导入剧本
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    );
  }

  const pendingRequired = candidates.filter(
    (candidate) => candidate.required && candidate.status === "pending",
  ).length;
  const canConfirm = batch?.status === "succeeded" && pendingRequired === 0;
  const confirmedVersionId = batch?.confirmed_script_version_id;

  return (
    <div className="grid gap-6 xl:grid-cols-[minmax(0,1.35fr)_minmax(360px,.65fr)]">
      <div className="grid gap-6">
        <Card>
          <CardHeader className="gap-4 md:flex-row md:items-start md:justify-between">
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline">{source?.title ?? "剧本来源"}</Badge>
                <Badge className="border-cyan-200 bg-cyan-50 text-[#087f91]" variant="outline">
                  {scriptStatusLabels[snapshot.script_summary.status]}
                </Badge>
              </div>
              <CardTitle className="mt-3">当前剧本文本</CardTitle>
              <CardDescription>
                v{editableVersion?.version_no ?? "-"} · {editableVersion?.status === "published" ? "已发布" : "草稿"}
              </CardDescription>
            </div>
            <Button
              disabled={busy || !source || !editorBody.trim()}
              onClick={() => onPublish(editorBody)}
            >
              <Save aria-hidden="true" />发布新版本
            </Button>
          </CardHeader>
          <CardContent>
            <Label className="sr-only" htmlFor="currentScriptBody">剧本文本</Label>
            <textarea
              aria-label="当前剧本文本"
              className="min-h-[400px] w-full resize-y rounded-xl border border-slate-200 bg-slate-50/50 px-4 py-3 font-mono text-sm leading-7 outline-none transition focus:border-cyan-400 focus:bg-white focus:ring-3 focus:ring-cyan-100"
              id="currentScriptBody"
              maxLength={20_000}
              value={editorBody}
              onChange={(event) => setEditorBody(event.target.value)}
            />
          </CardContent>
        </Card>

        {batch ? (
          <Card>
            <CardHeader className="gap-4 md:flex-row md:items-start md:justify-between">
              <div>
                <CardTitle>提取候选</CardTitle>
                <CardDescription>
                  {batch.candidate_count} 项建议 · {taskStatusLabels[batch.task.status]}
                </CardDescription>
              </div>
              <div className="flex flex-wrap gap-2">
                {canConfirm && !confirmedVersionId ? (
                  <Button disabled={busy} onClick={onConfirm}>
                    <CheckCircle2 aria-hidden="true" />确认剧本结构
                  </Button>
                ) : null}
                {confirmedVersionId && confirmedVersionId !== episode.current_script_version_id ? (
                  <Button disabled={busy} onClick={onSetCurrent}>
                    <GitCompareArrows aria-hidden="true" />使用确认版本
                  </Button>
                ) : null}
              </div>
            </CardHeader>
            <CardContent className="grid gap-3">
              {candidates.length === 0 ? (
                <Alert>
                  <LoaderCircle className={batch.status === "succeeded" ? "" : "animate-spin"} aria-hidden="true" />
                  <AlertTitle>{batch.status === "succeeded" ? "没有候选" : "等待提取结果"}</AlertTitle>
                  <AlertDescription>
                    任务状态会从服务端轮询恢复，不从浏览器本地推断。
                  </AlertDescription>
                </Alert>
              ) : (
                candidates.map((candidate) => {
                  const linkableAssets = assets.filter(
                    (asset) =>
                      candidate.proposal.kind === "asset" &&
                      asset.kind === candidate.proposal.asset_kind &&
                      asset.status === "active",
                  );
                  return (
                    <article className="rounded-xl border border-slate-200 p-4" key={candidate.id}>
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge variant="outline">{candidateKindLabels[candidate.kind]}</Badge>
                            {candidate.required ? <Badge variant="secondary">必需</Badge> : null}
                            <span className="text-xs text-slate-400">{candidate.status}</span>
                          </div>
                          <h3 className="mt-2 font-medium">{proposalTitle(candidate)}</h3>
                          <p className="mt-1 text-sm leading-6 text-slate-500">
                            {proposalDescription(candidate)}
                          </p>
                        </div>
                        {candidate.status === "pending" ? (
                          <div className="flex flex-wrap items-center justify-end gap-2">
                            <Button
                              disabled={busy || (candidate.kind === "asset" && !confirmedVersionId)}
                              size="sm"
                              onClick={() => onDecide(candidate, { action: "accept_new" })}
                            >
                              接受
                            </Button>
                            <Button
                              disabled={busy}
                              size="sm"
                              variant="outline"
                              onClick={() => onDecide(candidate, { action: "ignore" })}
                            >
                              忽略
                            </Button>
                          </div>
                        ) : null}
                      </div>
                      {candidate.status === "pending" && candidate.proposal.kind === "asset" ? (
                        <div className="mt-4 flex flex-wrap items-end gap-2 border-t border-slate-100 pt-4">
                          <div className="grid min-w-56 flex-1 gap-2">
                            <Label htmlFor={`link-${candidate.id}`}>
                              关联已有{assetKindLabels[candidate.proposal.asset_kind]}
                            </Label>
                            <select
                              className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm"
                              id={`link-${candidate.id}`}
                              value={linkedAssets[candidate.id] ?? ""}
                              onChange={(event) =>
                                setLinkedAssets((current) => ({
                                  ...current,
                                  [candidate.id]: event.target.value,
                                }))
                              }
                            >
                              <option value="">选择已有资产</option>
                              {linkableAssets.map((asset) => (
                                <option key={asset.id} value={asset.id}>{asset.name}</option>
                              ))}
                            </select>
                          </div>
                          <Button
                            disabled={busy || !confirmedVersionId || !linkedAssets[candidate.id]}
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              onDecide(candidate, {
                                action: "link_existing",
                                downstream_id: linkedAssets[candidate.id],
                              })
                            }
                          >
                            关联资产
                          </Button>
                        </div>
                      ) : null}
                    </article>
                  );
                })
              )}
            </CardContent>
          </Card>
        ) : null}
      </div>

      <aside className="grid content-start gap-6">
        <Card>
          <CardHeader>
            <CardTitle>下一动作</CardTitle>
            <CardDescription>由 ProductionSnapshot 计算，不在前端复制阶段规则。</CardDescription>
          </CardHeader>
          <CardContent>
            {snapshot.blocking_reasons[0] ? (
              <Alert className="border-amber-200 bg-amber-50 text-amber-800">
                <ShieldAlert aria-hidden="true" />
                <AlertTitle>{snapshot.next_actions[0]?.label ?? "需要处理"}</AlertTitle>
                <AlertDescription className="text-amber-700">
                  {snapshot.blocking_reasons[0].summary}
                </AlertDescription>
              </Alert>
            ) : null}
            {snapshot.script_summary.status === "published" ? (
              <Button className="mt-4 w-full" disabled={busy} onClick={onStartExtraction}>
                <Play aria-hidden="true" />开始结构提取
              </Button>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>版本历史</CardTitle>
            <CardDescription>{versions.length} 个不可变版本</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            {versions.map((version) => (
              <div className="flex items-center justify-between rounded-lg border border-slate-200 px-3 py-2" key={version.id}>
                <div>
                  <p className="text-sm font-medium">v{version.version_no}</p>
                  <p className="text-xs text-slate-500">{version.status === "published" ? "已发布" : "草稿"}</p>
                </div>
                {version.id === episode.current_script_version_id ? <Badge>当前</Badge> : null}
              </div>
            ))}
          </CardContent>
        </Card>
      </aside>
    </div>
  );
}
