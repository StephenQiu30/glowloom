"use client";

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type MouseEvent,
} from "react";
import { Button } from "@/components/ui/button";
import {
  advanceConnectionHandle,
  connectionHandleCounterScale,
  connectionHandleRest,
  connectionHandleTarget,
  connectionHandleTransform,
  type ConnectionHandleSide,
  type HandlePoint,
} from "./canvas-connection-handle-geometry";

type Props = {
  scale: number;
  side: ConnectionHandleSide;
  visible: boolean;
  disabled?: boolean;
  onMouseDown: (event: MouseEvent) => void;
};

export function CanvasConnectionHandle({
  scale,
  side,
  visible,
  disabled = false,
  onMouseDown,
}: Props) {
  const field = useRef<HTMLDivElement>(null);
  const glyph = useRef<HTMLButtonElement>(null);
  const frame = useRef<number | null>(null);
  const lastFrame = useRef<number | null>(null);
  const pointer = useRef<HandlePoint | null>(null);
  const current = useRef(connectionHandleRest(side, scale));
  const reduced = useRef(false);
  const [hovered, setHovered] = useState(false);
  const counterScale = connectionHandleCounterScale(scale);
  const shown = visible || (hovered && !disabled);

  const write = useCallback(
    (point: HandlePoint) => {
      current.current = point;
      if (glyph.current)
        glyph.current.style.transform = connectionHandleTransform(
          point,
          counterScale,
        );
    },
    [counterScale],
  );

  const stop = useCallback(() => {
    if (frame.current !== null) cancelAnimationFrame(frame.current);
    frame.current = null;
    lastFrame.current = null;
  }, []);

  const reset = useCallback(() => {
    stop();
    pointer.current = null;
    field.current?.removeAttribute("data-canvas-connection-glyph-tracking");
    write(connectionHandleRest(side, scale));
  }, [scale, side, stop, write]);

  // Keep animation writes outside React rendering, including zoom/availability resets.
  useLayoutEffect(() => {
    reset();
    return stop;
  }, [disabled, reset, stop]);

  const sampleTarget = useCallback(() => {
    if (!pointer.current || !field.current) return null;
    return connectionHandleTarget(
      side,
      pointer.current,
      field.current.getBoundingClientRect(),
    );
  }, [side]);

  useEffect(() => {
    const media = window.matchMedia("(prefers-reduced-motion: reduce)");
    const update = () => {
      reduced.current = media.matches;
      if (media.matches) {
        stop();
        const target = sampleTarget();
        if (target) write(target);
      }
    };
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, [sampleTarget, stop, write]);

  const followPointer = (clientX: number, clientY: number) => {
    if (disabled) return;
    pointer.current = { x: clientX, y: clientY };
    field.current?.setAttribute(
      "data-canvas-connection-glyph-tracking",
      "true",
    );
    const tick = (timestamp: number) => {
      frame.current = null;
      const target = sampleTarget();
      if (!target) return;
      if (reduced.current) {
        write(target);
        lastFrame.current = null;
        return;
      }
      const result = advanceConnectionHandle(
        current.current,
        target,
        lastFrame.current === null ? null : timestamp - lastFrame.current,
      );
      write(result.point);
      lastFrame.current = result.settled ? null : timestamp;
      if (!result.settled) frame.current = requestAnimationFrame(tick);
    };
    if (frame.current === null) frame.current = requestAnimationFrame(tick);
  };

  return (
    <div
      ref={field}
      data-canvas-no-pan
      data-canvas-connection-handle
      data-side={side}
      data-visible={shown}
      data-disabled={disabled}
      className="canvas-connection-handle"
      style={
        {
          width: 60,
          height: 120,
          [side]: -30,
          "--canvas-handle-counter-scale": counterScale,
        } as CSSProperties
      }
      onPointerEnter={(event) => {
        if (disabled) return;
        setHovered(true);
        followPointer(event.clientX, event.clientY);
      }}
      onPointerMove={(event) => followPointer(event.clientX, event.clientY)}
      onPointerLeave={() => {
        setHovered(false);
        reset();
      }}
      onPointerCancel={() => {
        setHovered(false);
        reset();
      }}
      onMouseDown={(event) => {
        if (!disabled && event.button === 0) onMouseDown(event);
      }}
    >
      <Button
        ref={glyph}
        variant="outline"
        size="icon-xs"
        aria-label={side === "left" ? "连接输入节点" : "连接输出节点"}
        aria-haspopup="menu"
        disabled={disabled}
        tabIndex={shown && !disabled ? 0 : -1}
        data-canvas-connection-glyph
        className="canvas-node-port"
        style={{
          width: 20,
          height: 20,
          transform: connectionHandleTransform(
            connectionHandleRest(side, scale),
            counterScale,
          ),
        }}
        onClick={(event) => {
          if (!disabled && event.detail === 0) onMouseDown(event);
        }}
      >
        <span aria-hidden="true" className="canvas-node-port-plus" />
      </Button>
    </div>
  );
}
