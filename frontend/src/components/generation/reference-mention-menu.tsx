"use client";

import { Fragment, useEffect, useEffectEvent, useRef, useState } from "react";
import {
  AudioLines,
  ChevronRight,
  ImageIcon,
  UserRound,
  Video,
  Plus,
  VideoIcon,
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
  CommandSeparator,
  CommandShortcut,
} from "@/components/ui/command";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from "@/components/ui/popover";
import { Empty, EmptyDescription } from "@/components/ui/empty";
import { ProjectMediaPreview } from "@/components/media/project-media";
import { inComposition, type ComposerSession } from "./composer-session";
import {
  labelKind,
  referenceLabel,
  type PromptDraft,
  type ReferenceBinding,
} from "./prompt-document";
import type { CanvasReferenceSource } from "./reference-source";
import { ReferenceThumbnail } from "./reference-thumbnail";

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
  canvas = false,
  projectID,
  onMotion,
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
  canvas?: boolean;
  projectID?: string;
  onMotion?: () => void;
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
    if (inComposition(session.editor, event)) {
      // Leave the native IME action intact while bypassing the editor keymaps.
      event.stopImmediatePropagation();
      return;
    }
    if (event.key === "ArrowLeft" && category) {
      event.preventDefault();
      event.stopImmediatePropagation();
      setColumn("categories");
      setCategory(null);
    } else if (event.key === "ArrowRight" && !query) {
      event.preventDefault();
      event.stopImmediatePropagation();
      const active = rootCommand.current
        ?.querySelector('[data-slot="command-item"][aria-selected="true"]')
        ?.getAttribute("data-value");
      const selected = categories.find((entry) => entry.kind === active);
      if (!selected && !category) return;
      setCategory(category ?? selected!.kind);
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
    if (ref.source.kind === "subject" || ref.source.kind === "canvas_subject")
      return ref.source.description;
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
  const quickHeading = "输入“@”引用素材";
  const rowClass = "min-h-11";
  function subjectRows(manage = false) {
    return (
      <>
        {manage && onCreateSubject && (
          <CommandItem
            value="create-subject"
            disabled={disabled || busy}
            onSelect={onCreateSubject}
          >
            <Plus />
            创建主体
          </CommandItem>
        )}
        {subjectLoading && (
          <CommandItem value="subjects-loading" disabled>
            正在读取主体
          </CommandItem>
        )}
        {subjectError && (
          <CommandItem value="subjects-error" disabled>
            {subjectError}
          </CommandItem>
        )}
        {onSubject &&
          subjectCandidates.map((subject) => (
            <CommandItem
              key={subject.id}
              value={`subject-${subject.id}`}
              disabled={disabled || busy}
              className={rowClass}
              onSelect={() => onSubject(subject)}
            >
              <ReferenceThumbnail aria-hidden="true">
                <UnifiedSubjectMedia
                  media={selectedSubjectMedia(subject, subject.primary)}
                  label={subject.name}
                />
              </ReferenceThumbnail>
              <span className="truncate">{subject.name}</span>
            </CommandItem>
          ))}
        {manage &&
          !subjectLoading &&
          !subjectError &&
          !subjectCandidates.length && (
            <CommandItem value="subjects-empty" disabled>
              {query ? "没有匹配的主体" : "暂无主体"}
            </CommandItem>
          )}
        {manage && onBrowseSubjects && (
          <CommandItem
            value="browse-subjects"
            disabled={disabled || busy}
            onSelect={onBrowseSubjects}
          >
            <UserRound />
            全部主体
            <CommandShortcut>
              <ChevronRight />
            </CommandShortcut>
          </CommandItem>
        )}
      </>
    );
  }
  function thumbnail(kind: Category, Icon: typeof ImageIcon, asset?: string) {
    return (
      <ReferenceThumbnail aria-hidden="true">
        {canvas &&
        projectID &&
        asset &&
        (kind === "image" || kind === "video") ? (
          <ProjectMediaPreview
            projectID={projectID}
            id={asset}
            kind={kind}
            compact
          />
        ) : (
          <Icon />
        )}
      </ReferenceThumbnail>
    );
  }
  function resourceRows(kind?: Category, readyOnly = false) {
    return categories
      .filter((entry) => !kind || kind === entry.kind)
      .map(({ kind: itemKind, icon: Icon }) => {
        const references = draft.references.filter(
          (ref) =>
            labelKind(ref) === itemKind &&
            matches(referenceLabel(draft.labelState, ref.id) + titleFor(ref)),
        );
        const connected = unbound.filter(
          (source) =>
            source.kind === itemKind &&
            matches(source.title) &&
            (!readyOnly || source.assetID),
        );
        return (
          <Fragment key={itemKind}>
            {references.map((ref) => (
              <CommandItem
                key={ref.id}
                value={ref.id}
                disabled={disabled || busy}
                data-checked={mentioned.has(ref.id)}
                className={rowClass}
                onSelect={() => onReference(ref)}
              >
                {thumbnail(
                  itemKind,
                  Icon,
                  ref.source.kind === "media" ? ref.source.sourceId : undefined,
                )}
                <span className="truncate">
                  {referenceLabel(draft.labelState, ref.id)}
                  {titleFor(ref) ? ` · ${titleFor(ref)}` : ""}
                </span>
                {mentioned.has(ref.id) && (
                  <span className="sr-only">已引用</span>
                )}
              </CommandItem>
            ))}
            {connected.map((source) => {
              const id = JSON.stringify([
                source.nodeID,
                source.assetID,
                "edgeID" in source ? source.edgeID : null,
              ]);
              return (
                <CommandItem
                  key={id}
                  value={id}
                  className={rowClass}
                  disabled={disabled || busy || !source.assetID}
                  onSelect={() => onSource(source)}
                >
                  {thumbnail(itemKind, Icon, source.assetID)}
                  <span className="truncate">{source.title}</span>
                  {!source.assetID && (
                    <CommandShortcut>待补内容</CommandShortcut>
                  )}
                </CommandItem>
              );
            })}
          </Fragment>
        );
      });
  }
  function candidates(kind?: Category) {
    const categoryName = categories.find((entry) => entry.kind === kind)?.label;
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
          aria-label={categoryName ? `${categoryName}引用候选` : "搜索引用候选"}
          className="max-h-[min(20rem,var(--radix-popover-content-available-height))]"
        >
          <CommandEmpty className="p-0">
            <Empty>
              <EmptyDescription>
                {query ? "没有匹配的参考" : "暂无相关节点"}
              </EmptyDescription>
            </Empty>
          </CommandEmpty>
          <CommandGroup
            heading={kind === "subject" && canvas ? "主体库" : undefined}
          >
            {(!kind || kind === "subject") &&
              subjectRows(!canvas && !!(onSubject || onCreateSubject))}
            {resourceRows(kind)}
          </CommandGroup>
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
      onValueChange={(value) => {
        if (!canvas)
          setCategory(
            categories.find((entry) => entry.kind === value)?.kind ?? null,
          );
      }}
      onMouseDown={(event) => event.preventDefault()}
    >
      <CommandList
        aria-label="引用类型"
        className="max-h-[min(28rem,var(--radix-popover-content-available-height))]"
      >
        {canvas ? (
          <>
            <CommandGroup
              heading={quickHeading}
              onPointerMove={() => setCategory(null)}
            >
              {subjectRows()}
              {resourceRows(undefined, true)}
            </CommandGroup>
            <CommandSeparator />
          </>
        ) : onSubject || onCreateSubject ? (
          <CommandGroup heading="可能@的内容">{subjectRows(true)}</CommandGroup>
        ) : null}
        <CommandGroup heading={canvas ? "添加参考" : "引用参考"}>
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
                  className={rowClass}
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
                  <CommandShortcut>
                    <ChevronRight />
                  </CommandShortcut>
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
                  sideOffset={4}
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
          {onMotion && (
            <CommandItem
              value="motion"
              className={rowClass}
              disabled={disabled || busy}
              onPointerMove={() => setCategory(null)}
              onSelect={onMotion}
            >
              <VideoIcon />
              运镜
            </CommandItem>
          )}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}
