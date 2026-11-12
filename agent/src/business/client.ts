import type { Scope, ControlInput } from '../sessions/types.js';
import * as api from './generated/business.js';
import { BusinessError } from './request.js';
export { BusinessError } from './request.js';
export type BusinessTool = Required<
  Pick<
    AgentBusinessApi.ToolDescriptor,
    'name' | 'description' | 'contract_version'
  >
> &
  Pick<AgentBusinessApi.ToolDescriptor, 'roles'> & {
    parameters: Record<string, unknown>;
    replay: 'safe' | 'unsafe';
    effect: 'read' | 'write' | 'proposal';
  };

export type GrantRef = Required<
  Pick<
    AgentBusinessApi.Grant,
    | 'grant_id'
    | 'root_run_id'
    | 'submission_id'
    | 'owner_epoch'
    | 'authorization_revision'
  >
>;
export type Grant = GrantRef &
  Required<Pick<AgentBusinessApi.Grant, 'available_micros' | 'expires_at'>> & {
    scope: Scope;
  };
export type CallIdentity = Required<AgentBusinessApi.CallIdentity>;
export type CallReceipt = Required<
  Pick<
    AgentBusinessApi.CallReceipt,
    'receipt_id' | 'root_run_id' | 'submission_id' | 'cost_micros'
  >
> & {
  call: CallIdentity;
  status: 'in_flight' | 'completed' | 'failed' | 'unknown';
  result: unknown;
};
export type BusinessDecision = ControlInput;
function checked<T>(value: unknown, fields: Record<string, string>): T {
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new BusinessError(502, 'business_response_invalid');
  for (const [key, type] of Object.entries(fields)) {
    if (
      typeof Reflect.get(value, key) !== type ||
      (type === 'object' &&
        (Reflect.get(value, key) === null ||
          Array.isArray(Reflect.get(value, key))))
    )
      throw new BusinessError(502, 'business_response_invalid');
  }
  return value as T;
}
const grantFields = {
  grant_id: 'string',
  root_run_id: 'string',
  submission_id: 'string',
  owner_epoch: 'number',
  authorization_revision: 'number',
  available_micros: 'number',
  expires_at: 'string',
  scope: 'object',
};
const callFields = {
  receipt_id: 'string',
  root_run_id: 'string',
  submission_id: 'string',
  call: 'object',
  status: 'string',
  cost_micros: 'number',
};

function checkedGrant(value: unknown): Grant {
  const grant = checked<Grant>(value, grantFields);
  checked<Scope>(grant.scope, {
    actor_id: 'string',
    org_id: 'string',
    scope_id: 'string',
    project_id: 'string',
  });
  return grant;
}
function checkedCall(value: unknown): CallReceipt {
  const receipt = checked<CallReceipt>(value, callFields);
  checked<CallIdentity>(receipt.call, {
    task_id: 'string',
    call_id: 'string',
    checkpoint_id: 'string',
    attempt: 'number',
    contract_version: 'string',
    input_hash: 'string',
  });
  if (!['in_flight', 'completed', 'failed', 'unknown'].includes(receipt.status))
    throw new BusinessError(502, 'business_response_invalid');
  return receipt;
}

export class BusinessClient {
  constructor(
    private origin: string,
    private token: string,
  ) {}
  private options(signal?: AbortSignal, timeoutMs?: number) {
    return {
      transport: {
        origin: this.origin,
        token: this.token,
        ...(signal ? { signal } : {}),
        ...(timeoutMs === undefined ? {} : { timeoutMs }),
      },
    };
  }
  async tools(): Promise<BusinessTool[]> {
    const result = await api.listTools(this.options());
    if (!Array.isArray(result))
      throw new BusinessError(502, 'business_response_invalid');
    return result.map((tool) =>
      checked<BusinessTool>(tool, {
        name: 'string',
        description: 'string',
        parameters: 'object',
        contract_version: 'string',
        replay: 'string',
        effect: 'string',
      }),
    );
  }
  async grant(input: {
    root_run_id: string;
    submission_id: string;
    owner_epoch: number;
    authorization_revision: number;
    lease_until: string;
  }) {
    return checkedGrant(await api.registerGrant(input, this.options()));
  }
  async validate(grant: GrantRef) {
    return checkedGrant(await api.validateGrant(grant, this.options()));
  }
  async validateInputSources(grant: GrantRef, draft: Record<string, unknown>) {
    const result = await api.validateInputSources(
      { grant, draft },
      this.options(),
    );
    if (result.valid !== true)
      throw new BusinessError(502, 'input_sources_receipt_invalid');
  }
  async decisions(run: string) {
    const value = await api.getDecisions(
      { rid: encodeURIComponent(run) },
      this.options(),
    );
    return checked<{
      current: BusinessDecision | null;
      authorization_revision: number;
      barrier: boolean;
      terminal: boolean;
    }>(value, {
      authorization_revision: 'number',
      barrier: 'boolean',
      terminal: 'boolean',
    });
  }
  async quiesced(
    run: string,
    input: {
      receipt_id: string;
      decision_id: string;
      authorization_revision: number;
      owner_epoch: number;
      continuation_id: string;
      lease_until: string;
    },
  ) {
    return checkedGrant(
      await api.quiesceRun(
        { rid: encodeURIComponent(run) },
        input,
        this.options(),
      ),
    );
  }
  receipt(
    run: string,
    input: {
      receipt_id: string;
      decision_id: string;
      authorization_revision: number;
      status: 'applied' | 'superseded';
      execution_revision: number;
    },
  ) {
    return api.acknowledgeReceipt(
      { rid: encodeURIComponent(run) },
      input,
      this.options(),
    );
  }
  async findCall(
    grant: Pick<GrantRef, 'root_run_id' | 'submission_id'>,
    call: CallIdentity,
  ): Promise<CallReceipt | undefined> {
    try {
      return checkedCall(
        await api.getCall(
          {
            rid: encodeURIComponent(grant.root_run_id),
            call_id: encodeURIComponent(call.call_id),
            submission_id: grant.submission_id,
            task_id: call.task_id,
            checkpoint_id: call.checkpoint_id,
            attempt: call.attempt,
            contract_version: call.contract_version,
            input_hash: call.input_hash,
          },
          this.options(),
        ),
      );
    } catch (error) {
      if (error instanceof BusinessError && error.status === 404)
        return undefined;
      throw error;
    }
  }
  async execute(
    kind: 'model' | 'tool',
    name: string,
    grant: GrantRef,
    call: CallIdentity,
    input: unknown,
    maxMicros: number,
    signal?: AbortSignal,
  ) {
    const body = {
      grant,
      call,
      input: input as Record<string, unknown>,
      max_micros: maxMicros,
    };
    const result =
      kind === 'model'
        ? await api.executeModel(body, this.options(signal, 180000))
        : await api.executeTool(
            { name: encodeURIComponent(name) },
            body,
            this.options(signal, 180000),
          );
    return checkedCall(result);
  }
  snapshot(grant: GrantRef) {
    return api.getSnapshot(grant, this.options());
  }
  async skill(grant: GrantRef, id: string, version: string, sha256: string) {
    return checked<{
      skill_id: string;
      version: string;
      resource_hash: string;
      content: string;
    }>(
      await api.loadSkill(
        { id: encodeURIComponent(id) },
        { grant, version, resource_hash: sha256 },
        this.options(),
      ),
      {
        skill_id: 'string',
        version: 'string',
        resource_hash: 'string',
        content: 'string',
      },
    );
  }
}
