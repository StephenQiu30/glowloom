"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useConversationContext } from "@/components/conversations/conversation-context";
import {
  cloneElement,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ReactElement,
} from "react";
import {
  ArrowUp,
  AtSign,
  ChevronDown,
  Plus,
  SlidersHorizontal,
  Sparkles,
} from "lucide-react";
import { useOptionalSession } from "@/components/auth/session";
import { useLoginDialog } from "@/components/layout/basic-layout";
import { FeedbackToast } from "@/components/feedback/feedback-toast";
import { UnavailableDialog } from "@/components/availability/unavailable";
import { RecentCanvases } from "@/components/layout/recent-canvases";
import { useSpace } from "@/components/spaces/space-context";
import { NewCanvasButton } from "@/components/projects/new-canvas-button";
import { ProjectPicker } from "@/components/projects/project-picker";
import { type Project } from "@/components/projects/project";
import { useConversationScope } from "@/components/conversations/use-conversation-scope";
import type { GenerationSeed } from "@/components/generation/generation-dialog";
import { ComposerToolbar } from "@/components/generation/composer-toolbar";
import { ModelPicker } from "@/components/generation/model-picker";
import { ReferenceStack } from "@/components/generation/reference-stack";
import { ProjectRequiredPopover } from "@/components/generation/project-required-popover";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldGroup } from "@/components/ui/field";
import { InputGroup, InputGroupAddon } from "@/components/ui/input-group";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useHomeDraftRecovery } from "./home-draft-recovery";
import { CreationEntryPrompt } from "@/components/generation/creation-entry-prompt";
import { ReferenceControls } from "@/components/generation/reference-controls";
import type { ComposerSession } from "@/components/generation/composer-session";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";

const ConversationWorkspace = dynamic(() =>
  import("@/components/conversations/conversation-workspace").then(
    (module) => module.ConversationWorkspace,
  ),
);

const RichGeneration = dynamic(
  () =>
    import("@/components/generation/rich-generation").then(
      (module) => module.RichGeneration,
    ),
  {
    loading: () => (
      <Skeleton
        className="creation-editor-skeleton"
        aria-label="正在加载创作编辑器"
      />
    ),
  },
);
import {
  creationTypes,
  creationCapability,
  creationComposer,
  executableCreationType,
  type CreationType,
} from "@/components/generation/creation-types";
import { CreationTypeSelect } from "@/components/generation/creation-type-select";
import { CreationTypeFields } from "@/components/generation/creation-type-fields";

type CreationMode = "generate" | "canvas";

type Workspace = {
  project: Project;
  type: Exclude<CreationType, "agent">;
  seed: GenerationSeed;
  draftID?: string;
};

/** Page-owned entry; generation, references and pricing remain owned by RichGeneration. */
export function HomeCreation({
  initialType = "image",
}: {
  initialType?: CreationType;
}) {
  const session = useOptionalSession();
  const { selected: space } = useSpace();
  return (
    <ScopedHomeCreation
      initialType={initialType}
      key={`${session?.actor.id ?? "guest"}/${space?.id ?? "none"}/${space?.writable !== false}/${initialType}`}
    />
  );
}

