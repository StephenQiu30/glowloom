"use client";

import {
  useEffect,
  useEffectEvent,
  useCallback,
  useRef,
  useState,
  type CSSProperties,
} from "react";
import dynamic from "next/dynamic";
import { History, Plus, Minimize2, WandSparkles, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  createCanvasAssistantSession,
  listCanvasAssistantSessions,
  readCanvasAssistantSession,
  appendCanvasAssistantTurn,
} from "@/api/canvasAssistant";
import { createPromptDraft, currentPromptSubmission } from "@/api/promptEditor";
import { getOperation } from "@/api/operations";
import { getMediaPreview } from "@/api/media";
import { draftRequest, requireSaved } from "@/components/generation/draft-save";
import type { ComposerSession } from "@/components/generation/composer-session";
import type { PromptDraft } from "@/components/generation/prompt-document";
import type {
  CanvasReferenceSource,
  CanvasSubjectReferenceSource,
} from "@/components/generation/reference-source";
import {
  RequestError,
  getSessionGeneration,
  getScopeGeneration,
} from "@/lib/request";
import { flowError } from "@/components/generation/contracts";
import {
  assistantSkills,
  assistantContinuation,
  assistantInitialDraft,
  requireAssistantSession,
  type AssistantSession,
  type AssistantSkill,
} from "./assistant-state";
import "./assistant.css";

