"use client";

import {
  AlertCircle,
  CheckCircle2,
  FileCheck2,
  FileText,
  FileUp,
  LoaderCircle,
} from "lucide-react";
import { type FormEvent, useMemo, useState } from "react";

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
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  appApiErrorMessage,
  useCompleteMediaUploadMutation,
  useImportScriptDocumentMutation,
  useInitializeMediaUploadMutation,
  useLazyMediaVersionQuery,
  useScriptDocumentsQuery,
} from "@/lib/server-state";

import { EpisodePlanWorkspace } from "./episode-plan-workspace";

type ImportMode = "text" | "media";

const RIGHTS_DECLARATION = "我确认拥有该剧本用于本项目制作与分析的权利";
const DOCUMENT_MAX_BYTES = 400_000;

const analysisLabels: Record<
  API.DocumentRevisionResponse["analysis_status"],
  string
> = {
  deterministic: "可确定性分集",
  ai_candidate_required: "需要 AI 分集候选",
  rejected: "需要修正格式",
};

const issueLabels: Record<string, string> = {
  duplicate_number: "集号重复",
  empty_episode: "存在空集",
  episode_limit_exceeded: "显式集数超过当前上限",
  no_marker: "未找到独占一行的集标记",
  number_gap: "集号不连续",
  number_out_of_order: "集号顺序错误",
  preamble_requires_decision: "第一集前存在待归属内容",
  utf8_bom_not_allowed: "文本包含 UTF-8 BOM",
};

const nextActionLabels: Record<string, string> = {
  generate_episode_plan: "下一步：生成并人工确认分集建议",
  reduce_episode_count: "下一步：减少显式集数后重试",
  remove_utf8_bom: "下一步：移除 BOM 后重新导入",
  reorder_episode_markers: "下一步：按顺序调整集标记",
  renumber_episode_markers: "下一步：修正集号后重新导入",
  resolve_preamble: "下一步：决定前言归属或删除前言",
};

