"use client";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { Button } from "@/components/ui/button";
import type { DirectorMotionClip } from "./director-model";

type Range = {
  start: number;
  duration: number;
  source_offset?: number;
  playback_rate?: number;
};
type DragMode = "key" | "move" | "start" | "end";
type Options = {
  baseline: string;
  disabled: boolean;
  value: Range;
  duration: number;
  fps: number;
  validate: (value: Range) => void;
  commit: (value: Range) => void;
  onError: (message: string) => void;
};
function dragValue(
  value: Range,
  mode: DragMode,
  delta: number,
  fps: number,
): Range {
  const snap = (time: number) => Math.round(time * fps) / fps;
  if (mode === "key" || mode === "move")
    return { ...value, start: snap(value.start + delta) };
  if (mode === "end")
    return {
      ...value,
      duration: snap(value.start + value.duration + delta) - value.start,
    };
  const start = snap(value.start + delta);
  return {
    ...value,
    start,
    duration: value.start + value.duration - start,
    source_offset:
      (value.source_offset ?? 0) +
      (start - value.start) * (value.playback_rate ?? 1),
  };
}
function useTimelineDrag(options: Options) {
  const session = useRef<null | {
    pointer: number;
    x: number;
    width: number;
    mode: DragMode;
    baseline: string;
    origin: Range;
    moved: boolean;
    element: HTMLElement;
  }>(null);
  const suppressClick = useRef(false);
  const stopListeners = useRef<null | (() => void)>(null);
  const [preview, setPreview] = useState<null | {
    baseline: string;
    value: Range;
    error: string;
  }>(null);
  const cancel = useCallback(() => {
    const current = session.current;
    if (!current) return;
    session.current = null;
    stopListeners.current?.();
    stopListeners.current = null;
    suppressClick.current = current.moved;
    setPreview(null);
    if (current.element.hasPointerCapture?.(current.pointer))
      current.element.releasePointerCapture(current.pointer);
  }, []);
  useEffect(() => () => cancel(), [cancel, options.baseline, options.disabled]);
  function candidate(event: ReactPointerEvent<HTMLElement>) {
    const current = session.current;
    if (!current || current.pointer !== event.pointerId) return null;
    const delta = event.clientX - current.x;
    current.moved ||= Math.abs(delta) >= 3;
    const value = dragValue(
      current.origin,
      current.mode,
      (delta / current.width) * options.duration,
      options.fps,
    );
    let error = "";
    try {
      if (options.disabled || options.baseline !== current.baseline)
        throw new Error("场景已改变，请重新拖动");
      if (
        !Number.isFinite(value.start) ||
        value.start < 0 ||
        value.start + value.duration > options.duration + 1e-6
      )
        throw new Error("拖动超出当前场景时间范围");
      options.validate(value);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : "拖动无效";
    }
    return { baseline: current.baseline, value, error };
  }
  const bind = (mode: DragMode) => ({
    onPointerDown(event: ReactPointerEvent<HTMLElement>) {
      if (
        options.disabled ||
        event.button !== 0 ||
        session.current ||
        options.duration <= 0 ||
        options.fps <= 0
      )
        return;
      const lane = event.currentTarget.closest(".director-track-lane");
      const width = lane?.getBoundingClientRect().width ?? 0;
      if (!width) return;
      event.preventDefault();
      event.stopPropagation();
      event.currentTarget.focus();
      suppressClick.current = false;
      session.current = {
        pointer: event.pointerId,
        x: event.clientX,
        width,
        mode,
        baseline: options.baseline,
        origin: { ...options.value },
        moved: false,
        element: event.currentTarget,
      };
      const escape = (event: KeyboardEvent) => {
        if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          cancel();
        }
      };
      window.addEventListener("blur", cancel);
      window.addEventListener("keydown", escape, true);
      stopListeners.current = () => {
        window.removeEventListener("blur", cancel);
        window.removeEventListener("keydown", escape, true);
      };
      event.currentTarget.setPointerCapture?.(event.pointerId);
    },
    onPointerMove(event: ReactPointerEvent<HTMLElement>) {
      const value = candidate(event);
      if (value && session.current?.moved) setPreview(value);
    },
    onPointerUp(event: ReactPointerEvent<HTMLElement>) {
      const value = candidate(event),
        current = session.current;
      if (!value || !current) return;
      const moved = current.moved;
      cancel();
      if (!moved) return;
      if (value.error) {
        options.onError(value.error);
        return;
      }
      if (
        value.value.start === current.origin.start &&
        value.value.duration === current.origin.duration
      )
        return;
      try {
        options.commit(value.value);
        options.onError("");
      } catch (cause) {
        options.onError(
          cause instanceof Error ? cause.message : "拖动提交失败",
        );
      }
    },
    onKeyDown(event: ReactKeyboardEvent<HTMLElement>) {
      if (event.key === "Enter" || event.key === " ")
        suppressClick.current = false;
    },
    onPointerCancel: cancel,
    onLostPointerCapture: cancel,
    onBlur: cancel,
  });
  const display =
    !options.disabled && preview?.baseline === options.baseline
      ? preview
      : null;
  return {
    bind,
    value: display?.value ?? options.value,
    invalid: !!display?.error,
    dragging: !!display,
    select(action: () => void) {
      if (suppressClick.current) suppressClick.current = false;
      else action();
    },
  };
}

