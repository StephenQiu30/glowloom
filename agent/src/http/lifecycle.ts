import type { OnApplicationShutdown, OnModuleDestroy } from '@nestjs/common';
import type { AgentApplication } from '../sessions/types.js';

export class AgentLifecycle implements OnModuleDestroy, OnApplicationShutdown {
  private readonly subscriptions = new Set<AbortController>();
  private stopping = false;

  constructor(private readonly service: AgentApplication) {}

  get draining(): boolean {
    return this.stopping;
  }

  subscribe(subscription: AbortController): () => void {
    if (this.stopping) {
      subscription.abort();
    } else {
      this.subscriptions.add(subscription);
    }
    return () => this.subscriptions.delete(subscription);
  }

  onModuleDestroy() {
    this.stopping = true;
    this.service.stopAccepting();
    for (const subscription of this.subscriptions) subscription.abort();
    this.subscriptions.clear();
  }

  onApplicationShutdown() {
    return this.service.close();
  }
}
