import type {
  CollaborationScope,
  JournalWriter,
  RecoveryRecord,
} from "../../types/session.js";

export type JournalAccount = Pick<CollaborationScope, "origin" | "actorKey">;
export type JournalCanvas = JournalAccount & Pick<CollaborationScope, "canvasId">;

interface JournalTx {
  meta: (id: string) => Promise<any>;
  rows: (owner: string) => Promise<any[]>;
  read: (id: string) => Promise<any>;
  writeMeta: (row: any) => void;
  write: (id: string, record: any) => void;
  remove: (id: string) => void;
  removeMeta: (id: string) => void;
}

const key = (scope: any) =>
  JSON.stringify([
    scope.origin,
    scope.actorKey,
    scope.canvasId,
    scope.epoch,
    scope.sessionId,
  ]);
const owner = (scope: any) => JSON.stringify([scope.origin, scope.actorKey]);
const bytes = (value: any) =>
  new TextEncoder().encode(JSON.stringify(value)).byteLength;
const fail = (code: string) => new Error(code);

function validate(record: any) {
  if (
    Object.keys(record).some(
      (name) =>
        ![
          "version",
          "scope",
          "generation",
          "confirmed",
          "inFlight",
          "acceptedReceipt",
          "recoverySources",
          "recoverySelection",
          "intents",
          "inFlightIntents",
          "automaticReplayAttempts",
          "pending",
          "snapshot",
          "forms",
          "textDrafts",
          "control",
          "sealed",
          "updatedAt",
        ].includes(name),
    )
  )
    throw fail("JOURNAL_FIELD");
  if (
    record.version !== 2 ||
    !Number.isSafeInteger(record.generation) ||
    record.generation < 0 ||
    !Array.isArray(record.intents) ||
    record.intents.length > 4501 ||
    !Array.isArray(record.inFlightIntents) ||
    record.inFlightIntents.length > 4501 ||
    !Array.isArray(record.recoverySources) ||
    record.recoverySources.length > 16 ||
    !Number.isSafeInteger(record.automaticReplayAttempts) ||
    record.automaticReplayAttempts < 0 ||
    record.automaticReplayAttempts > 2 ||
    !record.scope.origin ||
    !record.scope.epoch ||
    record.scope.epoch !== record.confirmed.epoch ||
    !/^USER:[1-9][0-9]*$/.test(record.scope.actorKey) ||
    !/^[1-9][0-9]*$/.test(record.scope.canvasId) ||
    !/^[a-f0-9-]{36}$/.test(record.scope.sessionId)
  )
    throw fail("JOURNAL_FORMAT");
  const visit = (value: any) => {
    if (
      value === null ||
      ["string", "boolean"].includes(typeof value) ||
      (typeof value === "number" && Number.isFinite(value))
    )
      return;
    if (Array.isArray(value)) {
      value.forEach(visit);
      return;
    }
    if (
      !value ||
      typeof value !== "object" ||
      Object.getPrototypeOf(value) !== Object.prototype
    )
      throw fail("JOURNAL_FIELD");
    for (const [name, item] of Object.entries(value)) {
      if (
        /^(cookie|csrf(token)?|authorization|accessToken|refreshToken|providerKey|apiKey|grant|credentials|secret|previewUrl)$/i.test(
          name,
        )
      )
        throw fail("JOURNAL_FIELD");
      visit(item);
    }
  };
  visit(record);
  if (bytes(record) > 4 * 1024 * 1024) throw fail("JOURNAL_BUDGET");
}

/** Recovery records never outlive this page/runtime instance. */
class MemoryJournalDatabase {
  metadata = new Map();
  records = new Map();
  queue: Promise<any> = Promise.resolve();
  run<T>(action: (tx: JournalTx) => Promise<T>): Promise<T> {
    const operation = this.queue
      .catch(() => {})
      .then(async () => {
        const meta = structuredClone(this.metadata),
          data = structuredClone(this.records);
        const result = await action({
          meta: async (id: string) => meta.get(id),
          rows: async (owner: string) =>
            [...meta.values()].filter((row: any) => row.owner === owner),
          read: async (id: string) => data.get(id),
          writeMeta: (row: any) => meta.set(row.id, structuredClone(row)),
          write: (id: string, record: any) => data.set(id, structuredClone(record)),
          remove: (id: string) => data.delete(id),
          removeMeta: (id: string) => meta.delete(id),
        });
        this.metadata = meta;
        this.records = data;
        return structuredClone(result);
      });
    this.queue = operation;
    return operation;
  }
}

