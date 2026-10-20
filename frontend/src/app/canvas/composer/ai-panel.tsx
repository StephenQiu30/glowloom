"use client";

import { useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  Clapperboard,
  Film,
  ScanEye,
  ScrollText,
  Sparkles,
  X,
  RotateCw,
} from "lucide-react";
import {
  cancelOperation,
  getOperation,
  listProjectOperations,
} from "@/api/operations";
import {
  activeTask,
  flowError,
  taskLabels,
  type Task,
} from "@/components/generation/contracts";
import type { GenerationSeed } from "@/components/generation/generation-dialog";
import type { Project } from "@/components/projects/project";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group";
import {
  Item,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemTitle,
} from "@/components/ui/item";
import { Spinner } from "@/components/ui/spinner";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  aiGenerationSeed,
  aiModes,
  completedText,
  requireAIHistory,
  requireAITask,
  selectedAIReference,
  type AIMode,
} from "./ai-model";
import type { Node } from "../document";

type Props = {
  project: Project;
  nodes: Node[];
  selectedNode?: Node;
  onGenerate: (seed: GenerationSeed) => void;
  onInsertText: (text: string) => Promise<void> | void;
  onClose: () => void;
};
const modeIcons = [ScanEye, Clapperboard, Sparkles, ScrollText, Film];

export function CanvasAIPanel(props: Props) {
  return <CanvasAIContent key={props.project.id} {...props} />;
}

