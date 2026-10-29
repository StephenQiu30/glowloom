"use client";

import dynamic from "next/dynamic";
import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  advanceAgentSession,
  confirmAgentStage,
  createAgentSession,
  getAgentSession,
  listAgentSessions,
  startAgentTurn,
  stopAgentSession,
} from "@/api/agent";
import { listProjectModels } from "@/api/models";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { readErrorMessage } from "@/lib/read-error";
import { fieldsForMode, initialParams, money } from "./contracts";
import { ModelPicker } from "./model-picker";
import { GenerationParameters } from "./generation-parameters";
import type { PromptDraft } from "./prompt-document";

const TaskDetail = dynamic(() =>
  import("./generation-dialog").then((m) => m.TaskDetail),
);
const terminal = new Set(["completed", "failed", "stopped", "expired"]);
const labels: Record<string, string> = {
  preparing: "正在准备报价",
  awaiting_confirmation: "等待确认积分",
  running: "正在执行",
  needs_reconciliation: "结果待确认",
  completed: "已完成",
  failed: "执行失败",
  stopped: "已停止后续步骤",
  expired: "报价已过期",
};
const phases: Record<string, string> = {
  plan: "规划",
  image: "图片生成",
  summary: "总结",
};

function checked(value: API.Session, project: string): API.Session {
  if (
    !value.id ||
    value.project_id !== project ||
    !Number.isSafeInteger(value.revision) ||
    !Array.isArray(value.turns) ||
    value.turns.length > 20 ||
    value.turns.some(
      (turn) =>
        !turn.id || !labels[turn.status ?? ""] || !phases[turn.phase ?? ""],
    )
  )
    throw new Error("会话回执不完整，请重新读取。");
  return value;
}

