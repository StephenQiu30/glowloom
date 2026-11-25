import { Mastra } from '@mastra/core';
import { Agent } from '@mastra/core/agent';
import {
  createCanvasWorkflow,
  type CanvasStageRunner,
  type CanvasWorkflow,
} from './canvas-workflow.js';
import type { MastraModelConfig } from '@mastra/core/llm';
import { Memory } from '@mastra/memory';
import type { PostgresStore } from '@mastra/pg';
import type { Logger } from 'pino';
import {
  createGatewayModel,
  type GatewayProviderConfig,
  type RunIdentityHeaders,
} from '../business/gateway-provider.js';
import type { PromptRegistry } from '../observability/langfuse.js';
import { SkillRegistry } from '../skills/loader.js';
import type { Workspace } from '@mastra/core/workspace';
import {
  createDynamicPipelineWorkflow,
  PipelineRegistry,
} from './pipeline-engine.js';
import { creativeDeliveryWorkflow } from './workflows.js';

export interface MastraRuntimeOptions {
  storage?: PostgresStore;
  gatewayConfig: GatewayProviderConfig;
  prompts: PromptRegistry;
  skills: SkillRegistry;
  pipelines: PipelineRegistry;
  logger: Logger;
  defaultModelFactory?: (
    identity: RunIdentityHeaders,
    modelKey?: string,
  ) => MastraModelConfig;
}

export interface AgentTeam {
  mastra: Mastra;
  producer: Agent;
  scriptwriter: Agent;
  assetDirector: Agent;
  production: Agent;
  continuityReviewer: Agent;
  runCanvasWorkflow(
    runId: string,
    graph: CanvasWorkflow,
    execute: CanvasStageRunner,
    signal: AbortSignal,
  ): Promise<string>;
  createScopedProducer(
    identity: RunIdentityHeaders,
    modelKey?: string,
    extraInstructions?: string,
    workspace?: Workspace,
    stageOnly?: boolean,
  ): Promise<Agent>;
}

