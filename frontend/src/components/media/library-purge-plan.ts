import { z } from "zod";
import {
  libraryIdentitySchema,
  libraryUUID,
  sameLibraryIdentity,
  sameLibraryScope,
  type LibraryIdentity,
} from "./library-model";
import {
  purgeInputSchema,
  purgeJobSchema,
  purgePlanTargetsSchema,
  purgeTerminal,
  type PurgeReview,
  type PurgeJob,
} from "./library-purge-model";
import { getLibraryPurge, reviewLibraryPurge } from "./library-purge-query";
const revision = z.number().int().min(0).max(2147483647);
export const purgePlanSchema = libraryIdentitySchema
  .extend({
    version: z.literal(1),
    id: libraryUUID,
    revision,
    projectRevision: revision,
    targets: purgePlanTargetsSchema,
    batches: z
      .array(
        z
          .object({
            key: libraryUUID,
            input: purgeInputSchema.nullable(),
            job: purgeJobSchema.nullable(),
          })
          .strict(),
      )
      .min(1)
      .max(10),
  })
  .strict()
  .refine((plan) => {
    if (
      plan.batches.length !== Math.ceil(plan.targets.length / 200) ||
      new Set(plan.batches.map((batch) => batch.key)).size !==
        plan.batches.length ||
      (plan.scope.kind === "personal"
        ? plan.projectRevision !== 0
        : plan.projectRevision < 1)
    )
      return false;
    return plan.batches.every((batch, index) => {
      const targets = plan.targets.slice(index * 200, (index + 1) * 200);
      if (batch.job && !batch.input) return false;
      if (
        batch.input &&
        (!sameLibraryScope(plan.scope, batch.input.scope) ||
          JSON.stringify(batch.input.items) !== JSON.stringify(targets) ||
          (index > 0 && !plan.batches[index - 1].job))
      )
        return false;
      if (
        batch.job &&
        (batch.job.current_actor_id !== plan.actorId ||
          batch.job.current_org_id !== plan.orgId ||
          !sameLibraryScope(batch.job.scope, plan.scope) ||
          batch.job.items.length !== targets.length ||
          batch.job.items.some((item, i) => item.item_id !== targets[i].id))
      )
        return false;
      return true;
    });
  });
