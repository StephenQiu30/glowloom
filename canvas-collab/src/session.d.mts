import type * as Y from "yjs";
import type {
  CanvasGraphShape,
  CanvasSnapshot,
  CanvasTextDraft,
} from "./model.mjs";

export type CollaborationScope = {
  origin: string;
  actorKey: string;
  canvasId: string;
  sessionId: string;
  epoch: string;
};
export type CommittedState = {
  epoch: string;
  commitSeq: string;
  contentRevision: string;
  graphHash: string;
  update: string;
  bindingRevision: string;
  modelVersion: number;
  lifecycle: string;
  edit: boolean;
};
export type FrozenUpdate = {
  reviewSeq?: string;
  epoch: string;
  sessionId: string;
  updateId: string;
  update: string;
  payloadHash: string;
};
export type CommittedUpdate = Pick<
  CommittedState,
  "epoch" | "commitSeq" | "contentRevision" | "graphHash" | "update"
> & {
  snapshot?: boolean;
};
export type DurableReceipt = Omit<CommittedUpdate, "update"> & {
  update?: string;
  updateId: string;
  payloadHash: string;
  effect: "NO_CHANGE" | "CAUSAL_CHANGE" | "VISIBLE_CHANGE";
  snapshotId?: string;
};
export type RecoveryRecord<G extends CanvasGraphShape = CanvasGraphShape> = {
  version: 2;
  generation: number;
  scope: CollaborationScope;
  confirmed: CommittedState;
  inFlight: FrozenUpdate | null;
  acceptedReceipt: DurableReceipt | null;
  recoverySources: RecoverySource<G>[];
  recoverySelection: RecoverySelection | null;
  intents: SemanticIntent[];
  inFlightIntents: SemanticIntent[];
  automaticReplayAttempts: number;
  pending: string[];
  snapshot: CanvasSnapshot<G>;
  sealed: null | {
    reason: string;
    failureReason?: string;
    snapshot: CanvasSnapshot<G>;
    inFlight: FrozenUpdate | null;
    pending: string[];
    nodeIds?: string[];
    originalOutcome?: "COMMITTED" | "NOT_FOUND";
    originalRejected?: boolean;
    originalReceipt?: DurableReceipt;
    intents?: SemanticIntent[];
    control?: ControlCommand | null;
    controlOutcome?: "COMMITTED" | "NOT_FOUND";
    controlReceipt?: Record<string, unknown>;
  };
  forms: Record<string, unknown>;
  textDrafts: Record<string, CanvasTextDraft>;
  control: ControlCommand | null;
  updatedAt: number;
};
export type SemanticIntent = {
  collection: "canvas" | "nodes" | "connections";
  id: string;
  field: string;
  generation: number;
  before: { exists: boolean; value?: unknown };
  after: { exists: boolean; value?: unknown };
  actual?: { exists: boolean; value?: unknown };
};
export type RecoverySource<G extends CanvasGraphShape = CanvasGraphShape> = {
  id: string;
  generation: number;
  scope: CollaborationScope;
  confirmed: CommittedState;
  sealed: NonNullable<RecoveryRecord<G>["sealed"]>;
  forms: Record<string, unknown>;
  textDrafts: Record<string, CanvasTextDraft>;
  textGenerations: Record<string, number>;
  locations: Record<string, string>;
  lastAttempt?: Pick<
    RecoverySource<G>,
    "scope" | "confirmed" | "sealed" | "forms" | "textDrafts"
  >;
};
export type RecoverySelection = {
  sourceId: string;
  updateId: string | null;
  mappings: {
    targetKey: string;
    sourceKeys: { key: string; generation: number }[];
  }[];
  locations: Record<string, string>;
  forms?: Record<string, unknown>;
};
export type ControlCommand = {
  operation: "DELETE" | "CHECKPOINT" | "RESTORE";
  status: "SENDING" | "UNKNOWN" | "REJECTED";
  body: Record<string, unknown> & { commandId: string };
  receipt?: Record<string, unknown>;
};
export type SessionTransport = {
  submit(update: FrozenUpdate): Promise<DurableReceipt>;
  query(update: FrozenUpdate): Promise<DurableReceipt | null>;
  confirmedSnapshot?(receipt: DurableReceipt): Promise<CanvasSnapshot>;
};
export type JournalWriter<G extends CanvasGraphShape = CanvasGraphShape> = {
  put(record: RecoveryRecord<G>): Promise<void>;
  close(): Promise<void>;
};
export class CollaborationSession<
  G extends CanvasGraphShape = CanvasGraphShape,
> {
  constructor(options: {
    scope: Omit<CollaborationScope, "epoch">;
    initial: CommittedState;
    storage?: "memory" | "journal";
    journal?: Pick<JournalWriter<G>, "put">;
    transport: SessionTransport;
    changed?: (session: CollaborationSession<G>) => void;
    recovery?: RecoveryRecord<G>;
  });
  readonly scope: CollaborationScope;
  doc: Y.Doc;
  confirmed: CommittedState;
  readonly snapshot: CanvasSnapshot<G>;
  readonly committedSnapshot: CanvasSnapshot<G>;
  readonly inFlight: FrozenUpdate | null;
  readonly acceptedReceipt: DurableReceipt | null;
  readonly recoverySources: RecoverySource<G>[];
  readonly recoverySelection: RecoverySelection | null;
  readonly automaticReplayAttempts: number;
  readonly pending: Uint8Array[];
  readonly sealed: RecoveryRecord<G>["sealed"];
  readonly forms: Record<string, unknown>;
  readonly textDrafts: Record<string, CanvasTextDraft>;
  readonly control: ControlCommand | null;
  readonly status: "CLEAN" | "DIRTY" | "SAVING" | "UNKNOWN" | "REJECTED";
  readonly protected: boolean;
  readonly journalError: unknown;
  readonly generation: number;
  readonly closed: boolean;
  change(after: CanvasSnapshot<G>, before?: CanvasSnapshot<G>): void;
  setForms(forms: Record<string, unknown>): void;
  setTextDraft(
    nodeId: string,
    document: CanvasTextDraft | null,
    source?: G["nodes"][number],
  ): void;
  setControl(control: ControlCommand | null): void;
  applyRecovery(
    sourceId: string,
    selected: import("./recovery.mjs").RecoveryField[],
  ): Promise<void>;
  retainSourceText(
    scope: CollaborationScope,
    nodeId: string,
    document: CanvasTextDraft | null,
    source?: G["nodes"][number],
    sourceId?: string,
  ): boolean;
  discardRecoverySource(sourceId: string): Promise<void>;
  releaseResolvedSources(retainedIds?: string[]): boolean;
  retainDetachedText(
    record: RecoveryRecord<G>,
    nodeId: string,
    document: CanvasTextDraft | null,
    sourceNode?: G["nodes"][number],
  ): string | undefined;
  record(): RecoveryRecord<G>;
  persist(): Promise<RecoveryRecord<G>>;
  protect(): Promise<boolean>;
  flush(options?: {
    automatic?: boolean;
    queryOnly?: boolean;
  }): Promise<DurableReceipt | null>;
  receive(update: CommittedUpdate, persist?: boolean): Promise<void>;
  destroy(): void;
}
