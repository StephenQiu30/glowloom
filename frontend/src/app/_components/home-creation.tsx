"use client";

import dynamic from "next/dynamic";
import { useRef, useState, type ReactElement } from "react";
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
import type { Project } from "@/components/projects/project";
import type { GenerationSeed } from "@/components/generation/generation-dialog";
import { ModelPicker } from "@/components/generation/model-picker";
import { ProjectRequiredPopover } from "@/components/generation/project-required-popover";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldGroup } from "@/components/ui/field";
import { InputGroup, InputGroupAddon } from "@/components/ui/input-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectGroup,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useHomeDraftRecovery } from "./home-draft-recovery";
import { HomePrompt } from "./home-prompt";
import { ReferenceControls } from "@/components/generation/reference-controls";
import type { ComposerSession } from "@/components/generation/composer-session";
import {
  emptyPromptDraft,
  type PromptDraft,
} from "@/components/generation/prompt-document";

const AgentWorkspace = dynamic(() =>
  import("@/components/generation/agent-workspace").then(
    (module) => module.AgentWorkspace,
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
  executableCreationType,
  type CreationType,
} from "./creation-types";
import { CreationTypeFields } from "./creation-type-fields";

type Workspace = {
  project: Project;
  type: Exclude<CreationType, "agent">;
  seed: GenerationSeed;
  draftID?: string;
};

/** Page-owned entry; generation, references and pricing remain owned by RichGeneration. */
export function HomeCreation() {
  const session = useOptionalSession();
  const { selected: space } = useSpace();
  return (
    <ScopedHomeCreation
      key={`${session?.actor.id ?? "guest"}/${space?.id ?? "none"}/${space?.writable !== false}`}
    />
  );
}

function ScopedHomeCreation() {
  const session = useOptionalSession();
  const login = useLoginDialog();
  const { selected: space } = useSpace();
  const [mode, setMode] = useState("generate");
  const [type, setType] = useState<CreationType>("image");
  const [project, setProject] = useState<Project | null>(null);
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
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
    Partial<Record<CreationType | "canvas", PromptDraft>>
  >({});
  const [empty] = useState(emptyPromptDraft);
  const inputKey = `${subjectScope}/home-input:${type}`;
  const homeDraft = inputs[type] ?? empty;
  const canvasDraft = inputs.canvas ?? empty;
  const prompt = homeDraft.document.content
    .map((p) =>
      (p.content ?? [])
        .map((n) =>
          n.type === "text" ? n.text : n.type === "hardBreak" ? "\n" : "",
        )
        .join(""),
    )
    .join("\n");
  const canvasPrompt = canvasDraft.document.content
    .map((p) =>
      (p.content ?? []).map((n) => (n.type === "text" ? n.text : "")).join(""),
    )
    .join("\n");
  const canRecover = !!session && !!space?.writable;
  const inputRecovery = useHomeDraftRecovery(
    canRecover ? inputKey : null,
    (draft) => setInputs((current) => ({ ...current, [type]: draft })),
  );
  const canvasRecovery = useHomeDraftRecovery(
    canRecover ? `${subjectScope}/home-input:canvas` : null,
    (draft) => setInputs((current) => ({ ...current, canvas: draft })),
  );
  const currentEditor =
    homeEditor?.key === inputKey ? homeEditor.session : null;
  const activeType = creationTypes.find((item) => item.id === type)!;
  const supported = true;
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
  async function select(nextType: CreationType, nextProject = project) {
    if (locked || switchFlight.current) return;
    switchFlight.current = true;
    setSwitching(true);
    try {
      if (guard.current && !(await guard.current())) {
        setError("当前草稿尚未确认保存，请处理保存或未决生成后再切换。");
        return;
      }
      setError("");
      setType(nextType);
      setProject(nextProject);
      setWorkspace(
        nextProject && executableCreationType(nextType)
          ? {
              project: nextProject,
              type: nextType,
              seed: {
                project: nextProject,
                capability: creationCapability(nextType),
                creationType: nextType,
                prompt: "",
                composer: creationComposer(nextType, inputs[nextType] ?? empty),
              },
              draftID: rememberedDraft(nextProject.id, nextType),
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
    setProjectOpen(true);
  }
  const projectControl = session ? (
    <ProjectPicker
      compact
      value={project}
      disabled={locked || switching}
      open={projectOpen}
      onOpenChange={setProjectOpen}
      onChange={(next) => void select(type, next)}
    />
  ) : null;
  function projectEntry(
    trigger: ReactElement,
    title: string,
    description: string,
  ) {
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
    <Select
      value={type}
      disabled={locked || switching}
      onValueChange={(value) => void select(value as CreationType)}
    >
      <SelectTrigger
        aria-label="创作类型"
        variant="quiet"
        className="w-auto max-md:min-h-11"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {creationTypes.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );

  return (
    <section className="creation-hero" aria-label="开始创作">
      <h1>你好，今天想要创作什么？</h1>
      {mode === "canvas" ? canvasRecovery.notice : inputRecovery.notice}
      {error ? <FeedbackToast message={error} /> : null}
      <ToggleGroup
        type="single"
        variant="pill"
        size="hero"
        value={mode}
        disabled={locked || switching}
        onValueChange={(value) => {
          if (value) setMode(value);
        }}
        className="creation-toggle"
        aria-label="创作模式"
      >
        <ToggleGroupItem value="generate">生成</ToggleGroupItem>
        <ToggleGroupItem value="canvas">画布</ToggleGroupItem>
      </ToggleGroup>
      <div className="creation-stage" data-testid="creation-stage">
        <div hidden={mode !== "generate"} className="creation-panel">
          <div className="creation-tabs">
            {/* One active panel, owned by the selected type; switching first flushes its saved draft. */}
            <div>
              {type === "agent" && project ? (
                <AgentWorkspace
                  key={`${subjectScope}/${project.id}`}
                  projectID={project.id}
                  initialMessage={prompt}
                  onMessageChange={(message) => {
                    const draft = emptyPromptDraft();
                    draft.document.content = message
                      .split("\n")
                      .map((text) => ({
                        type: "paragraph" as const,
                        content: text ? [{ type: "text" as const, text }] : [],
                      }));
                    inputRecovery.change(draft);
                  }}
                  headerActions={typeControl}
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
                    <div className="creation-project-control">
                      {projectControl}
                    </div>
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
                        className="creation-composer"
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
                                projectEntry(
                                  <Button
                                    type="button"
                                    variant="reference"
                                    size="reference"
                                    aria-label="添加参考素材"
                                    onClick={supported ? undefined : prepare}
                                    disabled={locked || switching}
                                  >
                                    <Plus data-icon="inline-start" />
                                  </Button>,
                                  "添加参考素材",
                                  "选择项目后，可从项目素材或上传内容添加参考。",
                                )
                              )}
                            </InputGroupAddon>
                          )}
                          {type !== "motion" && (
                            <HomePrompt
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
                        <InputGroupAddon
                          align="block-end"
                          className="creation-tools"
                        >
                          <div className="creation-primary-tools">
                            {typeControl}
                            {supported ? (
                              <ModelPicker
                                models={[]}
                                value={null}
                                disabled={locked || switching}
                                onSelect={() => {}}
                                onChooseProject={prepare}
                              />
                            ) : (
                              <Button
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
                              size="icon"
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
                            {supported ? "提交前确认积分" : "暂未开放"}
                          </span>
                          <Button
                            type="submit"
                            size="icon"
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
                        </InputGroupAddon>
                      </InputGroup>
                    </Field>
                  </FieldGroup>
                </form>
              )}
            </div>
          </div>
        </div>
        <div
          hidden={mode !== "canvas"}
          className="creation-panel home-canvas-launch"
        >
          {mode === "canvas" && session && space ? (
            <RecentCanvases
              key={`${session.actor.id}/${space.id}`}
              presentation="cards"
            >
              <NewCanvasButton variant="tile">
                <Plus />
                <span>新建画布</span>
              </NewCanvasButton>
            </RecentCanvases>
          ) : (
            <div className="home-canvas-list">
              <NewCanvasButton variant="tile">
                <Plus />
                <span>新建画布</span>
              </NewCanvasButton>
            </div>
          )}
          <form
            onSubmit={(event) => {
              event.preventDefault();
              if (!canvasPrompt.trim()) return;
              if (!session) login();
              else setFeature("从提示词创建画布");
            }}
          >
            <InputGroup variant="composer" className="canvas-idea">
              <HomePrompt
                label="画布创作想法"
                contextKey={`${subjectScope}/home-input:canvas`}
                draft={canvasDraft}
                onChange={canvasRecovery.change}
                placeholder="输入想法或上传参考，和 Agent 一起创作"
                onSubmit={() => {
                  if (!canvasPrompt.trim()) return;
                  if (!session) login();
                  else setFeature("从提示词创建画布");
                }}
              />
              <InputGroupAddon align="inline-end">
                <Button
                  type="submit"
                  size="icon"
                  className="creation-submit"
                  disabled={!canvasPrompt.trim()}
                  aria-label="从想法创建画布"
                >
                  <ArrowUp />
                </Button>
              </InputGroupAddon>
            </InputGroup>
          </form>
        </div>
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
    </section>
  );
}

function creationComposer(type: CreationType, value: PromptDraft): PromptDraft {
  const draft = structuredClone(value);
  if (type === "music") {
    draft.mode =
      draft.params.music_mode === "instrumental"
        ? "music_instrumental"
        : "music_song";
    delete draft.params.music_mode;
    delete draft.params.music_auto_duration;
    draft.params.duration_ms ??= 60000;
  }
  if (type === "avatar" || type === "motion") {
    draft.mode = type;
    if (
      type === "avatar" &&
      typeof draft.params.avatar_action === "string" &&
      draft.params.avatar_action
    )
      draft.document.content = [
        {
          type: "paragraph",
          content: [{ type: "text", text: draft.params.avatar_action }],
        },
      ];
    delete draft.params.avatar_action;
  }
  return draft;
}
