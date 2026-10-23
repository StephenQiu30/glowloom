"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import { notifyError } from "@/components/canvas/runtime/components/feedback/error-notification";
import {
  CanvasCollaborationController,
  type CanvasYjsRecovery,
} from "../lib/canvas-collaboration-controller";
import type { CanvasScope } from "./use-canvas-document";
import type { RecoveryField } from "@glowloom/canvas-collab/recovery";
import { useCanvasLeaveGuard } from "./use-canvas-leave-guard";

/** React subscribes to one Yjs session; it never owns another graph save queue. */
export function useCanvasSave(options: {
  scope: CanvasScope;
  canvasId: string;
  identity: string;
  initialDocument: CanvasContract.CanvasVO;
  enabled?: boolean;
  onRestoreConfirmed?: () => void;
  onAccessInvalidated?: () => void;
}) {
  const [controller] = useState(
    () => new CanvasCollaborationController(options),
  );
  const view = useSyncExternalStore(
    controller.subscribe,
    controller.getSnapshot,
    controller.getSnapshot,
  );
  useEffect(() => {
    void controller
      .start()
      .catch((error) => notifyError(error, "画布协同连接未完成"));
    return () => {
      void controller.stop();
    };
  }, [controller]);
  useEffect(() => {
    controller.setEnabled(options.enabled ?? true);
  }, [controller, options.enabled]);
  const allowLeave = useCanvasLeaveGuard(
    controller.hasPending() ||
      view.recoveryChoices.length > 0 ||
      Object.keys(view.pendingForms).length > 0,
  );
  useEffect(() => {
    const beforeSignout = (event: Event) => {
      const detail = (
        event as CustomEvent<{ pending: boolean; discard: (() => void)[] }>
      ).detail;
      if (!detail) return;
      if (
        controller.hasPending() ||
        controller.getSnapshot().recoveryChoices.length
      )
        detail.pending = true;
      detail.discard.push(() => {
        allowLeave();
        void controller.stop();
      });
    };
    window.addEventListener("glowloom-before-signout", beforeSignout);
    return () =>
      window.removeEventListener("glowloom-before-signout", beforeSignout);
  }, [controller, allowLeave]);
  return {
    ...view,
    textBinding: controller.textBinding,
    undo: { ...view.undo, undo: controller.undo, redo: controller.redo },
    updateDraft: (
      graph: CanvasContract.CanvasDocumentGraphDTO,
      stable = true,
    ) => controller.updateDraft(graph, stable, view.state.draft),
    setStable: (value: boolean) => controller.setStable(value),
    setPendingForm: (
      nodeId: string,
      form: (typeof view.pendingForms)[string] | undefined,
    ) => controller.setPendingForm(nodeId, form),
    saveNow: () => controller.saveNow(),
    prepareGeneration: () => controller.prepareGeneration(),
    runCommand: (
      operation: "EDIT" | "DELETE" | "CHECKPOINT",
      extra?: { title?: string },
    ) => controller.runCommand(operation, extra),
    restoreSnapshot: (id: string) => controller.restoreSnapshot(id),
    chooseRecovery: (record: CanvasYjsRecovery) =>
      controller.chooseRecovery(record),
    beginFieldRecovery: (record?: CanvasYjsRecovery) =>
      controller.beginFieldRecovery(record),
    applyRecoveryFields: (sourceId: string, fields: RecoveryField[]) =>
      controller.applyRecoveryFields(sourceId, fields),
    discardRecoverySource: (sourceId: string) =>
      controller.discardRecoverySource(sourceId),
    discardRecovery: (record: CanvasYjsRecovery) =>
      controller.discardRecovery(record),
    resolveUnknown: () => controller.resolveUnknown(),
    retryOriginal: () => controller.retryOriginal(),
    recoverConnection: controller.probe,
    resumeIdentity: controller.probe,
    resumeAfterFix: () =>
      controller
        .resumeAfterFix()
        .catch((error) => notifyError(error, "原命令仍需确认")),
    clearFormError: (key: string) => controller.clearFormError(key),
    draftError: (message: string, key?: string) =>
      controller.draftError(message, key),
    applyTextDraft: controller.applyTextDraft.bind(controller),
    exportDraft: () => controller.exportDraft(),
    adoptServer: () => controller.discardCurrent(),
  };
}
