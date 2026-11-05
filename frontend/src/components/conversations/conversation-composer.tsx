"use client";
import type { ConversationController } from "./use-conversation";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import {
  ArrowUp,
  AtSign,
  ChevronDown,
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
import { draftText } from "./conversation-state";

export function ConversationComposer({
  conversation: c,
  contextKey,
  projectID,
  authorized,
  onChooseProject,
  headerActions,
  projectControl,
}: {
  conversation: ConversationController;
  contextKey: string;
  projectID: string;
  authorized: boolean;
  onChooseProject: () => void;
  headerActions?: ReactNode;
  projectControl?: ReactNode;
}) {
  const [session, setSession] = useState<ComposerSession | null>(null),
    [expanded, setExpanded] = useState(false);
  const [replicaID] = useState(() => crypto.randomUUID());
  const [initial] = useState(() => structuredClone(c.draft));
  const last = useRef(canonical(initial));
  const budgetID = useId(),
    hintID = useId();
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
      className="conversation-composer relative w-full rounded-[24px] border bg-card p-4 text-left"
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
            <p className="mb-1 text-xs text-muted-foreground">引用上一条回复</p>
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
      <div className="flex min-h-[84px] min-w-0 items-start gap-3">
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
          className={`min-w-0 flex-1 overflow-y-auto ${expanded ? "h-[min(50dvh,24rem)]" : "h-[84px]"}`}
        >
          <PromptEditor
            label="对话创作输入"
            contextKey={contextKey}
            replicaId={replicaID}
            draft={initial}
            placeholder="输入想法、剧本或添加参考，和 Agent 一起创作"
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
      <div className="mt-3 flex min-w-0 flex-wrap items-center gap-1">
        {headerActions ?? (
          <span className="flex h-9 items-center gap-1.5 px-2 text-sm">
            <Sparkles className="size-4" />
            Agent 模式
            <ChevronDown className="size-3.5 text-muted-foreground" />
          </span>
        )}
        {projectControl}
        <Popover>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              className="max-md:min-h-11"
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
              规划模型用于理解需求。图片、视频等生成模型与参数会在具体方案中确认；实际费用按任务回执显示。
            </p>
          </PopoverContent>
        </Popover>
        <Popover>
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              aria-label="选择技能"
              className="max-md:min-h-11"
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
          >
            <PopoverTitle className="mb-3">本轮使用的技能</PopoverTitle>
            {!c.skills.length ? (
              <p className="text-sm text-muted-foreground">
                暂未读取到可用技能，可刷新连接后重试。
              </p>
            ) : (
              c.skills.map((skill) => {
                if (!skill.skill_id || !skill.version || !skill.resource_hash)
                  return null;
                const selected = c.settings.skills.some(
                  (pin) =>
                    pin.skill_id === skill.skill_id &&
                    pin.version === skill.version &&
                    pin.resource_hash === skill.resource_hash,
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
                              ? [
                                  ...c.settings.skills.filter(
                                    (pin) => pin.skill_id !== skill.skill_id,
                                  ),
                                  {
                                    skill_id: skill.skill_id!,
                                    version: skill.version!,
                                    resource_hash: skill.resource_hash!,
                                  },
                                ]
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
          size="icon"
          className="max-md:size-11"
          aria-label="添加主体或素材引用"
          disabled={!session}
          onClick={() => {
            session?.editor.commands.focus();
            session?.editor.commands.insertContent("@");
          }}
        >
          <AtSign className="size-4" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="max-md:size-11"
          aria-label={expanded ? "收起输入" : "展开输入"}
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? (
            <Minimize2 className="size-4" />
          ) : (
            <Maximize2 className="size-4" />
          )}
        </Button>
        <div className="ml-auto flex min-w-0 items-center gap-1">
          <span className="sr-only">规划模型</span>
          <ModelPicker
            label="规划模型"
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
            size="icon-round"
            aria-label={label}
            aria-describedby={hintID}
            disabled={disabled}
            className="size-9 shrink-0 rounded-full max-md:size-11"
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
        </div>
      </div>
      <p id={hintID} className="sr-only">
        {c.sendReason ?? "发送当前需求"}
      </p>
    </div>
  );
}
