"use client";
import type { ConversationController } from "./use-conversation";
import {
  useEffect,
  useId,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type Ref,
} from "react";
import {
  ArrowUp,
  AtSign,
  LoaderCircle,
  Maximize2,
  Minimize2,
  SlidersHorizontal,
  Sparkles,
  Square,
  WandSparkles,
  X,
  RefreshCw,
} from "lucide-react";
import { cn } from "cn";
import { PromptEditor } from "@/components/generation/prompt-editor";
import type {
  PromptDocument,
  ReferenceBinding,
} from "@/components/generation/prompt-document";
import type { ComposerSession } from "@/components/generation/composer-session";
import { canonical } from "@/components/generation/draft-save";
import { ReferenceControls } from "@/components/generation/reference-controls";
import { ReferenceStack } from "@/components/generation/reference-stack";
import { ModelPicker } from "@/components/generation/model-picker";
import {
  ComposerToolbar,
  ComposerToolbarActions,
  ComposerToolbarButton,
  ComposerPopoverContent,
} from "@/components/generation/composer-toolbar";
import { Field, FieldLabel } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Popover, PopoverTrigger, PopoverTitle } from "@/components/ui/popover";
import {
  draftText,
  isSameSkillVersion,
  isVersionedSkill,
  selectSkillVersion,
} from "./conversation-state";

export type DraftTextReference = (
  text: string,
) => { key: string; label: string } | undefined;

export function paragraphTextReference(
  paragraph: PromptDocument["content"][number],
  reference?: DraftTextReference,
) {
  const inline = paragraph.content?.[0];
  if (paragraph.content?.length === 1 && inline?.type === "text")
    return reference?.(inline.text);
}

export type ConversationComposerHandle = {
  focusPrompt: () => void;
  attachReference: (reference: ReferenceBinding) => boolean;
};