async function sha256(value: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", value);
  return Array.from(new Uint8Array(digest), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}

async function sha256Text(value: string): Promise<string> {
  return sha256(new TextEncoder().encode(value).buffer);
}

function documentMimeType(file: File): "text/plain" | "text/markdown" {
  return file.name.toLowerCase().endsWith(".md")
    ? "text/markdown"
    : "text/plain";
}

function sleep(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

export function ScriptDocumentImportCard({
  canWrite,
  language,
  projectId,
  projectName,
  targetDurationMs,
  workspaceId,
}: {
  canWrite: boolean;
  language: string;
  projectId: string;
  projectName: string;
  targetDurationMs: number;
  workspaceId: string;
}) {
  const documents = useScriptDocumentsQuery(projectId);
  const [initializeUpload, initializeState] = useInitializeMediaUploadMutation();
  const [completeUpload, completeState] = useCompleteMediaUploadMutation();
  const [loadMediaVersion] = useLazyMediaVersionQuery();
  const [importDocument, importState] = useImportScriptDocumentMutation();
  const [mode, setMode] = useState<ImportMode>("text");
  const [text, setText] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [rightsConfirmed, setRightsConfirmed] = useState(false);
  const [analysis, setAnalysis] =
    useState<API.ScriptDocumentAnalysisResponse | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const busy =
    initializeState.isLoading || completeState.isLoading || importState.isLoading;
  const episodeMarkerCount = useMemo(
    () =>
      analysis?.blocks.filter((block) => block.kind === "episode_marker").length ??
      0,
    [analysis],
  );

  async function waitUntilReady(versionId: string): Promise<void> {
    for (let attempt = 0; attempt < 25; attempt += 1) {
      const version = await loadMediaVersion(versionId, false).unwrap();
      if (version.probe_status === "ready") return;
      if (
        version.probe_status === "failed" ||
        version.probe_status === "quarantined"
      ) {
        throw new Error(
          version.probe_error_summary ?? "剧本文档探测失败，请检查 UTF-8 编码。",
        );
      }
      await sleep(400);
    }
    throw new Error("文件已上传，格式探测仍在进行；请稍后再次提交同一文件。");
  }

  async function uploadDocument(selected: File): Promise<string> {
    if (!/\.(?:txt|md)$/i.test(selected.name)) {
      throw new Error("只接受 UTF-8 编码的 .txt 或 .md 文件。");
    }
    if (selected.size > DOCUMENT_MAX_BYTES) {
      throw new Error("文件不能超过 400 KB。");
    }
    const fileHash = await sha256(await selected.arrayBuffer());
    const mimeType = documentMimeType(selected);
    const uploadKey = await sha256Text(
      `${selected.name}\u0000${mimeType}\u0000${fileHash}`,
    );
    const initialized = await initializeUpload({
      workspace_id: workspaceId,
      kind: "document",
      filename: selected.name,
      size_bytes: selected.size,
      mime_type: mimeType,
      sha256: fileHash,
      idempotency_key: `script-document-upload:${uploadKey}`,
    }).unwrap();
    if (!initialized.upload.url || !initialized.upload.method) {
      throw new Error("对象存储未返回有效上传地址。");
    }
    const uploaded = await fetch(initialized.upload.url, {
      method: initialized.upload.method,
      headers: initialized.upload.headers as HeadersInit,
      body: selected,
    });
    if (!uploaded.ok) throw new Error(`对象存储上传失败（${uploaded.status}）。`);
    const completed = await completeUpload({
      uploadSessionId: initialized.upload_session.id,
      workspaceId,
    }).unwrap();
    await waitUntilReady(completed.version.id);
    return completed.version.id;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    setActionError(null);
    setNotice(null);
    setAnalysis(null);
    if (!canWrite) {
      setActionError("当前身份只能查看整剧文档，不能导入新原稿。");
      return;
    }
    if (!rightsConfirmed) {
      setActionError("请先确认剧本使用权声明。");
      return;
    }
    const title = String(new FormData(form).get("title") ?? "").trim();
    if (!title) {
      setActionError("请填写剧本文档标题。");
      return;
    }
    try {
      let body: API.ScriptDocumentImportRequest;
      if (mode === "text") {
        if (!text.trim()) throw new Error("请粘贴整剧文本。");
        const idempotencyHash = await sha256Text(
          JSON.stringify({ mode, title, language, text, rights: RIGHTS_DECLARATION }),
        );
        body = {
          input_type: "text",
          title,
          text,
          media_version_id: null,
          language,
          rights_declaration: RIGHTS_DECLARATION,
          idempotency_key: `script-document:${idempotencyHash}`,
        };
      } else {
        if (!file) throw new Error("请选择 .txt 或 .md 文件。");
        const mediaVersionId = await uploadDocument(file);
        const idempotencyHash = await sha256Text(
          JSON.stringify({
            mode,
            title,
            language,
            mediaVersionId,
            rights: RIGHTS_DECLARATION,
          }),
        );
        body = {
          input_type: "media",
          title,
          text: null,
          media_version_id: mediaVersionId,
          language,
          rights_declaration: RIGHTS_DECLARATION,
          idempotency_key: `script-document:${idempotencyHash}`,
        };
      }
      const result = await importDocument({ projectId, body }).unwrap();
      setAnalysis(result);
      setNotice("整剧原稿已保存为不可变修订，格式体检结果已生成。");
    } catch (error: unknown) {
      const apiError = error as { code?: string };
      setActionError(
        apiError.code === "resource_conflict"
          ? "同一导入请求已用于另一份内容，请刷新后重新提交。"
          : error instanceof Error
            ? error.message
            : appApiErrorMessage(error),
      );
    }
  }

  return (
    <>
    <Card
      className="mt-8"
      aria-label="整剧导入与格式体检"
      role="region"
    >
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FileText className="size-5" aria-hidden="true" />整剧导入与格式体检
        </CardTitle>
        <CardDescription>
          先保存整部原稿和精确字符范围，不会在这一阶段自动创建单集或分镜。
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-7 pt-6 xl:grid-cols-[minmax(0,1fr)_360px]">
        <form className="grid gap-5" onSubmit={submit}>
          <div className="grid gap-2">
            <Label htmlFor="scriptDocumentTitle">文档标题</Label>
            <Input
              defaultValue={`${projectName} · 整剧原稿`}
              disabled={!canWrite || busy}
              id="scriptDocumentTitle"
              maxLength={120}
              name="title"
              required
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="scriptDocumentMode">导入方式</Label>
            <Select
              disabled={!canWrite || busy}
              onValueChange={(value) => setMode(value as ImportMode)}
              value={mode}
            >
              <SelectTrigger className="w-full" id="scriptDocumentMode">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="text">粘贴整剧文本</SelectItem>
                <SelectItem value="media">上传 UTF-8 .txt / .md</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {mode === "text" ? (
            <div className="grid gap-2">
              <Label htmlFor="scriptDocumentText">整剧文本</Label>
              <Textarea
                className="min-h-64 resize-y font-mono text-sm leading-6"
                disabled={!canWrite || busy}
                id="scriptDocumentText"
                maxLength={100_000}
                placeholder={"第一集\n场景1：控制室，夜\n角色：对白……"}
                required
                value={text}
                onChange={(event) => setText(event.target.value)}
              />
              <p className="text-xs text-muted-foreground">
                保留原始换行；最多 100,000 个 Unicode 字符。
              </p>
            </div>
          ) : (
            <div className="grid gap-2">
              <Label htmlFor="scriptDocumentFile">剧本文档</Label>
              <Input
                accept=".txt,.md,text/plain,text/markdown"
                className="h-auto py-2 file:mr-3 file:rounded-md file:border-0 file:bg-muted file:px-3 file:py-1.5 file:text-sm file:font-medium"
                disabled={!canWrite || busy}
                id="scriptDocumentFile"
                type="file"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <p className="text-xs text-muted-foreground">
                严格 UTF-8、无 BOM，最多 400 KB；上传后固定到不可变媒体版本。
              </p>
            </div>
          )}
          <div className="flex items-start gap-3 bg-muted/45 p-4">
            <Checkbox
              checked={rightsConfirmed}
              disabled={!canWrite || busy}
              id="scriptDocumentRights"
              onCheckedChange={(checked) => setRightsConfirmed(checked === true)}
            />
            <Label className="font-normal leading-6" htmlFor="scriptDocumentRights">
              {RIGHTS_DECLARATION}
            </Label>
          </div>
          {actionError ? (
            <Alert variant="destructive">
              <AlertCircle aria-hidden="true" />
              <AlertTitle>导入未完成</AlertTitle>
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}
          {notice ? (
            <Alert className="border-0 bg-muted/50" role="status">
              <CheckCircle2 aria-hidden="true" />
              <AlertTitle>格式体检已完成</AlertTitle>
              <AlertDescription>{notice}</AlertDescription>
            </Alert>
          ) : null}
          <Button disabled={!canWrite || busy} type="submit">
            {busy ? (
              <LoaderCircle className="animate-spin" aria-hidden="true" />
            ) : mode === "media" ? (
              <FileUp aria-hidden="true" />
            ) : (
              <FileCheck2 aria-hidden="true" />
            )}
            {mode === "media" ? "上传并分析" : "导入并分析"}
          </Button>
        </form>

        <aside className="grid content-start gap-5">
          <div className="bg-muted/45 p-5">
            <p className="text-sm font-medium">已导入整剧文档</p>
            {documents.isLoading ? (
              <p className="mt-2 text-sm text-muted-foreground">正在读取……</p>
            ) : documents.error ? (
              <p className="mt-2 text-sm text-destructive">
                {appApiErrorMessage(documents.error)}
              </p>
            ) : documents.data?.items.length ? (
              <div className="mt-3 grid gap-3">
                {documents.data.items.map((document) => (
                  <div className="bg-background p-3" key={document.id}>
                    <div className="flex items-center justify-between gap-3">
                      <p className="truncate text-sm font-medium">{document.title}</p>
                      <Badge variant="outline">
                        {document.source_type === "media" ? "文件" : "粘贴"}
                      </Badge>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {document.language} · revision {document.revision}
                    </p>
                  </div>
                ))}
              </div>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">尚未导入整剧原稿。</p>
            )}
          </div>

          {analysis ? (
            <div className="bg-muted/45 p-5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="font-medium">最近一次格式体检</p>
                <Badge
                  variant={
                    analysis.revision.analysis_status === "rejected"
                      ? "destructive"
                      : "outline"
                  }
                >
                  {analysisLabels[analysis.revision.analysis_status]}
                </Badge>
              </div>
              <dl className="mt-4 grid grid-cols-2 gap-3 text-sm">
                <div className="bg-background p-3">
                  <dt className="text-muted-foreground">集标记</dt>
                  <dd className="mt-1 text-lg font-semibold">{episodeMarkerCount}</dd>
                </div>
                <div className="bg-background p-3">
                  <dt className="text-muted-foreground">结构块</dt>
                  <dd className="mt-1 text-lg font-semibold">
                    {analysis.blocks.length}
                  </dd>
                </div>
              </dl>
              {analysis.issues.length ? (
                <div className="mt-4 grid gap-3">
                  {analysis.issues.map((issue) => (
                    <div className="bg-muted p-3" key={issue.id}>
                      <p className="text-sm font-medium">
                        {issueLabels[issue.code] ?? issue.code} · 第 {issue.line_number} 行，第 {issue.column_number} 列
                      </p>
                      <p className="mt-1 text-xs leading-5 text-muted-foreground">
                        {nextActionLabels[issue.next_action] ?? issue.next_action}
                      </p>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="mt-4 text-sm text-muted-foreground">
                  集号连续且没有阻断问题；下一阶段可生成分集计划。
                </p>
              )}
            </div>
          ) : null}
        </aside>
      </CardContent>
    </Card>
    {analysis ? (
      <EpisodePlanWorkspace
        analysis={analysis}
        canWrite={canWrite}
        targetDurationMs={targetDurationMs}
      />
    ) : null}
    </>
  );
}