const RichGeneration = dynamic(
  () =>
    import("@/components/generation/rich-generation").then(
      (m) => m.RichGeneration,
    ),
  { loading: () => <p role="status">正在加载输入框…</p> },
);
const TaskDetail = dynamic(
  () =>
    import("@/components/generation/generation-dialog").then(
      (m) => m.TaskDetail,
    ),
  { loading: () => <p role="status">正在读取对话…</p> },
);
type Props = {
  canvasID: string;
  projectID: string;
  onClose: () => void;
  initialDraft?: PromptDraft;
  referenceSources?: CanvasReferenceSource[];
  subjectSources?: CanvasSubjectReferenceSource[];
  onAdopt?: (assetID: string, kind: string) => Promise<void> | void;
  onAdoptText?: (operationID: string, outputID: string) => Promise<void> | void;
};
export function CanvasAssistantPanel(props: Props) {
  // Remounting at this boundary prevents old asynchronous identities from reaching a new canvas.
  return (
    <AssistantPanel key={`${props.canvasID}:${props.projectID}`} {...props} />
  );
}
function AssistantPanel({
  canvasID,
  projectID,
  onClose,
  initialDraft,
  onAdopt,
  onAdoptText,
  referenceSources,
  subjectSources,
}: Props) {
  const [sessions, setSessions] = useState<AssistantSession[]>([]),
    [current, setCurrent] = useState<AssistantSession | null>(null);
  const [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [reload, setReload] = useState(0);
  const [width, setWidth] = useState(400),
    [search, setSearch] = useState("");
  const [loaded, setLoaded] = useState(false),
    [skillsOpen, setSkillsOpen] = useState(false);
  const initialized = useRef(false),
    composer = useRef<ComposerSession | null>(null);
  const onComposerSession = useCallback((value: ComposerSession | null) => {
    composer.current = value;
  }, []);
  const onOperationReady = useCallback(
    (operationID: string, draftID: string) => {
      const update = (session: AssistantSession) => {
        const last = session.turns.at(-1);
        if (
          !last ||
          last.draft_id !== draftID ||
          last.operation_id === operationID
        )
          return session;
        return {
          ...session,
          turns: session.turns.map((t, i) =>
            i === session.turns.length - 1
              ? { ...t, operation_id: operationID }
              : t,
          ),
        };
      };
      setCurrent((old) => (old ? update(old) : old));
      setSessions((old) => old.map(update));
    },
    [],
  );
  const controller = useRef<AbortController | null>(null),
    mounted = useRef(true),
    guard = useRef<(() => Promise<boolean>) | null>(null);
  const intent = useRef<{
    sessionID: string;
    draftKey: string;
    draftID?: string;
    draft?: PromptDraft;
    expectedRevision?: number;
    previousOperationID?: string;
    skill: AssistantSkill;
    execution: "discussion" | "image";
  } | null>(null);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      controller.current?.abort();
    };
  }, []);
  useEffect(() => {
    const abort = new AbortController();
    controller.current = abort;
    void listCanvasAssistantSessions({ id: canvasID }, { signal: abort.signal })
      .then((data) => {
        if (abort.signal.aborted) return;
        if (!Array.isArray(data.items))
          throw new RequestError("invalid_receipt");
        const items = data.items.map((s) =>
          requireAssistantSession(s, canvasID, projectID),
        );
        setError("");
        setSessions(items);
        setCurrent(
          (old) =>
            items.find((s) => s.id === old?.id) ??
            items.find((s) => s.turns.every((t) => !t.operation_id)) ??
            items[0] ??
            null,
        );
        setLoaded(true);
      })
      .catch((e) => {
        if (!abort.signal.aborted) setError(flowError(e));
      });
    return () => abort.abort();
  }, [canvasID, projectID, reload]);
  function accept(session: AssistantSession) {
    setCurrent(session);
    setSessions((old) => [session, ...old.filter((s) => s.id !== session.id)]);
  }
  async function close() {
    if (pending) return;
    try {
      if (!guard.current || (await guard.current())) onClose();
      else setError("输入尚未保存，请恢复保存后再关闭。");
    } catch (e) {
      setError(flowError(e));
    }
  }
  async function choose(session: AssistantSession) {
    if (pending) return;
    try {
      if (guard.current && !(await guard.current())) throw new Error("unsaved");
      intent.current = null;
      setCurrent(session);
      setError("");
    } catch {
      setError("输入尚未保存，请恢复保存后再切换会话。");
    }
  }
  async function start(newSession: boolean) {
    if (pending) return;
    setPending(true);
    setError("");
    const abort = new AbortController();
    controller.current?.abort();
    controller.current = abort;
    const sessionGeneration = getSessionGeneration(),
      scopeGeneration = getScopeGeneration();
    const valid = () =>
      mounted.current &&
      !abort.signal.aborted &&
      sessionGeneration === getSessionGeneration() &&
      scopeGeneration === getScopeGeneration();
    try {
      if (guard.current && !(await guard.current())) throw new Error("unsaved");
      if (!intent.current)
        intent.current = {
          sessionID: !newSession && current ? current.id : crypto.randomUUID(),
          draftKey: crypto.randomUUID(),
          skill: "",
          execution: "discussion",
        };
      const work = intent.current;
      let session: AssistantSession;
      if (!newSession && current)
        session = requireAssistantSession(
          await readCanvasAssistantSession(
            { id: canvasID, sid: work.sessionID },
            { signal: abort.signal },
          ),
          canvasID,
          projectID,
        );
      else
        session = requireAssistantSession(
          await createCanvasAssistantSession(
            { id: canvasID },
            { id: work.sessionID, title: "新对话" },
            { signal: abort.signal },
          ),
          canvasID,
          projectID,
        );
      if (!valid()) return;
      if (
        work.draftID &&
        session.turns.some((t) => t.draft_id === work.draftID)
      ) {
        accept(session);
        intent.current = null;
        return;
      }
      if (!work.draft) {
        const last = session.turns.at(-1);
        if (last) {
          const submission = await currentPromptSubmission(
            {
              pid: projectID,
              owner_kind: "prompt_draft",
              draft_id: last.draft_id,
            },
            { signal: abort.signal },
          );
          const operationID =
            submission.submission?.operation_id ?? last.operation_id;
          if (!operationID)
            throw new RequestError("canvas_assistant_turn_active");
          const task = await getOperation(
            { id: operationID, project_id: projectID },
            { signal: abort.signal },
          );
          const previews = await Promise.all(
            (task.outputs ?? [])
              .filter(
                (o) =>
                  o.kind === "image" &&
                  o.moderation_status === "passed" &&
                  o.media_asset_id,
              )
              .map((o) =>
                getMediaPreview(
                  { pid: projectID, asset_id: o.media_asset_id! },
                  { signal: abort.signal },
                ),
              ),
          );
          if (previews.some((p) => !p.url || !p.asset))
            throw new RequestError("assistant_reply_unavailable");
          work.draft = assistantContinuation(
            task,
            projectID,
            operationID,
            previews.map((p) => p.asset!),
          );
          work.previousOperationID = operationID;
        } else
          work.draft = assistantInitialDraft(
            initialDraft,
            work.skill,
            work.execution,
          );
        work.expectedRevision = session.revision;
      }
      if (!valid()) return;
      if (!work.draftID) {
        const saved = requireSaved(
          await createPromptDraft(
            { pid: projectID },
            { expected_revision: 0, draft: draftRequest(work.draft) },
            {
              headers: { "Idempotency-Key": work.draftKey },
              signal: abort.signal,
            },
          ),
        );
        work.draftID = saved.draft_id;
      }
      if (!valid()) return;
      const updated = requireAssistantSession(
        await appendCanvasAssistantTurn(
          { id: canvasID, sid: session.id },
          {
            expected_revision: work.expectedRevision!,
            draft_id: work.draftID,
            skill: work.skill,
            execution: work.execution,
            ...(work.previousOperationID
              ? { previous_operation_id: work.previousOperationID }
              : {}),
          },
          { signal: abort.signal },
        ),
        canvasID,
        projectID,
      );
      if (valid()) {
        accept(updated);
        intent.current = null;
      }
    } catch (e) {
      if (
        valid() &&
        e instanceof RequestError &&
        e.code === "canvas_assistant_empty_session"
      ) {
        intent.current = null;
        initialized.current = false;
        setReload((v) => v + 1);
      }
      if (valid())
        setError(
          e instanceof RequestError &&
            e.code === "canvas_assistant_empty_session"
            ? "仅支持新建一个空会话，正在恢复已有会话。"
            : e instanceof RequestError &&
                e.code === "canvas_assistant_turn_active"
              ? "请先完成或取消当前任务，再继续对话。"
              : e instanceof RequestError && e.code === "assistant_context_full"
                ? "当前会话已达到上下文容量，请新建会话。"
                : flowError(e),
        );
    } finally {
      if (valid()) setPending(false);
    }
  }
  const initialize = useEffectEvent(() => {
    void start(!current);
  });
  useEffect(() => {
    if (loaded && !initialized.current) {
      initialized.current = true;
      if (!current?.turns.length)
        queueMicrotask(() => {
          if (mounted.current) initialize();
        });
    }
  }, [loaded, current]);
  const turn = current?.turns.at(-1);
  const emptySession = sessions.some((item) => {
    const value = current?.id === item.id ? current : item;
    return value.turns.every((t) => !t.operation_id);
  });
  const hasConversation = current?.turns.some((t) => t.operation_id);
  const filtered = assistantSkills.filter((s) => s.title.includes(search));
  function insertSkill(title: string, description: string) {
    if (!composer.current?.editor.isEditable) return;
    composer.current.editor
      .chain()
      .focus(undefined, { scrollIntoView: false })
      .insertContent([
        {
          type: "paragraph",
          content: [{ type: "text", text: `/ ${title}：${description}` }],
        },
      ])
      .run();
    setSkillsOpen(false);
  }
  const skillControl = (
    <Popover open={skillsOpen} onOpenChange={setSkillsOpen}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="sm" aria-label="使用技能">
          <WandSparkles />
          使用技能
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        side="top"
        className="canvas-assistant-skills"
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        onMouseDown={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="搜索技能"
          aria-label="搜索技能"
        />
        {filtered.map((s) => (
          <Button
            key={s.id}
            variant="ghost"
            className="canvas-assistant-skill"
            onClick={() => insertSkill(s.title, s.description)}
          >
            <span>{s.title}</span>
            <small>{s.description}</small>
          </Button>
        ))}
        <p className="text-xs text-muted-foreground">
          选择后可补充创作需求，再发送讨论方案。
        </p>
      </PopoverContent>
    </Popover>
  );
  const shortcuts = [
    assistantSkills[0],
    assistantSkills[1],
    assistantSkills[2],
    {
      id: "script",
      title: "剧本开发",
      description: "根据我的创意完善故事、人物和剧本",
    },
    assistantSkills[3],
  ];
  return (
    <aside
      className="canvas-assistant-panel"
      data-canvas-no-zoom
      data-canvas-shortcuts-ignore
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onKeyDownCapture={(event) => {
        if (
          event.key === "/" &&
          !event.nativeEvent.isComposing &&
          (event.target as HTMLElement).closest('[contenteditable="true"]')
        ) {
          event.preventDefault();
          setSkillsOpen(true);
        }
      }}
      aria-label="与 AI 对话"
      style={{ "--assistant-width": `${width}px` } as CSSProperties}
    >
      <div
        role="separator"
        aria-label="调整对话宽度"
        aria-orientation="vertical"
        aria-valuemin={320}
        aria-valuemax={800}
        aria-valuenow={width}
        tabIndex={0}
        className="canvas-assistant-resize"
        onKeyDown={(e) => {
          if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
            e.preventDefault();
            setWidth((w) =>
              Math.max(
                320,
                Math.min(800, w + (e.key === "ArrowLeft" ? 20 : -20)),
              ),
            );
          }
        }}
        onPointerDown={(e) => {
          e.currentTarget.setPointerCapture(e.pointerId);
        }}
        onPointerMove={(e) => {
          if (e.currentTarget.hasPointerCapture(e.pointerId))
            setWidth(
              Math.max(320, Math.min(800, window.innerWidth - e.clientX)),
            );
        }}
      />
      <header>
        <strong>
          {current?.title === "新对话"
            ? "新会话"
            : (current?.title ?? "新会话")}
        </strong>
        <div>
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="icon-sm" aria-label="会话列表">
                <History />
              </Button>
            </PopoverTrigger>
            <PopoverContent
              align="end"
              className="canvas-assistant-sessions"
              data-canvas-no-zoom
              data-canvas-shortcuts-ignore
              onMouseDown={(event) => event.stopPropagation()}
              onPointerDown={(event) => event.stopPropagation()}
            >
              {sessions.length ? (
                sessions.map((s) => (
                  <Button
                    key={s.id}
                    variant={s.id === current?.id ? "secondary" : "ghost"}
                    disabled={pending}
                    onClick={() => void choose(s)}
                  >
                    {s.title}
                    <small>{s.turns.length} 轮</small>
                  </Button>
                ))
              ) : (
                <p>还没有会话</p>
              )}
            </PopoverContent>
          </Popover>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="新建会话"
            title={emptySession ? "仅支持新建一个空会话" : "新建会话"}
            disabled={pending || !loaded || emptySession}
            onClick={() => {
              intent.current = null;
              void start(true);
            }}
          >
            <Plus />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="收起"
            disabled={pending}
            onClick={() => void close()}
          >
            <Minimize2 />
          </Button>
        </div>
      </header>
      <div className="canvas-assistant-scroll">
        {!hasConversation ? (
          <div className="canvas-assistant-empty">
            <h2>探索更多专业创作模式</h2>
            <div className="canvas-assistant-shortcuts">
              {shortcuts.map((item) => (
                <Button
                  key={item.id}
                  variant="secondary"
                  disabled={pending || !turn}
                  onClick={() => insertSkill(item.title, item.description)}
                >
                  ／ {item.title}
                </Button>
              ))}
            </div>
          </div>
        ) : null}
        {current?.turns
          .filter((t) => t.operation_id)
          .map((t, index) => (
            <section key={t.draft_id} className="canvas-assistant-turn">
              <h3>第 {index + 1} 轮</h3>
              {t.operation_id ? (
                <TaskDetail
                  projectID={projectID}
                  id={t.operation_id}
                  {...(onAdopt ? { onAdopt } : {})}
                  {...(onAdoptText ? { onAdoptText } : {})}
                />
              ) : null}
            </section>
          ))}
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>
              {error}
              <Button
                variant="ghost"
                size="sm"
                disabled={pending}
                onClick={() =>
                  intent.current
                    ? void start(
                        !current || intent.current.sessionID !== current.id,
                      )
                    : setReload((v) => v + 1)
                }
              >
                <RotateCw />
                重试
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
      </div>
      <footer>
        {turn ? (
          <RichGeneration
            key={turn.draft_id}
            presentation="assistant"
            headerActions={skillControl}
            onComposerSession={onComposerSession}
            onOperationReady={onOperationReady}
            projectID={projectID}
            draftID={turn.draft_id}
            seed={{
              prompt: "",
              capability: turn.execution === "image" ? "image" : "text",
            }}
            onCloseGuard={(g) => {
              guard.current = g;
            }}
            {...(referenceSources ? { referenceSources } : {})}
            {...(subjectSources ? { subjectSources } : {})}
            {...(onAdopt ? { onAdopt } : {})}
            {...(onAdoptText ? { onAdoptText } : {})}
          />
        ) : (
          <p role="status" className="canvas-assistant-preparing">
            {pending || !loaded
              ? "正在准备输入框…"
              : "输入框尚未就绪，请重试。"}
          </p>
        )}
        {turn?.operation_id ? (
          <Button
            variant="ghost"
            disabled={pending}
            onClick={() => void start(false)}
          >
            {pending ? "正在准备…" : "继续对话"}
          </Button>
        ) : null}
      </footer>
    </aside>
  );
}
