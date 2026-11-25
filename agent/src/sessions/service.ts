import { randomUUID } from 'node:crypto';
import type { Logger } from 'pino';
import {
  createBusinessMcpClient,
  type BusinessMcpConfig,
} from '../business/mcp-client.js';
import type { AgentTeam } from '../mastra/runtime.js';
import {
  filterToolsByAllowlist,
  type DeclarativeSkill,
} from '../skills/loader.js';
import type { RunEventBus } from './event-bus.js';
import type { SessionStore } from './store.js';
import type {
  AgentApplication,
  ApplyDecisionInput,
  CreateSessionInput,
  Identity,
  PublicMessageView,
  RunEvent,
  RunView,
  SessionView,
  StartRunInput,
  UpdateSessionInput,
} from './types.js';

export interface SessionServiceOptions {
  store: SessionStore;
  eventBus: RunEventBus;
  agentTeam: AgentTeam;
  mcpConfig: BusinessMcpConfig;
  skills: DeclarativeSkill[];
  logger: Logger;
  leaseMs: number;
  recoveryPollMs: number;
}

export class SessionService implements AgentApplication {
  private readonly ownerId = `agent-${randomUUID()}`;
  private readonly activeControllers = new Map<string, AbortController>();
  private recoveryTimer: NodeJS.Timeout | undefined;
  private stopping = false;

  constructor(private readonly options: SessionServiceOptions) {}

  async ready(): Promise<boolean> {
    if (this.stopping) return false;
    return this.options.store.ready();
  }

  createSession(input: CreateSessionInput): Promise<SessionView> {
    return this.options.store.createSession(input);
  }

  listSessions(
    identity: Identity,
    limit?: number,
  ): Promise<{ items: SessionView[]; next_cursor?: string }> {
    return this.options.store.listSessions(identity, limit);
  }

  getSession(identity: Identity, sessionId: string): Promise<SessionView> {
    return this.options.store.getSession(identity, sessionId);
  }

  updateSession(input: UpdateSessionInput): Promise<SessionView> {
    return this.options.store.updateSession(input);
  }

  async startRun(input: StartRunInput): Promise<Record<string, unknown>> {
    const admitted = await this.options.store.admitRun(
      input,
      this.ownerId,
      this.options.leaseMs,
    );

    if (!admitted.deduplicated) {
      await this.emitDurableEvent(admitted.runId, 'run.accepted', {
        root_run_id: admitted.runId,
        session_id: input.sessionId,
        accepted_revision: admitted.acceptedRevision,
      });
      void this.executeRun(admitted.runId, input);
    }

    return {
      root_run_id: admitted.runId,
      session_id: input.sessionId,
      submission_id: admitted.submissionId,
      accepted_revision: admitted.acceptedRevision,
      status: 'running',
    };
  }

  listRuns(
    identity: Identity,
    sessionId: string,
    limit?: number,
  ): Promise<{ items: Array<Record<string, unknown>>; next_cursor?: string }> {
    return this.options.store.listRuns(identity, sessionId, limit);
  }

  getRun(identity: Identity, runId: string): Promise<RunView> {
    return this.options.store.getRun(identity, runId);
  }

  getCurrentSubmission(
    identity: Identity,
    sessionId: string,
  ): Promise<Record<string, unknown>> {
    return this.options.store.getCurrentSubmission(identity, sessionId);
  }

  getSubmission(
    identity: Identity,
    sessionId: string,
    requestId: string,
  ): Promise<Record<string, unknown>> {
    return this.options.store.getSubmission(identity, sessionId, requestId);
  }

  listMessages(
    identity: Identity,
    runId: string,
    limit?: number,
    before?: string,
    through?: string,
  ): Promise<{ items: PublicMessageView[]; next_cursor?: string; through: string }> {
    return this.options.store.listMessages(
      identity,
      runId,
      limit,
      before,
      through,
    );
  }

  getMessage(
    identity: Identity,
    runId: string,
    messageId: string,
  ): Promise<PublicMessageView> {
    return this.options.store.getMessage(identity, runId, messageId);
  }

