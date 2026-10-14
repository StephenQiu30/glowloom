import { RequestError } from "@/lib/request";
import { isProjectID } from "@/components/projects/project";
export const kinds = {
  character: "角色",
  location: "场景",
  prop: "物品",
} as const;
export type Subject = API.Detail & {
  head: API.Head & {
    id: string;
    project_id: string;
    kind: API.Kind;
    revision: number;
  };
  current: API.Version;
};
export function requireSubject(
  v: API.Detail,
  project: string,
  kind: API.Kind,
  id?: string,
): Subject {
  if (
    !v?.head ||
    !isProjectID(v.head.id) ||
    v.head.project_id !== project ||
    v.head.kind !== kind ||
    (id && v.head.id !== id) ||
    !Number.isSafeInteger(v.head.revision) ||
    v.head.revision! < 1 ||
    !v.current ||
    v.current.entry_id !== v.head.id ||
    v.current.project_id !== project ||
    v.current.id !== v.head.current_version_id ||
    typeof v.current[kind]?.name !== "string"
  )
    throw new RequestError("invalid_receipt");
  return v as Subject;
}
export function requireSubjectReceipt(
  v: API.Receipt,
  kind: API.Kind,
  id?: string,
) {
  if (
    !v ||
    !isProjectID(v.entry_id) ||
    v.kind !== kind ||
    (id && v.entry_id !== id) ||
    !Number.isSafeInteger(v.revision) ||
    v.revision! < 1 ||
    !isProjectID(v.version_id)
  )
    throw new RequestError("invalid_receipt");
  return v as API.Receipt & { entry_id: string; revision: number };
}
export const subjectText = (s: Subject) => s.current[s.head.kind]!;
export const subjectImage = (s: Subject) =>
  s.current.character?.looks
    ?.find((l) => l.default)
    ?.references?.find((r) => r.role === "primary")?.media?.asset_id;
export function impactProof(
  meta?: Record<string, unknown>,
): API.ImpactProof | undefined {
  const p = meta?.impact as API.ImpactProof | undefined;
  return p &&
    typeof p.sha256 === "string" &&
    /^[a-f0-9]{64}$/.test(p.sha256) &&
    Number.isSafeInteger(p.revision) &&
    p.revision! >= 1 &&
    [p.current_lines, p.historical_lines, p.aliases].every(
      (n) => Number.isSafeInteger(n) && n! >= 0,
    )
    ? {
        sha256: p.sha256,
        revision: p.revision,
        current_lines: p.current_lines,
        historical_lines: p.historical_lines,
        aliases: p.aliases,
      }
    : undefined;
}