export async function createMastraRuntime(
  options: MastraRuntimeOptions,
): Promise<AgentTeam> {
  const [
    producerPrompt,
    scriptwriterPrompt,
    assetDirectorPrompt,
    productionPrompt,
    reviewerPrompt,
  ] = await Promise.all([
    options.prompts.resolve('producer'),
    options.prompts.resolve('scriptwriter'),
    options.prompts.resolve('asset-director'),
    options.prompts.resolve('production'),
    options.prompts.resolve('continuity-reviewer'),
  ]);

  const memory = options.storage
    ? new Memory({
        storage: options.storage,
        options: {
          lastMessages: 20,
          workingMemory: {
            enabled: true,
          },
        },
      })
    : undefined;

  const modelFactory =
    options.defaultModelFactory ??
    ((identity: RunIdentityHeaders, modelKey?: string) =>
      createGatewayModel(options.gatewayConfig, identity, modelKey));

  const systemIdentity: RunIdentityHeaders = {
    actorId: '00000000-0000-0000-0000-000000000000',
    orgId: '00000000-0000-0000-0000-000000000000',
    scopeId: '00000000-0000-0000-0000-000000000000',
    projectId: '00000000-0000-0000-0000-000000000000',
  };

  const baseModel = modelFactory(systemIdentity);

  const scriptwriter = new Agent({
    id: 'scriptwriter',
    name: 'Glowloom ScriptAgent',
    instructions: scriptwriterPrompt,
    model: baseModel,
    ...(memory ? { memory } : {}),
  });

  const assetDirector = new Agent({
    id: 'asset_director',
    name: 'Glowloom AssetDirectorAgent',
    instructions: assetDirectorPrompt,
    model: baseModel,
    ...(memory ? { memory } : {}),
  });

  const production = new Agent({
    id: 'production',
    name: 'Glowloom ProductionAgent',
    instructions: productionPrompt,
    model: baseModel,
    ...(memory ? { memory } : {}),
  });

  const continuityReviewer = new Agent({
    id: 'continuity_reviewer',
    name: 'Glowloom ContinuityReviewerAgent',
    instructions: reviewerPrompt,
    model: baseModel,
  });

  const dynamicPipelineWorkflow = createDynamicPipelineWorkflow({
    pipelines: options.pipelines,
    skills: options.skills,
  });

  const producer = new Agent({
    id: 'producer',
    name: 'Glowloom Producer',
    instructions: producerPrompt,
    model: baseModel,
    agents: {
      scriptwriter,
      assetDirector,
      production,
      continuityReviewer,
    },
    workflows: { creativeDeliveryWorkflow, dynamicPipelineWorkflow },
    ...(memory ? { memory } : {}),
  });

  const mastra = new Mastra({
    agents: {
      producer,
      scriptwriter,
      assetDirector,
      production,
      continuityReviewer,
    },
    workflows: { creativeDeliveryWorkflow, dynamicPipelineWorkflow },
    ...(options.storage ? { storage: options.storage } : {}),
  });

  return {
    mastra,
    producer,
    scriptwriter,
    assetDirector,
    production,
    continuityReviewer,
    async runCanvasWorkflow(runId, graph, execute, signal) {
      const workflow = createCanvasWorkflow(execute);
      // The closure is scoped to this invocation. The registered workflow
      // definitions and unrelated agents cannot observe its tools or skills.
      workflow.__registerMastra(mastra);
      const run = await workflow.createRun({ runId });
      const cancel = () => {
        void run.cancel().catch(() => undefined);
      };
      signal.throwIfAborted();
      signal.addEventListener('abort', cancel, { once: true });
      try {
        const outcome = await run.start({ inputData: { runId, graph } });
        signal.throwIfAborted();
        if (outcome.status !== 'success') {
          throw new Error(`Canvas workflow ended with ${outcome.status}`);
        }
        return outcome.result.results
          .map((result) => `## ${result.title}\n\n${result.text}`)
          .join('\n\n');
      } finally {
        signal.removeEventListener('abort', cancel);
      }
    },
    async createScopedProducer(
      identity: RunIdentityHeaders,
      modelKey?: string,
      extraInstructions?: string,
      workspace?: Workspace,
      stageOnly?: boolean,
    ): Promise<Agent> {
      const basePrompt = await options.prompts.resolve('producer');
      const instructions = extraInstructions
        ? `${basePrompt}\n\n${extraInstructions}`
        : basePrompt;
      const scopedModel = modelFactory(identity, modelKey);
      if (stageOnly) {
        return new Agent({
          id: 'workflow-stage',
          name: 'Glowloom Workflow Stage',
          model: scopedModel,
          maxRetries: 0,
          instructions: extraInstructions ?? basePrompt,
          ...(workspace ? { workspace } : {}),
        });
      }

      const scopedScriptwriter = new Agent({
        id: 'scriptwriter',
        name: 'Glowloom ScriptAgent',
        instructions: await options.prompts.resolve('scriptwriter'),
        model: scopedModel,
        maxRetries: 0,
        ...(workspace ? { workspace } : {}),
        ...(memory ? { memory } : {}),
      });
      const scopedAssetDirector = new Agent({
        id: 'asset_director',
        name: 'Glowloom AssetDirectorAgent',
        instructions: await options.prompts.resolve('asset-director'),
        model: scopedModel,
        maxRetries: 0,
        ...(workspace ? { workspace } : {}),
        ...(memory ? { memory } : {}),
      });
      const scopedProduction = new Agent({
        id: 'production',
        name: 'Glowloom ProductionAgent',
        instructions: await options.prompts.resolve('production'),
        model: scopedModel,
        maxRetries: 0,
        ...(workspace ? { workspace } : {}),
        ...(memory ? { memory } : {}),
      });
      const scopedReviewer = new Agent({
        id: 'continuity_reviewer',
        name: 'Glowloom ContinuityReviewerAgent',
        instructions: await options.prompts.resolve('continuity-reviewer'),
        model: scopedModel,
        maxRetries: 0,
        ...(workspace ? { workspace } : {}),
      });

      return new Agent({
        id: 'producer',
        name: 'Glowloom Producer',
        instructions,
        model: scopedModel,
        maxRetries: 0,
        ...(workspace ? { workspace } : {}),
        agents: {
          scriptwriter: scopedScriptwriter,
          assetDirector: scopedAssetDirector,
          production: scopedProduction,
          continuityReviewer: scopedReviewer,
        },
        workflows: { creativeDeliveryWorkflow },
        ...(memory ? { memory } : {}),
      });
    },
  };
}
