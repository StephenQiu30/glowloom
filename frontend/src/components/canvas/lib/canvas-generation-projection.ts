import { canvasGraphFingerprint } from "./canvas-document-adapter";
import type { CanvasNodeData } from "../types/canvas";
import { isFixedCanvasMedia } from "./canvas-media-source";

export const completedGenerationStates = new Set([
  "SUCCEEDED",
  "FAILED",
  "CANCELLED",
  "DISPATCH_FAILED",
]);

// Mention labels are presentation; only their source ID participates in input.
function generationTextDocument(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(generationTextDocument);
  if (!value || typeof value !== "object") return value;
  const entry = value as Record<string, unknown>;
  return Object.fromEntries(
    Object.entries(entry).map(([key, child]) => [
      key,
      entry.type === "mention" &&
      key === "attrs" &&
      child &&
      typeof child === "object"
        ? Object.fromEntries(
            Object.entries(child).filter(
              ([name]) => name !== "label" && name !== "token",
            ),
          )
        : generationTextDocument(child),
    ]),
  );
}

export function matchesFrozenNode(
  node: CanvasNodeData,
  run: CanvasContract.CanvasGenerationVO,
) {
  if (
    !run.current ||
    !run.frozenNode ||
    !node.documentData ||
    isFixedCanvasMedia(node)
  )
    return false;
  const semantic = (value: CanvasContract.CanvasDocumentNodeDTO) => ({
    type: value.type,
    // The API serializes absent DTO properties as null, while editor patches
    // omit them. Normalize only the DTO envelope, never provider parameters.
    parameters: value.parameters
      ? {
          mode: value.parameters.mode ?? undefined,
          modelSelection: value.parameters.modelSelection
            ? {
                connectionId: String(
                  value.parameters.modelSelection.connectionId,
                ),
                modelId: value.parameters.modelSelection.modelId,
              }
            : undefined,
          modelParameters: value.parameters.modelParameters ?? undefined,
          modelParameterSchemaId:
            value.parameters.modelParameterSchemaId ?? undefined,
          values: value.parameters.values ?? undefined,
        }
      : undefined,
    textDocument: generationTextDocument(value.textDocument ?? undefined),
    fileReferences: value.fileReferences ?? [],
  });
  return (
    canvasGraphFingerprint(semantic(node.documentData)) ===
    canvasGraphFingerprint(semantic(run.frozenNode))
  );
}

export type CanvasGenerationProjection = CanvasContract.CanvasGenerationVO & {
  previewUrl?: string;
  previewFailed?: boolean;
};

/** Displaying an archived result does not grant it current-input execution rights. */
export function canDisplayCanvasGeneration(
  node: CanvasNodeData,
  run: CanvasGenerationProjection | undefined,
) {
  return (
    !isFixedCanvasMedia(node) &&
    ["image", "video"].includes(node.type) &&
    run?.state === "SUCCEEDED" &&
    run.nodeId === node.id &&
    run.frozenNode?.type === node.type &&
    !!run.outputs?.length
  );
}

/** User-facing media state; a completed command is not yet a readable image. */
export function canvasMediaState(
  node: CanvasNodeData,
  run: CanvasGenerationProjection | undefined,
  submitting: boolean,
) {
  if (!["image", "video"].includes(node.type)) return null;
  if (isFixedCanvasMedia(node)) return null;
  if (submitting) return run?.state === "SUCCEEDED" ? "loading" : "generating";
  if (
    !run ||
    (!matchesFrozenNode(node, run) && !canDisplayCanvasGeneration(node, run))
  )
    return null;
  if (run.state === "SUCCEEDED") {
    if (run.previewUrl) return null;
    return run.previewFailed || !run.outputs?.length
      ? "preview-error"
      : "loading";
  }
  if (run.state === "ARCHIVE_FAILED") return "archive-error";
  if (["UNCONFIRMED", "SUBMISSION_UNKNOWN"].includes(run.state ?? ""))
    return "unconfirmed";
  if (["FAILED", "DISPATCH_FAILED"].includes(run.state ?? "")) return "error";
  if (run.state === "CANCELLED") return null;
  return run.pending ? "generating" : null;
}
