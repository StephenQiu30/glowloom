import type { CanvasGraphShape, CanvasSnapshot } from "./model.mjs";
import type {
  CommittedState,
  RecoveryRecord,
  RecoverySource,
  RecoverySelection,
  SemanticIntent,
} from "./session.mjs";
export type RecoveryField = SemanticIntent & {
  key: string;
  targetId: string;
  current: { exists: boolean; value?: unknown };
  copyRequired: boolean;
  copyNode?: boolean;
};
export function intentKey(
  intent: Pick<SemanticIntent, "collection" | "id" | "field">,
): string;
export function archiveRecovery<G extends CanvasGraphShape>(
  record: RecoveryRecord<G>,
  initial: CommittedState,
): RecoveryRecord<G>;
export function recoveryFields<G extends CanvasGraphShape>(
  source: RecoverySource<G>,
  current: CanvasSnapshot<G>,
): RecoveryField[];
export function planRecovery<G extends CanvasGraphShape>(
  source: RecoverySource<G>,
  current: CanvasSnapshot<G>,
  selected: RecoveryField[],
  allocations?: Record<string, string>,
): {
  snapshot: CanvasSnapshot<G>;
  forms: Record<string, unknown>;
  selection: RecoverySelection;
};
