import { execFileSync } from "node:child_process";
import { readFile, realpath } from "node:fs/promises";
import { resolve } from "node:path";
import { loadSnapshot } from "../../content/catalog";
import { digest } from "../../content/parse";
import {
  contextMatches,
  parseWorkflow,
  type TaskContext,
} from "../../workflows/definition";
export async function liveContext(
  repo: string,
  project: string,
  workflowId: string,
  selected: string[] = [],
) {
  const root = await realpath(repo);
  const workspace = resolve(root, "workspace");
  const projects = JSON.parse(
    await readFile(resolve(workspace, "projects.json"), "utf8"),
  ) as { id: string; path: string }[];
  const entry = projects.find((p) => p.id === project);
  if (
    !entry ||
    entry.path !== project ||
    !["workspace", "frontend", "backend", "agent", "canvas"].includes(project)
  )
    throw new Error("unknown_project");
  const projectPath = resolve(root, entry.path);
  if ((await realpath(projectPath)) !== projectPath)
    throw new Error("project_path_escape");
  const tracked = execFileSync(
    "git",
    [
      "ls-files",
      "--cached",
      "--others",
      "--exclude-standard",
      "-z",
      "--",
      entry.path,
      ...(project === "workspace"
        ? [
            "backend/db/schema.sql",
            "frontend/src/components/ui",
            "frontend/src/app/globals.css",
          ]
        : []),
    ],
    { cwd: root },
  )
    .toString()
    .split("\0")
    .filter(Boolean)
    .sort();
  const hashes = await Promise.all(
    tracked.map(async (name) => {
      try {
        return name + ":" + digest(await readFile(resolve(root, name)));
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code === "ENOENT")
          return name + ":deleted";
        throw error;
      }
    }),
  );
  const snapshot = await loadSnapshot({
    workspaceRoot: workspace,
    scope: "internal",
  });
  const workflowSource = snapshot.documents.find(
    (d) => d.id === `workflow-${workflowId}`,
  )?.source;
  if (!workflowSource) throw new Error("workflow_not_found");
  const workflow = parseWorkflow(workflowSource);
  return {
    projectPath,
    codeDigest: digest(hashes.join("\n")),
    repoCommit: snapshot.repoCommit,
    revisions: Object.fromEntries(
      snapshot.documents.map((d) => [d.id, d.revision]),
    ),
    skillRevisions: Object.fromEntries(
      snapshot.documents
        .filter((d) => d.category === "skills")
        .map((d) => [d.id, d.revision]),
    ),
    workflowRevision: workflow.revision,
    workflowSource,
    documents: snapshot.documents
      .filter((d) => selected.includes(d.id))
      .map(({ id, revision, source }) => ({ id, revision, source })),
  };
}
export async function currentContext(
  repo: string,
  context: TaskContext,
  workflowId: string,
) {
  const current = await liveContext(repo, context.project, workflowId);
  return {
    current: contextMatches(context, current, Date.now()),
    live: current,
  };
}
