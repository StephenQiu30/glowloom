import { EventEmitter } from 'node:events';
import { Redis } from 'ioredis';
import type { Logger } from 'pino';
import type { RunEvent } from './types.js';

export interface RunEventBus {
  publish(event: RunEvent): Promise<void>;
  subscribe(runId: string, handler: (event: RunEvent) => void): () => void;
  close(): Promise<void>;
}

export class InMemoryEventBus implements RunEventBus {
  private readonly emitter = new EventEmitter();

  constructor() {
    this.emitter.setMaxListeners(0);
  }

  async publish(event: RunEvent): Promise<void> {
    this.emitter.emit(event.runId, event);
  }

  subscribe(runId: string, handler: (event: RunEvent) => void): () => void {
    this.emitter.on(runId, handler);
    return () => {
      this.emitter.off(runId, handler);
    };
  }

  async close(): Promise<void> {
    this.emitter.removeAllListeners();
  }
}

const channelPrefix = 'glowloom:agent:run:';

export class RedisEventBus implements RunEventBus {
  private readonly pub: Redis;
  private readonly sub: Redis;
  private readonly handlers = new Map<string, Set<(event: RunEvent) => void>>();

  constructor(
    redisUrl: string,
    private readonly logger: Logger,
  ) {
    this.pub = new Redis(redisUrl, { lazyConnect: false, maxRetriesPerRequest: 3 });
    this.sub = new Redis(redisUrl, { lazyConnect: false, maxRetriesPerRequest: 3 });
    this.sub.on('message', (channel: string, message: string) => {
      if (!channel.startsWith(channelPrefix)) return;
      const runId = channel.slice(channelPrefix.length);
      const listeners = this.handlers.get(runId);
      if (!listeners || listeners.size === 0) return;
      try {
        const parsed = JSON.parse(message) as RunEvent;
        for (const fn of listeners) {
          fn(parsed);
        }
      } catch (error: unknown) {
        this.logger.warn(
          { err: error instanceof Error ? error.message : String(error) },
          'Failed to decode Redis run event',
        );
      }
    });
  }

  async publish(event: RunEvent): Promise<void> {
    await this.pub.publish(
      `${channelPrefix}${event.runId}`,
      JSON.stringify(event),
    );
  }

  subscribe(runId: string, handler: (event: RunEvent) => void): () => void {
    let listeners = this.handlers.get(runId);
    if (!listeners) {
      listeners = new Set();
      this.handlers.set(runId, listeners);
      void this.sub.subscribe(`${channelPrefix}${runId}`);
    }
    listeners.add(handler);
    return () => {
      const set = this.handlers.get(runId);
      if (!set) return;
      set.delete(handler);
      if (set.size === 0) {
        this.handlers.delete(runId);
        void this.sub.unsubscribe(`${channelPrefix}${runId}`);
      }
    };
  }

  async close(): Promise<void> {
    this.handlers.clear();
    await Promise.allSettled([this.sub.quit(), this.pub.quit()]);
  }
}

export function createEventBus(
  redisUrl: string | undefined,
  logger: Logger,
): RunEventBus {
  if (redisUrl) {
    return new RedisEventBus(redisUrl, logger);
  }
  return new InMemoryEventBus();
}
