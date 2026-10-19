import { getProject } from "@/api/projects";
import {
  getSessionGeneration,
  getScopeGeneration,
  getCreationScope,
  RequestError,
} from "@/lib/request";
import { isProjectID, requireProject, type Project } from "./project";

export type RenameAttempt = {
  key: string;
  body: { name: string; expected_revision: number };
  phase: "submitting" | "unknown" | "confirmed";
};
export type ProjectProof = {
  actorId: string;
  scopeId: string;
  sessionGeneration: number;
  scopeGeneration: number;
};
export const projectNamespace = (
  proof: Pick<ProjectProof, "actorId" | "scopeId">,
) => `${proof.actorId}/${proof.scopeId}`;
export const isProjectProofCurrent = (proof: ProjectProof) =>
  proof.sessionGeneration === getSessionGeneration() &&
  proof.scopeGeneration === getScopeGeneration() &&
  proof.scopeId === getCreationScope();

export class ProjectPresentationStore {
  private values = new Map<string, Project>();
  private renames = new Map<string, RenameAttempt>();
  private listeners = new Set<() => void>();
  private key(namespace: string, id: string) {
    return `${namespace}/${id}`;
  }
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private notify() {
    for (const listener of this.listeners) listener();
  }
  read(namespace: string, id: string) {
    return this.values.get(this.key(namespace, id)) ?? null;
  }
  accept(namespace: string, project: Project) {
    const key = this.key(namespace, project.id),
      previous = this.values.get(key);
    if (previous && previous.revision > project.revision) return previous;
    if (previous?.canvas_id && project.canvas_id !== previous.canvas_id)
      throw new RequestError("invalid_receipt");
    if (
      previous &&
      previous.revision === project.revision &&
      previous.name !== project.name
    )
      throw new RequestError("invalid_receipt");
    if (
      previous?.last_modified_at &&
      (!project.last_modified_at ||
        Date.parse(previous.last_modified_at) >
          Date.parse(project.last_modified_at))
    ) {
      project = {
        ...project,
        last_modified_at: previous.last_modified_at,
        last_modified_basis: previous.last_modified_basis,
      };
    }
    if (previous && JSON.stringify(previous) === JSON.stringify(project))
      return previous;
    this.values.set(key, project);
    this.notify();
    return project;
  }
  rename(namespace: string, id: string) {
    const value = this.renames.get(this.key(namespace, id));
    return value ? structuredClone(value) : undefined;
  }
  setRename(namespace: string, id: string, attempt: RenameAttempt) {
    const key = this.key(namespace, id),
      previous = this.renames.get(key);
    if (
      previous &&
      (previous.key !== attempt.key ||
        previous.body.name !== attempt.body.name ||
        previous.body.expected_revision !== attempt.body.expected_revision)
    )
      throw new RequestError("rename_pending", 409);
    if (previous?.phase === "confirmed" && attempt.phase !== "confirmed")
      return;
    this.renames.set(key, structuredClone(attempt));
  }
  dropRename(namespace: string, id: string, key: string) {
    if (this.renames.get(this.key(namespace, id))?.key === key)
      this.renames.delete(this.key(namespace, id));
  }
  clear() {
    this.values.clear();
    this.renames.clear();
    this.notify();
  }
}
export const projectPresentationStore = new ProjectPresentationStore();
const inFlight = new Map<string, Promise<Project>>();

export function refreshProjectPresentation(
  proof: ProjectProof,
  id: string,
): Promise<Project> {
  if (!isProjectProofCurrent(proof) || !isProjectID(id))
    return Promise.reject(new RequestError("session_invalid", 401));
  const key = `${projectNamespace(proof)}/${id}/${proof.sessionGeneration}/${proof.scopeGeneration}`;
  const previous = inFlight.get(key);
  if (previous) return previous;
  const request = getProject({ pid: id })
    .then((value) => {
      if (!isProjectProofCurrent(proof))
        throw new RequestError("session_invalid", 401);
      const project = requireProject(value);
      if (project.id !== id) throw new RequestError("invalid_receipt");
      return projectPresentationStore.accept(projectNamespace(proof), project);
    })
    .finally(() => {
      if (inFlight.get(key) === request) inFlight.delete(key);
    });
  inFlight.set(key, request);
  return request;
}

const eventName = "glowloom:project-presentation-invalidated";
export function publishProjectInvalidation(projectId: string) {
  if (typeof window === "undefined" || !isProjectID(projectId)) return;
  window.dispatchEvent(new CustomEvent(eventName, { detail: { projectId } }));
  if (typeof BroadcastChannel !== "undefined") {
    const channel = new BroadcastChannel(eventName);
    channel.postMessage({ projectId });
    channel.close();
  }
}
export function subscribeProjectInvalidation(listener: (id: string) => void) {
  const receive = (value: unknown) => {
    const id = (value as { projectId?: unknown } | null)?.projectId;
    if (isProjectID(id)) listener(id);
  };
  const local = (event: Event) => receive((event as CustomEvent).detail);
  window.addEventListener(eventName, local);
  const channel =
    typeof BroadcastChannel !== "undefined"
      ? new BroadcastChannel(eventName)
      : null;
  if (channel) channel.onmessage = (event) => receive(event.data);
  return () => {
    window.removeEventListener(eventName, local);
    channel?.close();
  };
}