function ScopedHomeCreation({ initialType }: { initialType: CreationType }) {
  const session = useOptionalSession();
  const login = useLoginDialog();
  const router = useRouter();
  const conversationMemory = useConversationContext();
  const { selected: space } = useSpace();
  const [mode, setMode] = useState<CreationMode>("generate");
  const [generationType, setGenerationType] = useState(initialType);
  const type = mode === "canvas" ? "agent" : generationType;
  const [project, setProject] = useState<Project | null>(null);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const independentType =
    type === "image" || type === "video" || type === "agent";
  const scope = useConversationScope(undefined, independentType);
  const independent = scope.data ?? null;
  const [projectOpen, setProjectOpen] = useState(false);
  const [locked, setLocked] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [error, setError] = useState("");
  const [feature, setFeature] = useState<string | null>(null);
  const guard = useRef<(() => Promise<boolean>) | null>(null);
  const switchFlight = useRef(false);
  const drafts = useRef(new Map<string, string>());
  const subjectScope = `${session?.actor.id ?? "guest"}/${space?.id ?? "none"}`;
  const [homeEditor, setHomeEditor] = useState<{
    key: string;
    session: ComposerSession;
  } | null>(null);
  const [inputs, setInputs] = useState<
    Partial<Record<CreationType, PromptDraft>>
  >(() => {
    const pending = conversationMemory?.readDraft("/new");
    return pending ? { agent: pending } : {};
  });
  const [empty] = useState(emptyPromptDraft);
  const inputKey = `${subjectScope}/home-input:${type}`;
  const homeDraft = inputs[type] ?? empty;
  const prompt = homeDraft.document.content
    .map((p) =>
      (p.content ?? [])
        .map((n) =>
          n.type === "text" ? n.text : n.type === "hardBreak" ? "\n" : "",
        )
        .join(""),
    )
    .join("\n");
  const canRecover = !!session && !!space?.writable;
  const inputRecovery = useHomeDraftRecovery(
    canRecover ? inputKey : null,
    (draft) => setInputs((current) => ({ ...current, [type]: draft })),
    mode === "canvas",
  );
  const currentEditor =
    homeEditor?.key === inputKey ? homeEditor.session : null;
  const activeType = creationTypes.find((item) => item.id === type)!;
  const supported = true;
  const openIndependent = useEffectEvent((context: Project) => {
    if ((type === "image" || type === "video") && workspace?.type !== type)
      void select(type, project, context);
  });
  useEffect(() => {
    if (canRecover && independent) openIndependent(independent);
  }, [canRecover, independent, type]);
  const acceptsReferences =
    type === "image" ||
    type === "video" ||
    (type === "audio" && homeDraft.mode === "omni_audio");
  const hasInput = !!prompt.trim() || homeDraft.references.length > 0;

  function rememberedDraft(projectID: string, kind: CreationType) {
    return drafts.current.get(`${subjectScope}/${projectID}/${kind}`);
  }
  function rememberDraft(projectID: string, kind: CreationType, id: string) {
    drafts.current.set(`${subjectScope}/${projectID}/${kind}`, id);
  }
  async function select(
    nextType: CreationType,
    nextProject = project,
    context = independent,
    nextMode = mode,
  ) {
    const execution =
      nextType === "image" || nextType === "video" ? context : nextProject;
    if (locked || switchFlight.current) return;
    switchFlight.current = true;
    setSwitching(true);
    try {
      if (guard.current && !(await guard.current())) {
        setError("当前草稿尚未确认保存，请处理保存或未决生成后再切换。");
        return;
      }
      setError("");
      if (type === "agent") conversationMemory?.saveDraft("/new", homeDraft);
      if (nextMode === "generate") setGenerationType(nextType);
      setMode(nextMode);
      setProject(nextProject);
      setWorkspace(
        execution && executableCreationType(nextType)
          ? {
              project: execution,
              type: nextType,
              seed: {
                project: execution,
                capability: creationCapability(nextType),
                creationType: nextType,
                prompt: "",
                composer: creationComposer(nextType, inputs[nextType] ?? empty),
              },
              draftID: rememberedDraft(execution.id, nextType),
            }
          : null,
      );
    } catch {
      setError("草稿保存尚未确认，当前输入已保留，请重试。");
    } finally {
      switchFlight.current = false;
      setSwitching(false);
    }
  }
  function prepare() {
    if (!supported) {
      setFeature(activeType.label);
      return;
    }
    if (!session) {
      login();
      return;
    }
    if (independentType) {
      setError("");
      if (independent) void select(type);
      else void scope.mutate();
      return;
    }
    setProjectOpen(true);
  }
  const projectControl =
    session && !independentType ? (
      <ProjectPicker
        compact
        value={project}
        disabled={locked || switching}
        open={projectOpen}
        onOpenChange={setProjectOpen}
        onChange={(next) => {
          void select(type, next);
        }}
      />
    ) : null;
  function projectEntry(
    trigger: ReactElement,
    title: string,
    description: string,
  ) {
    if (independentType) {
      return cloneElement(trigger as ReactElement<{ onClick?: () => void }>, {
        onClick: prepare,
      });
    }
    return supported ? (
      <ProjectRequiredPopover
        title={title}
        description={description}
        onChooseProject={prepare}
      >
        {trigger}
      </ProjectRequiredPopover>
    ) : (
      trigger
    );
  }
  const typeControl = (
    <CreationTypeSelect
      value={type}
      disabled={locked || switching}
      onChange={(value) => void select(value)}
    />
  );
  return (
    <section className="creation-hero" data-mode={mode} aria-label="开始创作">
      <h1>你好，今天想要创作什么？</h1>
      {inputRecovery.notice}
      {error || scope.error ? (
        <FeedbackToast message={error || "创作空间暂不可用，请重试。"} />
      ) : null}
      <ToggleGroup
        type="single"
        variant="pill"
        size="hero"
        value={mode}
        disabled={locked || switching}
        onValueChange={(value) => {
          if (value === "canvas" || value === "generate")
            void select(
              value === "canvas" ? "agent" : generationType,
              project,
              independent,
              value,
            );
        }}
        className="creation-toggle"
        aria-label="创作模式"
      >
        <ToggleGroupItem value="generate">生成</ToggleGroupItem>
        <ToggleGroupItem value="canvas">画布</ToggleGroupItem>
      </ToggleGroup>
      <div className="creation-stage" data-testid="creation-stage">
        {mode === "canvas" && (
          <div className="home-canvas-launch">
            <div className="mb-2 flex items-center justify-between gap-2">
              <h2 className="text-sm font-semibold">最近画布</h2>
              <Button asChild size="sm" variant="ghost" className="max-md:h-11">
                <Link href="/assets?view=canvas" aria-label="全部画布">
                  全部
                </Link>
              </Button>
            </div>
            {session && space ? (
              <RecentCanvases key={subjectScope} presentation="cards">
                <NewCanvasTile />
              </RecentCanvases>
            ) : (
              <div className="home-canvas-list">
                <NewCanvasTile />
              </div>
            )}
          </div>
        )}
        <div className="creation-panel">
          <div className="creation-tabs">
            {/* One active panel, owned by the selected type; switching first flushes its saved draft. */}
            <div>
              {type === "agent" ? (
                <ConversationWorkspace
                  key={`${subjectScope}/${independent?.id ?? "unscoped"}`}
                  ownerKey={subjectScope}
                  projectID={independent?.id ?? ""}
                  enabled={!!session && !!space?.writable && !!independent}
                  independent
                  initialDraft={homeDraft}
                  onDraftChange={inputRecovery.change}
                  embedded
                  compactEntry={mode === "canvas"}
                  homeGeneration={mode === "generate"}
                  onChooseProject={prepare}
                  onAccepted={(sid) => router.push(`/conversations/${sid}`)}
                  headerActions={mode === "generate" ? typeControl : undefined}
                  projectControl={projectControl}
                  onLock={setLocked}
                />
              ) : workspace ? (
                <RichGeneration
                  key={`${workspace.project.id}/${workspace.type}`}
                  projectID={workspace.project.id}
                  seed={workspace.seed}
                  draftID={workspace.draftID}
                  presentation="home"
                  headerActions={typeControl}
                  projectControl={
                    projectControl && (
                      <div className="creation-project-control">
                        {projectControl}
                      </div>
                    )
                  }
                  onLock={setLocked}
                  onDraftReady={(id) =>
                    rememberDraft(workspace.project.id, workspace.type, id)
                  }
                  onCloseGuard={(next) => {
                    guard.current = next;
                  }}
                />
              ) : (
                <form
                  onSubmit={(event) => {
                    event.preventDefault();
                    if (hasInput || !supported) prepare();
                  }}
                >
                  <FieldGroup>
                    <Field>
                      <InputGroup
                        variant="composer"
                        className="creation-input-surface creation-composer"
                      >
                        <div
                          className={
                            type === "motion"
                              ? "hidden"
                              : "creation-editor-body"
                          }
                        >
                          {acceptsReferences && (
                            <InputGroupAddon
                              align="inline-start"
                              className="creation-reference-tools max-w-1/2"
                            >
                              {currentEditor && session && space ? (
                                <ReferenceControls
                                  session={currentEditor}
                                  draft={homeDraft}
                                  projectID=""
                                  authorized
                                  compact
                                  home
                                  onChooseProject={prepare}
                                />
                              ) : (
                                <ReferenceStack
                                  entries={[]}
                                  disabled={locked || switching}
                                  addLabel="添加参考素材"
                                  renderAdd={(trigger) =>
                                    projectEntry(
                                      trigger,
                                      "添加参考素材",
                                      "选择项目后，可从项目素材或上传内容添加参考。",
                                    )
                                  }
                                  onAdd={() => {
                                    if (!supported) prepare();
                                  }}
                                  onSelect={() => {}}
                                  onRemove={() => {}}
                                />
                              )}
                            </InputGroupAddon>
                          )}
                          {type !== "motion" && (
                            <CreationEntryPrompt
                              key={inputKey}
                              contextKey={inputKey}
                              draft={homeDraft}
                              onChange={inputRecovery.change}
                              placeholder={
                                type === "audio" &&
                                homeDraft.mode === "omni_audio"
                                  ? "描述角色、台词、音效与时间段，@ 引用音频。"
                                  : activeType.placeholder
                              }
                              onReady={(value) =>
                                setHomeEditor({
                                  key: inputKey,
                                  session: value,
                                })
                              }
                              onSubmit={() => {
                                if (hasInput || !supported) prepare();
                              }}
                            />
                          )}
                        </div>
                        <CreationTypeFields
                          type={type}
                          draft={homeDraft}
                          onChange={inputRecovery.change}
                          onUnavailable={setFeature}
                        />
                        <ComposerToolbar
                          aria-label="生成工具"
                          className="creation-tools"
                        >
                          <div className="creation-primary-tools">
                            {typeControl}
                            {supported && !independentType ? (
                              <ModelPicker
                                models={[]}
                                value={null}
                                disabled={locked || switching}
                                onSelect={() => {}}
                                onChooseProject={prepare}
                              />
                            ) : (
                              <Button
                                size="toolbar"
                                type="button"
                                variant="ghost"
                                onClick={prepare}
                              >
                                <Sparkles data-icon="inline-start" />
                                选择模型
                                <ChevronDown data-icon="inline-end" />
                              </Button>
                            )}
                          </div>
                          {projectEntry(
                            <Button
                              size="toolbar"
                              type="button"
                              variant="ghost"
                              onClick={supported ? undefined : prepare}
                              disabled={locked || switching}
                            >
                              <SlidersHorizontal data-icon="inline-start" />
                              参数
                            </Button>,
                            "生成参数",
                            "选择项目和可用模型后，可配置比例、清晰度等生成参数。",
                          )}
                          {acceptsReferences && (
                            <Button
                              type="button"
                              variant="ghost"
                              size="toolbar-icon"
                              aria-label="添加主体或素材引用"
                              onClick={() => {
                                if (!session) login();
                                else
                                  currentEditor?.editor
                                    .chain()
                                    .focus(undefined, { scrollIntoView: false })
                                    .insertContent("@")
                                    .run();
                              }}
                              disabled={locked || switching || !currentEditor}
                            >
                              <AtSign />
                            </Button>
                          )}
                          <div className="creation-project-control">
                            {projectControl}
                          </div>
                          <span className="creation-credit-hint">
                            {supported ? "费用待确认" : "暂未开放"}
                          </span>
                          <Button
                            type="submit"
                            size="toolbar-icon"
                            className="creation-submit"
                            disabled={supported && !hasInput}
                            aria-label={
                              supported
                                ? "开始生成"
                                : `${activeType.label}暂未开放`
                            }
                          >
                            <ArrowUp />
                          </Button>
                        </ComposerToolbar>
                      </InputGroup>
                    </Field>
                  </FieldGroup>
                </form>
              )}
            </div>
          </div>
        </div>
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
    </section>
  );
}

function NewCanvasTile() {
  return (
    <NewCanvasButton variant="tile" className="home-canvas-card" type="button">
      <Plus aria-hidden="true" data-icon="inline-start" />
      <span className="home-canvas-card-title">新建画布</span>
    </NewCanvasButton>
  );
}
