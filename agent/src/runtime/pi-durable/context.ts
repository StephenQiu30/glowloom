import {
  defineExtension,
  type ConversationId,
} from '@earendil-works/pi-durable';
import { StateDoc } from './docs.js';
import type { FrozenManifest } from '../../sessions/manifest.js';

/** Official prompt sections reread the typed document after compaction; no synthetic summary/checkpoint. */
export function contextExtension(
  manifest: Readonly<FrozenManifest>,
  rootId: () => ConversationId,
) {
  return defineExtension({
    name: 'glowloom.context-state',
    sections: [
      {
        key: 'glowloom_constraints',
        render: async (input, context) => {
          const state = await input.read.snapshot(StateDoc, rootId(), context);
          if (!state)
            throw new Error('Committed creative context is unavailable');
          const own = state.delegations?.find(
            (x) => x.conversation_id === input.conversationId,
          );
          return JSON.stringify({
            goal: state.goal,
            manifest: {
              sha256: state.manifest_hash,
              model_key: manifest.model_key,
              skills: manifest.skills,
              tools: input.agent.tools.map((t) => t.name),
            },
            plan: state.plan,
            adopted: state.adopted,
            loaded_skills: state.loaded_skills ?? [],
            pending_proposal: state.pending_proposal,
            budget: {
              model_steps: state.model_steps,
              tool_steps: state.tool_steps,
              limit: 40,
            },
            specialist: own ?? null,
            constraints:
              'Only explicitly adopted specialist result and source references may support further work. A failed specialist is a local failure; redo only that task with redo_of and adopted sources. Skill scripts require the authorized Go isolation tool; never execute host code. Plans and specialist prose never establish business approval or media completion.',
          });
        },
      },
    ],
  });
}
