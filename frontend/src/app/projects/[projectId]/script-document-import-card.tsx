"use client";

import {
  AlertCircle,
  CheckCircle2,
  FileText,
  FileUp,
  LoaderCircle,
  RotateCcw,
} from "lucide-react";
import { type FormEvent, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

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
import {
  Item,
  ItemContent,
  ItemDescription,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item";
import { Label } from "@/components/ui/label";
import {
  appApiErrorMessage,
  useCompleteMediaUploadMutation,
  useImportScriptDocumentMutation,
  useInitializeMediaUploadMutation,
  useLazyMediaVersionQuery,
  usePreviewScriptDocumentMutation,
  useScriptDocumentsQuery,
} from "@/lib/server-state";

import { EpisodePlanWorkspace } from "./episode-plan-workspace";

const RIGHTS_DECLARATION = "我确认拥有该剧本用于本项目制作与分析的权利";
const DOCX_MIME_TYPE =
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
const ACCEPTED_SCRIPT_FILES = `.docx,.md,${DOCX_MIME_TYPE},text/markdown`;

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

function documentMimeType(file: File): "text/markdown" | typeof DOCX_MIME_TYPE {
  return file.name.toLowerCase().endsWith(".docx")
    ? DOCX_MIME_TYPE
    : "text/markdown";
}

function documentKind(file: File): string {
  return file.name.toLowerCase().endsWith(".docx")
    ? "DOCX 剧本"
    : "Markdown 剧本";
}

function fileSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

function sleep(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}

export function ScriptDocumentImportCard({
  canWrite,
  language,
  projectId,
  targetDurationMs,
  workspaceId,
}: {
  canWrite: boolean;
  language: string;
  projectId: string;
  targetDurationMs: number;
  workspaceId: string;
}) {
  const documents = useScriptDocumentsQuery(projectId);
  const [initializeUpload, initializeState] = useInitializeMediaUploadMutation();
  const [completeUpload, completeState] = useCompleteMediaUploadMutation();
  const [loadMediaVersion] = useLazyMediaVersionQuery();
  const [previewDocument, previewState] = usePreviewScriptDocumentMutation();
  const [importDocument, importState] = useImportScriptDocumentMutation();
  const fileInput = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [mediaVersionId, setMediaVersionId] = useState<string | null>(null);
  const [preview, setPreview] =
    useState<API.ScriptDocumentPreviewResponse | null>(null);
  const [rightsConfirmed, setRightsConfirmed] = useState(false);
  const [analysis, setAnalysis] =
    useState<API.ScriptDocumentAnalysisResponse | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const uploadBusy =
    initializeState.isLoading ||
    completeState.isLoading ||
    previewState.isLoading;
  const busy = uploadBusy || importState.isLoading;
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
          version.probe_error_summary ?? "剧本文档读取失败，请检查文件格式。",
        );
      }
      await sleep(400);
    }
    throw new Error("文件已上传，格式读取仍在进行；请稍后重新预览。");
  }

  async function uploadDocument(selected: File): Promise<string> {
    if (!/\.(?:docx|md)$/i.test(selected.name)) {
      throw new Error("只接受 .docx 或 .md 剧本文件。");
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

  function selectFile(selected: File | null): void {
    setFile(selected);
    setMediaVersionId(null);
    setPreview(null);
    setAnalysis(null);
    setActionError(null);
    setNotice(null);
  }

  function resetFile(): void {
    selectFile(null);
    setRightsConfirmed(false);
    if (fileInput.current) fileInput.current.value = "";
  }

  async function createPreview(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setActionError(null);
    setNotice(null);
    if (!canWrite) {
      setActionError("当前身份只能查看剧本文档，不能导入新原稿。");
      return;
    }
    if (!file) {
      setActionError("请选择 .docx 或 .md 剧本文件。");
      return;
    }
    if (!rightsConfirmed) {
      setActionError("请先确认剧本使用权声明。");
      return;
    }
    try {
      const versionId = await uploadDocument(file);
      const result = await previewDocument({
        projectId,
        body: { media_version_id: versionId },
      }).unwrap();
      setMediaVersionId(versionId);
      setPreview(result);
      setNotice("文档读取完成，请确认预览内容后开始解析。");
    } catch (error: unknown) {
      setActionError(
        error instanceof Error ? error.message : appApiErrorMessage(error),
      );
    }
  }

  async function confirmAndAnalyze(): Promise<void> {
    setActionError(null);
    setNotice(null);
    if (!file || !mediaVersionId || !preview) {
      setActionError("请先上传文件并完成内容预览。");
      return;
    }
    try {
      const idempotencyHash = await sha256Text(
        JSON.stringify({
          title: file.name,
          language,
          mediaVersionId,
          rawHash: preview.raw_hash,
          rights: RIGHTS_DECLARATION,
        }),
      );
      const result = await importDocument({
        projectId,
        body: {
          input_type: "media",
          title: file.name,
          text: null,
          media_version_id: mediaVersionId,
          language,
          rights_declaration: RIGHTS_DECLARATION,
          idempotency_key: `script-document:${idempotencyHash}`,
        },
      }).unwrap();
      setAnalysis(result);
      setNotice("剧本已固定为不可变修订，格式解析结果已生成。");
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
        aria-label="整剧导入与格式体检"
        className="mt-8 scroll-mt-32"
        id="script-import"
        role="region"
      >
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FileText className="size-5" aria-hidden="true" />
            整剧导入与格式体检
          </CardTitle>
          <CardDescription>
            上传 DOCX 或 Markdown 原稿，先确认内容预览，再固定版本并进入剧本解析。
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-7 pt-6 xl:grid-cols-[minmax(0,1fr)_360px]">
          <form className="grid gap-5" onSubmit={createPreview}>
            <div className="grid gap-2">
              <Label htmlFor="scriptDocumentFile">剧本文档</Label>
              <Input
                accept={ACCEPTED_SCRIPT_FILES}
                disabled={!canWrite || busy}
                id="scriptDocumentFile"
                ref={fileInput}
                type="file"
                onChange={(event) => selectFile(event.target.files?.[0] ?? null)}
              />
              <p className="text-xs text-muted-foreground">
                支持 .docx 和 .md；文件容量由对象存储安全策略校验，不设置剧本业务上限。
              </p>
            </div>

            {file ? (
              <Item variant="outline">
                <ItemMedia variant="icon">
                  <FileText className="size-5" aria-hidden="true" />
                </ItemMedia>
                <ItemContent>
                  <ItemTitle>{file.name}</ItemTitle>
                  <ItemDescription>
                    {documentKind(file)} · {fileSize(file.size)}
                  </ItemDescription>
                </ItemContent>
                <Badge variant="outline">{preview ? "已读取" : "等待预览"}</Badge>
              </Item>
            ) : null}

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
                <AlertTitle>操作未完成</AlertTitle>
                <AlertDescription>{actionError}</AlertDescription>
              </Alert>
            ) : null}
            {notice ? (
              <Alert className="border-0 bg-muted/50" role="status">
                <CheckCircle2 aria-hidden="true" />
                <AlertTitle>{analysis ? "格式解析已完成" : "预览已就绪"}</AlertTitle>
                <AlertDescription>{notice}</AlertDescription>
              </Alert>
            ) : null}

            {!preview ? (
              <Button
                disabled={!canWrite || uploadBusy || !file || !rightsConfirmed}
                type="submit"
              >
                {uploadBusy ? (
                  <LoaderCircle className="animate-spin" aria-hidden="true" />
                ) : (
                  <FileUp aria-hidden="true" />
                )}
                上传并预览
              </Button>
            ) : (
              <section
                aria-label="剧本内容预览"
                className="grid gap-4 bg-muted/35 p-5"
                role="region"
              >
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <h3 className="font-medium">剧本内容预览</h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {preview.codepoint_count.toLocaleString()} 个字符 · 尚未创建剧集
                    </p>
                  </div>
                  <Badge variant="outline">等待确认</Badge>
                </div>
                <div className="max-h-[520px] overflow-auto rounded-lg border bg-background p-5 text-sm leading-7 whitespace-pre-wrap [&_a]:text-primary [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:pl-4 [&_blockquote]:text-muted-foreground [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_h1]:mb-4 [&_h1]:text-2xl [&_h1]:font-semibold [&_h2]:my-4 [&_h2]:text-xl [&_h2]:font-semibold [&_h3]:my-3 [&_h3]:text-lg [&_h3]:font-medium [&_li]:ml-5 [&_li]:list-disc [&_ol_li]:list-decimal [&_p]:my-3 [&_pre]:overflow-auto [&_pre]:bg-muted [&_pre]:p-4 [&_table]:w-full [&_table]:border-collapse [&_td]:border [&_td]:p-2 [&_th]:border [&_th]:bg-muted [&_th]:p-2">
                  <ReactMarkdown remarkPlugins={[remarkGfm]}>
                    {preview.raw_text}
                  </ReactMarkdown>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button disabled={busy} onClick={resetFile} type="button" variant="outline">
                    <RotateCcw aria-hidden="true" />
                    重新选择
                  </Button>
                  <Button
                    disabled={!canWrite || busy || Boolean(analysis)}
                    onClick={confirmAndAnalyze}
                    type="button"
                  >
                    {importState.isLoading ? (
                      <LoaderCircle className="animate-spin" aria-hidden="true" />
                    ) : (
                      <CheckCircle2 aria-hidden="true" />
                    )}
                    {analysis ? "剧本解析已完成" : "确认剧本并开始解析"}
                  </Button>
                </div>
              </section>
            )}
          </form>

          <aside className="grid content-start gap-5">
            <div className="bg-muted/45 p-5">
              <p className="text-sm font-medium">已导入剧本文档</p>
              {documents.isLoading ? (
                <p className="mt-2 text-sm text-muted-foreground">正在读取……</p>
              ) : documents.error ? (
                <p className="mt-2 text-sm text-destructive">
                  {appApiErrorMessage(documents.error)}
                </p>
              ) : documents.data?.items.length ? (
                <div className="mt-3 grid gap-3">
                  {documents.data.items.map((document) => (
                    <Item key={document.id} size="sm" variant="outline">
                      <ItemContent>
                        <ItemTitle>{document.title}</ItemTitle>
                        <ItemDescription>
                          {document.language} · revision {document.revision}
                        </ItemDescription>
                      </ItemContent>
                      <Badge variant="outline">文件</Badge>
                    </Item>
                  ))}
                </div>
              ) : (
                <p className="mt-2 text-sm text-muted-foreground">
                  尚未导入剧本原稿。
                </p>
              )}
            </div>

            {analysis ? (
              <div className="bg-muted/45 p-5">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <p className="font-medium">最近一次格式解析</p>
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
                          {issueLabels[issue.code] ?? issue.code} · 第 {issue.line_number}
                          行，第 {issue.column_number} 列
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
