import { Readable } from 'node:stream';
import { setTimeout as delay } from 'node:timers/promises';
import type { FastifyReply } from 'fastify';
import { AgentError } from '../sessions/errors.js';
import type {
  AgentApplication,
  Identity,
  RuntimeEvent,
} from '../sessions/types.js';
import type { AgentLifecycle } from './lifecycle.js';

export async function streamEvents(
  reply: FastifyReply,
  lifecycle: AgentLifecycle,
  service: AgentApplication,
  runId: string,
  identity: Identity,
  cursor: string | undefined,
) {
  const subscription = new AbortController();
  const { signal } = subscription;
  const onClose = () => subscription.abort();
  const onAbort = () => {
    if (!reply.raw.destroyed) reply.raw.destroy();
  };
  signal.addEventListener('abort', onAbort, { once: true });
  const unsubscribe = lifecycle.subscribe(subscription);
  reply.raw.once('close', onClose);
  const cleanup = () => {
    unsubscribe();
    reply.raw.off('close', onClose);
    signal.removeEventListener('abort', onAbort);
  };

  async function* committedEvents(first: RuntimeEvent[]) {
    let batch = first;
    try {
      while (!signal.aborted) {
        if (batch.length === 0) yield ': keepalive\n\n';
        for (const event of batch) {
          if (signal.aborted) return;
          cursor = event.cursor;
          yield `id: ${event.cursor}\nevent: ${event.type}\ndata: ${JSON.stringify(event.payload)}\n\n`;
        }
        await delay(1000, undefined, { signal });
        batch = await service.events(runId, identity, cursor);
      }
    } catch (error) {
      if (!signal.aborted) throw error;
    } finally {
      cleanup();
    }
  }

  try {
    if (signal.aborted) throw new AgentError('shutting_down');
    // Scope and cursor errors must be returned before streaming headers.
    const first = await service.events(runId, identity, cursor);
    if (signal.aborted) {
      cleanup();
      return;
    }
    const stream = Readable.from(committedEvents(first));
    stream.once('close', cleanup);
    reply
      .header('content-type', 'text/event-stream')
      .header('cache-control', 'no-cache, no-transform')
      .header('x-accel-buffering', 'no')
      .send(stream);
  } catch (error) {
    cleanup();
    throw error;
  }
}
