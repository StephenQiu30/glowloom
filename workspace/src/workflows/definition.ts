export type Workflow = {
  id: string;
  revision: string;
  stages: {
    id: string;
    label: string;
    needs: string[];
    outputs: string[];
    optional: boolean;
  }[];
};
export type TaskContext = {
  goal: string;
  project: string;
  repoCommit: string;
  codeDigest: string;
  documents: { id: string; revision: string; excerpt: string }[];
  skillRevisions: Record<string, string>;
  workflowRevision: string;
  allowedActions: string[];
  expiresAt: number;
  acceptance: string[];
  stageIds: string[];
  digest: string;
};
export function parseWorkflow(source: string): Workflow {
  const match = source.match(/```workspace-workflow\s*\n([\s\S]*?)\n```/);
  if (!match) throw new Error("Workflow definition missing");
  const parsed = JSON.parse(match[1]) as Omit<Workflow, "revision">;
  if (
    !/^[a-z][\w-]*$/.test(parsed.id) ||
    !Array.isArray(parsed.stages) ||
    !parsed.stages.length
  )
    throw new Error("Invalid workflow");
  const stages = new Map(parsed.stages.map((s) => [s.id, s]));
  if (stages.size !== parsed.stages.length) throw new Error("Duplicate stage");
  const done = new Set<string>(),
    active = new Set<string>();
  const walk = (id: string) => {
    if (active.has(id)) throw new Error("Dependency cycle 循环");
    if (done.has(id)) return;
    const stage = stages.get(id);
    if (
      !stage ||
      !stage.label ||
      !Array.isArray(stage.outputs) ||
      !stage.outputs.length ||
      !Array.isArray(stage.needs)
    )
      throw new Error("Missing stage input/output");
    active.add(id);
    for (const parent of stage.needs) walk(parent);
    active.delete(id);
    done.add(id);
  };
  for (const stage of parsed.stages) walk(stage.id);
  return { ...parsed, revision: digest(source) };
}
export function buildContext(input: {
  goal: string;
  project: string;
  repoCommit: string;
  codeDigest: string;
  documents: { id: string; revision: string; source: string }[];
  skillRevisions: Record<string, string>;
  workflowRevision: string;
  allowedActions: string[];
  expiresAt: number;
  acceptance: string[];
  budget?: number;
  stageIds?: string[];
}): TaskContext {
  if (
    !input.goal.trim() ||
    input.goal.length > 8000 ||
    !input.acceptance.length ||
    !input.allowedActions.length ||
    !Number.isFinite(input.expiresAt)
  )
    throw new Error("Incomplete task context");
  let remaining = Math.min(input.budget ?? 12000, 48000);
  const documents = input.documents.map((d) => {
    const excerpt = d.source.slice(0, Math.max(remaining, 0));
    remaining -= excerpt.length;
    return { id: d.id, revision: d.revision, excerpt };
  });
  const context = {
    goal: input.goal.trim(),
    project: input.project,
    repoCommit: input.repoCommit,
    codeDigest: input.codeDigest,
    documents,
    skillRevisions: input.skillRevisions,
    workflowRevision: input.workflowRevision,
    allowedActions: input.allowedActions,
    expiresAt: input.expiresAt,
    acceptance: input.acceptance,
    stageIds: input.stageIds ?? ["accept"],
  };
  return { ...context, digest: digest(JSON.stringify(context)) };
}
export function contextMatches(
  context: TaskContext,
  current: {
    codeDigest: string;
    revisions: Record<string, string>;
    skillRevisions: Record<string, string>;
    workflowRevision: string;
  },
  now: number,
) {
  return (
    context.expiresAt > now &&
    context.codeDigest === current.codeDigest &&
    context.workflowRevision === current.workflowRevision &&
    context.documents.every((d) => current.revisions[d.id] === d.revision) &&
    Object.entries(context.skillRevisions).every(
      ([id, rev]) => current.skillRevisions[id] === rev,
    )
  );
}
import { digest } from "../content/parse";
