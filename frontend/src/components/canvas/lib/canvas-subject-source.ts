import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
import {
  requireUnifiedSubject,
  type UnifiedSubject,
} from "@/components/subjects/unified-contracts";
import { unifiedSubjectBinding } from "@/components/subjects/unified-reference";

/** Immutable identity; the subject head revision is only a settings CAS token. */
export type CanvasSubjectPin = Readonly<{
  subject_id: string;
  version_id: string;
}>;

/** Reopening a canvas must never substitute a newer head for its pinned version. */
export function requireCanvasSubjectVersion(
  value: unknown,
  scopeID: string,
  pin: CanvasSubjectPin,
): UnifiedSubject {
  const subject = requireUnifiedSubject(value, scopeID, pin.subject_id);
  if (subject.version_id !== pin.version_id) {
    throw new RequestError("invalid_receipt");
  }
  return subject;
}

/** Keep the existing Subject binding proofs and kind-specific media expansion. */
export function canvasSubjectReference(
  value: unknown,
  scopeID: string,
  pin: CanvasSubjectPin,
) {
  return unifiedSubjectBinding(
    requireCanvasSubjectVersion(value, scopeID, pin),
  );
}

/** A local subject is a canvas input, never an alias for its unchanged library version. */
export function localCanvasSubjectReference(
  document: API.DocumentDTO,
  projectID: string,
  canvasID: string,
  nodeID: string,
  inputRevision: number,
): import("@/components/generation/prompt-document").ReferenceBinding {
  const node = document.nodes?.find((item) => item.id === nodeID);
  const local = node?.config?.subject?.local;
  if (
    document.id !== canvasID ||
    document.project_id !== projectID ||
    !document.permissions?.read ||
    node?.node_type !== "subject" ||
    node.node_action !== "resource" ||
    !Number.isSafeInteger(inputRevision) ||
    inputRevision < 1 ||
    node.input_revision !== inputRevision ||
    !local?.primary ||
    typeof local.description !== "string" ||
    !Array.isArray(local.references) ||
    !["image", "video"].includes(local.primary.kind ?? "")
  )
    throw new RequestError("reference_unavailable");
  const media = [
    local.primary,
    ...local.references,
    ...(local.voice ? [local.voice.media] : []),
  ];
  if (
    media.some(
      (item) =>
        !item ||
        !isProjectID(item.asset_id) ||
        !Number.isSafeInteger(item.asset_revision) ||
        item.asset_revision! < 1 ||
        !["image", "video", "audio"].includes(item.kind ?? "") ||
        (item.binding_id !== undefined && !isProjectID(item.binding_id)),
    )
  )
    throw new RequestError("invalid_receipt");
  return {
    id: crypto.randomUUID(),
    bindingGeneration: 1,
    mediaKind: local.primary.kind as "image" | "video",
    role: "subject",
    order: 0,
    source: {
      kind: "canvas_subject",
      sourceId: nodeID,
      sourceVersion: String(inputRevision),
      description:
        (node.title ?? "") +
        (local.description ? `：${local.description}` : ""),
      media: media.map((item) => ({
        sourceId: item!.asset_id!,
        sourceVersion: String(item!.asset_revision),
        mediaKind: item!.kind as "image" | "video" | "audio",
        ...(item!.binding_id ? { bindingId: item!.binding_id } : {}),
      })),
    },
  };
}

export function canvasSubjectSources(
  nodes: import("../types/canvas").CanvasNodeData[],
  canvasID: string,
): import("@/components/generation/reference-source").CanvasSubjectReferenceSource[] {
  return nodes.flatMap<
    import("@/components/generation/reference-source").CanvasSubjectReferenceSource
  >((node) => {
    const config = node.serviceData?.config?.subject;
    if (node.type !== "subject" || !config) return [];
    if (config.local) {
      const revision = node.serviceData?.input_revision;
      return config.local.primary &&
        Number.isSafeInteger(revision) &&
        revision! > 0
        ? [
            {
              nodeID: node.id,
              title: node.title,
              source: "canvas_subject" as const,
              canvasID,
              inputRevision: revision!,
            },
          ]
        : [];
    }
    return config.subject_id && config.version_id
      ? [
          {
            nodeID: node.id,
            title: node.title,
            source: "subject" as const,
            subjectID: config.subject_id,
            versionID: config.version_id,
          },
        ]
      : [];
  });
}