  async applyDecision(
    input: ApplyDecisionInput,
  ): Promise<Record<string, unknown>> {
    const recorded = await this.options.store.recordDecision(input);
    if (recorded.deduplicated) {
      return { decision_id: input.decisionId, applied: true, deduplicated: true };
    }

    if (input.action === 'stop' || input.action === 'cancel') {
      this.activeControllers.get(input.runId)?.abort();
      await this.options.store.updateRunStatus(input.runId, 'cancelled');
      await this.emitDurableEvent(input.runId, 'run.cancelled', {
        root_run_id: input.runId,
        decision_id: input.decisionId,
      });
      return { decision_id: input.decisionId, applied: true, status: 'cancelled' };
    }

    if (
      (input.action === 'approve' || input.action === 'reject') &&
      recorded.workflowRunId
    ) {
      const workflow = this.options.agentTeam.mastra.getWorkflow(
        'creativeDeliveryWorkflow',
      );
      const wfRun = await workflow.createRun({
        runId: recorded.workflowRunId,
      });
      const resumed = await wfRun.resume({
        step: 'approval-gate',
        resumeData: {
          approved: input.action === 'approve',
          decisionId: input.decisionId,
        },
      });
      const completed = resumed.status === 'success';
      await this.options.store.updateRunStatus(
        input.runId,
        completed ? 'completed' : 'failed',
      );
      await this.emitDurableEvent(input.runId, 'workflow.resumed', {
        root_run_id: input.runId,
        workflow_run_id: recorded.workflowRunId,
        status: resumed.status,
      });
      return {
        decision_id: input.decisionId,
        applied: true,
        workflow_status: resumed.status,
      };
    }

    return { decision_id: input.decisionId, applied: true };
  }

  listRunEventsAfter(
    identity: Identity,
    runId: string,
    lastEventId?: string,
  ): Promise<{ events: RunEvent[]; terminal: boolean }> {
    return this.options.store.listRunEventsAfter(identity, runId, lastEventId);
  }

  subscribeRunEvents(
    runId: string,
    handler: (event: RunEvent) => void,
  ): () => void {
    return this.options.eventBus.subscribe(runId, handler);
  }

  startRecovery(): void {
    if (this.recoveryTimer) return;
    this.recoveryTimer = setInterval(() => {
      void this.runRecoverySweep();
    }, this.options.recoveryPollMs);
    this.recoveryTimer.unref();
  }

  async stopRecovery(): Promise<void> {
    this.stopping = true;
    if (this.recoveryTimer) {
      clearInterval(this.recoveryTimer);
      this.recoveryTimer = undefined;
    }
    for (const controller of this.activeControllers.values()) {
      controller.abort();
    }
    this.activeControllers.clear();
  }

  private async runRecoverySweep(): Promise<void> {
    if (this.stopping) return;
    try {
      const expired = await this.options.store.claimExpiredRuns(10);
      for (const run of expired) {
        if (run.workflow_run_id) {
          await this.options.store.updateRunStatus(run.id, 'waiting_approval');
          await this.emitDurableEvent(run.id, 'run.recovered_waiting', {
            root_run_id: run.id,
            workflow_run_id: run.workflow_run_id,
          });
        } else {
          await this.options.store.updateRunStatus(run.id, 'failed', {
            errorCode: 'lease_expired_recovered',
          });
          await this.emitDurableEvent(run.id, 'run.failed', {
            root_run_id: run.id,
            code: 'lease_expired_recovered',
          });
        }
      }
    } catch (error: unknown) {
      this.options.logger.warn(
        { err: error instanceof Error ? error.message : String(error) },
        'Recovery sweep failed',
      );
    }
  }

