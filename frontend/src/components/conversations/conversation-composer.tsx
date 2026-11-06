"use client";
import type { ConversationController } from "./use-conversation";
import {
  useEffect,
  useId,
  useImperativeHandle,
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
import { PromptEditor } from "@/components/generation/prompt-editor";
import type { ComposerSession } from "@/components/generation/composer-session";
import { canonical } from "@/components/generation/draft-save";
import { ReferenceControls } from "@/components/generation/reference-controls";
import { ReferenceStack } from "@/components/generation/reference-stack";
import { ModelPicker } from "@/components/generation/model-picker";
import { Toolbar, ToolbarGroup } from "@/components/ui/toolbar";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
  PopoverTitle,
} from "@/components/ui/popover";
import {
  draftText,
  isSameSkillVersion,
  isVersionedSkill,
  selectSkillVersion,
} from "./conversation-state";

export type ConversationComposerHandle = { focusPrompt: () => void };

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
  const [initial] = useState(() => structuredClone(c.draft));
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
    }),
    [session],
  );
  useEffect(() => {
    if (
      !session ||
      session.editor.isDestroyed ||
      canonical(session.snapshot().draft) === canonical(c.draft)
    )
      return;
    last.current = canonical(c.draft);
    session.adoptDraft(c.draft);
  }, [session, c.draft]);
  const unknown = c.phase === "unknown",
    submitting = c.phase === "submitting";
  const hasInput = !!draftText(c.draft).trim() || c.draft.references.length > 0;
  const label = c.active
    ? c.stopping
      ? "正在停止"
      : "停止"
    : unknown
      ? "核对原发送"
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
  return (
    <div
      className="conversation-composer relative w-full rounded-[24px] border bg-card p-4 text-left data-[welcome=true]:md:py-[15px]"
      data-welcome={welcome}
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
      <div
        data-slot="conversation-reference-input"
        data-expanded={expanded}
        className="flex min-h-[84px] min-w-0 items-start gap-3"
      >
        {session ? (
          <ReferenceControls
            session={session}
            draft={c.draft}
            projectID={projectID}
            model={c.model}
            compact
            home
            authorized={authorized && !c.denied}
            onChooseProject={authorized ? undefined : onChooseProject}
          />
        ) : (
          <ReferenceStack
            entries={[]}
            onAdd={onChooseProject}
            onSelect={() => {}}
            onRemove={() => {}}
          />
        )}
        <div
          data-slot="conversation-prompt"
          className={`min-w-0 flex-1 overflow-y-auto ${expanded ? "h-[min(50dvh,24rem)]" : "h-[84px]"}`}
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
              skillsFromEditor.current = true;
              setSkillsOpen(true);
            }
          }}
        >
          <PromptEditor
            label="对话创作输入"
            contextKey={contextKey}
            replicaId={replicaID}
            draft={initial}
            placeholder="输入想法、剧本或添加参考，支持 / 使用技能，@ 添加主体"
            className="prompt-editor h-full min-h-[84px] outline-none"
            inputGroup
            onReady={setSession}
            onChange={(snapshot) => {
              const value = canonical(snapshot.draft);
              if (last.current === value) return;
              last.current = value;
              c.changeDraft(snapshot.draft);
            }}
            onRequestSubmit={() => {
              if (c.canSend) void c.send();
            }}
          />
        </div>
      </div>
      <Toolbar
        className="mt-3 flex-wrap"
        aria-label="对话创作工具"
        role="group"
      >
        {headerActions ?? (
          <span
            className="flex h-(--toolbar-control-size) items-center gap-1.5 px-2 text-sm data-[welcome=true]:text-[var(--navigation-accent)]"
            data-welcome={welcome}
          >
            <Sparkles className="size-4" />
            Agent 模式
          </span>
        )}
        {projectControl}
        <Popover>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="toolbar"
              aria-label="创作设置"
            >
              <SlidersHorizontal className="size-4" />
              <span className="max-sm:sr-only">创作设置</span>
            </Button>
          </PopoverTrigger>
          <PopoverContent
            side="top"
            align="start"
            className="w-72 space-y-3 p-4"
          >
            <PopoverTitle>创作设置</PopoverTitle>
            {projectSetting}
            {welcome && (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setExpanded((value) => !value)}
              >
                {expanded ? "收起输入" : "展开输入"}
              </Button>
            )}
            <div className="space-y-1.5">
              <Label htmlFor={budgetID}>本轮预算上限（元）</Label>
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
            </div>
            <p className="text-xs leading-5 text-muted-foreground">
              对话模型用于理解需求。图片、视频等生成模型与参数会在具体方案中确认；实际费用按任务回执显示。
            </p>
          </PopoverContent>
        </Popover>
        <Popover
          open={skillsOpen}
          onOpenChange={(open) => {
            setSkillsOpen(open);
            if (!open) setSkillSearch("");
          }}
        >
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              aria-label="选择技能"
              size="toolbar"
            >
              <WandSparkles className="size-4" />
              <span className="max-sm:sr-only">
                技能
                {c.settings.skills.length ? ` ${c.settings.skills.length}` : ""}
              </span>
            </Button>
          </PopoverTrigger>
          <PopoverContent
            side="top"
            align="start"
            className="max-h-80 w-72 overflow-y-auto p-3"
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
          </PopoverContent>
        </Popover>
        <Button
          type="button"
          variant="ghost"
          size="toolbar-icon"
          aria-label="添加主体或素材引用"
          disabled={!session}
          onClick={() => {
            session?.editor.commands.focus();
            session?.editor.commands.insertContent("@");
          }}
        >
          <AtSign className="size-4" />
        </Button>
        {!welcome && (
          <Button
            type="button"
            variant="ghost"
            size="toolbar-icon"
            aria-label={expanded ? "收起输入" : "展开输入"}
            onClick={() => setExpanded((value) => !value)}
          >
            {expanded ? (
              <Minimize2 className="size-4" />
            ) : (
              <Maximize2 className="size-4" />
            )}
          </Button>
        )}
        <ToolbarGroup
          appearance="plain"
          aria-label="模型与发送"
          className="ml-auto max-w-full"
        >
          <span className="sr-only">对话模型</span>
          <ModelPicker
            label="对话模型"
            models={c.models}
            value={c.model?.current_version?.id}
            disabled={!authorized}
            onChooseProject={authorized ? undefined : onChooseProject}
            onSelect={(model) =>
              c.changeSettings({ ...c.settings, modelKey: model.key })
            }
          />
          <Button
            type="button"
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
          </Button>
        </ToolbarGroup>
      </Toolbar>
      <p id={hintID} className="sr-only">
        {c.sendReason ?? "发送当前需求"}
      </p>
    </div>
  );
}
