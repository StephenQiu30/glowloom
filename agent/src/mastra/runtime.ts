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
import { creativeDeliveryWorkflow } from './workflows.js';

export interface MastraRuntimeOptions {
  storage?: PostgresStore;
  gatewayConfig: GatewayProviderConfig;
  prompts: PromptRegistry;
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
  storyboarder: Agent;
  createScopedProducer(
    identity: RunIdentityHeaders,
    modelKey?: string,
    extraInstructions?: string,
  ): Promise<Agent>;
}

export async function createMastraRuntime(
  options: MastraRuntimeOptions,
): Promise<AgentTeam> {
  const [producerPrompt, scriptwriterPrompt, storyboarderPrompt] =
    await Promise.all([
      options.prompts.resolve('producer'),
      options.prompts.resolve('scriptwriter'),
      options.prompts.resolve('storyboarder'),
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

  const scriptwriter = new Agent({
    id: 'scriptwriter',
    name: 'Glowloom Scriptwriter',
    instructions: scriptwriterPrompt,
    model: modelFactory(systemIdentity),
    ...(memory ? { memory } : {}),
  });

  const storyboarder = new Agent({
    id: 'storyboarder',
    name: 'Glowloom Storyboarder',
    instructions: storyboarderPrompt,
    model: modelFactory(systemIdentity),
    ...(memory ? { memory } : {}),
  });

  const producer = new Agent({
    id: 'producer',
    name: 'Glowloom Producer',
    instructions: producerPrompt,
    model: modelFactory(systemIdentity),
    agents: { scriptwriter, storyboarder },
    workflows: { creativeDeliveryWorkflow },
    ...(memory ? { memory } : {}),
  });

  const mastra = new Mastra({
    agents: { producer, scriptwriter, storyboarder },
    workflows: { creativeDeliveryWorkflow },
    ...(options.storage ? { storage: options.storage } : {}),
  });

  return {
    mastra,
    producer,
    scriptwriter,
    storyboarder,
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
        name: 'Glowloom Scriptwriter',
        instructions: await options.prompts.resolve('scriptwriter'),
        model: scopedModel,
        ...(memory ? { memory } : {}),
      });
      const scopedStoryboarder = new Agent({
        id: 'storyboarder',
        name: 'Glowloom Storyboarder',
        instructions: await options.prompts.resolve('storyboarder'),
        model: scopedModel,
        ...(memory ? { memory } : {}),
      });
      return new Agent({
        id: 'producer',
        name: 'Glowloom Producer',
        instructions,
        model: scopedModel,
        agents: {
          scriptwriter: scopedScriptwriter,
          storyboarder: scopedStoryboarder,
        },
        workflows: { creativeDeliveryWorkflow },
        ...(memory ? { memory } : {}),
      });
    },
  };
}
