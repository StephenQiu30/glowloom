"use client";

import { useEffect, useEffectEvent, useRef, useState } from "react";
import {
  AudioLines,
  ChevronRight,
  ImageIcon,
  UserRound,
  Video,
  Plus,
} from "lucide-react";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import { selectedSubjectMedia } from "@/components/subjects/unified-contracts";
import { UnifiedSubjectMedia } from "@/components/subjects/unified-media";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from "@/components/ui/popover";
import type { ComposerSession } from "./composer-session";
import {
  labelKind,
  referenceLabel,
  type PromptDraft,
  type ReferenceBinding,
} from "./prompt-document";
import type { CanvasReferenceSource } from "./reference-source";

const categories = [
  { kind: "subject", label: "主体", icon: UserRound },
  { kind: "image", label: "图片", icon: ImageIcon },
  { kind: "video", label: "视频", icon: Video },
  { kind: "audio", label: "音频", icon: AudioLines },
] as const;
type Category = (typeof categories)[number]["kind"];

/** Both columns belong to the caret's popover; the editor keeps typing focus. */
export function ReferenceMentionMenu({
  session,
  draft,
  sources = [],
  query,
  busy,
  disabled,
  onReference,
  onSource,
  subjectCandidates = [],
  subjectLoading = false,
  subjectError = "",
  onSubject,
  onCreateSubject,
  onBrowseSubjects,
}: {
  session: ComposerSession;
  draft: PromptDraft;
  sources?: CanvasReferenceSource[];
  query: string;
  busy: boolean;
  disabled: boolean;
  onReference: (reference: ReferenceBinding) => void;
  onSource: (source: CanvasReferenceSource) => void;
  subjectCandidates?: UnifiedSubject[];
  subjectLoading?: boolean;
  subjectError?: string;
  onSubject?: (subject: UnifiedSubject) => void;
  onCreateSubject?: () => void;
  onBrowseSubjects?: () => void;
}) {
  const [category, setCategory] = useState<Category | null>(null);
  const [column, setColumn] = useState<"categories" | "items">("categories");
  const [narrow, setNarrow] = useState(false);
  const rootCommand = useRef<HTMLDivElement>(null);
  const itemsCommand = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const media = window.matchMedia("(max-width: 639px)");
    const update = () => setNarrow(media.matches);
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  const keyDown = useEffectEvent((event: KeyboardEvent) => {
    if (event.isComposing || session.editor.view.composing) return;
    if (event.key === "ArrowLeft" && category) {
      event.preventDefault();
      event.stopImmediatePropagation();
      setColumn("categories");
      setCategory(null);
    } else if (event.key === "ArrowRight" && !query) {
      event.preventDefault();
      event.stopImmediatePropagation();
      setCategory(category ?? "subject");
      setColumn("items");
    } else if (["ArrowDown", "ArrowUp", "Enter"].includes(event.key)) {
      event.preventDefault();
      event.stopImmediatePropagation();
      const target = query || column === "items" ? itemsCommand : rootCommand;
      target.current?.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: event.key,
          bubbles: true,
          cancelable: true,
        }),
      );
    }
  });
  useEffect(() => {
    const editor = session.editor.view.dom;
    const handle = (event: KeyboardEvent) => keyDown(event);
    editor.addEventListener("keydown", handle, true);
    return () => editor.removeEventListener("keydown", handle, true);
  }, [session]);
  const titleFor = (ref: ReferenceBinding) => {
    if (ref.source.kind === "subject") return ref.source.description;
    if (ref.source.kind !== "media") return "";
    const id = ref.source.sourceId;
    return (
      sources.find(
        (source) => source.assetID === id && source.kind === ref.mediaKind,
      )?.title ?? ""
    );
  };
  const matches = (text: string) =>
    text.toLocaleLowerCase().includes(query.toLocaleLowerCase());
  const unbound = sources.filter(
    (source) =>
      !draft.references.some(
        (ref) =>
          ref.source.kind === "media" &&
          ref.source.sourceId === source.assetID &&
          ref.mediaKind === source.kind,
      ),
  );
  const mentioned = new Set(
    draft.document.content.flatMap(
      (p) =>
        p.content?.flatMap((node) =>
          node.type === "reference" ? [node.attrs.referenceId] : [],
        ) ?? [],
    ),
  );
  function subjectItems() {
    return onSubject || onCreateSubject ? (
      <CommandGroup heading="可能@的内容">
        {onCreateSubject ? (
          <CommandItem
            value="create-subject"
            disabled={disabled || busy}
            onSelect={onCreateSubject}
          >
            <Plus />
            <span>创建主体</span>
          </CommandItem>
        ) : null}
        {subjectLoading ? (
          <CommandItem value="subjects-loading" disabled>
            正在读取主体
          </CommandItem>
        ) : null}
        {subjectError ? (
          <CommandItem value="subjects-error" disabled>
            {subjectError}
          </CommandItem>
        ) : null}
        {subjectCandidates.map((subject) => (
          <CommandItem
            key={`subject-${subject.id}`}
            value={`subject-${subject.id}`}
            disabled={disabled || busy}
            className="min-h-11"
            onSelect={() => onSubject?.(subject)}
          >
            <span className="relative size-8 shrink-0 overflow-hidden rounded-md bg-muted">
              <UnifiedSubjectMedia
                media={selectedSubjectMedia(subject, subject.primary)}
                label={subject.name}
              />
            </span>
            <span className="truncate">{subject.name}</span>
          </CommandItem>
        ))}
        {!subjectLoading && !subjectError && !subjectCandidates.length ? (
          <CommandItem value="subjects-empty" disabled>
            {query ? "没有匹配的主体" : "暂无主体"}
          </CommandItem>
        ) : null}
        {onBrowseSubjects ? (
          <CommandItem
            value="browse-subjects"
            disabled={disabled || busy}
            onSelect={onBrowseSubjects}
          >
            <UserRound />
            <span>全部主体</span>
            <ChevronRight className="ml-auto" />
          </CommandItem>
        ) : null}
      </CommandGroup>
    ) : null;
  }
  function candidates(kind?: Category) {
    return (
      <Command
        key={kind ?? "search"}
        ref={itemsCommand}
        shouldFilter={false}
        className="h-auto"
        onMouseDown={(event) => event.preventDefault()}
        onPointerMove={() => setColumn("items")}
      >
        <CommandList
          aria-label={
            kind
              ? `${categories.find((c) => c.kind === kind)!.label}引用候选`
              : "搜索引用候选"
          }
          className="max-h-[min(20rem,var(--radix-popover-content-available-height))]"
        >
          <CommandEmpty>暂无相关节点</CommandEmpty>
          {!kind || kind === "subject" ? subjectItems() : null}
          {categories
            .filter((c) => !kind || c.kind === kind)
            .map(({ kind: itemKind, label, icon: Icon }) => {
              const references = draft.references.filter(
                (ref) =>
                  labelKind(ref) === itemKind &&
                  matches(
                    referenceLabel(draft.labelState, ref.id) + titleFor(ref),
                  ),
              );
              const connected = unbound.filter(
                (source) => source.kind === itemKind && matches(source.title),
              );
              if (!references.length && !connected.length) return null;
              return (
                <CommandGroup key={itemKind} heading={label}>
                  {references.map((ref) => (
                    <CommandItem
                      key={ref.id}
                      value={ref.id}
                      disabled={disabled || busy}
                      data-checked={mentioned.has(ref.id)}
                      className="min-h-11 data-selected:bg-accent"
                      onSelect={() => onReference(ref)}
                    >
                      <Icon />
                      <span className="truncate">
                        {referenceLabel(draft.labelState, ref.id)}
                        {titleFor(ref) ? ` · ${titleFor(ref)}` : ""}
                      </span>
                      {mentioned.has(ref.id) && (
                        <span className="sr-only">已引用</span>
                      )}
                    </CommandItem>
                  ))}
                  {connected.map((source) => (
                    <CommandItem
                      key={source.nodeID}
                      value={source.nodeID}
                      className="min-h-11 data-selected:bg-accent"
                      disabled={disabled || busy || !source.assetID}
                      onSelect={() => onSource(source)}
                    >
                      <Icon />
                      <span className="truncate">{source.title}</span>
                      <span className="ml-auto text-xs text-muted-foreground">
                        {source.assetID ? "已连接" : "内容为空"}
                      </span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              );
            })}
        </CommandList>
      </Command>
    );
  }
  if (query) return candidates();
  return (
    <Command
      ref={rootCommand}
      className="h-auto"
      shouldFilter={false}
      disablePointerSelection={narrow}
      onValueChange={(value) =>
        setCategory(
          categories.some((entry) => entry.kind === value)
            ? (value as Category)
            : null,
        )
      }
      onMouseDown={(event) => event.preventDefault()}
    >
      <CommandList
        aria-label="引用类型"
        className="max-h-[min(28rem,var(--radix-popover-content-available-height))]"
      >
        {subjectItems()}
        <CommandGroup heading="引用参考">
          {categories.map(({ kind, label, icon: Icon }) => (
            <Popover
              key={kind}
              open={!narrow && category === kind}
              onOpenChange={(open) => {
                if (!open && category === kind) setCategory(null);
              }}
            >
              <PopoverAnchor asChild>
                <CommandItem
                  value={kind}
                  className="min-h-11 data-selected:bg-accent"
                  aria-haspopup="listbox"
                  aria-expanded={category === kind}
                  onPointerMoveCapture={(event) => {
                    if (!narrow && event.pointerType !== "touch") {
                      setCategory(kind);
                      setColumn("categories");
                    }
                  }}
                  onSelect={() => {
                    setCategory(kind);
                    setColumn("items");
                  }}
                >
                  <Icon />
                  <span>{label}</span>
                  <ChevronRight className="ml-auto" />
                </CommandItem>
              </PopoverAnchor>
              {narrow && category === kind ? (
                <div
                  className="pl-4"
                  onKeyDown={(event) => event.stopPropagation()}
                >
                  {candidates(kind)}
                </div>
              ) : null}
              {!narrow ? (
                <PopoverContent
                  side="right"
                  align="start"
                  sideOffset={12}
                  collisionPadding={16}
                  className="max-h-(--radix-popover-content-available-height) w-60 overflow-hidden p-0"
                  aria-label={`${label}引用候选`}
                  onClick={(event) => event.stopPropagation()}
                  onKeyDown={(event) => event.stopPropagation()}
                  onOpenAutoFocus={(event) => event.preventDefault()}
                  onCloseAutoFocus={(event) => event.preventDefault()}
                  onInteractOutside={(event) => {
                    if (
                      session.editor.view.dom.contains(event.target as Node) ||
                      rootCommand.current?.contains(event.target as Node)
                    )
                      event.preventDefault();
                  }}
                >
                  {candidates(kind)}
                </PopoverContent>
              ) : null}
            </Popover>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}
