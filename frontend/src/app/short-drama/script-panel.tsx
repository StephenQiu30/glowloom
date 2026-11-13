"use client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { FieldError } from "@/components/ui/field";

import { useEffect, useId, useRef, useState } from "react";
import useSWR from "swr";
import {
  createScriptFileImport,
  createScriptSource,
  getScriptSource,
  listScriptFileImports,
  listScriptSources,
  reconcileScriptFileImport,
  retryScriptFileImport,
  cancelScriptFileImport,
} from "@/api/script";
import { uploadMediaAsset } from "@/api/media";
import { isProjectID } from "@/components/projects/project";
import { ContentState } from "@/components/layout/content-state";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { RequestError } from "@/lib/request";
import { useScriptWrite } from "./use-script-write";

const scriptLimit = 100_000;
const documentLimit = 20 * 1024 * 1024;

function documentFromText(text: string): API.RichDocument {
  return {
    type: "doc",
    content: text
      .trim()
      .replace(/\r\n?/g, "\n")
      .split("\n")
      .map((line) => ({
        type: "paragraph",
        ...(line ? { content: [{ type: "text", text: line }] } : {}),
      })),
  };
}

const importStatuses = {
  queued: "等待解析",
  running: "正在解析",
  partial: "部分完成",
  succeeded: "已导入",
  failed: "导入失败",
  cancel_requested: "正在取消",
  cancelled: "已取消",
} satisfies Record<API.ImportStatus, string>;

const working = (job: API.ImportJob) =>
  job.status === "queued" ||
  job.status === "running" ||
  job.status === "cancel_requested" ||
  job.reconciliation_requested === true;

