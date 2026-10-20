"use client";

import { useEffect, useRef, useState, type PointerEvent } from "react";
import { createPortal } from "react-dom";
import { ImageIcon, Plus, Type, VideoIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverAnchor,
  PopoverContent,
} from "@/components/ui/popover";
import {
  Command,
  CommandGroup,
  CommandItem,
  CommandList,
} from "@/components/ui/command";

export type ConnectionDrop = { x: number; y: number };
export type AdjacentKind = "text" | "image_generation" | "video_generation";

/** Pointer capture belongs to the port, never to the node's move gesture. */
export function ConnectionPort({
  side,
  disabled,
  onAdd,
  onConnect,
}: {
  side: "left" | "right";
  disabled?: boolean;
  onAdd: (kind: AdjacentKind, point?: ConnectionDrop) => void;
  onConnect?: (targetID: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [drop, setDrop] = useState<ConnectionDrop>();
  const [dragging, setDragging] = useState(false);
  const button = useRef<HTMLButtonElement>(null);
  const path = useRef<SVGPathElement>(null);
  const gesture = useRef<{
    id: number;
    start: ConnectionDrop;
    origin: ConnectionDrop;
    moved: boolean;
  } | null>(null);
  const target = useRef<HTMLElement | null>(null);
  function clear() {
    const current = gesture.current;
    gesture.current = null;
    target.current?.removeAttribute("data-connection-target");
    target.current = null;
    if (current && button.current?.hasPointerCapture(current.id))
      button.current.releasePointerCapture(current.id);
    setDragging(false);
  }
  useEffect(() => {
    const cancel = (event: KeyboardEvent) => {
      if (event.key === "Escape" && gesture.current) {
        event.preventDefault();
        clear();
      }
    };
    window.addEventListener("keydown", cancel, true);
    return () => {
      window.removeEventListener("keydown", cancel, true);
      target.current?.removeAttribute("data-connection-target");
    };
  }, []);
  function move(event: PointerEvent<HTMLButtonElement>) {
    const g = gesture.current;
    if (!g || event.pointerId !== g.id) return;
    if (
      !g.moved &&
      Math.hypot(event.clientX - g.start.x, event.clientY - g.start.y) < 5
    )
      return;
    g.moved = true;
    setDragging(true);
    const direction = side === "right" ? 1 : -1;
    const bend = Math.max(50, Math.abs(event.clientX - g.origin.x) / 2);
    path.current?.setAttribute(
      "d",
      `M ${g.origin.x} ${g.origin.y} C ${g.origin.x + direction * bend} ${g.origin.y}, ${event.clientX - direction * bend} ${event.clientY}, ${event.clientX} ${event.clientY}`,
    );
    target.current?.removeAttribute("data-connection-target");
    target.current =
      document
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest<HTMLElement>("[data-node-id]") ?? null;
    const owner = button.current?.closest<HTMLElement>("[data-node-id]");
    const valid =
      target.current &&
      target.current !== owner &&
      target.current.dataset.nodeKind !== "group";
    target.current?.setAttribute(
      "data-connection-target",
      valid ? "true" : "invalid",
    );
  }
  return (
    <>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverAnchor
          virtualRef={
            drop
              ? {
                  current: {
                    getBoundingClientRect: () =>
                      new DOMRect(drop.x, drop.y, 0, 0),
                  },
                }
              : undefined
          }
          asChild={!drop}
        >
          {!drop ? (
            <span className={`canvas-port-anchor canvas-port-anchor-${side}`} />
          ) : null}
        </PopoverAnchor>
        <Button
          ref={button}
          variant="ghost"
          className={`canvas-node-port canvas-node-port-${side}`}
          disabled={disabled}
          aria-label={`在${side === "left" ? "左" : "右"}侧添加节点`}
          aria-haspopup="dialog"
          aria-expanded={open}
          data-canvas-no-zoom
          data-dragging={dragging || undefined}
          onPointerDown={(event) => {
            event.stopPropagation();
            if (event.button !== 0) return;
            const rect = event.currentTarget.getBoundingClientRect();
            gesture.current = {
              id: event.pointerId,
              start: { x: event.clientX, y: event.clientY },
              origin: {
                x: rect.left + rect.width / 2,
                y: rect.top + rect.height / 2,
              },
              moved: false,
            };
            event.currentTarget.setPointerCapture(event.pointerId);
          }}
          onPointerMove={move}
          onPointerUp={(event) => {
            const g = gesture.current;
            if (!g) return;
            const hit = target.current;
            if (g.moved) {
              event.preventDefault();
              if (hit) {
                if (hit.dataset.connectionTarget === "true")
                  onConnect?.(hit.dataset.nodeId!);
              } else {
                setDrop({ x: event.clientX, y: event.clientY });
                setOpen(true);
              }
            } else {
              setDrop(undefined);
              setOpen(true);
            }
            clear();
          }}
          onPointerCancel={clear}
          onLostPointerCapture={clear}
          onClick={(event) => {
            event.stopPropagation();
            if (event.detail === 0) {
              setDrop(undefined);
              setOpen(true);
            }
          }}
        >
          <span>
            <Plus />
          </span>
        </Button>
        <PopoverContent
          side={side}
          align="start"
          sideOffset={8}
          collisionPadding={16}
          className="w-60 p-0"
          aria-label="添加节点"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            button.current?.focus();
          }}
        >
          <Command>
            <CommandList>
              <CommandGroup heading="添加节点">
                {(
                  [
                    { kind: "text", label: "文本", icon: Type },
                    {
                      kind: "image_generation",
                      label: "图片",
                      icon: ImageIcon,
                    },
                    {
                      kind: "video_generation",
                      label: "视频",
                      icon: VideoIcon,
                    },
                  ] as const
                ).map(({ kind, label, icon: Icon }) => (
                  <CommandItem
                    key={kind}
                    className="min-h-11"
                    onSelect={() => {
                      onAdd(kind, drop);
                      setOpen(false);
                      setDrop(undefined);
                    }}
                  >
                    <Icon />
                    {label}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      {typeof document !== "undefined" &&
        createPortal(
          <svg
            className="pointer-events-none fixed inset-0 z-40 size-full"
            aria-hidden="true"
            style={{
              visibility: dragging || (open && drop) ? "visible" : "hidden",
            }}
          >
            <path
              ref={path}
              fill="none"
              stroke="var(--connection-active)"
              strokeWidth="1.5"
            />
          </svg>,
          document.body,
        )}
    </>
  );
}
