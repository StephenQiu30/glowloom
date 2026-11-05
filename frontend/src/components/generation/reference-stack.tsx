"use client";
import {
  useRef,
  useState,
  type ReactNode,
  type Ref,
  type ReactElement,
} from "react";
import { Plus, X, ImageIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
export interface ReferenceStackEntry {
  id: string;
  label: string;
  preview?: ReactNode;
  status?: string;
}
export function ReferenceStack({
  entries,
  disabled = false,
  onAdd,
  onSelect,
  onRemove,
  addButtonRef,
  addLabel = "添加参考",
  renderAdd,
}: {
  entries: ReferenceStackEntry[];
  disabled?: boolean;
  onAdd: () => void;
  onSelect: (id: string) => void;
  onRemove: (id: string) => void;
  addButtonRef?: Ref<HTMLButtonElement>;
  addLabel?: string;
  renderAdd?: (trigger: ReactElement) => ReactNode;
}) {
  const [open, setOpen] = useState(false),
    [hovered, setHovered] = useState(false);
  const main = useRef<HTMLButtonElement>(null);
  const root = useRef<HTMLDivElement>(null);
  function remove(id: string) {
    onRemove(id);
    requestAnimationFrame(() =>
      root.current?.querySelector<HTMLButtonElement>("button")?.focus(),
    );
  }
  const expanded = entries.length > 0 && (open || hovered);
  const card =
    "absolute top-2 left-2 h-16 w-12 overflow-hidden rounded-[2px] border-[0.8px] border-foreground/80 bg-muted";
  const trigger = (
    <Button
      ref={entries.length ? main : addButtonRef}
      type="button"
      variant="ghost"
      disabled={disabled}
      className="relative h-[84px] w-16 rounded-none bg-transparent p-0 hover:bg-transparent"
      aria-label={
        entries.length ? `展开参考，已添加${entries.length}项` : addLabel
      }
      aria-expanded={entries.length ? expanded : undefined}
      onClick={() => (entries.length ? setOpen((value) => !value) : onAdd())}
    >
      <span
        className={`${card} pointer-events-none -rotate-[8deg] ${entries.length ? "" : "reference-stack-empty flex items-center justify-center border-0 bg-foreground/8 group-hover/reference:bg-foreground/12"}`}
      >
        {entries[0]?.preview ??
          (entries.length ? (
            <ImageIcon className="m-auto mt-5 size-4" />
          ) : (
            <Plus className="size-4" />
          ))}
      </span>
    </Button>
  );
  return (
    <div
      ref={root}
      className="reference-stack group/reference relative h-[84px] w-16 shrink-0"
      data-reference-count={entries.length}
      onPointerEnter={(event) => {
        if (event.pointerType !== "touch" && entries.length > 1)
          setHovered(true);
      }}
      onPointerLeave={() => setHovered(false)}
      onBlurCapture={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") {
          event.stopPropagation();
          setOpen(false);
          setHovered(false);
          main.current?.focus();
        }
      }}
    >
      {entries.length > 1 ? (
        <span
          aria-hidden
          className={`${card} pointer-events-none translate-x-1.5 rotate-[6deg]`}
        >
          {entries[1]?.preview}
        </span>
      ) : null}
      {renderAdd && !entries.length ? renderAdd(trigger) : trigger}
      {entries.length > 0 ? (
        <Button
          ref={addButtonRef}
          type="button"
          variant="secondary"
          disabled={disabled}
          aria-label="继续添加参考"
          onClick={onAdd}
          className="absolute right-0 bottom-1 z-10 size-7 rounded-full p-0 max-md:size-11"
        >
          <Plus className="size-4" />
        </Button>
      ) : null}
      {!expanded && entries.length === 1 ? (
        <Button
          type="button"
          variant="secondary"
          aria-label={`移除${entries[0]!.label}`}
          disabled={disabled}
          onClick={() => remove(entries[0]!.id)}
          className="absolute top-0 right-0 z-10 size-[22px] rounded-full p-0 opacity-0 group-focus-within/reference:opacity-100 group-hover/reference:opacity-100 max-md:size-11 max-md:opacity-100"
        >
          <X className="size-3" />
        </Button>
      ) : null}
      {expanded ? (
        <div
          role="group"
          aria-label="已添加的参考"
          className="absolute bottom-full left-0 z-40 mb-1 grid max-h-72 w-36 grid-cols-2 gap-1 overflow-y-auto rounded-lg border bg-popover p-1 text-popover-foreground shadow-sm"
        >
          {entries.map((entry) => (
            <div key={entry.id} className="relative w-16">
              <Button
                type="button"
                variant="ghost"
                disabled={disabled}
                aria-label={`查看${entry.label}`}
                onClick={() => onSelect(entry.id)}
                className="h-24 w-16 flex-col gap-1 p-0"
              >
                <span className="relative block h-16 w-12 shrink-0 overflow-hidden rounded-[2px] border-[0.8px] border-foreground/80 bg-muted">
                  {entry.preview ?? (
                    <ImageIcon className="m-auto mt-5 size-4" />
                  )}
                </span>
                <span className="w-full truncate text-[10px]">
                  {entry.status ?? entry.label}
                </span>
              </Button>
              <Button
                type="button"
                variant="secondary"
                disabled={disabled}
                aria-label={`移除${entry.label}`}
                onClick={() => remove(entry.id)}
                className="absolute top-0 right-0 size-[22px] rounded-full p-0 max-md:size-11"
              >
                <X className="size-3" />
              </Button>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
