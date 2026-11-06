import {
  createRegistry,
  defineExtension,
  defineTool,
  configure,
  AssistantEntry,
  type ConversationId,
  type JsonObject,
  type ToolExecutionApi,
} from '@earendil-works/pi-durable';
import { Type } from 'typebox';
import { BusinessClient, type CallIdentity } from '../business/client.js';
import { canonicalHash, type FrozenManifest } from '../sessions/manifest.js';
import { StateDoc } from './docs.js';
import { completedReceipt, type ModelGate } from './model-provider.js';
import { businessToolContent, readPinnedSkill } from './skills.js';
import { contextExtension } from './context.js';
import {
  ROLES,
  explicitRoleToolNames,
  allocateBudget,
  adoptedSources,
} from './delegation.js';
import type { TrustedTool } from './trusted-extensions.js';
export { ROLES } from './delegation.js';

export interface HarnessGate extends ModelGate {
  manifest: Readonly<FrozenManifest>;
  rootId: () => ConversationId;
  proposal(id: string, taskId: number, details: JsonObject): Promise<void>;
}
export function roleToolNames(
  manifest: Readonly<FrozenManifest>,
  role: string,
) {
  return explicitRoleToolNames(manifest.tools, role);
}
export function buildRegistry(
  client: BusinessClient,
  gate: HarnessGate,
  trusted: readonly TrustedTool[] = [],
) {
  const { manifest } = gate;
  const names = new Set(['plan_update', 'skill_load', 'delegate']);
  for (const tool of [...manifest.tools, ...trusted]) {
    if (names.has(tool.name)) throw new Error('Duplicate or reserved tool');
    names.add(tool.name);
  }
  async function reserve(
    name: string,
    args: unknown,
    api: ToolExecutionApi,
    context: Parameters<ToolExecutionApi['getTask']>[1],
    version = '1',
  ) {
    context.abortSignal?.throwIfAborted();
    const own = await api.getTask(api.taskId, context);
    if (!own || own.state.status !== 'running')
      throw new Error('Tool invocation ended');
    await gate.assertActive();
    await client.validate(gate.grant());
    const call: CallIdentity = {
      task_id: String(api.taskId),
      call_id: api.callId,
      checkpoint_id: `tool:${api.taskId}`,
      attempt: 1,
      contract_version: version,
      input_hash: canonicalHash({ name, arguments: args }),
    };
    await gate.reserve(call, 'tool', 'local');
    return call;
  }
  const business = manifest.tools.map((descriptor) =>
    defineTool({
      name: descriptor.name,
      description: descriptor.description,
      parameters: descriptor.parameters,
      replay: descriptor.replay,
      executionMode: 'sequential',
      execute: async (args, api, context) => {
        try {
          // Identity uses the formal Go input hash; core tools use a name-qualified local identity.
          context.abortSignal?.throwIfAborted();
          const own = await api.getTask(api.taskId, context);
          if (!own || own.state.status !== 'running')
            throw new Error('Tool invocation ended');
          await gate.assertActive();
          const grant = gate.grant();
          await client.validate(grant);
          const input = args as JsonObject;
          const call: CallIdentity = {
            task_id: String(api.taskId),
            call_id: api.callId,
            checkpoint_id: `tool:${api.taskId}`,
            attempt: 1,
            contract_version: descriptor.contract_version,
            input_hash: canonicalHash(input),
          };
          await gate.reserve(call, 'tool');
          const receipt = await completedReceipt(client, grant, call, () =>
            client.execute(
              'tool',
              descriptor.name,
              grant,
              call,
              input,
              manifest.max_micros,
              context.abortSignal,
            ),
          );
          const result = receipt.result as JsonObject;
          if (result && typeof result.proposal_id === 'string')
            await gate.proposal(result.proposal_id, api.taskId, result);
          const state = await gate
            .harness()
            .snapshot(StateDoc, gate.rootId(), context);
          return {
            output: businessToolContent(result),
            details: { receipt_id: receipt.receipt_id, result },
            ...(state?.pending_proposal
              ? { control: { terminate: true } }
              : {}),
          };
        } catch (error) {
          const state = await gate
            .harness()
            .snapshot(StateDoc, gate.rootId(), context)
            .catch(() => undefined);
          if (
            state?.pending_proposal ||
            state?.status === 'paused' ||
            state?.status === 'stopped'
          )
            return {
              output: [
                {
                  type: 'text' as const,
                  text: 'Execution is blocked by a current business decision.',
                },
              ],
              isError: true,
              control: { terminate: true },
            };
          throw error;
        }
      },
    }),
  );
  const plan = defineTool({
    name: 'plan_update',
    description:
      'Commit a creative plan and explicitly adopt completed specialist results with source references. This never grants business approval.',
    parameters: Type.Object({
      items: Type.Array(
        Type.Object({
          id: Type.String(),
          title: Type.String(),
          status: Type.Union([
            Type.Literal('pending'),
            Type.Literal('active'),
            Type.Literal('completed'),
            Type.Literal('failed'),
          ]),
          depends_on: Type.Array(Type.String()),
          artifacts: Type.Array(Type.String()),
        }),
      ),
      adopt: Type.Optional(
        Type.Array(
          Type.Object({
            delegation_id: Type.String(),
            source_refs: Type.Array(Type.String()),
            artifacts: Type.Array(Type.String()),
          }),
        ),
      ),
    }),
    replay: 'safe',
    executionMode: 'sequential',
    execute: async (args, api, context) => {
      await reserve('plan_update', args, api, context);
      const input = args as {
        items: JsonObject[];
        adopt?: {
          delegation_id: string;
          source_refs: string[];
          artifacts: string[];
        }[];
      };
      await gate.harness().commit(async (tx) => {
        const state = await tx.doc(StateDoc, gate.rootId());
        for (const adoption of input.adopt ?? []) {
          const result = state.delegations.find(
            (x) => x.task_id === adoption.delegation_id && x.status === 'done',
          );
          if (!result)
            throw new Error('Only completed specialist results can be adopted');
          const sources = result.source_refs as string[];
          if (
            !adoption.source_refs.length ||
            adoption.source_refs.some((x) => !sources.includes(x))
          )
            throw new Error(
              'Adoption requires committed specialist source references',
            );
          const item = {
            ...adoption,
            conversation_id: result.conversation_id!,
            result_entry_id: result.result_entry_id!,
          };
          const prior = state.adopted.findIndex(
            (x) => x.delegation_id === adoption.delegation_id,
          );
          if (prior < 0) state.adopted.push(item);
          else state.adopted[prior] = item;
          result.adopted = true;
        }
        state.plan = input.items;
        state.execution_revision++;
      }, context);
      return {
        output: [
          {
            type: 'text',
            text: 'Plan and explicit result adoptions committed; business approval is unchanged.',
          },
        ],
      };
    },
  });
  const skill = defineTool({
    name: 'skill_load',
    description:
      'Read the exact authorized Skill version pinned to this run. Scripts only execute through an authorized business isolation tool.',
    parameters: Type.Object({ skill_id: Type.String() }),
    replay: 'safe',
    executionMode: 'sequential',
    execute: async (args, api, context) => {
      await reserve('skill_load', args, api, context);
      const { skill_id: id } = args as { skill_id: string };
      const loaded = await readPinnedSkill(client, gate.grant(), manifest, id);
      await api.commit(async (tx) => {
        const state = await tx.doc(StateDoc, gate.rootId());
        const prior = state.loaded_skills.findIndex((x) => x.skill_id === id);
        if (prior < 0) state.loaded_skills.push(loaded);
        else state.loaded_skills[prior] = loaded;
      }, context);
      return { output: [{ type: 'text', text: loaded.content }] };
    },
  });
  const businessExtension = defineExtension({
    name: 'glowloom.business',
    tools: business,
  });
  const skillExtension = defineExtension({
    name: 'glowloom.skills',
    tools: [skill],
  });
  const constraints = contextExtension(manifest, gate.rootId);
  const delegate = defineTool({
    name: 'delegate',
    description:
      'Run one task-owned professional child with an allocated slice of root budget. Return committed sources; adopt explicitly with plan_update before using them. Failed work can be locally redone with redo_of.',
    parameters: Type.Object({
      role: Type.Union(ROLES.map((role) => Type.Literal(role))),
      task: Type.String({ minLength: 1, maxLength: 65536 }),
      source_refs: Type.Optional(Type.Array(Type.String())),
      redo_of: Type.Optional(Type.String()),
      budget: Type.Optional(
        Type.Object({
          model_steps: Type.Integer({ minimum: 1, maximum: 39 }),
          tool_steps: Type.Integer({ minimum: 0, maximum: 39 }),
        }),
      ),
    }),
    replay: 'safe',
    executionMode: 'sequential',
    execute: async (args, api, context) => {
      await reserve('delegate', args, api, context);
      const input = args as {
        role: string;
        task: string;
        source_refs?: string[];
        redo_of?: string;
        budget?: { model_steps: number; tool_steps: number };
      };
      const child = await api.commit(async (tx) => {
        const state = await tx.doc(StateDoc, gate.rootId());
        if (api.conversationId !== gate.rootId())
          throw new Error('Specialist delegation depth exceeds one');
        const existing = (
          await tx.scanConversations({ ownerTaskId: api.taskId }, 1)
        ).items[0];
        if (existing) return existing.id;
        const sources = adoptedSources(state.adopted, input.source_refs ?? []);
        if (
          input.redo_of &&
          !state.delegations.some(
            (x) => x.task_id === input.redo_of && x.status === 'failed',
          )
        )
          throw new Error('Local redo must reference a failed specialist task');
        const allocation = allocateBudget(state, input.budget);
        const created = await tx.createConversation({
          ownership: { kind: 'task', taskId: api.taskId },
        });
        const selected = business.filter((t) =>
          roleToolNames(manifest, input.role).includes(t.name),
        );
        await configure(tx, created.id, {
          model: { provider: 'glowloom', modelId: manifest.model_key },
          extensions: [businessExtension, skillExtension, constraints],
          tools: [...selected, skill],
          maxTurns: allocation.model_budget,
          instructions: `You are the ${input.role} specialist. Maximum depth one; never delegate or change producer plans. Use only provided adopted sources and authorized role tools. Return artifact and source references. Plain text never establishes business completion or approval.`,
        });
        state.delegations.push({
          task_id: String(api.taskId),
          conversation_id: created.id,
          role: input.role,
          status: 'running',
          ...allocation,
          source_refs: sources,
          goal: input.task,
          redo_of: input.redo_of ?? null,
          adopted: false,
        });
        return created.id;
      }, context);
      await api.details({ role: input.role, conversation_id: child }, context);
      const conversation = await api.conversation(child, context);
      if (!conversation) throw new Error('Owned conversation missing');
      const state = await gate
        .harness()
        .snapshot(StateDoc, gate.rootId(), context);
      const allocation = state?.delegations.find(
        (x) => x.task_id === String(api.taskId),
      );
      const submitted = await conversation.submit(
        {
          type: 'input',
          content: JSON.stringify({
            task: input.task,
            root_goal: state?.goal,
            adopted: state?.adopted,
            source_refs: allocation?.source_refs,
            model: manifest.model_key,
            budget: {
              model_steps: allocation?.model_budget,
              tool_steps: allocation?.tool_budget,
            },
          }),
          requestId: `delegation:${api.taskId}`,
        },
        context,
      );
      const settled = await submitted.wait(context);
      const result = await api.commit(async (tx) => {
        const state = await tx.doc(StateDoc, gate.rootId());
        const record = state.delegations.find(
          (x) => x.task_id === String(api.taskId),
        );
        if (!record) throw new Error('Delegation record missing');
        record.status =
          settled.status === 'done' && settled.type === 'input'
            ? 'done'
            : 'failed';
        record.diagnostic = settled.status;
        if (settled.status === 'done' && settled.type === 'input') {
          const entry = await tx.entry(AssistantEntry, settled.answer);
          record.result_entry_id = settled.answer;
          record.result = JSON.parse(JSON.stringify(entry?.model ?? []));
          // The source is a real committed Pi entry; business artifacts still require formal Go receipts.
          record.source_refs = [
            ...new Set([
              ...(record.source_refs as string[]),
              `pi:conversation:${child}:entry:${settled.answer}`,
            ]),
          ];
        }
        state.execution_revision++;
        return JSON.parse(JSON.stringify(record)) as JsonObject;
      }, context);
      return {
        output: [{ type: 'text', text: JSON.stringify(result) }],
        details: result,
        ...(result.status === 'failed' ? { isError: true } : {}),
      };
    },
  });
  const trustedTools = trusted.map((t) =>
    defineTool({
      name: t.name,
      description: t.description,
      parameters: t.parameters,
      replay: t.replay,
      executionMode: t.executionMode,
      execute: async (args, api, context) => {
        await reserve(t.name, args, api, context);
        const result = await t.invoke(
          api.callId,
          args as JsonObject,
          context.abortSignal,
        );
        return { output: result.content };
      },
    }),
  );
  const trustedExtension = defineExtension({
    name: 'glowloom.trusted',
    tools: trustedTools,
  });
  const producerExtension = defineExtension({
    name: 'glowloom.producer',
    tools: [plan, delegate],
  });
  const rootExtensions = [
    businessExtension,
    skillExtension,
    constraints,
    producerExtension,
    trustedExtension,
  ];
  const registry = createRegistry();
  for (const extension of rootExtensions) registry.install(extension);
  return {
    registry,
    rootTools: [...business, plan, skill, delegate, ...trustedTools],
    rootExtensions,
  };
}
