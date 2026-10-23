import { useState } from "react";
import dynamic from "next/dynamic";
import { useGenerationResult } from "@/components/canvas/runtime/components/generation/hooks/use-generation-result";
import { taskLabels } from "@/components/canvas/runtime/components/generation/constants/task-state";
import { TaskCreditSummary } from "@/components/canvas/runtime/components/credits/task-credit-summary";
import {
  generationFailurePresentation,
  type GenerationFailure,
} from "@/components/canvas/runtime/components/generation/lib/generation-failure";
import { MediaPreview } from "@/components/canvas/runtime/components/generation/workspace/media-preview";
import { ZipDownloadDialog } from "@/components/canvas/runtime/components/assets/transfer/zip-download-dialog";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Spinner } from "@/components/ui/spinner";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const MarkdownContent = dynamic(() =>
  import("@/components/canvas/runtime/components/editor/markdown-content").then(
    (module) => module.MarkdownContent,
  ),
);

export function HistoryTaskDetail({
  task,
  updated,
}: {
  task: CanvasContract.GenerationTaskVO;
  updated: () => Promise<unknown>;
}) {
  const modality =
    task.modality === "video"
      ? "video"
      : task.modality === "text"
        ? "text"
        : "image";
  const result = useGenerationResult(task, modality, updated);
  const failure = generationFailurePresentation(task);
  const diagnostic = (task as GenerationFailure).failureDiagnostic;
  const [failedUrl, setFailedUrl] = useState<string>();
  const [expanded, setExpanded] = useState(false);
  const src = result.file.data?.url;
  const showMedia = !!src && src !== failedUrl;
  const succeeded = task.state === "SUCCEEDED";
  const recoverable = ["WAITING", "ARCHIVE_FAILED", "OUTPUT_READY"].includes(
    task.state ?? "",
  );
  const outputs = task.outputs ?? [];
  const reload = async () => {
    setFailedUrl(undefined);
    await result.file.mutate();
  };
  const more = result.exportFiles.length + (task.textOutput ? 1 : 0) > 1;
  return (
    <section className="history-task-detail" aria-label="当前生成结果">
      <ScrollArea className="history-detail-scroll min-h-0 flex-1">
        <div className="history-detail-content">
          <div className="flex min-w-0 items-center justify-between gap-3">
            <div className="min-w-0">
              <h3
                className="truncate text-sm font-medium"
                title={task.modelName}
              >
                {task.modelName || "生成任务"}
              </h3>
              <p className="text-xs text-muted-foreground">
                {{ image: "图片", video: "视频", text: "文字" }[modality]} ·{" "}
                {task.createTime
                  ? new Date(task.createTime).toLocaleString()
                  : ""}
              </p>
            </div>
            <span
              className="shrink-0 text-xs text-muted-foreground"
              role="status"
            >
              {task.state ? taskLabels[task.state] : "状态待核对"}
            </span>
          </div>
          <TaskCreditSummary billing={task.billing} />
          {outputs.length > 1 && (
            <ToggleGroup
              type="single"
              value={String(result.outputIndex)}
              onValueChange={(value) => {
                if (value)
                  result.setSelection({
                    taskId: task.id,
                    index: Number(value),
                  });
              }}
              aria-label="生成产物"
              className="max-w-full flex-wrap"
            >
              {outputs.map((output) => (
                <ToggleGroupItem
                  key={output.index}
                  value={String(output.index)}
                  className="max-w-full truncate"
                  title={
                    typeof output.metadata?.name === "string"
                      ? output.metadata.name
                      : undefined
                  }
                >
                  {typeof output.metadata?.name === "string"
                    ? output.metadata.name
                    : `${output.modality === "video" ? "视频" : "图片"} ${output.index}`}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          )}
          {modality === "text" ? (
            <div className="history-text-result">
              {task.textOutput ? (
                <MarkdownContent value={task.textOutput} />
              ) : (
                <p className="text-sm text-muted-foreground">
                  {succeeded
                    ? "暂无生成正文"
                    : task.state
                      ? taskLabels[task.state]
                      : "状态待核对"}
                </p>
              )}
            </div>
          ) : (
            <div className="canvas-history-media" aria-label="生成媒体">
              {result.file.isLoading ? (
                <p
                  role="status"
                  className="flex items-center gap-2 text-sm text-muted-foreground"
                >
                  <Spinner />
                  正在读取结果
                </p>
              ) : showMedia ? (
                result.outputModality === "image" ? (
                  <MediaPreview
                    key={src}
                    src={src}
                    alt="生成图片"
                    className="size-full object-contain"
                    onError={() => setFailedUrl(src)}
                  />
                ) : result.outputModality === "audio" ? (
                  <audio
                    key={src}
                    src={src}
                    controls
                    preload="metadata"
                    onError={() => setFailedUrl(src)}
                  />
                ) : (
                  <video
                    key={src}
                    src={src}
                    controls
                    preload="metadata"
                    className="size-full object-contain"
                    onError={() => setFailedUrl(src)}
                  />
                )
              ) : (
                <p className="text-sm text-muted-foreground">
                  {succeeded
                    ? result.file.error ||
                      failedUrl ||
                      result.file.data?.status === "FAILED"
                      ? "预览暂不可用"
                      : "正在准备预览"
                    : task.state
                      ? taskLabels[task.state]
                      : "状态待核对"}
                </p>
              )}
            </div>
          )}
          <Collapsible open={expanded} onOpenChange={setExpanded}>
            <div className="flex items-center justify-between gap-3">
              <p className="text-xs text-muted-foreground">提示词</p>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" size="sm">
                  {expanded ? "收起" : "展开"}
                </Button>
              </CollapsibleTrigger>
            </div>
            {!expanded && (
              <p className="history-prompt line-clamp-2 text-sm">
                {task.prompt || "无提示词"}
              </p>
            )}
            <CollapsibleContent>
              <p className="history-prompt text-sm">
                {task.prompt || "无提示词"}
              </p>
            </CollapsibleContent>
          </Collapsible>
          {task.state === "SUBMISSION_UNKNOWN" && (
            <Alert>
              <AlertDescription>
                提交结果待核对，请先到供应商控制台确认，不会自动重新生成。
              </AlertDescription>
            </Alert>
          )}
          {(task.state === "FAILED" || task.state === "DISPATCH_FAILED") && (
            <p className="text-sm text-muted-foreground">{failure.reason}</p>
          )}
          {(!!diagnostic || !!task.failureKind) && (
            <Collapsible className="text-xs leading-5 text-muted-foreground">
              <CollapsibleTrigger asChild>
                <Button variant="ghost" size="sm">
                  任务诊断
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 break-all">
                  <dt>任务编号</dt>
                  <dd>{String(task.id ?? "未提供")}</dd>
                  {diagnostic?.providerCode && (
                    <>
                      <dt>错误码</dt>
                      <dd>{diagnostic.providerCode}</dd>
                    </>
                  )}
                  {diagnostic?.httpStatus != null && (
                    <>
                      <dt>HTTP 状态</dt>
                      <dd>{diagnostic.httpStatus}</dd>
                    </>
                  )}
                  {diagnostic?.providerRequestId && (
                    <>
                      <dt>供应商请求编号</dt>
                      <dd>{diagnostic.providerRequestId}</dd>
                    </>
                  )}
                  {!diagnostic && (
                    <>
                      <dt>诊断</dt>
                      <dd>未保存具体原因</dd>
                    </>
                  )}
                </dl>
              </CollapsibleContent>
            </Collapsible>
          )}
          {task.failureKind === "output_truncated" && (
            <p className="text-sm text-muted-foreground">
              输出达到长度上限，已保留返回的文本。
            </p>
          )}
        </div>
      </ScrollArea>
      <div className="history-result-actions flex shrink-0 flex-wrap justify-end gap-2">
        {succeeded && (result.exportFiles.length > 0 || task.textOutput) && (
          <Button onClick={() => void result.downloadCurrent()}>下载</Button>
        )}
        {succeeded && modality !== "text" && (
          <Button
            variant="outline"
            disabled={result.file.isValidating}
            onClick={() => void reload()}
          >
            {result.file.isValidating && <Spinner data-icon="inline-start" />}
            重新读取
          </Button>
        )}
        {recoverable && (
          <Button
            variant="outline"
            disabled={result.busy}
            onClick={() => void result.refresh()}
          >
            {result.busy && <Spinner data-icon="inline-start" />}
            {task.state === "WAITING" ? "查询进度" : "恢复归档"}
          </Button>
        )}
        {succeeded && more && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost">更多</Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" side="top">
              <DropdownMenuGroup>
                <DropdownMenuItem
                  onSelect={() => {
                    if (
                      result.outputs.some(
                        (output) =>
                          "modality" in output &&
                          output.modality !== "text" &&
                          (!output.fileId || !output.fileVersion),
                      )
                    ) {
                      notifyError(
                        new Error("部分产物尚无固定文件版本"),
                        "暂不可打包",
                      );
                      return;
                    }
                    result.setDownloadOpen(true);
                  }}
                >
                  打包下载
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      {result.downloadOpen && (
        <ZipDownloadDialog
          request={{
            ...(result.exportFiles.length ? { files: result.exportFiles } : {}),
            ...(task.textOutput
              ? {
                  texts: [
                    { fileName: "生成文本.txt", content: task.textOutput },
                  ],
                }
              : {}),
          }}
          onClose={() => result.setDownloadOpen(false)}
        />
      )}
    </section>
  );
}
