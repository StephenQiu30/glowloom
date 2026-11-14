import { mkdir, readFile, realpath } from "node:fs/promises";
import { resolve, relative } from "node:path";
import { digest, parseMarkdown } from "../../content/parse";
import {
  loadSnapshot,
  validateLinks,
  type CatalogEntry,
} from "../../content/catalog";
import { lockDocument } from "./document-lock";
import { atomicWrite, readJson } from "./files";
export type ProposalInput = {
  id: string;
  documentId: string;
  baseRevision: string;
  text: string;
  actor: string;
};
type Receipt = {
  status: string;
  receiptId: string;
  documentId: string;
  beforeRevision: string;
  revision: string;
  requestDigest: string;
  actor: string;
  appliedAt?: string;
};
export async function readDocument(repo: string, id: string) {
  const root = await realpath(repo);
  const workspace = resolve(root, "workspace");
  const catalog = JSON.parse(
    await readFile(resolve(workspace, "catalog.json"), "utf8"),
  ) as CatalogEntry[];
  const doc = catalog.find((d) => d.id === id);
  if (!doc || !doc.path.startsWith("content/") || !doc.path.endsWith(".md"))
    throw new Error("document_readonly");
  const path = resolve(workspace, doc.path);
  if (
    !path.startsWith(resolve(workspace, "content") + "/") ||
    (await realpath(path)) !== path
  )
    throw new Error("document_path_escape");
  const text = await readFile(path, "utf8");
  return {
    id,
    path,
    sourcePath: relative(root, path),
    text,
    revision: digest(text),
  };
}
export async function applyProposal(
  repo: string,
  storage: string,
  input: ProposalInput,
): Promise<Record<string, unknown>> {
  if (
    !/^[a-f\d-]{36}$/.test(input.id) ||
    input.actor !== "local-owner" ||
    input.text.length > 2_000_000
  )
    throw new Error("invalid_proposal");
  const root = await realpath(repo);
  const doc = await readDocument(root, input.documentId);
  const requestDigest = digest(
    JSON.stringify([
      input.documentId,
      input.baseRevision,
      input.text,
      input.actor,
    ]),
  );
  const receiptPath = resolve(storage, "proposals", input.id + ".json");
  const existing = await readJson<Receipt>(receiptPath);
  if (existing) {
    if (existing.requestDigest !== requestDigest)
      throw new Error("idempotency_conflict");
    if (existing.status === "applied") return existing;
    if (doc.revision === existing.revision) {
      const recovered = {
        ...existing,
        status: "applied",
        appliedAt: new Date().toISOString(),
      };
      await atomicWrite(receiptPath, JSON.stringify(recovered));
      return recovered;
    }
    throw new Error("application_unknown");
  }
  const lockDir = resolve(storage, "locks");
  await mkdir(lockDir, { recursive: true, mode: 0o700 });
  const lockPath = resolve(lockDir, input.documentId + ".lock");
  const release = await lockDocument(lockPath);
  try {
    const latest = await readDocument(root, input.documentId);
    if (latest.revision !== input.baseRevision)
      throw new Error("revision_conflict");
    const parsed = await parseMarkdown(input.text);
    if (!parsed.title) throw new Error("document_title_required");
    const snapshot = await loadSnapshot({
      workspaceRoot: resolve(root, "workspace"),
      scope: "internal",
    });
    const target = snapshot.documents.find((d) => d.id === input.documentId);
    if (!target) throw new Error("document_not_found");
    Object.assign(target, parsed, {
      source: input.text,
      revision: digest(input.text),
    });
    const errors = await validateLinks(snapshot);
    if (errors.length) throw new Error("proposal_links_invalid");
    const receipt: Receipt = {
      status: "applying",
      receiptId: input.id,
      documentId: input.documentId,
      beforeRevision: latest.revision,
      revision: digest(input.text),
      requestDigest,
      actor: input.actor,
    };
    await atomicWrite(
      resolve(storage, "proposals", input.id + ".before.md"),
      latest.text,
    );
    await atomicWrite(receiptPath, JSON.stringify(receipt));
    const before = await readDocument(root, input.documentId);
    if (before.revision !== latest.revision)
      throw new Error("revision_conflict");
    await atomicWrite(doc.path, input.text);
    receipt.status = "applied";
    receipt.appliedAt = new Date().toISOString();
    await atomicWrite(receiptPath, JSON.stringify(receipt));
    return receipt;
  } finally {
    await release();
  }
}
