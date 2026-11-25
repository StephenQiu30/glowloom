import { Mastra } from '@mastra/core';
import { Agent } from '@mastra/core/agent';
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
  createScopedProducer(
    identity: RunIdentityHeaders,
    modelKey?: string,
    extraInstructions?: string,
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
    async createScopedProducer(
      identity: RunIdentityHeaders,
      modelKey?: string,
      extraInstructions?: string,
    ): Promise<Agent> {
      const basePrompt = await options.prompts.resolve('producer');
      const instructions = extraInstructions
        ? `${basePrompt}\n\n${extraInstructions}`
        : basePrompt;
      const scopedModel = modelFactory(identity, modelKey);

      const scopedScriptwriter = new Agent({
        id: 'scriptwriter',
        name: 'Glowloom ScriptAgent',
        instructions: await options.prompts.resolve('scriptwriter'),
        model: scopedModel,
        ...(memory ? { memory } : {}),
      });
      const scopedAssetDirector = new Agent({
        id: 'asset_director',
        name: 'Glowloom AssetDirectorAgent',
        instructions: await options.prompts.resolve('asset-director'),
        model: scopedModel,
        ...(memory ? { memory } : {}),
      });
      const scopedProduction = new Agent({
        id: 'production',
        name: 'Glowloom ProductionAgent',
        instructions: await options.prompts.resolve('production'),
        model: scopedModel,
        ...(memory ? { memory } : {}),
      });
      const scopedReviewer = new Agent({
        id: 'continuity_reviewer',
        name: 'Glowloom ContinuityReviewerAgent',
        instructions: await options.prompts.resolve('continuity-reviewer'),
        model: scopedModel,
      });

      return new Agent({
        id: 'producer',
        name: 'Glowloom Producer',
        instructions,
        model: scopedModel,
        agents: {
          scriptwriter: scopedScriptwriter,
          assetDirector: scopedAssetDirector,
          production: scopedProduction,
          continuityReviewer: scopedReviewer,
        },
        workflows: { creativeDeliveryWorkflow, dynamicPipelineWorkflow },
        ...(memory ? { memory } : {}),
      });
    },
  };
}
