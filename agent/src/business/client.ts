import type { Scope, ControlInput } from "../../contracts/agent.js";
import type { BusinessTool } from "../sessions/manifest.js";
export interface GrantRef {
  grant_id: string;
  root_run_id: string;
  submission_id: string;
  owner_epoch: number;
  authorization_revision: number;
}
export interface Grant extends GrantRef {
  available_micros: number;
  expires_at: string;
  scope: Scope;
}
export interface CallIdentity {
  task_id: string;
  call_id: string;
  checkpoint_id: string;
  attempt: number;
  contract_version: string;
  input_hash: string;
}
export interface CallReceipt {
  receipt_id: string;
  root_run_id: string;
  submission_id: string;
  call: CallIdentity;
  status: "in_flight" | "completed" | "failed" | "unknown";
  result: unknown;
  cost_micros: number;
}
export type BusinessDecision = ControlInput;
export class BusinessError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
  ) {
    super(code);
  }
}
export class BusinessClient {
  constructor(
    private origin: string,
    private token: string,
  ) {}
  async request<T>(
    path: string,
    body?: unknown,
    signal?: AbortSignal,
  ): Promise<T> {
    const timeout = AbortSignal.timeout(
      path === "/models/stream" || path.includes("/execute") ? 180000 : 10000,
    );
    const response = await fetch(
      new URL("/internal/agent-business" + path, this.origin),
      {
        method: body === undefined ? "GET" : "POST",
        headers: {
          authorization: `Bearer ${this.token}`,
          "x-agent-audience": "agent-business",
          ...(body === undefined ? {} : { "content-type": "application/json" }),
        },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
        signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
        redirect: "error",
      },
    );
    const reader = response.body?.getReader();
    if (!reader) throw new BusinessError(502, "business_response_invalid");
    const chunks: Uint8Array[] = [];
    let size = 0;
    try {
      for (;;) {
        const next = await reader.read();
        if (next.done) break;
        size += next.value.byteLength;
        if (size > 8 * 1024 * 1024)
          throw new BusinessError(502, "business_response_too_large");
        chunks.push(next.value);
      }
    } finally {
      await reader.cancel().catch(() => {});
      reader.releaseLock();
    }
    let value: unknown;
    try {
      value = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    } catch {
      throw new BusinessError(502, "business_response_invalid");
    }
    if (!response.ok) {
      const code =
        value && typeof value === "object" && "code" in value
          ? String(value.code)
          : "business_request_failed";
      throw new BusinessError(response.status, code);
    }
    return value as T;
  }
  tools(): Promise<BusinessTool[]> {
    return this.request<BusinessTool[]>("/tools");
  }

  grant(input: {
    root_run_id: string;
    submission_id: string;
    owner_epoch: number;
    authorization_revision: number;
    lease_until: string;
  }) {
    return this.request<Grant>("/grants", input);
  }
  validate(grant: GrantRef) {
    return this.request<Grant>("/grants/validate", grant);
  }
  async validateInputSources(grant: GrantRef, draft: Record<string, unknown>) {
    const result = await this.request<{ valid: boolean }>(
      "/input-sources/validate",
      { grant, draft },
    );
    if (result.valid !== true)
      throw new BusinessError(502, "input_sources_receipt_invalid");
  }
  decisions(run: string) {
    return this.request<{
      current: BusinessDecision | null;
      authorization_revision: number;
      barrier: boolean;
      terminal: boolean;
    }>(`/runs/${run}/decisions`);
  }
  quiesced(
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
    return this.request<Grant>(`/runs/${run}/quiesced`, input);
  }
  receipt(
    run: string,
    input: {
      receipt_id: string;
      decision_id: string;
      authorization_revision: number;
      status: "applied" | "superseded";
      execution_revision: number;
    },
  ) {
    return this.request<unknown>(`/runs/${run}/receipts`, input);
  }
  async findCall(
    grant: Pick<GrantRef, "root_run_id" | "submission_id">,
    call: CallIdentity,
  ): Promise<CallReceipt | undefined> {
    const query = new URLSearchParams({
      submission_id: grant.submission_id,
      task_id: call.task_id,
      checkpoint_id: call.checkpoint_id,
      attempt: String(call.attempt),
      contract_version: call.contract_version,
    });
    try {
      return await this.request<CallReceipt>(
        `/runs/${grant.root_run_id}/calls/${encodeURIComponent(call.call_id)}?${query}`,
      );
    } catch (error) {
      if (error instanceof BusinessError && error.status === 404)
        return undefined;
      throw error;
    }
  }
  execute(
    kind: "model" | "tool",
    name: string,
    grant: GrantRef,
    call: CallIdentity,
    input: unknown,
    maxMicros: number,
    signal?: AbortSignal,
  ) {
    return this.request<CallReceipt>(
      kind === "model"
        ? "/models/stream"
        : `/tools/${encodeURIComponent(name)}/execute`,
      { grant, call, input, max_micros: maxMicros },
      signal,
    );
  }
  snapshot(grant: GrantRef) {
    return this.request<Record<string, unknown>>("/snapshot", grant);
  }
  skill(grant: GrantRef, id: string, version: string, sha256: string) {
    return this.request<{
      skill_id: string;
      version: string;
      resource_hash: string;
      content: string;
    }>(`/skills/${encodeURIComponent(id)}/load`, {
      grant,
      version,
      resource_hash: sha256,
    });
  }
}
