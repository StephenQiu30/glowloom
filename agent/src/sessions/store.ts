import { createHash, randomUUID } from 'node:crypto';
import { sql, type Kysely } from 'kysely';
import type { AgentDatabase, RunStatus } from '../database/schema.js';
import { runIdentifier } from '../skills/run-skills.js';
import { pinnedSkillSchema, type PinnedSkill } from '../skills/loader.js';
import { AgentError } from './errors.js';
import type {
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

export function hashContent(value: unknown): string {
  return createHash('sha256').update(JSON.stringify(value)).digest('hex');
}

export function isTerminalStatus(status: RunStatus): boolean {
  return (
    status === 'completed' || status === 'failed' || status === 'cancelled'
  );
}

function cursorSequence(cursor: string): string {
  const sequence = /^(0|[1-9]\d{0,18}):0$/.exec(cursor)?.[1];
  if (sequence === undefined || BigInt(sequence) > 9223372036854775807n) {
    throw new AgentError('invalid_cursor', 'Invalid run cursor');
  }
  return sequence;
}

export class SessionStore {
  constructor(private readonly db: Kysely<AgentDatabase>) {}

  async ready(): Promise<boolean> {
    try {
      await sql`SELECT 1`.execute(this.db);
      return true;
    } catch {
      return false;
    }
  }

  async createSession(input: CreateSessionInput): Promise<SessionView> {
    const contentHash = hashContent({
      title: input.title,
      canvasId: input.canvasId ?? null,
    });
    const existing = await this.db
      .selectFrom('agent.service_sessions')
      .selectAll()
      .where('actor_id', '=', input.identity.actorId)
      .where('project_id', '=', input.identity.projectId)
      .where('request_id', '=', input.requestId)
      .executeTakeFirst();

    if (existing) {
      if (existing.content_hash !== contentHash) {
        throw new AgentError(
          'idempotency_conflict',
          'Session request_id reused with different content',
        );
      }
      return this.toSessionView(existing);
    }

    const id = randomUUID();
    const inserted = await this.db
      .insertInto('agent.service_sessions')
      .values({
        id,
        actor_id: input.identity.actorId,
        org_id: input.identity.orgId ?? input.identity.actorId,
        scope_id: input.identity.scopeId ?? input.identity.projectId,
        project_id: input.identity.projectId,
        canvas_id: input.canvasId ?? null,
        title: input.title,
        request_id: input.requestId,
        content_hash: contentHash,
        execution_revision: 0,
      })
      .returningAll()
      .executeTakeFirstOrThrow();

    return this.toSessionView(inserted);
  }

  async listSessions(
    identity: Identity,
    limit = 20,
  ): Promise<{ items: SessionView[]; next_cursor?: string }> {
    const safeLimit = Math.min(Math.max(limit, 1), 50);
    const rows = await this.db
      .selectFrom('agent.service_sessions')
      .selectAll()
      .where('actor_id', '=', identity.actorId)
      .where('project_id', '=', identity.projectId)
      .orderBy('created_at', 'desc')
      .orderBy('id', 'desc')
      .limit(safeLimit)
      .execute();

    return { items: rows.map((row) => this.toSessionView(row)) };
  }

  async getSession(
    identity: Identity,
    sessionId: string,
  ): Promise<SessionView> {
    const row = await this.db
      .selectFrom('agent.service_sessions')
      .selectAll()
      .where('id', '=', sessionId)
      .where('actor_id', '=', identity.actorId)
      .where('project_id', '=', identity.projectId)
      .executeTakeFirst();

    if (!row) {
      throw new AgentError('session_not_found', 'Session not found');
    }

    const latestRun = await this.db
      .selectFrom('agent.service_runs')
      .selectAll()
      .where('session_id', '=', sessionId)
      .orderBy('accepted_revision', 'desc')
      .limit(1)
      .executeTakeFirst();

    const view = this.toSessionView(row);
    if (latestRun) {
      view.run = await this.buildRunView(latestRun);
    }
    return view;
  }

  async updateSession(input: UpdateSessionInput): Promise<SessionView> {
    const updates: Record<string, unknown> = {};
    if (input.title !== undefined) updates.title = input.title;
    if (input.pinned !== undefined) updates.pinned = input.pinned;
    if (input.archived !== undefined) {
      updates.archived_at = input.archived ? sql`clock_timestamp()` : null;
    }

    const row = await this.db
      .updateTable('agent.service_sessions')
      .set(updates)
      .where('id', '=', input.sessionId)
      .where('actor_id', '=', input.identity.actorId)
      .where('project_id', '=', input.identity.projectId)
      .returningAll()
      .executeTakeFirst();

    if (!row) {
      throw new AgentError('session_not_found', 'Session not found');
    }
    return this.toSessionView(row);
  }

  async admitRun(
    input: StartRunInput,
    ownerId: string,
    leaseMs: number,
    prepareSkills: (
      runId: string,
      submissionId: string,
    ) => Promise<PinnedSkill[]>,
  ): Promise<{
    runId: string;
    submissionId: string;
    acceptedRevision: number;
    deduplicated: boolean;
  }> {
    const contentHash = hashContent({
      goal: input.goal,
      manifest: input.manifest,
      input: input.input ?? null,
    });

    return this.db.transaction().execute(async (trx) => {
      const session = await trx
        .selectFrom('agent.service_sessions')
        .selectAll()
        .where('id', '=', input.sessionId)
        .where('actor_id', '=', input.identity.actorId)
        .where('project_id', '=', input.identity.projectId)
        .forUpdate()
        .executeTakeFirst();

      if (!session) {
        throw new AgentError('session_not_found', 'Session not found');
      }

      const workflow = input.manifest.workflow;
      const source =
        workflow && typeof workflow === 'object' && 'source' in workflow
          ? workflow.source
          : undefined;
      if (
        source &&
        typeof source === 'object' &&
        'canvasId' in source &&
        source.canvasId !== session.canvas_id
      ) {
        throw new AgentError(
          'invalid_request',
          'Skill workflow canvas does not match the session',
        );
      }
      if (workflow && !session.canvas_id) {
        throw new AgentError(
          'invalid_request',
          'A Skill workflow requires a canvas session',
        );
      }

      const existing = await trx
        .selectFrom('agent.service_runs')
        .selectAll()
        .where('session_id', '=', input.sessionId)
        .where('request_id', '=', input.requestId)
        .executeTakeFirst();

      if (existing) {
        if (existing.content_hash !== contentHash) {
          throw new AgentError(
            'idempotency_conflict',
            'Run request_id reused with different payload',
          );
        }
        return {
          runId: existing.id,
          submissionId: existing.submission_id,
          acceptedRevision: Number(existing.accepted_revision),
          deduplicated: true,
        };
      }

      const currentRev = Number(session.execution_revision);
      if (
        input.expectedExecutionRevision >= 0 &&
        input.expectedExecutionRevision !== currentRev
      ) {
        throw new AgentError(
          'revision_conflict',
          `Expected revision ${input.expectedExecutionRevision}, current ${currentRev}`,
        );
      }

      const active = await trx
        .selectFrom('agent.service_runs')
        .select('id')
        .where('session_id', '=', input.sessionId)
        .where('status', 'in', ['queued', 'running'])
        .executeTakeFirst();

      if (active) {
        throw new AgentError(
          'run_already_active',
          'Another run is currently active in this session',
        );
      }

      const nextRev = currentRev + 1;
      const runId = runIdentifier(
        input.sessionId,
        `run:${input.requestId.toLowerCase()}`,
      );
      const submissionId = runIdentifier(runId, 'submission:initial');
      let skillRevisions: string[];
      if (input.retryOfRunId) {
        const original = await trx
          .selectFrom('agent.service_runs')
          .select('skill_revisions')
          .where('id', '=', input.retryOfRunId)
          .where('session_id', '=', input.sessionId)
          .executeTakeFirst();
        if (!original)
          throw new AgentError('run_not_found', 'Original run not found');
        skillRevisions = original.skill_revisions;
      } else {
        const skills = await prepareSkills(runId, submissionId);
        skillRevisions = skills.map((skill) => skill.digest);
        if (skills.length > 0) {
          await trx
            .insertInto('agent.skill_revisions')
            .values(
              skills.map((skill) => ({
                digest: skill.digest,
                bundle: JSON.stringify(skill),
              })),
            )
            .onConflict((conflict) => conflict.column('digest').doNothing())
            .execute();
        }
      }

      await trx
        .updateTable('agent.service_sessions')
        .set({
          execution_revision: nextRev,
          last_input_at: sql`clock_timestamp()`,
        })
        .where('id', '=', input.sessionId)
        .execute();

      await trx
        .insertInto('agent.service_runs')
        .values({
          id: runId,
          session_id: input.sessionId,
          request_id: input.requestId,
          content_hash: contentHash,
          goal: input.goal,
          manifest: input.manifest,
          skill_revisions: sql<
            string[]
          >`CAST(${JSON.stringify(skillRevisions)} AS jsonb)`,
          submission_id: submissionId,
          accepted_revision: nextRev,
          status: 'running',
          lease_owner: ownerId,
          lease_until: sql`clock_timestamp() + (${leaseMs} || ' milliseconds')::interval`,
          workflow_run_id: null,
          error_code: null,
          input: input.input ?? null,
          retry_of_run_id: input.retryOfRunId ?? null,
        })
        .execute();

      return {
        runId,
        submissionId,
        acceptedRevision: nextRev,
        deduplicated: false,
      };
    });
  }

  async skillsForRun(runId: string): Promise<PinnedSkill[]> {
    const run = await this.db
      .selectFrom('agent.service_runs')
      .select('skill_revisions')
      .where('id', '=', runId)
      .executeTakeFirstOrThrow();
    if (run.skill_revisions.length === 0) return [];
    const rows = await this.db
      .selectFrom('agent.skill_revisions')
      .selectAll()
      .where('digest', 'in', run.skill_revisions)
      .execute();
    const byDigest = new Map(
      rows.map((row) => [row.digest, pinnedSkillSchema.parse(row.bundle)]),
    );
    return run.skill_revisions.map((digest) => {
      const skill = byDigest.get(digest);
      if (!skill || skill.digest !== digest)
        throw new Error('Pinned Skill revision unavailable');
      return skill;
    });
  }

  async appendEvent(
    runId: string,
    eventType: string,
    payload: Record<string, unknown>,
  ): Promise<RunEvent> {
    return this.db.transaction().execute(async (trx) => {
      const last = await trx
        .selectFrom('agent.service_run_events')
        .select('seq')
        .where('run_id', '=', runId)
        .orderBy('seq', 'desc')
        .limit(1)
        .executeTakeFirst();

      const nextSeq = last ? Number(last.seq) + 1 : 1;
      await trx
        .insertInto('agent.service_run_events')
        .values({
          run_id: runId,
          seq: nextSeq,
          event_type: eventType,
          payload,
        })
        .execute();

      return {
        id: `${nextSeq}:0`,
        seq: nextSeq,
        runId,
        event: eventType,
        data: payload,
      };
    });
  }

  async appendPublicMessage(
    runId: string,
    kind: string,
    content: string,
  ): Promise<PublicMessageView> {
    return this.db.transaction().execute(async (trx) => {
      const last = await trx
        .selectFrom('agent.service_public_messages')
        .select('seq')
        .where('run_id', '=', runId)
        .orderBy('seq', 'desc')
        .limit(1)
        .executeTakeFirst();

      const nextSeq = last ? Number(last.seq) + 1 : 1;
      const messageId = randomUUID();
      await trx
        .insertInto('agent.service_public_messages')
        .values({
          run_id: runId,
          seq: nextSeq,
          part_index: 0,
          message_id: messageId,
          kind,
          content,
        })
        .execute();

      return {
        message_id: messageId,
        run_id: runId,
        conversation_id: 1,
        sequence: `${nextSeq}:0`,
        part_index: 0,
        kind,
        content,
      };
    });
  }

  async updateRunStatus(
    runId: string,
    status: RunStatus,
    extra?: { workflowRunId?: string | null; errorCode?: string | null },
  ): Promise<void> {
    await this.db
      .updateTable('agent.service_runs')
      .set({
        status,
        ...(extra?.workflowRunId !== undefined
          ? { workflow_run_id: extra.workflowRunId }
          : {}),
        ...(extra?.errorCode !== undefined
          ? { error_code: extra.errorCode }
          : {}),
        updated_at: sql`clock_timestamp()`,
        ...(isTerminalStatus(status)
          ? { completed_at: sql`clock_timestamp()`, lease_until: null }
          : {}),
      })
      .where('id', '=', runId)
      .execute();
  }

  async renewLease(
    runId: string,
    ownerId: string,
    leaseMs: number,
  ): Promise<void> {
    await this.db
      .updateTable('agent.service_runs')
      .set({
        lease_until: sql`clock_timestamp() + (${leaseMs} || ' milliseconds')::interval`,
        updated_at: sql`clock_timestamp()`,
      })
      .where('id', '=', runId)
      .where('lease_owner', '=', ownerId)
      .execute();
  }

  async claimExpiredRuns(limit = 10): Promise<
    Array<{
      id: string;
      session_id: string;
      workflow_run_id: string | null;
      status: RunStatus;
    }>
  > {
    return this.db
      .selectFrom('agent.service_runs')
      .select(['id', 'session_id', 'workflow_run_id', 'status'])
      .where('status', 'in', ['queued', 'running'])
      .where('lease_until', '<', sql<Date>`clock_timestamp()`)
      .limit(limit)
      .execute();
  }

  async recordDecision(
    input: ApplyDecisionInput,
  ): Promise<{ deduplicated: boolean; workflowRunId: string | null }> {
    const contentHash = hashContent(input.payload);
    return this.db.transaction().execute(async (trx) => {
      const run = await trx
        .selectFrom('agent.service_runs as r')
        .innerJoin('agent.service_sessions as s', 's.id', 'r.session_id')
        .select(['r.id', 'r.workflow_run_id'])
        .where('r.id', '=', input.runId)
        .where('s.actor_id', '=', input.identity.actorId)
        .where('s.project_id', '=', input.identity.projectId)
        .executeTakeFirst();

      if (!run) {
        throw new AgentError('run_not_found', 'Run not found');
      }

      const existing = await trx
        .selectFrom('agent.service_control_inbox')
        .selectAll()
        .where('decision_id', '=', input.decisionId)
        .executeTakeFirst();

      if (existing) {
        if (existing.content_hash !== contentHash) {
          throw new AgentError(
            'idempotency_conflict',
            'Decision ID reused with different payload',
          );
        }
        return { deduplicated: true, workflowRunId: run.workflow_run_id };
      }

      await trx
        .insertInto('agent.service_control_inbox')
        .values({
          decision_id: input.decisionId,
          root_run_id: input.runId,
          authorization_revision: input.authorizationRevision,
          content_hash: contentHash,
          input: input.payload,
          receipt: { accepted: true, action: input.action },
        })
        .execute();

      return { deduplicated: false, workflowRunId: run.workflow_run_id };
    });
  }

  async getRun(identity: Identity, runId: string): Promise<RunView> {
    const run = await this.db
      .selectFrom('agent.service_runs as r')
      .innerJoin('agent.service_sessions as s', 's.id', 'r.session_id')
      .selectAll('r')
      .where('r.id', '=', runId)
      .where('s.actor_id', '=', identity.actorId)
      .where('s.project_id', '=', identity.projectId)
      .executeTakeFirst();

    if (!run) {
      throw new AgentError('run_not_found', 'Run not found');
    }
    return this.buildRunView(run);
  }

  async listRuns(
    identity: Identity,
    sessionId: string,
    limit = 20,
  ): Promise<{ items: Array<Record<string, unknown>>; next_cursor?: string }> {
    const safeLimit = Math.min(Math.max(limit, 1), 50);
    const rows = await this.db
      .selectFrom('agent.service_runs as r')
      .innerJoin('agent.service_sessions as s', 's.id', 'r.session_id')
      .selectAll('r')
      .where('r.session_id', '=', sessionId)
      .where('s.actor_id', '=', identity.actorId)
      .where('s.project_id', '=', identity.projectId)
      .orderBy('r.accepted_revision', 'desc')
      .limit(safeLimit)
      .execute();

    return {
      items: rows.map((r) => ({
        root_run_id: r.id,
        session_id: r.session_id,
        submission_id: r.submission_id,
        accepted_revision: Number(r.accepted_revision),
        status: r.status,
        goal: r.goal,
        created_at: r.created_at.toISOString(),
        ...(r.retry_of_run_id ? { retry_of_run_id: r.retry_of_run_id } : {}),
      })),
    };
  }

  async getSubmission(
    identity: Identity,
    sessionId: string,
    requestId: string,
  ): Promise<Record<string, unknown>> {
    const row = await this.db
      .selectFrom('agent.service_runs as r')
      .innerJoin('agent.service_sessions as s', 's.id', 'r.session_id')
      .selectAll('r')
      .where('r.session_id', '=', sessionId)
      .where('r.request_id', '=', requestId)
      .where('s.actor_id', '=', identity.actorId)
      .where('s.project_id', '=', identity.projectId)
      .executeTakeFirst();

    if (!row) {
      throw new AgentError('submission_not_found', 'Submission not found');
    }
    return {
      root_run_id: row.id,
      session_id: row.session_id,
      submission_id: row.submission_id,
      accepted_revision: Number(row.accepted_revision),
      status: row.status,
    };
  }

  async getCurrentSubmission(
    identity: Identity,
    sessionId: string,
  ): Promise<Record<string, unknown>> {
    const latest = await this.db
      .selectFrom('agent.service_runs as r')
      .innerJoin('agent.service_sessions as s', 's.id', 'r.session_id')
      .selectAll('r')
      .where('r.session_id', '=', sessionId)
      .where('s.actor_id', '=', identity.actorId)
      .where('s.project_id', '=', identity.projectId)
      .orderBy('r.accepted_revision', 'desc')
      .limit(1)
      .executeTakeFirst();

    if (!latest) {
      return {
        active_run: null,
        latest_accepted: null,
        unresolved_run_ids: [],
      };
    }

    const summary = {
      root_run_id: latest.id,
      session_id: latest.session_id,
      submission_id: latest.submission_id,
      accepted_revision: Number(latest.accepted_revision),
      status: latest.status,
    };

    return {
      active_run: isTerminalStatus(latest.status) ? null : summary,
      latest_accepted: summary,
      unresolved_run_ids: isTerminalStatus(latest.status) ? [] : [latest.id],
    };
  }

  async listMessages(
    identity: Identity,
    runId: string,
    limit = 20,
    before?: string,
    through?: string,
  ): Promise<{
    items: PublicMessageView[];
    next_cursor?: string;
    through: string;
  }> {
    await this.getRun(identity, runId);
    const safeLimit = Math.min(Math.max(limit, 1), 100);

    let query = this.db
      .selectFrom('agent.service_public_messages')
      .selectAll()
      .where('run_id', '=', runId);

    if (before) query = query.where('seq', '<', cursorSequence(before));
    if (through) query = query.where('seq', '<=', cursorSequence(through));

    const rows = await query
      .orderBy('seq', 'desc')
      .orderBy('part_index', 'desc')
      .limit(safeLimit)
      .execute();

    const items = rows.reverse().map((row) => ({
      message_id: row.message_id,
      run_id: row.run_id,
      conversation_id: 1,
      sequence: `${row.seq}:0`,
      part_index: row.part_index,
      kind: row.kind,
      content: row.content,
    }));

    const watermark = through ?? items.at(-1)?.sequence ?? '0:0';
    return { items, through: watermark };
  }

  async getMessage(
    identity: Identity,
    runId: string,
    messageId: string,
  ): Promise<PublicMessageView> {
    await this.getRun(identity, runId);
    const row = await this.db
      .selectFrom('agent.service_public_messages')
      .selectAll()
      .where('run_id', '=', runId)
      .where('message_id', '=', messageId)
      .executeTakeFirst();

    if (!row) {
      throw new AgentError('message_not_found', 'Message not found');
    }
    return {
      message_id: row.message_id,
      run_id: row.run_id,
      conversation_id: 1,
      sequence: `${row.seq}:0`,
      part_index: row.part_index,
      kind: row.kind,
      content: row.content,
    };
  }

  async listRunEventsAfter(
    identity: Identity,
    runId: string,
    lastEventId?: string,
  ): Promise<{ events: RunEvent[]; terminal: boolean }> {
    const run = await this.getRun(identity, runId);
    let query = this.db
      .selectFrom('agent.service_run_events')
      .selectAll()
      .where('run_id', '=', runId);

    if (lastEventId)
      query = query.where('seq', '>', cursorSequence(lastEventId));

    const rows = await query.orderBy('seq', 'asc').execute();
    return {
      events: rows.map((row) => ({
        id: `${row.seq}:0`,
        seq: Number(row.seq),
        runId: row.run_id,
        event: row.event_type,
        data: row.payload,
      })),
      terminal: isTerminalStatus(run.state.status),
    };
  }

  private toSessionView(row: {
    id: string;
    actor_id: string;
    org_id: string;
    scope_id: string;
    project_id: string;
    canvas_id: string | null;
    title: string;
    execution_revision: string;
    pinned: boolean;
    archived_at: Date | null;
    created_at: Date;
    last_input_at: Date | null;
  }): SessionView {
    return {
      id: row.id,
      actor_id: row.actor_id,
      org_id: row.org_id,
      scope_id: row.scope_id,
      project_id: row.project_id,
      ...(row.canvas_id ? { canvas_id: row.canvas_id } : {}),
      title: row.title,
      execution_revision: Number(row.execution_revision),
      pinned: row.pinned,
      ...(row.archived_at
        ? { archived_at: row.archived_at.toISOString() }
        : {}),
      created_at: row.created_at.toISOString(),
      ...(row.last_input_at
        ? { last_input_at: row.last_input_at.toISOString() }
        : {}),
    };
  }

  private async buildRunView(row: {
    id: string;
    session_id: string;
    submission_id: string;
    goal: string;
    manifest: Record<string, unknown>;
    accepted_revision: string;
    status: RunStatus;
    workflow_run_id: string | null;
    error_code: string | null;
  }): Promise<RunView> {
    const events = await this.db
      .selectFrom('agent.service_run_events')
      .selectAll()
      .where('run_id', '=', row.id)
      .orderBy('seq', 'asc')
      .limit(100)
      .execute();

    const messages = await this.db
      .selectFrom('agent.service_public_messages')
      .selectAll()
      .where('run_id', '=', row.id)
      .orderBy('seq', 'asc')
      .limit(50)
      .execute();

    const lastSeq =
      events.length > 0 ? String(events[events.length - 1]!.seq) : '0';

    return {
      root_run_id: row.id,
      session_id: row.session_id,
      submission_id: row.submission_id,
      goal: row.goal,
      manifest: row.manifest,
      execution_available: true,
      state: {
        status: row.status,
        execution_revision: Number(row.accepted_revision),
        ...(row.workflow_run_id
          ? { workflow_run_id: row.workflow_run_id }
          : {}),
        ...(row.error_code ? { error_code: row.error_code } : {}),
      },
      snapshot_cursor: `${lastSeq}:0`,
      messages_page: {
        items: messages.map((m) => ({
          message_id: m.message_id,
          run_id: m.run_id,
          conversation_id: 1,
          sequence: `${m.seq}:0`,
          part_index: m.part_index,
          kind: m.kind,
          content: m.content,
        })),
        through: `${messages.at(-1)?.seq ?? '0'}:0`,
      },
      events: events.map((e) => ({
        event_type: e.event_type,
        payload: e.payload,
      })),
    };
  }
}
