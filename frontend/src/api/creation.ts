import request from "@/lib/request";

type Envelope<T> = { data: T };
export const listCreationRuns = (projectId: string) => request<Envelope<API.CreationRunResponse[]>>(`/api/projects/${projectId}/creation-runs`, { method: "GET", params: { limit: 100 } });
export const getCreationRun = (runId: string) => request<Envelope<API.CreationRunResponse>>(`/api/creation-runs/${runId}`, { method: "GET" });
export const resumeCreationRun = (runId: string, body: API.ResumeCreationRunRequest) => request<Envelope<API.CreationResumeResponse>>(`/api/creation-runs/${runId}/resume`, { method: "POST", data: body });
export const createCreationRun = (projectId: string, body: API.CreateCreationRunRequest) => request<Envelope<API.CreationRunResponse>>(`/api/projects/${projectId}/creation-runs`, { method: "POST", data: body });
export const retryCreationDelivery = (runId: string, body: API.RetryCreationDeliveryRequest) => request<Envelope<API.CreationRunResponse>>(`/api/creation-runs/${runId}/retry-delivery`, { method: "POST", data: body });
export const getCreationExecution = (runId: string) => request<Envelope<API.CreationExecution>>(`/api/creation-runs/${runId}/execution`, { method: "GET" });
export const listCreationProposals = (runId: string) => request<Envelope<API.CreationProposal[]>>(`/api/creation-runs/${runId}/proposals`, { method: "GET" });
export const syncCreationProposals = (runId: string) => request<Envelope<API.CreationSyncResult>>(`/api/creation-runs/${runId}/sync-proposals`, { method: "POST", data: {} });
export const adoptCreationProposal = (runId: string, proposalId: string, body: API.AdoptCreationProposalRequest) => request<Envelope<API.CreationAdoptionReceipt>>(`/api/creation-runs/${runId}/proposals/${proposalId}/adopt`, { method: "POST", data: body });
export const getCurrentScriptSource = (projectId: string) => request<Envelope<API.AcceptedScriptSourceResponse>>(`/api/projects/${projectId}/current-script-source`, { method: "GET" });
export const acceptScriptSource = (projectId: string, body: API.AcceptScriptSourceRequest) => request<Envelope<API.AcceptedScriptSourceResponse>>(`/api/projects/${projectId}/script-sources`, { method: "POST", data: body });
