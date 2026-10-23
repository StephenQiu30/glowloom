import type { CanvasGraphShape } from "./model.mjs";
import type {
  CollaborationScope,
  JournalWriter,
  RecoveryRecord,
} from "./session.mjs";

export type JournalAccount = Pick<CollaborationScope, "origin" | "actorKey">;
export type JournalCanvas = JournalAccount &
  Pick<CollaborationScope, "canvasId">;
export class CollaborationJournal {
  open<G extends CanvasGraphShape>(
    scope: CollaborationScope,
  ): Promise<JournalWriter<G>>;
  transplant<G extends CanvasGraphShape>(
    source: RecoveryRecord<G>,
    target: RecoveryRecord<G>,
  ): Promise<JournalWriter<G>>;
  list<G extends CanvasGraphShape>(
    scope: JournalCanvas,
  ): Promise<RecoveryRecord<G>[]>;
  summarizeActor(
    origin: string,
    actorKey: string,
  ): Promise<
    {
      canvasId: string;
      scope: "USER";
      count: number;
      bytes: number;
      unsupported: false;
    }[]
  >;
  countActor(origin: string, actorKey: string): Promise<number>;
  removeCanvas(scope: JournalCanvas): Promise<void>;
  removeRecord(scope: CollaborationScope): Promise<void>;
  removeActor(origin: string, actorKey: string): Promise<void>;
}
export const collaborationJournal: CollaborationJournal;
