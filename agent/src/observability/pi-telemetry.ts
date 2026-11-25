import type { HarnessOptions } from '@earendil-works/pi-durable';
import {
  currentTraceContext,
  emitSpan,
  newSpanId,
  newTraceId,
  runWithTraceContext,
  type TraceContext,
} from './otel.js';

type Telemetry = NonNullable<HarnessOptions['telemetry']>;
type Options = Parameters<Telemetry['startSpan']>[0];
type Span = Parameters<Parameters<Telemetry['startSpan']>[1]>[0];

export interface ModelPhase {
  taskId: number;
  taskName: string;
  phase: string;
  attempt: number;
}

export class PhaseTelemetry implements Span {
  private readonly attrs: Record<string, string | number | boolean> = {};
  private failedMessage: string | undefined;

  constructor(
    readonly phase?: ModelPhase,
    readonly trace?: TraceContext,
  ) {}

  async startSpan<T>(
    options: Options,
    callback: (span: Span) => T | Promise<T>,
  ): Promise<T> {
    let inherited = this.phase;
    const rawAttributes = options.attributes ?? {};
    if (options.name === 'pi.durable.phase') {
      if (
        typeof rawAttributes['task.id'] === 'number' &&
        typeof rawAttributes['task.name'] === 'string' &&
        typeof rawAttributes.phase === 'string'
      ) {
        inherited = {
          taskId: rawAttributes['task.id'],
          taskName: rawAttributes['task.name'],
          phase: rawAttributes.phase,
          attempt:
            typeof rawAttributes.attempt === 'number'
              ? rawAttributes.attempt
              : 1,
        };
      }
    }
    const parent = this.trace ?? currentTraceContext();
    const traceContext: TraceContext = {
      traceId: parent?.traceId ?? newTraceId(),
      spanId: newSpanId(),
      traceFlags: parent?.traceFlags ?? '01',
    };
    const child = new PhaseTelemetry(inherited, traceContext);
    for (const [key, value] of Object.entries(rawAttributes)) {
      if (
        typeof value === 'string' ||
        typeof value === 'number' ||
        typeof value === 'boolean'
      ) {
        child.attrs[key] = value;
      }
    }
    const startedAtMs = Date.now();
    try {
      return await runWithTraceContext(traceContext, () => callback(child));
    } catch (error) {
      child.failedMessage =
        error instanceof Error ? error.message : 'span_failed';
      throw error;
    } finally {
      emitSpan({
        name: options.name,
        traceId: traceContext.traceId,
        spanId: traceContext.spanId,
        ...(parent ? { parentSpanId: parent.spanId } : {}),
        startedAtMs,
        durationMs: Math.max(0, Date.now() - startedAtMs),
        status: child.failedMessage ? 'error' : 'ok',
        attributes: { ...child.attrs },
        ...(child.failedMessage ? { errorMessage: child.failedMessage } : {}),
      });
    }
  }

  addLink(): void {}
  addEvent(): void {}

  setAttributes(attributes: Record<string, unknown>): void {
    for (const [key, value] of Object.entries(attributes)) {
      if (
        typeof value === 'string' ||
        typeof value === 'number' ||
        typeof value === 'boolean'
      ) {
        this.attrs[key] = value;
      }
    }
  }

  setStatus(status: Parameters<Span['setStatus']>[0]): void {
    if (status && typeof status === 'object' && status.status === 'error') {
      this.failedMessage =
        'message' in status && typeof status.message === 'string'
          ? status.message
          : 'error';
    }
  }
}

export function modelPhase(context: Telemetry | undefined): ModelPhase {
  if (!(context instanceof PhaseTelemetry) || !context.phase)
    throw new Error('Missing trusted model task correlation');
  return context.phase;
}
