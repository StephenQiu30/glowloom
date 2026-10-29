import type { ReactNode } from "react";
import type { CanvasEditorState } from "../lib/canvas-document-adapter";
import type { CanvasGenerationProjection } from "../lib/canvas-generation-projection";
import type { CanvasPendingForms } from "./canvas-recovery";
import type { NodeTextBinding } from "@glowloom/canvas-collab/model";

export type CanvasPreparedDocument = CanvasContract.CanvasVO & {
  epoch: string;
  commitSeq: string;
};

export type CanvasDocumentIntegration = {
  renderToolNode?: (nodeId: string, type: string) => ReactNode;
  presenceOverlay?: ReactNode;
  onCursor?: (cursor: { x: number; y: number } | null) => void;
  mediaContext?: import("../lib/canvas-file-preview").CanvasFilePreviewContext;
  mediaEnabled?: boolean;
  textBinding?: (nodeId: string) => NodeTextBinding | undefined;
  undo?: {
    canUndo: boolean;
    canRedo: boolean;
    undo: () => void;
    redo: () => void;
  };
  viewStateKey?: string;
  projectId?: string;
  initialState: CanvasEditorState;
  toolbar: (
    controls: Omit<
      React.ComponentProps<
        typeof import("../controls/canvas-top-bar").CanvasTopBar
      >,
      "title" | "onRenameProject" | "onDeleteProject"
    >,
  ) => ReactNode;
  canNavigate: boolean;
  frozen: boolean;
  admit?: (state: CanvasEditorState) => boolean;
  admitConnectionGraph?: (state: CanvasEditorState) => boolean;
  resolveConnectionMedia?: (
    node: CanvasEditorState["nodes"][number],
  ) => CanvasEditorState["nodes"][number];
  connectDomain?: (
    request: import("../lib/canvas-domain-connection").CanvasDomainConnectionRequest,
  ) => Promise<string>;
  onChange: (state: CanvasEditorState, stable: boolean) => void;
  pendingForms?: CanvasPendingForms;
  onPendingForm?: (
    nodeId: string,
    form: CanvasPendingForms[string] | undefined,
  ) => void;
  workflow?: {
    enabled: boolean;
    agentEnabled?: boolean;
    agentProfiles?: CanvasContract.WorkflowAgentProfileVO[];
    accepting: boolean;
    busy: boolean;
    choose: (nodeIds: string[]) => Promise<void>;
  };
  generation?: {
    ready?: boolean;
    runs: CanvasGenerationProjection[];
    submitting: string[];
    generate: (nodeId: string) => Promise<void>;
    previewLoaded?: (nodeId: string, source: string) => void;
    recoverPreview?: (nodeId: string, source: string) => void;
    refresh: (
      run: CanvasGenerationProjection,
      retry?: boolean,
    ) => Promise<void>;
  };
};
/** Recheck the whole connection candidate before a gesture or a visible graph commit. */
export function admitCanvasConnectionGraph(
  integration:
    | Pick<
        CanvasDocumentIntegration,
        "initialState" | "frozen" | "admit" | "admitConnectionGraph"
      >
    | undefined,
  candidate: Pick<CanvasEditorState, "nodes" | "connections">,
): boolean {
  if (!integration) return true;
  if (integration.frozen) return false;
  try {
    const admit = integration.admitConnectionGraph ?? integration.admit;
    return admit?.({ ...integration.initialState, ...candidate }) ?? true;
  } catch {
    return false;
  }
}