export function ScriptPanel({
  projectID,
  owner,
  head,
  writable,
  generatedText,
  initialInput,
  onChange,
  onLock,
}: {
  projectID: string;
  owner: string;
  head: API.ProjectState;
  writable: boolean;
  generatedText?: string;
  initialInput?: { text: string; file?: File };
  onLock: (locked: boolean) => void;
  onChange: () => Promise<unknown>;
}) {
  const id = useId();
  const [text, setText] = useState(initialInput?.text ?? "");
  const [file, setFile] = useState(initialInput?.file);
  const [title, setTitle] = useState("");
  const [rights, setRights] = useState(false);
  const [selected, setSelected] = useState<string>();
  const [sourcePage, setSourcePage] = useState(0);
  const write = useScriptWrite();
  const fileInput = useRef<HTMLInputElement>(null);
  useEffect(() => {
    onLock(write.locked);
    return () => onLock(false);
  }, [onLock, write.locked]);
  const version = head.draft_version_id ?? head.adopted_version_id;
  const revision = head.revision ?? 0;
  const sourceParams = { pid: projectID, version_id: version };
  const sources = useSWR(
    version ? [owner, projectID, "script-sources", version, sourcePage] : null,
    () => listScriptSources({ ...sourceParams, after: sourcePage, limit: 50 }),
  );
  const detail = useSWR(
    version && selected
      ? [owner, projectID, "source", version, selected]
      : null,
    () => getScriptSource({ ...sourceParams, lineage: selected! }),
  );
  const imports = useSWR(
    [owner, projectID, "script-imports"],
    () => listScriptFileImports({ pid: projectID, limit: 25 }),
    {
      refreshInterval: (value) => (value?.items?.some(working) ? 2000 : 0),
      onSuccess(value) {
        if (
          value.items?.some(
            (job) => (job.latest_script_revision ?? 0) > revision,
          )
        )
          void onChange();
      },
    },
  );
  const count = Array.from(text).length;
  const fileError =
    file &&
    (!/\.(txt|docx)$/i.test(file.name) ||
      file.size < 1 ||
      file.size > documentLimit)
      ? "请选择 20 MB 以内的 TXT 或 DOCX 剧本。"
      : "";

  function save() {
    if (
      write.locked ||
      !writable ||
      !rights ||
      fileError ||
      (!file && (!text.trim() || count > scriptLimit || text.includes("\0")))
    )
      return;
    const key = crypto.randomUUID();
    const uploadKey = crypto.randomUUID();
    const base = {
      expected_revision: revision,
      base_version_id: version,
      rights_confirmed: true,
    };
    let assetID: string | undefined;
    void write.submit(async ({ signal }) => {
      const options = { signal, headers: { "Idempotency-Key": key } };
      if (file) {
        if (!assetID) {
          const uploaded = await uploadMediaAsset(
            { pid: projectID },
            { local_review_confirmed: true },
            file,
            {
              signal,
              headers: { "Idempotency-Key": uploadKey },
            },
          );
          if (
            !isProjectID(uploaded.asset?.id) ||
            uploaded.asset.project_id !== projectID ||
            uploaded.asset.kind !== "document"
          )
            throw new RequestError("invalid_receipt");
          assetID = uploaded.asset.id;
        }
        const job = await createScriptFileImport(
          { pid: projectID },
          { ...base, asset_ids: [assetID] },
          options,
        );
        if (!isProjectID(job.id) || job.project_id !== projectID)
          throw new RequestError("invalid_receipt");
      } else {
        const receipt = await createScriptSource(
          { pid: projectID },
          {
            ...base,
            source_kind: "chapter",
            status: "draft",
            title: title.trim() || "未命名剧本",
            document: documentFromText(text),
            provenance: {},
          },
          options,
        );
        if (
          !isProjectID(receipt.version_id) ||
          (receipt.script_revision ?? 0) <= revision
        )
          throw new RequestError("invalid_receipt");
      }
      if (signal.aborted) return;
      setRights(false);
      setTitle("");
      setText("");
      setFile(undefined);
      if (fileInput.current) fileInput.current.value = "";
      setSourcePage(0);
      void imports.mutate();
      void onChange();
    });
  }

  function control(
    job: API.ImportJob,
    action: "retry" | "cancel" | "reconcile",
  ) {
    if (
      !writable ||
      write.locked ||
      !job.can_control ||
      !isProjectID(job.id) ||
      !Number.isSafeInteger(job.revision)
    )
      return;
    const params = { pid: projectID, id: job.id };
    const key = crypto.randomUUID();
    const send = {
      retry: retryScriptFileImport,
      cancel: cancelScriptFileImport,
      reconcile: reconcileScriptFileImport,
    }[action];
    void write.submit(async ({ signal }) => {
      const result = await send(
        params,
        { expected_revision: job.revision },
        { signal, headers: { "Idempotency-Key": key } },
      );
      if (result.id !== job.id || result.project_id !== projectID)
        throw new RequestError("invalid_receipt");
      void imports.mutate();
    });
  }

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      {writable && (
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            save();
          }}
        >
          <h2 className="font-semibold">上传剧本</h2>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor={id + "-file"}>上传文件</FieldLabel>
              <Input
                ref={fileInput}
                id={id + "-file"}
                type="file"
                accept=".txt,.docx"
                disabled={write.locked}
                onChange={(event) => setFile(event.target.files?.[0])}
              />
              {file && (
                <div className="flex flex-wrap items-center gap-2">
                  <span>{file.name}</span>
                  <Button
                    variant="ghost"
                    size="sm"
                    type="button"
                    disabled={write.locked}
                    onClick={() => {
                      setFile(undefined);
                      if (fileInput.current) fileInput.current.value = "";
                    }}
                  >
                    移除文件
                  </Button>
                </div>
              )}
              {fileError && <FieldError>{fileError}</FieldError>}
            </Field>
            {!file && (
              <>
                <Field>
                  <FieldLabel htmlFor={id + "-title"}>剧本名称</FieldLabel>
                  <Input
                    id={id + "-title"}
                    maxLength={100}
                    value={title}
                    disabled={write.locked}
                    onChange={(event) => setTitle(event.target.value)}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor={id + "-text"}>粘贴文本</FieldLabel>
                  {generatedText && (
                    <Button
                      type="button"
                      variant="outline"
                      disabled={write.locked || text === generatedText}
                      onClick={() =>
                        setText((value) =>
                          value.trim()
                            ? value + "\n\n" + generatedText
                            : generatedText,
                        )
                      }
                    >
                      加入生成的剧本
                    </Button>
                  )}
                  <Textarea
                    id={id + "-text"}
                    className="min-h-56"
                    value={text}
                    disabled={write.locked}
                    onChange={(event) => setText(event.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">
                    {count.toLocaleString()} / 100,000
                  </p>
                </Field>
              </>
            )}
            <Field orientation="horizontal">
              <Checkbox
                id={id + "-rights"}
                checked={rights}
                disabled={write.locked}
                onCheckedChange={(checked) => setRights(checked === true)}
              />
              <FieldLabel htmlFor={id + "-rights"}>
                我有权使用剧本，并已审核上传内容。
              </FieldLabel>
            </Field>
          </FieldGroup>
          <Button
            className="self-start"
            disabled={
              write.locked ||
              !rights ||
              !!fileError ||
              (!file &&
                (!text.trim() || count > scriptLimit || text.includes("\0")))
            }
          >
            {write.pending
              ? "正在提交…"
              : file
                ? "上传并解析"
                : "保存为剧本草稿"}
          </Button>
        </form>
      )}
      <section className="flex min-w-0 flex-col gap-4">
        <h2 className="font-semibold">剧本来源</h2>
        {sources.error && (
          <ContentState
            state="error"
            title="无法读取剧本来源"
            actions={
              <Button onClick={() => void sources.mutate()}>重试</Button>
            }
          />
        )}
        {sources.isLoading && <p role="status">正在读取剧本来源…</p>}
        {(!version || (sources.data && !sources.data.items?.length)) && (
          <p className="text-sm text-muted-foreground">暂无剧本来源。</p>
        )}
        {sources.data?.items?.map(
          (source) =>
            isProjectID(source.source_lineage_id) && (
              <Button
                className="justify-start"
                key={source.id}
                variant={
                  selected === source.source_lineage_id
                    ? "secondary"
                    : "outline"
                }
                onClick={() => setSelected(source.source_lineage_id)}
              >
                <span className="truncate">{source.title}</span>
                <span className="text-muted-foreground">
                  {source.char_count} 字
                </span>
              </Button>
            ),
        )}
        {sourcePage > 0 && (
          <Button variant="ghost" onClick={() => setSourcePage(0)}>
            返回首批来源
          </Button>
        )}
        {sources.data?.next_position !== undefined && (
          <Button
            variant="ghost"
            onClick={() => setSourcePage(sources.data!.next_position!)}
          >
            下一批来源
          </Button>
        )}
        {detail.error && (
          <Alert role="alert">
            <AlertDescription>无法读取此剧本正文。</AlertDescription>
          </Alert>
        )}
        {detail.isLoading && <p role="status">正在读取正文…</p>}
        {detail.data && (
          <pre className="max-h-96 overflow-auto rounded-md bg-muted p-4 font-sans text-sm break-words whitespace-pre-wrap">
            {detail.data.plain_text}
          </pre>
        )}
        <h2 className="font-semibold">文件导入记录</h2>
        {imports.error && (
          <Alert role="alert">
            <AlertDescription>无法读取文件导入记录。</AlertDescription>
          </Alert>
        )}
        {imports.data?.items?.map((job) => (
          <div
            key={job.id}
            className="flex flex-col gap-2 rounded-lg border p-3 text-sm"
          >
            <p>{job.status ? importStatuses[job.status] : "状态待核对"}</p>
            {job.files?.map((entry) => (
              <p key={entry.asset_id}>
                {entry.file_name ?? "剧本文件"} ·{" "}
                {entry.status === "failed"
                  ? (entry.failure_code ?? "解析失败")
                  : entry.status === "succeeded"
                    ? "已归档"
                    : "待解析"}
              </p>
            ))}
            {job.can_control && writable && (
              <div className="flex flex-wrap gap-2">
                {working(job) &&
                  job.status !== "cancel_requested" &&
                  !job.reconciliation_requested && (
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={write.locked}
                      onClick={() => control(job, "cancel")}
                    >
                      取消导入
                    </Button>
                  )}
                {job.needs_reconciliation && (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={write.locked}
                    onClick={() => control(job, "reconcile")}
                  >
                    核对结果
                  </Button>
                )}
                {job.retryable && (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={write.locked}
                    onClick={() => control(job, "retry")}
                  >
                    重试解析
                  </Button>
                )}
              </div>
            )}
          </div>
        ))}
      </section>
      {write.error && (
        <Alert role="alert">
          <AlertDescription>
            {write.error}
            {write.unknown ? (
              <Button
                disabled={write.pending}
                onClick={() => void write.submit()}
              >
                重试原操作
              </Button>
            ) : (
              <Button variant="outline" onClick={() => void onChange()}>
                刷新剧本版本
              </Button>
            )}
          </AlertDescription>
        </Alert>
      )}
    </div>
  );
}
