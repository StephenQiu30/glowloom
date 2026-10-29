import {
  createContext,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { connectionFlowMesh } from "./connection-flow-model";
import { createConnectionFlowSurface } from "./connection-flow-surface";

type FlowEntry = { path: string; element: SVGPathElement; fanout: boolean };
class FlowRegistry {
  entries = new Map<symbol, FlowEntry>();
  listeners = new Set<() => void>();
  set(id: symbol, entry: FlowEntry) {
    this.entries.set(id, entry);
    this.emit();
  }
  remove(id: symbol) {
    this.entries.delete(id);
    this.emit();
  }
  private emit() {
    for (const listener of this.listeners) listener();
  }
}
const Context = createContext<FlowRegistry | null>(null);

export function ConnectionFlowProvider({ children }: { children: ReactNode }) {
  const [registry] = useState(() => new FlowRegistry());
  return <Context.Provider value={registry}>{children}</Context.Provider>;
}

export function useConnectionFlow(
  path: string,
  enabled: boolean,
  element: RefObject<SVGPathElement | null>,
  fanout = false,
) {
  const registry = useContext(Context);
  useLayoutEffect(() => {
    if (!enabled || !registry || !element.current) return;
    const id = Symbol("connection");
    registry.set(id, { path, element: element.current, fanout });
    return () => registry.remove(id);
  }, [registry, path, enabled, element, fanout]);
}

/** Inverse SVG transform keeps the shared raster surface in CSS pixels during pan/zoom previews. */
export function ConnectionFlowLayer({
  containerRef,
}: {
  containerRef: RefObject<HTMLDivElement | null>;
}) {
  const registry = useContext(Context);
  const foreignRef = useRef<SVGForeignObjectElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = canvasRef.current,
      foreign = foreignRef.current,
      container = containerRef.current;
    if (!registry || !canvas || !foreign || !container) return;
    let surface: ReturnType<typeof createConnectionFlowSurface>;
    try {
      surface = createConnectionFlowSurface(canvas);
    } catch {
      canvas.dataset.rendering = "unavailable";
      return;
    }
    let frame: number | null = null,
      dirty = true,
      failed = false;
    const motion = window.matchMedia("(prefers-reduced-motion: reduce)");
    const cancel = () => {
      if (frame !== null) cancelAnimationFrame(frame);
      frame = null;
    };
    const schedule = () => {
      if (failed) return;
      if (document.hidden || motion.matches || registry.entries.size === 0) {
        cancel();
        surface.clear();
        canvas.dataset.rendering = "suspended";
        return;
      }
      if (frame === null) frame = requestAnimationFrame(draw);
    };
    const draw = (timestamp: number) => {
      frame = null;
      try {
        const first = registry.entries.values().next().value;
        const matrix = first?.element.getScreenCTM();
        if (matrix && matrix.a > 0) {
          const bounds = container.getBoundingClientRect();
          const x = matrix.e - bounds.left,
            y = matrix.f - bounds.top,
            zoom = matrix.a;
          foreign.setAttribute(
            "transform",
            `matrix(${1 / zoom},0,0,${1 / zoom},${-x / zoom},${-y / zoom})`,
          );
          foreign.setAttribute("width", String(bounds.width));
          foreign.setAttribute("height", String(bounds.height));
          if (dirty) {
            const meshes = [...registry.entries.values()].map((entry) =>
              connectionFlowMesh(entry.path, entry.fanout),
            );
            const vertices = new Float32Array(
              meshes.reduce((total, mesh) => total + mesh.length, 0),
            );
            let offset = 0;
            for (const mesh of meshes) {
              vertices.set(mesh, offset);
              offset += mesh.length;
            }
            surface.upload(vertices);
            dirty = false;
          }
          surface.render(
            {
              width: bounds.width,
              height: bounds.height,
              dpr: window.devicePixelRatio || 1,
              x,
              y,
              zoom,
            },
            timestamp,
          );
          canvas.dataset.rendering = "running";
        }
      } catch {
        failed = true;
        canvas.dataset.rendering = "failed";
        surface.clear();
      }
      schedule();
    };
    const invalidate = () => {
      dirty = true;
      schedule();
    };
    const contextLost = (event: Event) => {
      event.preventDefault();
      failed = true;
      cancel();
      canvas.dataset.rendering = "context-lost";
    };
    registry.listeners.add(invalidate);
    document.addEventListener("visibilitychange", schedule);
    motion.addEventListener("change", schedule);
    canvas.addEventListener("webglcontextlost", contextLost);
    schedule();
    return () => {
      cancel();
      registry.listeners.delete(invalidate);
      document.removeEventListener("visibilitychange", schedule);
      motion.removeEventListener("change", schedule);
      canvas.removeEventListener("webglcontextlost", contextLost);
      surface.dispose();
    };
  }, [registry, containerRef]);
  return (
    <foreignObject
      ref={foreignRef}
      x={0}
      y={0}
      width={1}
      height={1}
      style={{ pointerEvents: "none", overflow: "visible" }}
      aria-hidden="true"
    >
      <canvas
        ref={canvasRef}
        data-testid="canvas-connection-flow-layer"
        data-period-ms="2600"
        style={{
          display: "block",
          width: "100%",
          height: "100%",
          pointerEvents: "none",
        }}
      />
    </foreignObject>
  );
}
