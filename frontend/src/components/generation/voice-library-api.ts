import {
  createReferenceVoice,
  favoriteSystemVoice,
  listReferenceVoices,
  listSystemVoices,
  updateReferenceVoice,
} from "@/api/voiceLibrary";
import { isProjectID } from "@/components/projects/project";
export type ReferenceVoice = Required<API.VoiceProfile>;
export type SystemVoice = Required<API.SystemVoice>;
export type VoiceSelection =
  | { kind: "system"; voice: SystemVoice }
  | { kind: "reference"; voice: ReferenceVoice };
function checked(value: API.VoiceProfile, project: string): ReferenceVoice {
  if (
    !isProjectID(value.id) ||
    value.project_id !== project ||
    !isProjectID(value.asset_id) ||
    !Number.isSafeInteger(value.revision) ||
    Number(value.revision) < 1 ||
    !Number.isSafeInteger(value.asset_revision) ||
    Number(value.asset_revision) < 1 ||
    typeof value.name !== "string" ||
    !value.name.trim() ||
    typeof value.favorite !== "boolean" ||
    !Number.isFinite(Date.parse(value.updated_at ?? ""))
  )
    throw new Error("音色回执不完整，请重新读取。");
  return value as ReferenceVoice;
}
export async function readVoices(project: string, signal?: AbortSignal) {
  const items: ReferenceVoice[] = [];
  let cursor: string | undefined;
  const seen = new Set<string>();
  do {
    const page = await listReferenceVoices(
      { pid: project, ...(cursor ? { cursor } : {}) },
      { signal },
    );
    if (!Array.isArray(page.items)) throw new Error("音色回执不完整");
    items.push(...page.items.map((v) => checked(v, project)));
    cursor = page.next_cursor ?? undefined;
    if (cursor) {
      if (!isProjectID(cursor) || seen.has(cursor))
        throw new Error("音色分页回执重复");
      seen.add(cursor);
    }
  } while (cursor);
  return items;
}
export async function readSystemVoices(project: string, signal?: AbortSignal) {
  const values = await listSystemVoices({ pid: project }, { signal });
  if (
    !Array.isArray(values) ||
    values.some((v) => !v.id || !v.name || typeof v.favorite !== "boolean")
  )
    throw new Error("系统音色回执不完整");
  return values as SystemVoice[];
}
export async function saveVoice(
  project: string,
  input: Required<API.VoiceCreate>,
) {
  return checked(await createReferenceVoice({ pid: project }, input), project);
}
export async function changeVoice(
  project: string,
  voice: ReferenceVoice,
  patch: { name?: string; favorite?: boolean; delete?: boolean },
) {
  return checked(
    await updateReferenceVoice(
      { pid: project, vid: voice.id },
      {
        revision: voice.revision,
        name: voice.name,
        favorite: voice.favorite,
        ...patch,
      },
    ),
    project,
  );
}
export const favoriteSystem = (project: string, voice: SystemVoice) =>
  favoriteSystemVoice(
    { pid: project, speaker: voice.id },
    { favorite: !voice.favorite },
  );
