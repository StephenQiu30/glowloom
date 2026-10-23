export type WorkspaceResource = {
  kind: "project" | "canvas";
  id: string;
  title: string;
  updatedAt: string;
  href: string;
  revision?: number | string;
  projectBindingRevision?: number | string;
  epoch?: string;
  commitSeq?: string;
  parentId?: string;
  coverFileId?: string;
  coverFileVersion?: string;
};

export type WorkspaceResourceTarget = Pick<
  WorkspaceResource,
  "kind" | "id" | "title" | "parentId" | "coverFileId" | "coverFileVersion"
> & {
  revision: number | string;
  projectBindingRevision?: number | string;
  epoch?: string;
  commitSeq?: string;
};

export const resourceKey = (item: Pick<WorkspaceResource, "kind" | "id">) =>
  `${item.kind}:${item.id}`;

export function resourceTarget(
  item: WorkspaceResource,
): WorkspaceResourceTarget | null {
  if (
    item.revision == null ||
    (item.kind === "canvas" && item.projectBindingRevision == null)
  )
    return null;
  return {
    kind: item.kind,
    id: item.id,
    title: item.title,
    revision: item.revision,
    projectBindingRevision: item.projectBindingRevision,
    parentId: item.parentId,
    coverFileId: item.coverFileId,
    coverFileVersion: item.coverFileVersion,
  };
}

type Page<T> = { records: T[]; total: number | string };

/** Read enough of one ordered source to compute a complete merged page. */
export async function readResourcePrefix<T>(
  read: (current: number, pageSize: number) => Promise<Page<T>>,
  count: number,
): Promise<Page<T>> {
  const pageSize = Math.min(count, 50);
  const first = await read(1, pageSize);
  const records = [...first.records];
  const target = Math.min(count, Number(first.total));
  if (records.length < Math.min(pageSize, target))
    throw new Error("列表已变化，请重试");
  for (let current = 2; records.length < target; current++) {
    const next = await read(current, pageSize);
    if (next.records.length < Math.min(pageSize, target - records.length))
      throw new Error("列表已变化，请重试");
    records.push(...next.records);
  }
  return { records: records.slice(0, count), total: first.total };
}

export function mergeResourcePage(
  projects: WorkspaceResource[],
  canvases: WorkspaceResource[],
  page: number,
  pageSize: number,
  oldest = false,
) {
  // Stable sort preserves each service's ID tiebreaker without losing bigint precision.
  return [...projects, ...canvases]
    .sort((a, b) => {
      const time = Date.parse(a.updatedAt) - Date.parse(b.updatedAt);
      return time ? (oldest ? time : -time) : 0;
    })
    .slice((page - 1) * pageSize, page * pageSize);
}