export type PurgePlan = z.infer<typeof purgePlanSchema>;
export function createPurgePlan(
  identity: LibraryIdentity,
  review: PurgeReview,
): PurgePlan {
  return purgePlanSchema.parse({
    ...identity,
    version: 1,
    id: crypto.randomUUID(),
    revision: review.revision,
    projectRevision: review.projectRevision,
    targets: review.items.map(({ id, revision }) => ({ id, revision })),
    batches: Array.from(
      { length: Math.ceil(review.items.length / 200) },
      () => ({ key: crypto.randomUUID(), input: null, job: null }),
    ),
  });
}
export async function checkPurgePlan(
  plan: PurgePlan,
  signal?: AbortSignal,
): Promise<{ plan: PurgePlan; index: number | null }> {
  const checked = purgePlanSchema.parse(plan);
  const identity: LibraryIdentity = {
    origin: checked.origin,
    actorId: checked.actorId,
    orgId: checked.orgId,
    libraryId: checked.libraryId,
    scope: checked.scope,
  };
  const batches = await Promise.all(
    checked.batches.map(async (batch) => {
      if (!batch.job) return batch;
      const job = await getLibraryPurge(identity, batch.job.id, signal);
      if (!purgeTerminal(job))
        throw new Error(
          "上一子批尚未取得真实终态或仍有未知结果，计划已停止。请核对原任务并明确对账/取消。",
        );
      return { ...batch, job };
    }),
  );
  const succeeded = new Set<string>();
  let delta = 0;
  for (const batch of batches) {
    if (!batch.job) continue;
    if (batch.job.items.some((item) => item.status !== "blocked")) delta++;
    for (const item of batch.job.items) {
      if (item.status === "succeeded") succeeded.add(item.item_id);
      if (item.status === "succeeded" || item.status === "cancelled") delta++;
    }
  }
  const current = await reviewLibraryPurge(identity, undefined, signal, true),
    expectedRevision = checked.revision + delta,
    expectedProjectRevision =
      checked.scope.kind === "project" ? checked.projectRevision + delta : 0;
  const remaining = checked.targets.filter((item) => !succeeded.has(item.id)),
    actual = new Map(current.items.map((item) => [item.id, item.revision]));
  if (
    current.revision !== expectedRevision ||
    current.projectRevision !== expectedProjectRevision ||
    current.items.length !== remaining.length ||
    remaining.some((item) => actual.get(item.id) !== item.revision)
  )
    throw new Error(
      "完整回收站或项目出现本计划以外的变化（新增条目、正文/CAS或版本变化），已停止。请结束原计划后重新核查全部并明确确认。",
    );
  const index = batches.findIndex((batch) => !batch.job);
  if (index < 0)
    return {
      plan: purgePlanSchema.parse({ ...checked, batches }),
      index: null,
    };
  const input = purgeInputSchema.parse({
    scope: checked.scope,
    items: checked.targets.slice(index * 200, (index + 1) * 200),
    expected_revision: expectedRevision,
    expected_project_revision: expectedProjectRevision,
    permanent_delete_confirmed: true,
  });
  if (
    batches[index].input &&
    JSON.stringify(batches[index].input) !== JSON.stringify(input)
  )
    throw new Error(
      "原子批正文已冻结且与当前事实不符；停止并核验原键，不能自动替换版本。",
    );
  batches[index] = { ...batches[index], input };
  return { plan: purgePlanSchema.parse({ ...checked, batches }), index };
}
export function recordPurgePlanJob(plan: PurgePlan, job: PurgeJob): PurgePlan {
  const index = plan.batches.findIndex(
    (batch) =>
      batch.job?.id === job.id ||
      (batch.input &&
        batch.input.items.length === job.items.length &&
        batch.input.items.every((item, i) => item.id === job.items[i].item_id)),
  );
  if (index < 0) throw new Error("返回任务与冻结子批不匹配，计划已停止。");
  const batches = plan.batches.map((batch, i) =>
    i === index ? { ...batch, job } : batch,
  );
  return purgePlanSchema.parse({ ...plan, batches });
}
type PlanStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const storageKey = (identity: LibraryIdentity) =>
  [
    "lanverse:media-purge-plan:v1",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
  ]
    .map(encodeURIComponent)
    .join(":");
const failure = () =>
  new Error(
    "完整清理计划的原范围、子键或目标存储不可用。请恢复原计划，当前不会继续删除。",
  );
export function loadPurgePlan(
  storage: PlanStorage,
  identity: LibraryIdentity,
): PurgePlan | null {
  try {
    const raw = storage.getItem(storageKey(identity));
    if (raw === null) return null;
    if (new TextEncoder().encode(raw).length > 1024 * 1024) throw failure();
    const plan = purgePlanSchema.parse(JSON.parse(raw));
    if (!sameLibraryIdentity(plan, identity)) throw failure();
    return plan;
  } catch {
    throw failure();
  }
}
export function savePurgePlan(
  storage: PlanStorage,
  plan: PurgePlan,
  previous: PurgePlan | null,
) {
  try {
    const parsed = purgePlanSchema.parse(plan),
      stored = loadPurgePlan(storage, {
        origin: parsed.origin,
        actorId: parsed.actorId,
        orgId: parsed.orgId,
        libraryId: parsed.libraryId,
        scope: parsed.scope,
      });
    if (JSON.stringify(stored) !== JSON.stringify(previous)) throw failure();
    const encoded = JSON.stringify(parsed);
    if (new TextEncoder().encode(encoded).length > 1024 * 1024) throw failure();
    storage.setItem(storageKey(parsed), encoded);
    if (storage.getItem(storageKey(parsed)) !== encoded) throw failure();
  } catch {
    throw failure();
  }
}
export function clearPurgePlan(
  storage: PlanStorage,
  identity: LibraryIdentity,
) {
  try {
    storage.removeItem(storageKey(identity));
    if (storage.getItem(storageKey(identity)) !== null) throw failure();
  } catch {
    throw failure();
  }
}