export class CollaborationJournal {
  declare private database: MemoryJournalDatabase;
  constructor(database = new MemoryJournalDatabase()) {
    this.database = database;
  }

  async open(scope: CollaborationScope): Promise<JournalWriter> {
    const id = key(scope),
      account = owner(scope),
      marker = "actor:" + account;
    const writer = crypto.randomUUID();
    const actorGeneration = await this.database.run(async (tx: JournalTx) => {
      const previous = await tx.meta(id),
        actor = await tx.meta(marker);
      tx.writeMeta({
        id,
        owner: account,
        canvasId: scope.canvasId,
        epoch: scope.epoch,
        sessionId: scope.sessionId,
        writer,
        sequence: previous?.present ? previous.sequence : 0,
        bytes: previous?.bytes ?? 0,
        present: previous?.present ?? false,
      });
      return actor?.generation ?? 0;
    });
    return this.#writer(scope, writer, actorGeneration);
  }

  /** One local transaction retains the source inside the new record and never needs a seventeenth slot. */
  async transplant(source: any, target: any): Promise<JournalWriter> {
    validate(source);
    validate(target);
    for (const field of ["origin", "actorKey", "canvasId", "sessionId"])
      if (source.scope[field] !== target.scope[field])
        throw fail("JOURNAL_SCOPE");
    if (target.generation <= source.generation) throw fail("JOURNAL_SEQUENCE");
    const from = key(source.scope),
      to = key(target.scope),
      account = owner(source.scope);
    const expected = JSON.stringify(source),
      replacement = structuredClone(target),
      size = bytes(target);
    const writer = crypto.randomUUID();
    const actorGeneration = await this.database.run(async (tx: JournalTx) => {
      const previous = await tx.meta(from),
        current = await tx.read(from);
      if (!previous?.present || JSON.stringify(current) !== expected)
        throw fail("JOURNAL_SOURCE_CHANGED");
      if (previous.writer != null) throw fail("JOURNAL_SOURCE_BUSY");
      const retained = replacement.recoverySources;
      const same = (left: any, right: any) =>
        JSON.stringify(left) === JSON.stringify(right);
      if (
        !retained.length ||
        source.recoverySources.some(
          (old: any) =>
            !retained.some(
              (next: any) =>
                next.id === old.id &&
                same(next.sealed, old.sealed) &&
                same(next.forms, old.forms) &&
                same(next.textDrafts, old.textDrafts),
            ),
        )
      )
        throw fail("JOURNAL_SOURCE_RETENTION");
      const privateRoot =
        source.sealed ||
        source.inFlight ||
        source.pending.length ||
        Object.keys(source.forms).length ||
        Object.keys(source.textDrafts ?? {}).length;
      if (
        privateRoot &&
        !source.recoverySelection &&
        !retained.some(
          (next: any) =>
            same(next.scope, source.scope) &&
            same(
              next.sealed.snapshot,
              source.sealed?.snapshot ?? source.snapshot,
            ) &&
            same(next.forms, source.forms) &&
            same(next.textDrafts, source.textDrafts ?? {}),
        )
      )
        throw fail("JOURNAL_SOURCE_RETENTION");
      if (from !== to && (await tx.meta(to))) throw fail("JOURNAL_TARGET_BUSY");
      const others = (await tx.rows(account)).filter(
        (row: any) => row.present && row.id !== from && row.id !== to,
      );
      if (
        others.length >= 16 ||
        others.reduce((sum: number, row: any) => sum + row.bytes, size) > 64 * 1024 * 1024
      )
        throw fail("JOURNAL_BUDGET");
      const actor = await tx.meta("actor:" + account);
      tx.remove(from);
      tx.removeMeta(from);
      tx.writeMeta({
        id: to,
        owner: account,
        canvasId: target.scope.canvasId,
        epoch: target.scope.epoch,
        sessionId: target.scope.sessionId,
        writer,
        sequence: target.generation,
        bytes: size,
        present: true,
      });
      tx.write(to, replacement);
      return actor?.generation ?? 0;
    });
    return this.#writer(target.scope, writer, actorGeneration);
  }