  private async executeRun(runId: string, input: StartRunInput): Promise<void> {
    const controller = new AbortController();
    this.activeControllers.set(runId, controller);
    const leaseTimer = setInterval(() => {
      void this.options.store.renewLease(
        runId,
        this.ownerId,
        this.options.leaseMs,
      );
    }, Math.max(2000, Math.floor(this.options.leaseMs / 3)));
    leaseTimer.unref();

    const mcpClient = createBusinessMcpClient(this.options.mcpConfig, {
      actorId: input.identity.actorId,
      orgId: input.identity.orgId ?? input.identity.actorId,
      scopeId: input.identity.scopeId ?? input.identity.projectId,
      projectId: input.identity.projectId,
      runId,
      ...(input.traceparent ? { traceparent: input.traceparent } : {}),
    });

    try {
      const requiresApproval = Boolean(input.manifest.requires_approval);
      if (requiresApproval) {
        const workflow = this.options.agentTeam.mastra.getWorkflow(
          'creativeDeliveryWorkflow',
        );
        const wfRun = await workflow.createRun();
        const result = await wfRun.start({
          inputData: {
            runId,
            projectId: input.identity.projectId,
            goal: input.goal,
            estimatedCostMicros: input.budgetMicros ?? 0,
            requiresApproval: true,
          },
        });
        if (result.status === 'suspended') {
          await this.options.store.updateRunStatus(runId, 'waiting_approval', {
            workflowRunId: wfRun.runId,
          });
          await this.emitDurableEvent(runId, 'approval.required', {
            root_run_id: runId,
            workflow_run_id: wfRun.runId,
            suspended: result.suspended,
          });
          return;
        }
      }

      let toolsets: Record<string, unknown> = {};
      try {
        const rawTools = await mcpClient.listTools();
        const selectedSkillNames = Array.isArray(input.manifest.skills)
          ? input.manifest.skills.filter(
              (s): s is string => typeof s === 'string',
            )
          : [];
        if (selectedSkillNames.length > 0) {
          const allowlist = this.options.skills
            .filter((skill) => selectedSkillNames.includes(skill.name))
            .flatMap((skill) => skill.allowedTools);
          toolsets = filterToolsByAllowlist(rawTools, allowlist);
        } else {
          toolsets = rawTools;
        }
      } catch (error: unknown) {
        this.options.logger.debug(
          {
            runId,
            err: error instanceof Error ? error.message : String(error),
          },
          'Go MCP tool discovery unavailable; continuing with core agent',
        );
      }

      const modelKey =
        typeof input.manifest.model_key === 'string'
          ? input.manifest.model_key
          : undefined;
      const producer = await this.options.agentTeam.createScopedProducer(
        {
          actorId: input.identity.actorId,
          orgId: input.identity.orgId ?? input.identity.actorId,
          scopeId: input.identity.scopeId ?? input.identity.projectId,
          projectId: input.identity.projectId,
          runId,
          ...(input.traceparent ? { traceparent: input.traceparent } : {}),
        },
        modelKey,
      );

      const response = await producer.generate(input.goal, {
        maxSteps: 8,
        ...(Object.keys(toolsets).length > 0 ? { toolsets: { glowloom: toolsets } as never } : {}),
      });

      const assistantText = response.text ?? '';
      const msg = await this.options.store.appendPublicMessage(
        runId,
        'assistant',
        assistantText,
      );
      await this.emitDurableEvent(runId, 'message.completed', {
        root_run_id: runId,
        message: msg,
      });
      await this.options.store.updateRunStatus(runId, 'completed');
      await this.emitDurableEvent(runId, 'run.completed', {
        root_run_id: runId,
      });
    } catch (error: unknown) {
      if (controller.signal.aborted) {
        return;
      }
      this.options.logger.error(
        {
          runId,
          err: error instanceof Error ? error.message : String(error),
        },
        'Agent run execution failed',
      );
      await this.options.store.updateRunStatus(runId, 'failed', {
        errorCode: 'execution_failed',
      });
      await this.emitDurableEvent(runId, 'run.failed', {
        root_run_id: runId,
        code: 'execution_failed',
      });
    } finally {
      clearInterval(leaseTimer);
      this.activeControllers.delete(runId);
      await mcpClient.disconnect().catch(() => undefined);
    }
  }

  private async emitDurableEvent(
    runId: string,
    eventType: string,
    payload: Record<string, unknown>,
  ): Promise<void> {
    const event = await this.options.store.appendEvent(
      runId,
      eventType,
      payload,
    );
    await this.options.eventBus.publish(event);
  }
}
