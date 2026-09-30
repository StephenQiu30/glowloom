// Adapted from BeefTV canvas-toolbar.tsx, floating-dock.tsx and CanvasSelectionToolbar
// in canvas-workspace-overlays.tsx (MIT). Fixed-size rails and attached placement
// are retained; project commands use shadcn and have no registry or stored prefs.
import {
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { Button } from "@/components/ui/button";
import {
  subscribeCanvasNodeDragPreview,
  subscribeCanvasViewportPreview,
} from "./viewport-dom";

export type DockCommand = {
  id: string;
  label: string;
  icon: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  active?: boolean;
  danger?: boolean;
};
export function CanvasDock({
  commands,
  label = "画布工具",
}: {
  commands: DockCommand[];
  label?: string;
}) {
  return (
    <div
      role="toolbar"
      aria-label={label}
      data-canvas-no-zoom
      className="pointer-events-auto flex max-w-full items-center gap-1 overflow-x-auto rounded-2xl border border-border/60 bg-background/95 p-2 shadow-lg backdrop-blur-xl"
      onPointerDown={(event) => event.stopPropagation()}
      onWheel={(event) => event.stopPropagation()}
    >
      {commands.map((command) => (
        <Button
          key={command.id}
          size="icon-lg"
          variant={
            command.danger
              ? "destructive"
              : command.active
                ? "secondary"
                : "ghost"
          }
          aria-label={command.label}
          aria-pressed={command.active}
          title={command.label}
          disabled={command.disabled}
          onClick={command.onClick}
          className={command.danger ? "ml-2 border-l border-border" : ""}
        >
          {command.icon}
        </Button>
      ))}
    </div>
  );
}
export function CanvasSelectionToolbar({
  anchorRef,
  containerRef,
  count,
  children,
}: {
  anchorRef: RefObject<HTMLDivElement | null>;
  containerRef: RefObject<HTMLDivElement | null>;
  count: number;
  children: ReactNode;
}) {
  const toolbarRef = useRef<HTMLDivElement>(null);
  const [anchor, setAnchor] = useState<{
    left: number;
    top: number;
    placement: "above" | "below";
  } | null>(null);
  useLayoutEffect(() => {
    const element = anchorRef.current,
      container = containerRef.current;
    if (!element || !container) {
      setAnchor(null);
      return;
    }
    const update = () => {
      const bounds = element.getBoundingClientRect(),
        containerBounds = container.getBoundingClientRect();
      const toolbarWidth = toolbarRef.current?.offsetWidth || 340,
        toolbarHeight = toolbarRef.current?.offsetHeight || 52;
      const halfWidth = Math.min(
        toolbarWidth / 2,
        Math.max(0, containerBounds.width / 2 - 12),
      );
      const center = bounds.left - containerBounds.left + bounds.width / 2;
      const left = Math.min(
        Math.max(center, 12 + halfWidth),
        Math.max(12 + halfWidth, containerBounds.width - 12 - halfWidth),
      );
      const boundsTop = bounds.top - containerBounds.top,
        boundsBottom = bounds.bottom - containerBounds.top;
      const placement = boundsTop - toolbarHeight - 8 >= 16 ? "above" : "below";
      const top =
        placement === "above"
          ? boundsTop - 8
          : Math.max(
              12,
              Math.min(
                boundsBottom + 8,
                containerBounds.height - toolbarHeight - 84,
              ),
            );
      if (toolbarRef.current) {
        toolbarRef.current.style.left = `${left}px`;
        toolbarRef.current.style.top = `${top}px`;
        toolbarRef.current.classList.toggle(
          "-translate-y-full",
          placement === "above",
        );
        return;
      }
      setAnchor((current) =>
        current?.left === left &&
        current.top === top &&
        current.placement === placement
          ? current
          : { left, top, placement },
      );
    };
    update();
    const resize = new ResizeObserver(update);
    resize.observe(element);
    resize.observe(container);
    if (toolbarRef.current) resize.observe(toolbarRef.current);
    const mutation = new MutationObserver(update);
    mutation.observe(element, { attributes: true, attributeFilter: ["style"] });
    if (element.parentElement)
      mutation.observe(element.parentElement, {
        attributes: true,
        attributeFilter: ["style"],
      });
    const unsubscribeViewport = subscribeCanvasViewportPreview(
        container,
        update,
      ),
      unsubscribeDrag = subscribeCanvasNodeDragPreview(container, update);
    window.addEventListener("resize", update);
    return () => {
      resize.disconnect();
      mutation.disconnect();
      unsubscribeViewport();
      unsubscribeDrag();
      window.removeEventListener("resize", update);
    };
  }, [anchorRef, containerRef, count]);
  if (!anchor) return null;
  return (
    <div
      ref={toolbarRef}
      data-canvas-no-zoom
      className={`absolute z-30 max-w-[calc(100%_-_24px)] -translate-x-1/2 ${anchor.placement === "above" ? "-translate-y-full" : ""}`}
      style={{ left: anchor.left, top: anchor.top }}
      onPointerDown={(event) => event.stopPropagation()}
    >
      <div className="flex items-center gap-2">
        <span className="shrink-0 rounded-full border bg-background/95 px-3 py-2 text-xs font-semibold tabular-nums shadow-sm backdrop-blur-xl">
          已选 {count}
        </span>
        {children}
      </div>
    </div>
  );
}