export function AgentWorkspace({
  projectID,
  initialMessage,
  onMessageChange,
  headerActions,
  projectControl,
  onLock,
}: {
  projectID: string;
  initialMessage: string;
  onMessageChange?: (message: string) => void;
  headerActions: ReactNode;
  projectControl: ReactNode;
  onLock: (locked: boolean) => void;
}) {
  const [sessions, setSessions] = useState<API.Session[]>([]);
  const [session, setSession] = useState<API.Session | null>(null);
  const [models, setModels] = useState<API.ModelResponse[]>([]);
  const [planner, setPlanner] = useState<API.ModelResponse>();
  const [image, setImage] = useState<API.ModelResponse>();
  const [plannerParams, setPlannerParams] = useState<PromptDraft["params"]>({});
  const [imageParams, setImageParams] = useState<PromptDraft["params"]>({});
  const [localMessage, setLocalMessage] = useState(initialMessage);
  const message = onMessageChange ? initialMessage : localMessage;
  function setMessage(value: string) {
    setLocalMessage(value);
    onMessageChange?.(value);
  }
  const [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  const [task, setTask] = useState<string>();
  const createKey = useRef<string | null>(null),
    pending = useRef<{ hash: string; key: string } | null>(null);
  const mounted = useRef(true),
    flight = useRef(false);
  const active = session?.turns?.at(-1);
  const current = active?.stages?.at(-1);
  const running = !!active && !terminal.has(active.status ?? "");
  const imageMode =
    image?.current_version?.modes?.find(
      (m) => m === "text2image" || m === "text_to_image",
    ) ?? "text2image";
  const available = (capability: string) =>
    models.filter(
      (m) =>
        m.capability === capability &&
        m.status === "active" &&
        m.provider_status === "active" &&
        m.credential_present &&
        m.current_version,
    );

  useEffect(() => {
    onLock(busy);
    return () => onLock(false);
  }, [busy, onLock]);
  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    void Promise.all([
      listAgentSessions({ pid: projectID }, { signal: controller.signal }),
      listProjectModels(
        { project_id: projectID, limit: 200 },
        { signal: controller.signal },
      ),
    ])
      .then(async ([list, catalog]) => {
        if (controller.signal.aborted) return;
        setSessions(list);
        setModels(catalog.items ?? []);
        if (list[0]?.id) {
          const saved = await getAgentSession(
            { pid: projectID, sid: list[0].id },
            { signal: controller.signal },
          );
          if (!controller.signal.aborted) setSession(checked(saved, projectID));
        }
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(readErrorMessage(e));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => {
      mounted.current = false;
      controller.abort();
    };
  }, [projectID]);

  function receive(value: API.Session) {
    const saved = checked(value, projectID);
    setSession((current) =>
      current &&
      current.id === saved.id &&
      (current.revision ?? 0) > (saved.revision ?? 0)
        ? current
        : saved,
    );
    setSessions((items) => [saved, ...items.filter((s) => s.id !== saved.id)]);
    return saved;
  }
  async function run(action: () => Promise<unknown>) {
    if (flight.current) return;
    flight.current = true;
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (e) {
      if (mounted.current) setError(readErrorMessage(e));
    } finally {
      flight.current = false;
      if (mounted.current) setBusy(false);
    }
  }

  useEffect(() => {
    if (
      !session?.id ||
      !["preparing", "running"].includes(active?.status ?? "")
    )
      return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (flight.current) {
        timer = setTimeout(() => void poll(), 1500);
        return;
      }
      try {
        const value = await advanceAgentSession(
          { pid: projectID, sid: session.id! },
          { signal: controller.signal },
        );
        if (!controller.signal.aborted) receive(value);
      } catch (e) {
        if (!controller.signal.aborted) setError(readErrorMessage(e));
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void poll(), 3000);
    };
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
    // The server owns phases; polling never confirms a charge.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectID, session?.id, active?.status, active?.phase]);

  function choice(
    model: API.ModelResponse,
    mode: string,
    params: PromptDraft["params"],
  ): API.ModelChoice {
    const values = { ...params };
    delete values.output_count;
    return { key: model.key, mode, params: values };
  }
  async function start() {
    if (!planner?.key || !image?.key || !message.trim()) return;
    let saved = session;
    if (!saved) {
      createKey.current ??= crypto.randomUUID();
      saved = receive(
        await createAgentSession(
          { pid: projectID },
          {
            request_id: createKey.current,
            title: Array.from(message.trim()).slice(0, 60).join(""),
          },
        ),
      );
      createKey.current = null;
    }
    const body: API.TurnInput = {
      message,
      planner: choice(planner, "text_to_text", plannerParams),
      image: choice(image, imageMode, imageParams),
      expected_revision: saved.revision,
    };
    const hash = JSON.stringify({ sid: saved.id, ...body });
    if (pending.current?.hash !== hash)
      pending.current = { hash, key: crypto.randomUUID() };
    body.request_id = pending.current.key;
    receive(await startAgentTurn({ pid: projectID, sid: saved.id! }, body));
    pending.current = null;
    setMessage("");
  }
  return (
    <section
      className="w-full min-w-0 space-y-4 rounded-2xl border bg-card p-4 text-left"
      aria-label="Agent 图片创作会话"
    >
      <div className="flex flex-wrap items-center gap-2">
        {headerActions}
        {projectControl}
        <span className="ml-auto text-xs text-muted-foreground">
          图片创作 · 会话仅自己可见
        </span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Select
          value={session?.id ?? "new"}
          disabled={busy || loading}
          onValueChange={(id) =>
            void run(async () => {
              if (id === "new") {
                setSession(null);
                return;
              }
              receive(await getAgentSession({ pid: projectID, sid: id }));
            })
          }
        >
          <SelectTrigger aria-label="Agent 会话" className="max-w-80">
            <SelectValue placeholder="新会话" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="new">新会话</SelectItem>
            {sessions.map((s) => (
              <SelectItem key={s.id} value={s.id!}>
                {s.title}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          variant="ghost"
          disabled={busy || loading}
          onClick={() =>
            void run(async () => {
              createKey.current ??= crypto.randomUUID();
              receive(
                await createAgentSession(
                  { pid: projectID },
                  { request_id: createKey.current, title: "新会话" },
                ),
              );
              createKey.current = null;
              setMessage("");
            })
          }
        >
          新建会话
        </Button>
      </div>
      {loading && <p role="status">正在读取会话与模型…</p>}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
          <Button
            variant="link"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                if (session?.id)
                  receive(
                    await getAgentSession({ pid: projectID, sid: session.id }),
                  );
              })
            }
          >
            重新读取
          </Button>
        </p>
      )}
      <div
        className="max-h-[50dvh] space-y-5 overflow-auto"
        aria-label="会话消息"
      >
        {session?.turns?.map((turn) => (
          <article key={turn.id} className="space-y-3 border-b pb-4">
            <p className="text-sm whitespace-pre-wrap">{turn.input?.message}</p>
            {turn.plan?.message && (
              <p className="text-sm whitespace-pre-wrap text-muted-foreground">
                {turn.plan.message}
              </p>
            )}
            {turn.plan?.prompt && (
              <Collapsible>
                <CollapsibleTrigger asChild>
                  <Button variant="ghost" size="sm">
                    查看图片提示词
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="p-2 text-sm whitespace-pre-wrap">
                  {turn.plan.prompt}
                </CollapsibleContent>
              </Collapsible>
            )}
            {turn.media_asset_id && (
              <ProjectMediaPreview
                projectID={projectID}
                id={turn.media_asset_id}
                kind="image"
                large
              />
            )}
            {turn.reply && (
              <p className="text-sm whitespace-pre-wrap">{turn.reply}</p>
            )}
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>{labels[turn.status ?? ""]}</span>
              {turn.stages?.map((stage) => (
                <Button
                  key={stage.operation_id}
                  variant="ghost"
                  size="sm"
                  onClick={() => setTask(stage.operation_id)}
                >
                  {phases[stage.phase ?? ""]} · 查看任务
                </Button>
              ))}
            </div>
            {turn.failure_code && (
              <p className="text-sm text-destructive">
                当前阶段未完成，请查看原任务。
                {turn.failure_code === "invalid_planner_result"
                  ? "规划结果不符合单图创作规则，未执行图片生成。"
                  : ""}
              </p>
            )}
          </article>
        ))}
      </div>
      {active?.status === "awaiting_confirmation" && current && (
        <div className="space-y-2 rounded-lg border p-3">
          <p className="text-sm">
            {phases[active.phase ?? ""]}预计费用：
            {money(current.quote_micros ?? 0)}
          </p>
          <p className="text-xs text-muted-foreground">
            仅确认当前阶段，后续生成与总结会再次确认。
          </p>
          <Button
            disabled={busy}
            onClick={() =>
              void run(async () =>
                receive(
                  await confirmAgentStage(
                    { pid: projectID, sid: session!.id! },
                    {
                      turn_id: active.id,
                      operation_id: current.operation_id,
                      expected_revision: session!.revision,
                    },
                  ),
                ),
              )
            }
          >
            确认{phases[active.phase ?? ""]}
          </Button>
        </div>
      )}
      {active?.status === "needs_reconciliation" && (
        <div role="status" className="space-y-2">
          <p className="text-sm">请求结果尚未确认，已暂停后续步骤。</p>
          <Button
            variant="outline"
            disabled={busy}
            onClick={() =>
              void run(async () => {
                receive(
                  await advanceAgentSession({
                    pid: projectID,
                    sid: session!.id!,
                  }),
                );
              })
            }
          >
            查询原任务
          </Button>
        </div>
      )}
      {running ? (
        <Button
          variant="ghost"
          disabled={busy}
          onClick={() =>
            void run(async () => {
              receive(
                await stopAgentSession(
                  { pid: projectID, sid: session!.id! },
                  { expected_revision: session!.revision },
                ),
              );
            })
          }
        >
          停止后续步骤
        </Button>
      ) : (
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            void run(start);
          }}
        >
          <Textarea
            aria-label="Agent 创作目标"
            placeholder="描述图片创作目标，或继续提出修改要求。"
            maxLength={4000}
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            disabled={busy}
          />
          <div className="flex flex-wrap gap-3">
            <div>
              <p className="text-xs text-muted-foreground">规划模型</p>
              <ModelPicker
                models={available("text.generate")}
                value={planner?.current_version?.id}
                disabled={busy || loading}
                onSelect={(m) => {
                  setPlanner(m);
                  setPlannerParams(
                    initialParams(
                      fieldsForMode(
                        m.current_version?.param_schema,
                        "text_to_text",
                      ),
                    ),
                  );
                }}
              />
            </div>
            <div>
              <p className="text-xs text-muted-foreground">图片模型</p>
              <ModelPicker
                models={available("image.generate").filter((m) =>
                  m.current_version?.modes?.some(
                    (mode) => mode === "text2image" || mode === "text_to_image",
                  ),
                )}
                value={image?.current_version?.id}
                disabled={busy || loading}
                onSelect={(m) => {
                  setImage(m);
                  setImageParams(
                    initialParams(
                      fieldsForMode(
                        m.current_version?.param_schema,
                        m.current_version?.modes?.find(
                          (mode) =>
                            mode === "text2image" || mode === "text_to_image",
                        ) ?? "text2image",
                      ),
                    ),
                  );
                }}
              />
            </div>
          </div>
          {(planner || image) && (
            <Collapsible>
              <CollapsibleTrigger asChild>
                <Button variant="ghost" type="button">
                  模型参数
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="grid gap-4 py-3 sm:grid-cols-2">
                {planner && (
                  <GenerationParameters
                    fields={fieldsForMode(
                      planner.current_version?.param_schema,
                      "text_to_text",
                    )}
                    params={plannerParams}
                    maxOutputs={1}
                    disabled={busy}
                    compact
                    onChange={setPlannerParams}
                  />
                )}{" "}
                {image && (
                  <GenerationParameters
                    fields={fieldsForMode(
                      image.current_version?.param_schema,
                      imageMode,
                    )}
                    params={imageParams}
                    maxOutputs={1}
                    disabled={busy}
                    compact
                    onChange={setImageParams}
                  />
                )}
              </CollapsibleContent>
            </Collapsible>
          )}
          <Button
            type="submit"
            disabled={
              busy ||
              loading ||
              !planner ||
              !image ||
              !message.trim() ||
              (session?.turns?.length ?? 0) >= 20
            }
          >
            准备规划报价
          </Button>
          <p className="text-xs text-muted-foreground">
            每轮最多生成一张图片。选择已启用模型后，先确认规划积分。
          </p>
        </form>
      )}
      {task && (
        <Dialog open onOpenChange={(open) => !open && setTask(undefined)}>
          <DialogContent className="max-h-[85dvh] overflow-auto">
            <DialogTitle>Agent 任务</DialogTitle>
            <TaskDetail projectID={projectID} id={task} />
          </DialogContent>
        </Dialog>
      )}
    </section>
  );
}
