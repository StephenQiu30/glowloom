import { AsyncLocalStorage } from 'node:async_hooks';
import { randomBytes } from 'node:crypto';

export interface TraceContext {
  readonly traceId: string;
  readonly spanId: string;
  readonly traceFlags: string;
}

export interface GenAiSpanRecord {
  readonly name: string;
  readonly traceId: string;
  readonly spanId: string;
  readonly parentSpanId?: string;
  readonly startedAtMs: number;
  readonly durationMs: number;
  readonly status: 'ok' | 'error';
  readonly attributes: Record<string, string | number | boolean>;
  readonly errorMessage?: string;
}

export type SpanSink = (record: GenAiSpanRecord) => void;

const traceStorage = new AsyncLocalStorage<TraceContext>();
let activeSink: SpanSink | undefined;

export function setSpanSink(sink: SpanSink | undefined): void {
  activeSink = sink;
}

export function emitSpan(record: GenAiSpanRecord): void {
  activeSink?.(record);
}

export function newTraceId(): string {
  return randomBytes(16).toString('hex');
}

export function newSpanId(): string {
  return randomBytes(8).toString('hex');
}

export function parseTraceparent(
  header: string | undefined,
): TraceContext | undefined {
  if (!header) return undefined;
  const match = /^00-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$/.exec(
    header.trim().toLowerCase(),
  );
  if (!match || !match[1] || !match[2] || !match[3]) return undefined;
  if (/^0+$/.test(match[1]) || /^0+$/.test(match[2])) return undefined;
  return {
    traceId: match[1],
    spanId: match[2],
    traceFlags: match[3],
  };
}

export function formatTraceparent(context: TraceContext): string {
  return `00-${context.traceId}-${context.spanId}-${context.traceFlags}`;
}

export function currentTraceContext(): TraceContext | undefined {
  return traceStorage.getStore();
}

export function currentTraceparent(): string | undefined {
  const ctx = traceStorage.getStore();
  return ctx ? formatTraceparent(ctx) : undefined;
}

export function runWithTraceContext<T>(context: TraceContext, fn: () => T): T {
  return traceStorage.run(context, fn);
}
