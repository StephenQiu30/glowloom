import type { HarnessOptions } from "@earendil-works/pi-durable";
type Telemetry = NonNullable<HarnessOptions["telemetry"]>;
type Options = Parameters<Telemetry["startSpan"]>[0];
type Span = Parameters<Parameters<Telemetry["startSpan"]>[1]>[0];
export interface ModelPhase {
  taskId: number;
  taskName: string;
  phase: string;
  attempt: number;
}
export class PhaseTelemetry implements Span {
  constructor(readonly phase?: ModelPhase) {}
  async startSpan<T>(
    options: Options,
    callback: (span: Span) => T | Promise<T>,
  ): Promise<T> {
    let inherited = this.phase;
    if (options.name === "pi.durable.phase") {
      const a = options.attributes ?? {};
      if (
        typeof a["task.id"] === "number" &&
        typeof a["task.name"] === "string" &&
        typeof a.phase === "string"
      ) {
        inherited = {
          taskId: a["task.id"],
          taskName: a["task.name"],
          phase: a.phase,
          attempt: typeof a.attempt === "number" ? a.attempt : 1,
        };
      }
    }
    return callback(new PhaseTelemetry(inherited));
  }
  addLink(): void {}
  addEvent(): void {}
  setAttributes(): void {}
  setStatus(): void {}
}
export function modelPhase(context: Telemetry | undefined): ModelPhase {
  if (!(context instanceof PhaseTelemetry) || !context.phase)
    throw new Error("Missing trusted model task correlation");
  return context.phase;
}
