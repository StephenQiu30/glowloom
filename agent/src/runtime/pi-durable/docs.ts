import { defineDoc, type JsonObject } from '@earendil-works/pi-durable';
export interface AgentState extends JsonObject {
  root_run_id: string;
  segment_id: string;
  manifest_hash: string;
  status: string;
  execution_revision: number;
  authorization_revision: number;
  model_steps: number;
  tool_steps: number;
  goal: string;
  plan: JsonObject[];
  adopted: JsonObject[];
  skills: JsonObject[];
  reserved_calls: JsonObject[];
  delegations: JsonObject[];
  loaded_skills: JsonObject[];
  model_contexts: JsonObject[];
  pending_proposal: string | null;
  applied_decisions: JsonObject[];
  quiesced: JsonObject | null;
}
export const StateDoc = defineDoc<AgentState>({
  kind: 'glowloom.agent',
  version: 1,
  scope: 'conversation',
  history: 'latest',
  fork: 'initial',
  checkpointWhen: () => true,
  initial: () => ({
    root_run_id: '',
    segment_id: '',
    manifest_hash: '',
    status: 'accepted',
    execution_revision: 0,
    authorization_revision: 0,
    model_steps: 0,
    tool_steps: 0,
    goal: '',
    plan: [],
    adopted: [],
    skills: [],
    reserved_calls: [],
    delegations: [],
    loaded_skills: [],
    model_contexts: [],
    pending_proposal: null,
    applied_decisions: [],
    quiesced: null,
  }),
});

export function releaseOwnedAllocations(state: AgentState, reason: string) {
  for (const child of state.delegations) {
    if (child.status === 'running') {
      child.status = 'failed';
      child.failure = { code: 'parent_aborted', reason };
    }
  }
}