type Common = {
  baseline: string;
  disabled: boolean;
  duration: number;
  fps: number;
  onError: (message: string) => void;
  onSelect: () => void;
};
export function DirectorTimelineKey({
  time,
  label,
  validate,
  onCommit,
  ...props
}: Common & {
  time: number;
  label: string;
  validate: (time: number) => void;
  onCommit: (time: number) => void;
}) {
  const drag = useTimelineDrag({
    ...props,
    value: { start: time, duration: 0 },
    validate: (v) => validate(v.start),
    commit: (v) => onCommit(v.start),
  });
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      className="director-diamond"
      aria-label={label}
      data-dragging={drag.dragging || undefined}
      aria-invalid={drag.invalid || undefined}
      style={{
        left: `${Math.min(100, Math.max(0, (drag.value.start / props.duration) * 100))}%`,
        touchAction: "none",
        cursor: props.disabled ? undefined : "ew-resize",
        color: drag.invalid ? "var(--destructive)" : undefined,
      }}
      {...drag.bind("key")}
      onClick={() => drag.select(props.onSelect)}
    >
      ◆
    </Button>
  );
}
export function DirectorTimelineClip({
  clip,
  validate,
  onCommit,
  ...props
}: Common & {
  clip: DirectorMotionClip;
  validate: (clip: DirectorMotionClip) => void;
  onCommit: (clip: DirectorMotionClip) => void;
}) {
  const drag = useTimelineDrag({
    ...props,
    value: {
      start: clip.start!,
      duration: clip.duration!,
      source_offset: clip.source_offset,
      playback_rate: clip.playback_rate,
    },
    validate: (value) => validate({ ...clip, ...value }),
    commit: (value) => onCommit({ ...clip, ...value }),
  });
  return (
    <div
      className="absolute flex h-[26px] min-w-6"
      data-dragging={drag.dragging || undefined}
      style={{
        left: `${Math.max(0, (drag.value.start / props.duration) * 100)}%`,
        width: `${Math.max(0, (drag.value.duration / props.duration) * 100)}%`,
        outline: drag.invalid ? "1px solid var(--destructive)" : undefined,
      }}
    >
      <Button
        variant="secondary"
        size="sm"
        className="h-full min-w-0 flex-1 overflow-hidden px-3"
        aria-label={`移动${clip.name}`}
        style={{
          touchAction: "none",
          cursor: props.disabled ? undefined : "grab",
        }}
        {...drag.bind("move")}
        onClick={() => drag.select(props.onSelect)}
      >
        {clip.name}
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        className="absolute top-0 left-0 h-full w-3 rounded-none border-r"
        aria-label={`裁剪${clip.name}起点`}
        title="拖动调整源动作入点"
        disabled={props.disabled}
        style={{ touchAction: "none", cursor: "ew-resize" }}
        {...drag.bind("start")}
      />
      <Button
        variant="ghost"
        size="icon-sm"
        className="absolute top-0 right-0 h-full w-3 rounded-none border-l"
        aria-label={`裁剪${clip.name}终点`}
        title="拖动调整片段终点"
        disabled={props.disabled}
        style={{ touchAction: "none", cursor: "ew-resize" }}
        {...drag.bind("end")}
      />
    </div>
  );
}