  #writer(scope: any, writer: string, actorGeneration: number): JournalWriter {
    const id = key(scope),
      account = owner(scope),
      marker = "actor:" + account;
    return {
      put: async (input: any) => {
        const record = structuredClone(input);
        validate(record);
        if (key(record.scope) !== id) throw fail("JOURNAL_SCOPE");
        await this.database.run(async (tx: JournalTx) => {
          const previous = await tx.meta(id),
            actor = await tx.meta(marker);
          if (
            previous?.writer !== writer ||
            (actor?.generation ?? 0) !== actorGeneration
          )
            throw fail("JOURNAL_INVALIDATED");
          if ((previous?.sequence ?? 0) > record.generation)
            throw fail("JOURNAL_SEQUENCE");
          const clean =
            !record.inFlight &&
            !record.acceptedReceipt &&
            !record.recoverySources.length &&
            !record.recoverySelection &&
            !record.intents.length &&
            !record.inFlightIntents.length &&
            !record.pending.length &&
            !record.sealed &&
            !record.control &&
            !Object.keys(record.textDrafts ?? {}).length &&
            !Object.keys(record.forms ?? {}).length;
          const size = clean ? 0 : bytes(record),
            others = (await tx.rows(account)).filter(
              (row: any) => row.present && row.id !== id,
            );
          if (
            !clean &&
            (others.length >= 16 ||
              others.reduce((sum: number, row: any) => sum + row.bytes, size) >
                64 * 1024 * 1024)
          )
            throw fail("JOURNAL_BUDGET");
          tx.writeMeta({
            id,
            owner: account,
            canvasId: scope.canvasId,
            sessionId: scope.sessionId,
            writer,
            epoch: scope.epoch,
            sequence: record.generation,
            bytes: size,
            present: !clean,
          });
          if (clean) tx.remove(id);
          else tx.write(id, record);
        });
      },
      close: () =>
        this.database.run(async (tx: JournalTx) => {
          const current = await tx.meta(id);
          if (current?.writer === writer) {
            if (!current.present) tx.removeMeta(id);
            else tx.writeMeta({ ...current, writer: null });
          }
        }),
    };
  }

  list(scope: JournalCanvas): Promise<RecoveryRecord[]> {
    return this.database.run(async (tx: JournalTx) => {
      const rows = (await tx.rows(owner(scope))).filter(
        (row: any) => row.present && row.canvasId === scope.canvasId,
      );
      const records = await Promise.all(rows.map((row: any) => tx.read(row.id)));
      records.forEach(validate);
      return structuredClone(records);
    });
  }

  summarizeActor(origin: string, actorKey: string) {
    return this.database.run(async (tx: JournalTx) => {
      const rows = (await tx.rows(owner({ origin, actorKey }))).filter(
        (row: any) => row.present,
      );
      const summaries = new Map();
      for (const row of rows) {
        const value = summaries.get(row.canvasId) ?? {
          canvasId: row.canvasId,
          scope: "USER",
          count: 0,
          bytes: 0,
          unsupported: false,
        };
        value.count++;
        value.bytes += row.bytes;
        summaries.set(row.canvasId, value);
      }
      return [...summaries.values()];
    });
  }

  removeCanvas(scope: JournalCanvas) {
    return this.discard(scope, false);
  }
  removeRecord(scope: CollaborationScope) {
    return this.database.run(async (tx: JournalTx) => {
      tx.removeMeta(key(scope));
      tx.remove(key(scope));
    });
  }
  removeActor(origin: string, actorKey: string) {
    return this.discard({ origin, actorKey }, true);
  }
  countActor(origin: string, actorKey: string) {
    return this.summarizeActor(origin, actorKey).then((rows) =>
      rows.reduce((sum: number, row: any) => sum + row.count, 0),
    );
  }

  discard(scope: any, all: boolean) {
    return this.database.run(async (tx: JournalTx) => {
      const account = owner(scope);
      if (all) {
        const id = "actor:" + account,
          marker = await tx.meta(id);
        tx.writeMeta({
          id,
          owner: account,
          generation: (marker?.generation ?? 0) + 1,
          present: false,
        });
      }
      for (const row of await tx.rows(account)) {
        if (!row.canvasId || (!all && row.canvasId !== scope.canvasId))
          continue;
        tx.removeMeta(row.id);
        tx.remove(row.id);
      }
    });
  }
}

export const collaborationJournal = new CollaborationJournal();