function CanvasAIContent({
  project,
  nodes,
  selectedNode,
  onGenerate,
  onInsertText,
  onClose,
}: Props) {
  const [mode, setMode] = useState<AIMode>("storyboard");
  const [prompt, setPrompt] = useState("");
  const [tasks, setTasks] = useState<API.TaskSummary[] | null>(null);
  const [details, setDetails] = useState<Record<string, Task>>({});
  const [historyError, setHistoryError] = useState("");
  const [actionError, setActionError] = useState("");
  const [notice, setNotice] = useState("");
  const [working, setWorking] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [limit, setLimit] = useState(20);
  const [hasMore, setHasMore] = useState(false);
  const [contextID, setContextID] = useState("");
  const [omittedReferenceID, setOmittedReferenceID] = useState("");
  const detailCache = useRef<Record<string, Task>>({});
  const textarea = useRef<HTMLTextAreaElement>(null);
  const mounted = useRef(true);
  const actionLock = useRef(false);
  const selected =
    selectedNode && nodes.some((node) => node.id === selectedNode.id)
      ? selectedNode
      : undefined;
  const useReference = selected?.id !== omittedReferenceID;
  const reference = useReference ? selectedAIReference(selected) : undefined;
  const context = contextID ? details[contextID] : undefined;

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    function schedule() {
      timer = setTimeout(() => {
        if (document.visibilityState === "hidden") schedule();
        else void read();
      }, 5000);
    }
    async function read() {
      try {
        const page = requireAIHistory(
          await listProjectOperations(
            { pid: project.id, limit },
            { signal: abort.signal },
          ),
          project.id,
        );
        if (abort.signal.aborted) return;
        setTasks(page.items);
        setHasMore(!!page.next_cursor);
        const textTasks = page.items.filter(
          (task) => task.capability === "text.generate",
        );
        const receipts = await Promise.allSettled(
          textTasks.map(async (task) => {
            const cached = detailCache.current[task.id!];
            if (
              cached &&
              task.update_time &&
              cached.update_time === task.update_time &&
              cached.status === task.status
            )
              return cached;
            const result = requireAITask(
              await getOperation(
                { id: task.id!, project_id: project.id },
                { signal: abort.signal },
              ),
              project.id,
            );
            if (result.id !== task.id) throw new Error("invalid_receipt");
            return result;
          }),
        );
        if (abort.signal.aborted) return;
        const current: Record<string, Task> = {};
        for (const receipt of receipts)
          if (receipt.status === "fulfilled")
            current[receipt.value.id] = receipt.value;
        detailCache.current = current;
        setDetails(current);
        setHistoryError(
          receipts.some((receipt) => receipt.status === "rejected")
            ? "部分任务详情读取失败，请重试。"
            : "",
        );
      } catch (error) {
        if (!abort.signal.aborted)
          setHistoryError(`生成历史读取失败。${flowError(error)}`);
      } finally {
        if (!abort.signal.aborted) schedule();
      }
    }
    void read();
    return () => {
      abort.abort();
      clearTimeout(timer);
    };
  }, [project.id, limit, refresh]);

  function chooseMode(value: string) {
    const skill = aiModes.find((entry) => entry.id === value);
    if (!skill) return;
    setMode(skill.id);
    if (
      !prompt.trim() ||
      prompt.trim() === "/" ||
      aiModes.some((entry) => entry.starter === prompt)
    )
      setPrompt(skill.starter);
    textarea.current?.focus();
  }

  function submit() {
    try {
      setActionError("");
      const seed = aiGenerationSeed(
        project,
        mode,
        prompt,
        useReference ? selected : undefined,
        context,
      );
      onGenerate(seed);
    } catch (error) {
      setActionError(error instanceof Error ? error.message : flowError(error));
    }
  }

  async function action(id: string, run: () => Promise<void>) {
    if (actionLock.current) return;
    actionLock.current = true;
    setWorking(id);
    setActionError("");
    setNotice("");
    try {
      await run();
    } catch (error) {
      if (mounted.current)
        setActionError(
          error instanceof Error && !("code" in error)
            ? error.message
            : flowError(error),
        );
    } finally {
      actionLock.current = false;
      if (mounted.current) setWorking("");
    }
  }

  async function regenerate(summary: API.TaskSummary) {
    await action(summary.id!, async () => {
      const task = requireAITask(
        await getOperation({ id: summary.id!, project_id: project.id }),
        project.id,
      );
      if (task.id !== summary.id || task.status !== "failed" || !task.retryable)
        throw new Error("任务状态已变化，请刷新后重试。");
      const capability = task.capability?.split(".")[0];
      if (
        capability !== "image" &&
        capability !== "video" &&
        capability !== "text"
      )
        throw new Error("当前任务不支持重新生成。");
      if (mounted.current)
        onGenerate({
          project,
          capability,
          prompt:
            task.inputs?.find((input) => input.role === "prompt")?.text ?? "",
          reuse: task,
        });
    });
  }

  return (
    <aside
      className="canvas-ai-panel"
      aria-label="AI 创作"
      data-canvas-ai-panel
      data-canvas-no-zoom
      data-canvas-wheel-scroll
    >
      <header className="canvas-ai-header">
        <div className="flex items-center gap-2">
          <Sparkles aria-hidden="true" />
          <h2>AI 创作</h2>
        </div>
        <Button
          variant="ghost"
          size="icon"
          aria-label="关闭 AI 创作"
          onClick={onClose}
        >
          <X />
        </Button>
      </header>
      <div className="canvas-ai-intro">
        <p>从灵感到故事，一起开始创作</p>
        <p className="text-sm text-muted-foreground">
          先生成文字方案，确认报价后执行。
        </p>
      </div>
      <ToggleGroup
        type="single"
        variant="outline"
        value={mode}
        onValueChange={chooseMode}
        aria-label="创作模式"
        className="canvas-ai-modes grid w-full grid-cols-2"
      >
        {aiModes.map((skill, index) => {
          const Icon = modeIcons[index];
          return (
            <ToggleGroupItem
              key={skill.id}
              value={skill.id}
              aria-label={skill.label}
            >
              <Icon data-icon="inline-start" />
              {skill.label}
            </ToggleGroupItem>
          );
        })}
      </ToggleGroup>
      {mode === "video-analysis" ? (
        <p className="canvas-ai-mode-note text-sm text-muted-foreground">
          请提供镜头文字描述，当前文字创作不读取视频文件。
        </p>
      ) : null}
      <section className="canvas-ai-history" aria-label="真实生成历史">
        <div className="flex items-center justify-between gap-2">
          <h3>项目生成记录</h3>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="刷新生成记录"
            onClick={() => setRefresh((value) => value + 1)}
          >
            <RotateCw />
          </Button>
        </div>
        {historyError ? (
          <Alert>
            <AlertDescription>{historyError}</AlertDescription>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setRefresh((value) => value + 1)}
            >
              重试读取
            </Button>
          </Alert>
        ) : null}
        {tasks === null && !historyError ? (
          <Spinner aria-label="正在读取生成历史" />
        ) : null}
        {tasks?.length === 0 ? (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>还没有生成任务</EmptyTitle>
              <EmptyDescription>
                输入需求，选择模型并确认报价后开始。
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : null}
        <ItemGroup>
          {tasks?.map((summary) => {
            const task = details[summary.id!] ?? summary;
            const text = completedText(task);
            const input = details[summary.id!]?.inputs?.find(
              (entry) => entry.role === "prompt",
            )?.text;
            const insertable = text && Array.from(text).length <= 10000;
            return (
              <Item
                key={summary.id}
                variant="outline"
                className="canvas-ai-task"
                role="listitem"
              >
                <ItemContent>
                  <ItemTitle>
                    {task.model_name ||
                      (task.capability === "text.generate"
                        ? "文字创作"
                        : task.capability === "video.generate"
                          ? "视频生成"
                          : task.capability === "image.generate"
                            ? "图片生成"
                            : "生成任务")}
                    <Badge variant="secondary">
                      {taskLabels[task.status!]}
                    </Badge>
                  </ItemTitle>
                  {input ? (
                    <ItemDescription title={input}>{input}</ItemDescription>
                  ) : null}
                  {task.failure_code ? (
                    <p className="text-sm text-muted-foreground">
                      失败原因：{task.failure_code}
                    </p>
                  ) : null}
                  {text ? (
                    <p className="canvas-ai-result break-words whitespace-pre-wrap">
                      {text}
                    </p>
                  ) : null}
                  {task.capability === "text.generate" &&
                  task.status === "completed" &&
                  !text &&
                  details[summary.id!] ? (
                    <p className="text-sm text-muted-foreground">
                      此任务没有可读取的审核通过文本。
                    </p>
                  ) : null}
                  {text && !insertable ? (
                    <p className="text-sm text-muted-foreground">
                      文本超过画布单节点 10000 字限制。
                    </p>
                  ) : null}
                  <div className="flex flex-wrap gap-2">
                    {insertable ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={!!working}
                        onClick={() =>
                          void action(task.id!, async () => {
                            await onInsertText(text);
                            if (mounted.current) setNotice("文本已插入画布。");
                          })
                        }
                      >
                        插入画布
                      </Button>
                    ) : null}
                    {text ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setContextID(task.id!);
                          textarea.current?.focus();
                        }}
                      >
                        继续创作
                      </Button>
                    ) : null}
                    {task.status === "failed" && task.retryable ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={!!working}
                        onClick={() => void regenerate(summary)}
                      >
                        重新生成
                      </Button>
                    ) : null}
                    {(activeTask(task.status) &&
                      !["unknown", "reconciling", "cancelling"].includes(
                        task.status!,
                      )) ||
                    task.status === "quoted" ||
                    task.status === "draft" ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={!!working || task.cancel_requested}
                        onClick={() =>
                          void action(task.id!, async () => {
                            await cancelOperation(
                              { id: task.id! },
                              { project_id: project.id },
                            );
                            if (mounted.current) {
                              setNotice("已请求取消，最终状态以任务更新为准。");
                              setRefresh((value) => value + 1);
                            }
                          })
                        }
                      >
                        取消任务
                      </Button>
                    ) : null}
                  </div>
                </ItemContent>
              </Item>
            );
          })}
        </ItemGroup>
        {hasMore && limit < 200 ? (
          <Button
            variant="ghost"
            onClick={() => setLimit((value) => Math.min(200, value + 20))}
          >
            读取更多记录
          </Button>
        ) : null}
        {hasMore && limit === 200 ? (
          <p className="text-sm text-muted-foreground">
            当前显示最近 200 条项目任务。
          </p>
        ) : null}
      </section>
      <footer className="canvas-ai-footer">
        {actionError ? (
          <Alert variant="destructive">
            <AlertDescription>{actionError}</AlertDescription>
          </Alert>
        ) : null}
        {notice ? (
          <p role="status" className="text-sm text-muted-foreground">
            {notice}
          </p>
        ) : null}
        {context ? (
          <div className="flex items-center justify-between gap-2">
            <Badge variant="secondary">已选上一轮方案作为上下文</Badge>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="取消上一轮上下文"
              onClick={() => setContextID("")}
            >
              <X />
            </Button>
          </div>
        ) : null}
        {reference || (useReference && selected?.node_type === "text") ? (
          <div className="flex items-center justify-between gap-2">
            <Badge variant="secondary">参考：{selected?.title}</Badge>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="移除本次参考"
              onClick={() => setOmittedReferenceID(selected?.id ?? "")}
            >
              <X />
            </Button>
          </div>
        ) : null}
        <form
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="canvas-ai-prompt" className="sr-only">
                创作需求
              </FieldLabel>
              <InputGroup variant="composer">
                <InputGroupTextarea
                  ref={textarea}
                  id="canvas-ai-prompt"
                  value={prompt}
                  maxLength={10000}
                  onChange={(event) => setPrompt(event.target.value)}
                  placeholder={
                    aiModes.find((skill) => skill.id === mode)?.starter
                  }
                  rows={3}
                  onKeyDown={(event) => {
                    if (
                      (event.metaKey || event.ctrlKey) &&
                      event.key === "Enter"
                    ) {
                      event.preventDefault();
                      submit();
                    }
                  }}
                />
                <InputGroupAddon align="block-end">
                  <span className="text-xs text-muted-foreground">
                    / 选择创作模式 · 选中素材作为参考
                  </span>
                  <InputGroupButton
                    type="submit"
                    size="icon-sm"
                    variant="default"
                    className="ml-auto"
                    disabled={!prompt.trim() || prompt.trim() === "/"}
                    aria-label="选择模型并获取报价"
                  >
                    <ArrowUp />
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </Field>
          </FieldGroup>
        </form>
        {prompt.trim() === "/" ? (
          <div className="flex flex-wrap gap-2" aria-label="创作技能">
            {aiModes.map((skill) => (
              <Button
                key={skill.id}
                variant="ghost"
                size="sm"
                onClick={() => chooseMode(skill.id)}
              >
                {skill.label}
              </Button>
            ))}
          </div>
        ) : null}
        <p className="text-xs text-muted-foreground">
          文字方案可插入画布；媒体参考用于画布的图片与视频生成。
        </p>
      </footer>
    </aside>
  );
}