export function ConversationComposer({
  ref,
  conversation: c,
  contextKey,
  projectID,
  authorized,
  onChooseProject,
  headerActions,
  projectControl,
  projectSetting,
  welcome = false,
  compactEntry = false,
  canvasPanel = false,
  homeGeneration = false,
  textReference,
}: {
  ref?: Ref<ConversationComposerHandle>;
  conversation: ConversationController;
  contextKey: string;
  projectID: string;
  authorized: boolean;
  onChooseProject: () => void;
  headerActions?: ReactNode;
  projectControl?: ReactNode;
  projectSetting?: ReactNode;
  welcome?: boolean;
  compactEntry?: boolean;
  canvasPanel?: boolean;
  homeGeneration?: boolean;
  textReference?: DraftTextReference;
}) {
  const [session, setSession] = useState<ComposerSession | null>(null),
    [expanded, setExpanded] = useState(false);
  const [skillSearch, setSkillSearch] = useState("");
  const [skillsOpen, setSkillsOpen] = useState(false);
  const skillsFromEditor = useRef(false);
  const skillQuery = skillSearch.trim().toLowerCase();
  const skills = c.skills.filter(isVersionedSkill);
  const matchingSkills = skills.filter(
    (skill) =>
      !skillQuery ||
      skill.name?.toLowerCase().includes(skillQuery) ||
      skill.description?.toLowerCase().includes(skillQuery),
  );
  const [replicaID] = useState(() => crypto.randomUUID());
  const { editorDraft, textReferences } = useMemo(() => {
    const textReferences: {
      paragraph: PromptDocument["content"][number];
      key: string;
      label: string;
    }[] = [];
    if (!textReference) return { editorDraft: c.draft, textReferences };
    const content = c.draft.document.content.filter((paragraph) => {
      const entry = paragraphTextReference(paragraph, textReference);
      if (!entry) return true;
      textReferences.push({ ...entry, paragraph });
      return false;
    });
    return {
      editorDraft: {
        ...c.draft,
        document: {
          ...c.draft.document,
          content: content.length ? content : [{ type: "paragraph" as const }],
        },
      },
      textReferences,
    };
  }, [c.draft, textReference]);
  const [initial] = useState(() => structuredClone(editorDraft));
  const last = useRef(canonical(initial));
  const budgetID = useId(),
    hintID = useId();
  useImperativeHandle(
    ref,
    () => ({
      focusPrompt() {
        if (session && !session.editor.isDestroyed)
          session.editor.commands.focus();
      },
      attachReference(reference) {
        return session?.insertReference(reference, false) ?? false;
      },
    }),
    [session],
  );
  useEffect(() => {
    if (
      !session ||
      session.editor.isDestroyed ||
      canonical(session.snapshot().draft) === canonical(editorDraft)
    )
      return;
    last.current = canonical(editorDraft);
    session.adoptDraft(editorDraft);
  }, [session, editorDraft]);
  const unknown = c.phase === "unknown",
    submitting = c.phase === "submitting";
  const hasInput = !!draftText(c.draft).trim() || c.draft.references.length > 0;
  function openSkillShortcut() {
    skillsFromEditor.current = true;
    setSkillsOpen(true);
  }
  function mention() {
    session?.editor.commands.focus();
    session?.editor.commands.insertContent("@");
  }
  const label = c.active
    ? c.stopping
      ? "正在停止"
      : "停止"
    : unknown
      ? c.canRetrySubmission
        ? "重试发送"
        : "查看状态"
      : submitting
        ? "正在发送"
        : "发送";
  const disabled = c.active
    ? c.stopping || submitting
    : unknown
      ? submitting
      : authorized
        ? !c.canSend
        : !hasInput;
  const inlineReferences = compactEntry || !!textReference;
  const expandButton = !welcome && !inlineReferences && (
    <ComposerToolbarButton
      aria-label={expanded ? "收起输入" : "展开输入"}
      title={expanded ? "收起输入" : "展开输入"}
      size="toolbar-icon"
      className={homeGeneration ? "creation-expand" : undefined}
      onClick={() => setExpanded((value) => !value)}
    >
      {expanded ? <Minimize2 /> : <Maximize2 />}
    </ComposerToolbarButton>
  );
  const modelPicker = (
    <ModelPicker
      compact
      label={textReference && !compactEntry ? "创意模型" : "对话模型"}
      triggerClassName={compactEntry ? "conversation-model-trigger" : undefined}
      unavailableLabel={compactEntry ? "不可用" : undefined}
      models={c.models}
      value={c.model?.current_version?.id}
      disabled={c.denied}
      onChooseProject={authorized ? undefined : onChooseProject}
      onSelect={(model) =>
        c.changeSettings({ ...c.settings, modelKey: model.key })
      }
    />
  );
  const referenceControls = session ? (
    <ReferenceControls
      session={session}
      draft={c.draft}
      projectID={projectID}
      model={c.model}
      compact
      home
      inline={inlineReferences}
      allowDocuments
      authorized={authorized && !c.denied}
      onChooseProject={authorized ? undefined : onChooseProject}
    />
  ) : (
    <ReferenceStack
      entries={[]}
      inline={inlineReferences}
      onAdd={onChooseProject}
      onSelect={() => {}}
      onRemove={() => {}}
    />
  );
  return (
    <div
      className={cn(
        "conversation-composer creation-input-surface relative w-full p-4 text-left",
        textReference && "p-3",
      )}
      data-welcome={welcome}
      data-storyboard-input={!!textReference || undefined}
      data-compact-entry={compactEntry || undefined}
      data-home-generation={homeGeneration || undefined}
      role="group"
      aria-label="创作输入"
      onKeyDown={(event) => {
        if (
          event.key === "Escape" &&
          !event.defaultPrevented &&
          !event.nativeEvent.isComposing
        )
          setExpanded(false);
      }}
    >
      {homeGeneration && expandButton}
      {c.quote ? (
        <div className="mb-3 flex items-start gap-2 rounded-lg border-l-2 bg-muted/60 px-3 py-2 text-sm">
          <div className="min-w-0 flex-1">
            <p className="mb-1 text-xs text-muted-foreground">引用回复</p>
            <p className="line-clamp-2 break-words whitespace-pre-wrap">
              {c.quote.text}
            </p>
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            className="shrink-0 max-md:size-11"
            aria-label="移除引用"
            onClick={c.clearQuote}
          >
            <X />
          </Button>
        </div>
      ) : null}
      {textReferences.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-2" aria-label="镜头引用">
          {textReferences.map(({ key, label, paragraph }) => (
            <Button
              key={key}
              type="button"
              variant="secondary"
              size="sm"
              className="max-lg:h-11"
              aria-label={`移除 ${label}`}
              onClick={() => {
                const content = c.draft.document.content.filter(
                  (item) => item !== paragraph,
                );
                c.changeDraft({
                  ...c.draft,
                  document: {
                    ...c.draft.document,
                    content: content.length ? content : [{ type: "paragraph" }],
                  },
                });
                session?.editor.commands.focus();
              }}
            >
              {label}
              <X data-icon="inline-end" />
            </Button>
          ))}
        </div>
      )}
      <div
        data-slot="conversation-reference-input"
        data-expanded={expanded}
        className="flex min-w-0 items-start gap-3"
      >
        {!inlineReferences && referenceControls}
        <div
          data-slot="conversation-prompt"
          className={cn(
            "min-w-0 flex-1 overflow-y-auto",
            expanded
              ? "h-[50dvh]"
              : textReference
                ? "max-h-60 min-h-12"
                : "max-h-80 min-h-(--creation-prompt-min-height)",
          )}
          onKeyDownCapture={(event) => {
            const selection = session?.editor.state.selection;
            if (
              event.key === "/" &&
              !event.defaultPrevented &&
              !event.nativeEvent.isComposing &&
              !event.metaKey &&
              !event.ctrlKey &&
              !event.altKey &&
              selection?.empty &&
              selection.$from.parent.content.size === 0
            ) {
              event.preventDefault();
              openSkillShortcut();
            }
          }}
        >
          <PromptEditor
            label="对话创作输入"
            contextKey={contextKey}
            replicaId={replicaID}
            draft={initial}
            placeholder={
              textReference
                ? "输入想法，或用 @ 引用素材"
                : canvasPanel
                  ? "输入想法、剧本或参考，用 / 选技能，@ 引用素材"
                  : compactEntry
                    ? "输入想法、剧本或上传参考"
                    : homeGeneration
                      ? "输入想法、剧本或参考，用 / 选技能，@ 引用素材"
                      : "输入想法、剧本或上传参考，支持 / 使用技能，@ 添加主体，和 Agent 一起创作"
            }
            placeholderContent={
              inlineReferences ? undefined : (
                <>
                  {homeGeneration
                    ? "输入想法、剧本或参考，用 "
                    : "输入想法、剧本或上传参考，支持 "}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label="使用技能快捷入口"
                    onClick={openSkillShortcut}
                  >
                    /
                  </Button>{" "}
                  {homeGeneration ? "选技能， " : "使用技能， "}
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label="添加主体快捷入口"
                    disabled={!session}
                    onClick={mention}
                  >
                    @
                  </Button>{" "}
                  {homeGeneration ? "引用素材" : "添加主体，和 Agent 一起创作"}
                </>
              )
            }
            className={cn(
              "prompt-editor",
              textReference
                ? "min-h-12"
                : "min-h-(--creation-prompt-min-height)",
            )}
            inputGroup
            onReady={setSession}
            onChange={(snapshot) => {
              const value = canonical(snapshot.draft);
              if (last.current === value) return;
              last.current = value;
              c.changeDraft(
                textReferences.length
                  ? {
                      ...snapshot.draft,
                      document: {
                        ...snapshot.draft.document,
                        content: [
                          ...textReferences.map((item) => item.paragraph),
                          ...snapshot.draft.document.content,
                        ],
                      },
                    }
                  : snapshot.draft,
              );
            }}
            onRequestSubmit={() => {
              if (c.canSend) void c.send();
            }}
          />
        </div>
      </div>
      <ComposerToolbar
        className="mt-3 flex-wrap"
        aria-label="对话创作工具"
        role="group"
      >
        {inlineReferences && referenceControls}
        {inlineReferences && !canvasPanel && modelPicker}
        {headerActions ?? (
          <span
            className="flex h-(--toolbar-control-size) items-center gap-1.5 px-2 text-sm data-[welcome=true]:text-[var(--navigation-accent)]"
            data-welcome={welcome}
          >
            <Sparkles className="size-4" />
            Agent 模式
          </span>
        )}
        {!homeGeneration && projectControl}
        <Popover>
          <PopoverTrigger asChild>
            <ComposerToolbarButton
              type="button"
              variant="ghost"
              size="toolbar"
              aria-label="创作设置"
            >
              <SlidersHorizontal className="size-4" />
              <span
                className={cn(
                  "max-sm:sr-only",
                  (textReference || canvasPanel) && "sr-only",
                )}
              >
                创作设置
              </span>
            </ComposerToolbarButton>
          </PopoverTrigger>
          <ComposerPopoverContent
            width="compact"
            side="top"
            align="start"
            className="gap-3"
          >
            <PopoverTitle>创作设置</PopoverTitle>
            {projectSetting}
            {(welcome || inlineReferences) && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setExpanded((value) => !value)}
              >
                {expanded ? "收起输入" : "展开输入"}
              </Button>
            )}
            <Field>
              <FieldLabel htmlFor={budgetID}>本轮预算上限（元）</FieldLabel>
              <Input
                id={budgetID}
                inputMode="decimal"
                value={c.settings.budget}
                onChange={(event) =>
                  c.changeSettings({
                    ...c.settings,
                    budget: event.target.value,
                  })
                }
              />
            </Field>
            <p className="text-xs leading-5 text-muted-foreground">
              对话模型用于理解需求。图片、视频等生成模型与参数会在具体方案中确认；实际费用按任务回执显示。
            </p>
          </ComposerPopoverContent>
        </Popover>
        <Popover
          open={skillsOpen}
          onOpenChange={(open) => {
            setSkillsOpen(open);
            if (!open) setSkillSearch("");
          }}
        >
          <PopoverTrigger asChild>
            <ComposerToolbarButton
              type="button"
              variant="ghost"
              aria-label="选择技能"
              size="toolbar"
            >
              <WandSparkles className="size-4" />
              <span className={canvasPanel ? "sr-only" : "max-sm:sr-only"}>
                技能
                {c.settings.skills.length ? ` ${c.settings.skills.length}` : ""}
              </span>
            </ComposerToolbarButton>
          </PopoverTrigger>
          <ComposerPopoverContent
            width="compact"
            side="top"
            align="start"
            className="gap-3"
            onCloseAutoFocus={(event) => {
              if (!skillsFromEditor.current) return;
              skillsFromEditor.current = false;
              if (!session || session.editor.isDestroyed) return;
              event.preventDefault();
              session.editor.commands.focus();
            }}
          >
            <PopoverTitle className="mb-3">技能</PopoverTitle>
            {skills.length > 0 ? (
              <Input
                type="search"
                className="mb-2"
                aria-label="搜索技能"
                placeholder="搜索技能"
                value={skillSearch}
                onChange={(event) => setSkillSearch(event.target.value)}
              />
            ) : null}
            {!skills.length ? (
              <p className="text-sm text-muted-foreground">
                暂未读取到可用技能，可刷新连接后重试。
              </p>
            ) : matchingSkills.length === 0 ? (
              <p role="status" className="py-2 text-sm text-muted-foreground">
                没有匹配的技能
              </p>
            ) : (
              matchingSkills.map((skill) => {
                const selected = c.settings.skills.some((pin) =>
                  isSameSkillVersion(pin, skill),
                );
                return (
                  <Label
                    key={`${skill.skill_id}/${skill.version}`}
                    className="flex min-h-11 cursor-pointer items-center gap-2 py-2 text-sm"
                  >
                    <Checkbox
                      checked={selected}
                      disabled={!skill.enabled && !selected}
                      onCheckedChange={(checked) =>
                        c.changeSettings({
                          ...c.settings,
                          skills:
                            checked === true
                              ? selectSkillVersion(c.settings.skills, skill)
                              : c.settings.skills.filter(
                                  (pin) => pin.skill_id !== skill.skill_id,
                                ),
                        })
                      }
                    />
                    <span className="min-w-0">
                      <span className="block truncate">
                        {skill.name ?? "创作技能"}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {skill.version}
                        {!skill.enabled ? " · 当前不可用" : ""}
                      </span>
                      {skill.description ? (
                        <span className="mt-0.5 block text-xs text-muted-foreground">
                          {skill.description}
                        </span>
                      ) : null}
                    </span>
                  </Label>
                );
              })
            )}
            {c.settings.skills.length ? (
              <Button
                variant="ghost"
                size="sm"
                className="mt-2"
                onClick={() => c.changeSettings({ ...c.settings, skills: [] })}
              >
                清除技能选择
              </Button>
            ) : null}
          </ComposerPopoverContent>
        </Popover>
        {(!inlineReferences || canvasPanel) && (
          <ComposerToolbarButton
            type="button"
            variant="ghost"
            size="toolbar-icon"
            aria-label="添加主体或素材引用"
            disabled={!session}
            onClick={mention}
          >
            <AtSign className="size-4" />
          </ComposerToolbarButton>
        )}
        {homeGeneration && projectControl}
        {!homeGeneration && expandButton}
        <ComposerToolbarActions
          appearance="plain"
          aria-label="模型与发送"
          className="ml-auto max-w-full"
        >
          <span className="sr-only">对话模型</span>
          {(!inlineReferences || canvasPanel) && modelPicker}
          <ComposerToolbarButton
            type="button"
            variant="default"
            size="toolbar-icon"
            aria-label={label}
            aria-describedby={hintID}
            disabled={disabled}
            className="rounded-full"
            onClick={() => {
              if (!authorized) {
                onChooseProject();
                return;
              }
              if (c.active) void c.stop();
              else if (unknown && !c.canRetrySubmission) c.refresh();
              else void c.send();
            }}
          >
            {submitting || c.stopping ? (
              <LoaderCircle className="size-5 animate-spin motion-reduce:animate-none" />
            ) : c.active ? (
              <Square className="size-4 fill-current" />
            ) : unknown ? (
              <RefreshCw className="size-4" />
            ) : (
              <ArrowUp className="size-5" />
            )}
          </ComposerToolbarButton>
        </ComposerToolbarActions>
      </ComposerToolbar>
      <p id={hintID} className="sr-only">
        {c.sendReason ?? "发送当前需求"}
      </p>
    </div>
  );
}
