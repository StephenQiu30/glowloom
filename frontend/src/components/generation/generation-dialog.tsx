"use client";
import { useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  History,
  ImagePlus,
  LoaderCircle,
  RotateCw,
  X,
} from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { ProjectPicker } from "@/components/projects/project-picker";
import { isProjectID, type Project } from "@/components/projects/project";
import {
  ProjectMediaPicker,
  ProjectMediaPreview,
} from "@/components/media/project-media";
import { SubjectList } from "@/components/subjects/subject-list";
import { subjectImage, subjectText } from "@/components/subjects/contracts";
import { AuthError } from "@/components/auth/request";
import {
  listProjectModels,
  createFreeQuote,
  confirmOperationQuote,
  getOperation,
  cancelOperation,
  listProjectOperations,
} from "@/components/creation/generated/creation";
import {
  activeTask,
  fieldsForMode,
  flowError,
  initialParams,
  money,
  requireQuote,
  requireTask,
  taskLabels,
  type Quote,
  type Task,
} from "./contracts";
const referenceLabels: Record<string, string> = {
  reference_image: "参考图片",
  subject: "主体参考图",
  first_frame: "首帧图片",
  last_frame: "尾帧图片",
  reference_video: "参考视频",
  reference_audio: "参考音频",
};
export type GenerationSeed = {
  prompt: string;
  capability: "image" | "video";
  source?: CreationAPI.CanvasSource;
  project?: Project;
  reuse?: Task;
  draft?: {
    aspect_ratio: string;
    candidate_count: number;
    negative_prompt: string;
  };
  pickSubject?: boolean;
  subject?: import("@/components/subjects/contracts").Subject;
};
export function GenerationDialog({
  seed,
  onClose,
  onAdopt,
}: {
  seed: GenerationSeed;
  onClose: () => void;
  onAdopt?: (assetID: string, kind: string) => Promise<void> | void;
}) {
  const [project, setProject] = useState<Project | null>(seed.project ?? null);
  const [locked, setLocked] = useState(false);
  useEffect(() => {
    if (!locked) return;
    const guard = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [locked]);
  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v && !locked) onClose();
      }}
    >
      <DialogContent
        showCloseButton={!locked}
        className="generation-dialog sm:max-w-4xl"
        onInteractOutside={(e) => e.preventDefault()}
      >
        <DialogTitle>
          {seed.capability === "video" ? "视频生成" : "图片生成"}
        </DialogTitle>
        <DialogDescription>
          选择模型与参考素材，确认报价后开始生成。任务保留在所选项目中。
        </DialogDescription>
        {!seed.project && (
          <ProjectPicker
            value={project}
            onChange={setProject}
            disabled={locked}
          />
        )}
        {project ? (
          <GenerationForm
            key={project.id}
            project={project}
            seed={seed}
            onAdopt={onAdopt}
            locked={locked}
            onLock={setLocked}
          />
        ) : (
          <div className="empty-state">
            <p>先选择一个项目，继续这次创作。</p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
function GenerationForm({
  project,
  seed,
  onAdopt,
  locked,
  onLock,
}: {
  project: Project;
  seed: GenerationSeed;
  onAdopt?: (assetID: string, kind: string) => Promise<void> | void;
  locked: boolean;
  onLock: (v: boolean) => void;
}) {
  const [reuse, setReuse] = useState<Task | undefined>(seed.reuse);
  const [models, setModels] = useState<CreationAPI.ModelResponse[]>([]),
    [modelKey, setModelKey] = useState(seed.reuse?.model_key ?? ""),
    [loadError, setLoadError] = useState(""),
    [catalogRetry, setCatalogRetry] = useState(0),
    [loading, setLoading] = useState(true);
  useEffect(() => {
    const c = new AbortController();
    async function load() {
      try {
        let cursor: string | undefined;
        const all: CreationAPI.ModelResponse[] = [];
        const seen = new Set<string>();
        do {
          const p = await listProjectModels(
            {
              project_id: project.id,
              capability: seed.capability + ".generate",
              limit: 100,
              cursor,
            },
            { signal: c.signal },
          );
          if (
            !Array.isArray(p.items) ||
            p.items.some(
              (m) =>
                !isProjectID(m.id) ||
                !m.key ||
                m.capability !== seed.capability + ".generate",
            )
          )
            throw new AuthError("invalid_receipt");
          all.push(...p.items);
          cursor = p.next_cursor;
          if (cursor) {
            if (seen.has(cursor)) throw new AuthError("invalid_receipt");
            seen.add(cursor);
          }
        } while (cursor && !c.signal.aborted);
        if (!c.signal.aborted) {
          setModels(all);
          setLoadError("");
        }
      } catch (e) {
        if (!c.signal.aborted) setLoadError(flowError(e));
      } finally {
        if (!c.signal.aborted) setLoading(false);
      }
    }
    void load();
    return () => c.abort();
  }, [project.id, seed.capability, catalogRetry]);
  const model = models.find((m) => m.key === modelKey);
  return (
    <>
      {loading ? (
        <p role="status">正在读取可用模型…</p>
      ) : loadError ? (
        <div role="alert">
          {loadError}
          <Button
            onClick={() => {
              setLoading(true);
              setCatalogRetry((n) => n + 1);
            }}
          >
            重试读取模型
          </Button>
        </div>
      ) : (
        <>
          <label className="form-label">
            生成模型
            <select
              disabled={locked}
              value={modelKey}
              onChange={(e) => setModelKey(e.target.value)}
              aria-label="生成模型"
            >
              <option value="">请选择模型</option>
              {models.map((m) => (
                <option
                  key={m.id}
                  value={m.key}
                  disabled={
                    m.status !== "active" ||
                    m.provider_status !== "active" ||
                    !m.current_version ||
                    !m.credential_present
                  }
                >
                  {m.display_name}
                  {m.status !== "active" || !m.credential_present
                    ? " · 当前不可用"
                    : ""}
                </option>
              ))}
            </select>
          </label>
          {models.length === 0 && (
            <p className="flow-notice">
              项目中没有匹配的模型。可刷新目录，或请管理员配置可用的
              {seed.capability === "image" ? "图片" : "视频"}模型。
            </p>
          )}
          {model && (
            <ModelForm
              key={model.key + (reuse?.id ?? "")}
              project={project}
              model={model}
              seed={{ ...seed, reuse }}
              onAdopt={onAdopt}
              locked={locked}
              onLock={onLock}
            />
          )}
        </>
      )}
      <TaskHistory
        projectID={project.id}
        capability={seed.capability + ".generate"}
        onAdopt={onAdopt}
        onReuse={(t) => {
          if (!locked) {
            setReuse(t);
            setModelKey(t.model_key ?? "");
          }
        }}
        locked={locked}
      />
    </>
  );
}
function ModelForm({
  project,
  model,
  seed,
  onAdopt,
  locked,
  onLock,
}: {
  project: Project;
  model: CreationAPI.ModelResponse;
  seed: GenerationSeed;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  locked: boolean;
  onLock: (v: boolean) => void;
}) {
  const [mode, setMode] = useState(
    seed.reuse?.mode ?? model.current_version?.modes?.[0] ?? "",
  );
  let schemaError = "";
  try {
    fieldsForMode(model.current_version?.param_schema, mode);
  } catch {
    schemaError = "模型参数配置不完整，请刷新目录或联系管理员。";
  }
  return (
    <>
      <label className="form-label">
        生成模式
        <select
          disabled={locked}
          aria-label="生成模式"
          value={mode}
          onChange={(e) => setMode(e.target.value)}
        >
          {model.current_version?.modes?.map((m) => (
            <option key={m} value={m}>
              {(
                {
                  text_to_image: "文生图",
                  image_to_image: "图生图",
                  text_to_video: "文生视频",
                  image_to_video: "图生视频",
                } as Record<string, string>
              )[m] ?? m}
            </option>
          ))}
        </select>
      </label>
      {schemaError ? (
        <p role="alert">{schemaError}</p>
      ) : (
        <PromptForm
          key={mode}
          project={project}
          model={model}
          mode={mode}
          seed={seed}
          onAdopt={onAdopt}
          onLock={onLock}
        />
      )}
    </>
  );
}
function PromptForm({
  project,
  model,
  mode,
  seed,
  onAdopt,
  onLock,
}: {
  project: Project;
  model: CreationAPI.ModelResponse;
  mode: string;
  seed: GenerationSeed;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  onLock: (v: boolean) => void;
}) {
  const [subjectPicker, setSubjectPicker] = useState(seed.pickSubject ?? false);
  const [rerun, setRerun] = useState(!!seed.reuse);
  const fields = fieldsForMode(model.current_version?.param_schema, mode);
  const [params, setParams] = useState(() => ({
      ...initialParams(fields),
      ...Object.fromEntries(
        fields
          .filter(
            (f) =>
              seed.draft &&
              ["aspect_ratio", "negative_prompt"].includes(f.field),
          )
          .map((f) => [
            f.field,
            seed.draft![f.field as "aspect_ratio" | "negative_prompt"],
          ]),
      ),
      ...Object.fromEntries(
        fields
          .filter((f) => seed.reuse?.params?.[f.field] !== undefined)
          .map((f) => [
            f.field,
            seed.reuse!.params![f.field] as string | number | boolean,
          ]),
      ),
    })),
    [prompt, setPrompt] = useState(
      seed.reuse?.inputs?.find((i) => i.role === "prompt")?.text ?? seed.prompt,
    ),
    [count, setCount] = useState(
      seed.reuse?.output_count ?? seed.draft?.candidate_count ?? 1,
    ),
    [media, setMedia] = useState<CreationAPI.QuoteMediaInput[]>(
      seed.reuse?.inputs
        ?.filter((i) => i.media_asset_id)
        .map((i) => ({ role: i.role, media_asset_id: i.media_asset_id })) ??
        (() => {
          const image = seed.subject && subjectImage(seed.subject);
          const limits = model.current_version?.limits as
            { roles?: Record<string, { types?: string[] }> } | undefined;
          const role =
            model.input_roles?.find(
              (r) =>
                r === "subject" && limits?.roles?.[r]?.types?.includes("image"),
            ) ??
            model.input_roles?.find((r) =>
              limits?.roles?.[r]?.types?.includes("image"),
            );
          return image && role ? [{ role, media_asset_id: image }] : [];
        })(),
    ),
    [pickRole, setPickRole] = useState<string | null>(null),
    [quote, setQuote] = useState<Quote | null>(null),
    [task, setTask] = useState<string | null>(null),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [unconfirmed, setUnconfirmed] = useState(false),
    [now, setNow] = useState(() => Date.now());
  const intent = useRef<{
      key: string;
      body: CreationAPI.QuoteItemRequest;
    } | null>(null),
    confirmKey = useRef<string | null>(null),
    busy = useRef(false);
  useEffect(() => {
    if (!quote) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [quote]);
  const limits = model.current_version?.limits as
    | {
        max_outputs?: number;
        roles?: Record<string, { types?: string[]; max_count?: number }>;
      }
    | undefined;
  const maxCount = Math.min(150, limits?.max_outputs ?? 1);
  const locked = pending || unconfirmed || !!quote || !!task;
  useEffect(() => {
    onLock(pending || unconfirmed || !!quote);
    return () => onLock(false);
  }, [pending, unconfirmed, quote, onLock]);
  async function getQuote() {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    const op = intent.current ?? {
      key: crypto.randomUUID(),
      body: {
        model_key: model.key,
        capability: model.capability,
        mode,
        prompt,
        params,
        output_count: count,
        force_regenerate: rerun,
        media_inputs: media,
        source: seed.source,
      },
    };
    intent.current = op;
    try {
      const r = requireQuote(
        await createFreeQuote({ pid: project.id }, op.body, {
          headers: { "Idempotency-Key": op.key },
        }),
      );
      setQuote(r);
      setNow(Date.now());
      setUnconfirmed(false);
      confirmKey.current = crypto.randomUUID();
    } catch (e) {
      setError(flowError(e));
      const uncertain = !(
        e instanceof AuthError &&
        e.status >= 400 &&
        e.status < 500
      );
      setUnconfirmed(uncertain);
      if (!uncertain) intent.current = null;
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  async function confirm() {
    if (!quote || busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      const r = await confirmOperationQuote(
        { id: quote.operation_id },
        { project_id: project.id },
        { headers: { "Idempotency-Key": confirmKey.current! } },
      );
      if (
        !isProjectID(r.reservation_id) ||
        !Number.isSafeInteger(r.available_micros) ||
        !Number.isSafeInteger(r.budget_revision)
      )
        throw new AuthError("invalid_receipt");
      setTask(quote.operation_id);
      setQuote(null);
      setUnconfirmed(false);
      intent.current = null;
    } catch (e) {
      setError(flowError(e));
      setUnconfirmed(
        !(e instanceof AuthError && e.status >= 400 && e.status < 500),
      );
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  return (
    <>
      {subjectPicker && (
        <Dialog open onOpenChange={setSubjectPicker}>
          <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-4xl">
            <DialogTitle>选择角色主体</DialogTitle>
            <DialogDescription>
              添加角色描述；模型支持角色参考时，同时引用主图。
            </DialogDescription>
            <SubjectList
              projectID={project.id}
              onSelect={(s) => {
                const text = subjectText(s);
                setPrompt(
                  (p) => `${p}\n主体：${text.name}。${text.description ?? ""}`,
                );
                const image = subjectImage(s);
                const role =
                  model.input_roles?.find(
                    (r) =>
                      r === "subject" &&
                      limits?.roles?.[r]?.types?.includes("image"),
                  ) ??
                  model.input_roles?.find((r) =>
                    limits?.roles?.[r]?.types?.includes("image"),
                  );
                if (image && role)
                  setMedia((v) => [
                    ...v.filter((m) => m.role !== role),
                    { role, media_asset_id: image },
                  ]);
                setSubjectPicker(false);
              }}
            />
          </DialogContent>
        </Dialog>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void getQuote();
        }}
      >
        <fieldset disabled={locked}>
          <label className="form-label">
            提示词
            <Textarea
              aria-label="生成提示词"
              value={prompt}
              required
              maxLength={10000}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="描述主体、环境、光线与镜头…"
              className="min-h-28"
            />
          </label>
          <div className="generation-params">
            {fields.map((f) => (
              <label className="form-label" key={f.field}>
                {f.label}
                {f.required ? " *" : ""}
                {f.enum ? (
                  <select
                    aria-label={f.label}
                    required={f.required}
                    value={String(params[f.field] ?? "")}
                    onChange={(e) =>
                      setParams((p) => ({
                        ...p,
                        [f.field]:
                          f.type === "string"
                            ? e.target.value
                            : Number(e.target.value),
                      }))
                    }
                  >
                    <option value="">请选择</option>
                    {f.enum.map((v) => (
                      <option key={String(v)} value={String(v)}>
                        {String(v)}
                      </option>
                    ))}
                  </select>
                ) : f.type === "boolean" ? (
                  <Checkbox
                    aria-label={f.label}
                    checked={params[f.field] === true}
                    onCheckedChange={(v) =>
                      setParams((p) => ({ ...p, [f.field]: v === true }))
                    }
                  />
                ) : (
                  <Input
                    aria-label={f.label}
                    required={f.required}
                    type={f.type === "string" ? "text" : "number"}
                    min={f.min}
                    max={f.max}
                    step={f.step ?? (f.type === "integer" ? 1 : "any")}
                    value={String(params[f.field] ?? "")}
                    onChange={(e) =>
                      setParams((p) => {
                        const next = { ...p };
                        if (e.target.value === "") delete next[f.field];
                        else
                          next[f.field] =
                            f.type === "string"
                              ? e.target.value
                              : Number(e.target.value);
                        return next;
                      })
                    }
                  />
                )}
              </label>
            ))}
            <label className="form-label">
              生成数量
              <Input
                aria-label="生成数量"
                type="number"
                min={1}
                max={maxCount}
                value={count}
                onChange={(e) => setCount(Number(e.target.value))}
              />
            </label>
          </div>
          <div className="reference-list">
            <Button
              type="button"
              variant="outline"
              onClick={() => setSubjectPicker(true)}
            >
              添加主体
            </Button>
            {media.map((m, i) => (
              <div key={m.media_asset_id} className="reference-thumb">
                <ProjectMediaPreview
                  projectID={project.id}
                  id={m.media_asset_id!}
                />
                <Button
                  variant="secondary"
                  size="icon-sm"
                  type="button"
                  aria-label={`移除参考 ${i + 1}`}
                  onClick={() => setMedia((a) => a.filter((_, n) => n !== i))}
                >
                  <X />
                </Button>
                <small>{referenceLabels[m.role ?? ""] ?? "参考素材"}</small>
              </div>
            ))}
            {model.input_roles
              ?.filter((role) => !!limits?.roles?.[role]?.types?.length)
              .map((role) => (
                <Button
                  key={role}
                  type="button"
                  variant="outline"
                  disabled={
                    media.filter((m) => m.role === role).length >=
                    (limits?.roles?.[role]?.max_count ?? 0)
                  }
                  onClick={() => setPickRole(role)}
                >
                  <ImagePlus />
                  {(
                    {
                      reference_image: "参考图片",
                      subject: "主体参考图",
                      first_frame: "首帧图片",
                      last_frame: "尾帧图片",
                      reference_video: "参考视频",
                      reference_audio: "参考音频",
                    } as Record<string, string>
                  )[role] ?? "添加参考素材"}
                </Button>
              ))}
          </div>
        </fieldset>
        {error && (
          <p role="alert" className="flow-error">
            {error}
          </p>
        )}
        {!quote && !task && (
          <Button
            type="submit"
            disabled={
              pending ||
              !prompt.trim() ||
              !Number.isInteger(count) ||
              count < 1 ||
              count > maxCount
            }
          >
            {pending ? <LoaderCircle className="animate-spin" /> : <ArrowUp />}
            {unconfirmed ? "重试原报价" : "获取生成报价"}
          </Button>
        )}
      </form>
      {quote && (
        <section className="quote-card" aria-label="生成报价">
          <h3>确认本次生成</h3>
          <p>
            预计费用 <strong>{money(quote.quote_micros)}</strong> · 可用余额{" "}
            {money(quote.available_micros)}
          </p>
          <p>
            {now >= Date.parse(quote.expires_at)
              ? "报价已过期，请重新获取。"
              : `有效至 ${new Date(quote.expires_at).toLocaleTimeString("zh-CN")}`}
          </p>
          {!quote.confirmable && (
            <p role="alert">
              当前报价无法确认，请检查余额、模型可用性或刷新报价。
            </p>
          )}
          <div className="flex gap-3">
            <Button
              variant="outline"
              disabled={pending || unconfirmed}
              onClick={() => {
                setQuote(null);
                intent.current = null;
                confirmKey.current = null;
                setError("");
              }}
            >
              返回编辑
            </Button>
            <Button
              disabled={
                pending ||
                (!unconfirmed &&
                  (!quote.confirmable || now >= Date.parse(quote.expires_at)))
              }
              onClick={() => void confirm()}
            >
              {pending
                ? "正在确认…"
                : unconfirmed
                  ? "重试原确认"
                  : "确认并开始生成"}
            </Button>
          </div>
        </section>
      )}
      {task && (
        <TaskDetail
          projectID={project.id}
          id={task}
          onAdopt={onAdopt}
          onReuse={(t) => {
            setPrompt(t.inputs?.find((i) => i.role === "prompt")?.text ?? "");
            setRerun(true);
            setTask(null);
          }}
        />
      )}
      {pickRole && (
        <ProjectMediaPicker
          projectID={project.id}
          kind={limits?.roles?.[pickRole]?.types?.[0] ?? "image"}
          onClose={() => setPickRole(null)}
          onSelect={(a) => {
            setMedia((v) => [
              ...v.filter((i) => i.media_asset_id !== a.id),
              { role: pickRole, media_asset_id: a.id },
            ]);
            setPickRole(null);
          }}
        />
      )}
    </>
  );
}
function TaskHistory({
  projectID,
  capability,
  onAdopt,
  onReuse,
  locked,
}: {
  projectID: string;
  capability: string;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  onReuse: (t: Task) => void;
  locked: boolean;
}) {
  const [open, setOpen] = useState(false),
    [items, setItems] = useState<CreationAPI.TaskSummary[]>([]),
    [cursor, setCursor] = useState<string | undefined>(),
    [error, setError] = useState(""),
    [pending, setPending] = useState(false),
    [selected, setSelected] = useState<string | null>(null);
  async function load(next?: string) {
    if (pending) return;
    setPending(true);
    setError("");
    try {
      const r = await listProjectOperations({
        pid: projectID,
        capability,
        limit: 30,
        cursor: next,
      });
      if (!Array.isArray(r.items)) throw new AuthError("invalid_receipt");
      r.items.forEach((i) => requireTask(i, projectID));
      setItems((v) => (next ? [...v, ...r.items!] : r.items!));
      setCursor(r.next_cursor);
      setOpen(true);
    } catch (e) {
      setError(flowError(e));
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="task-history">
      <Button variant="ghost" onClick={() => void load()} disabled={pending}>
        <History />
        {pending ? "正在读取…" : "查看生成历史"}
      </Button>
      {error && <p role="alert">{error}</p>}
      {open && (
        <>
          <div className="task-list">
            {items.map((t) => (
              <button key={t.id} onClick={() => setSelected(t.id!)}>
                <span>
                  {t.model_name} · {taskLabels[t.status!]}
                </span>
                <small>
                  {new Date(t.create_time!).toLocaleString("zh-CN")}
                </small>
              </button>
            ))}
          </div>
          {items.length === 0 && (
            <p>还没有生成任务。提交后的任务会保留在这里。</p>
          )}
          {cursor && (
            <Button
              variant="ghost"
              disabled={pending}
              onClick={() => void load(cursor)}
            >
              更多任务
            </Button>
          )}
        </>
      )}
      {selected && (
        <TaskDetail
          key={selected}
          id={selected}
          projectID={projectID}
          onAdopt={onAdopt}
          onReuse={
            locked
              ? undefined
              : (t) => {
                  onReuse(t);
                  setSelected(null);
                }
          }
        />
      )}
    </section>
  );
}
export function TaskDetail({
  projectID,
  id,
  onAdopt,
  onReuse,
}: {
  projectID: string;
  id: string;
  onAdopt?: (id: string, kind: string) => Promise<void> | void;
  onReuse?: (task: Task) => void;
}) {
  const [task, setTask] = useState<Task | null>(null),
    [error, setError] = useState(""),
    [retry, setRetry] = useState(0),
    [confirmCancel, setConfirmCancel] = useState(false),
    [pending, setPending] = useState(false);
  const cancelKey = useRef<string | null>(null);
  const [adopting, setAdopting] = useState(false);
  async function adopt(asset: string, kind: string) {
    if (adopting) return;
    setAdopting(true);
    setError("");
    try {
      await onAdopt?.(asset, kind);
    } catch (e) {
      setError(e instanceof Error ? e.message : "采用结果未确认，请重试。");
    } finally {
      setAdopting(false);
    }
  }
  useEffect(() => {
    const c = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function read() {
      try {
        const t = requireTask(
          await getOperation(
            { id, project_id: projectID },
            { signal: c.signal },
          ),
          projectID,
        );
        if (c.signal.aborted) return;
        if (t.id !== id) throw new AuthError("invalid_receipt");
        setTask(t);
        setError("");
        if (activeTask(t.status)) timer = setTimeout(() => void read(), 3000);
      } catch (e) {
        if (!c.signal.aborted) setError(flowError(e));
      }
    }
    void read();
    return () => {
      c.abort();
      clearTimeout(timer);
    };
  }, [id, projectID, retry]);
  async function cancel() {
    if (pending) return;
    setPending(true);
    cancelKey.current ??= crypto.randomUUID();
    try {
      const receipt = await cancelOperation(
        { id },
        { project_id: projectID },
        { headers: { "Idempotency-Key": cancelKey.current } },
      );
      if (
        receipt.accepted !== true ||
        receipt.target_id !== id ||
        receipt.target_type !== "operation" ||
        receipt.action !== "cancel"
      )
        throw new AuthError("invalid_receipt");
      setConfirmCancel(false);
      setRetry((n) => n + 1);
    } catch (e) {
      setError(flowError(e));
    } finally {
      setPending(false);
    }
  }
  return (
    <section className="task-detail" aria-live="polite">
      <div className="flex items-center gap-3">
        <h3>{task ? taskLabels[task.status] : "读取任务…"}</h3>
        {task && activeTask(task.status) && (
          <LoaderCircle className="animate-spin" size={16} />
        )}
        <Button
          variant="ghost"
          className="ml-auto"
          aria-label="刷新任务"
          onClick={() => setRetry((n) => n + 1)}
        >
          <RotateCw />
        </Button>
      </div>
      {error && (
        <p role="alert" className="flow-error">
          {error}
        </p>
      )}
      {task?.failure_code && (
        <p>
          生成未能完成，请核对输入或稍后重试。
          <small className="block text-muted-foreground">
            错误编号：{task.failure_code}
          </small>
        </p>
      )}
      {task?.status === "completed" &&
        !task.outputs?.some(
          (o) => o.moderation_status === "passed" && o.media_asset_id,
        ) && (
          <p>任务已结束，当前没有通过审核的可用结果。请调整描述后重新生成。</p>
        )}
      {task && ["unknown", "reconciling", "manual"].includes(task.status) && (
        <p>结果正在核对，请保留此任务，避免重复提交。</p>
      )}
      {task?.status === "cancelled" && (
        <p>任务已取消，你可以调整输入重新生成。</p>
      )}
      {task?.outputs && (
        <div className="generation-results">
          {task.outputs
            .filter((o) => o.media_asset_id && o.moderation_status === "passed")
            .map((o) => (
              <div key={o.id}>
                <div className="generation-result-preview">
                  <ProjectMediaPreview
                    projectID={projectID}
                    id={o.media_asset_id!}
                    kind={o.kind}
                    large
                  />
                </div>
                {onAdopt && (
                  <Button
                    disabled={adopting}
                    onClick={() => void adopt(o.media_asset_id!, o.kind!)}
                  >
                    {adopting ? "正在保存结果…" : "采用结果到画布"}
                  </Button>
                )}
              </div>
            ))}
        </div>
      )}
      {task && activeTask(task.status) && task.status !== "cancelling" && (
        <Button variant="outline" onClick={() => setConfirmCancel(true)}>
          取消任务
        </Button>
      )}
      {task &&
        ["failed", "cancelled", "expired", "completed"].includes(task.status) &&
        onReuse && (
          <Button variant="outline" onClick={() => onReuse(task)}>
            复用参数重新报价
          </Button>
        )}
      {confirmCancel && (
        <div className="quote-card">
          <h4>取消这个任务？</h4>
          <p>已开始的任务可能仍会完成，取消与费用结果以任务状态为准。</p>
          <div className="flex gap-3">
            <Button
              variant="outline"
              disabled={pending}
              onClick={() => setConfirmCancel(false)}
            >
              继续等待
            </Button>
            <Button disabled={pending} onClick={() => void cancel()}>
              {pending ? "正在请求…" : "确认取消"}
            </Button>
          </div>
        </div>
      )}
    </section>
  );
}
