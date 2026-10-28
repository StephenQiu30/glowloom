import type { ReferenceBinding } from "@/components/generation/prompt-document";
import type { UnifiedSubject } from "./unified-contracts";
import type { SubjectPreviewMedia } from "./unified-media";

/** A pinned subject's primary thumbnail uses its exact authorized binding. */
export function unifiedReferencePreview(
  reference: ReferenceBinding,
): SubjectPreviewMedia | null {
  const source = reference.source;
  if (source.kind !== "subject") return null;
  const primary = source.media[0];
  const revision = Number(primary?.sourceVersion);
  if (!primary?.bindingId || !Number.isSafeInteger(revision) || revision < 1)
    return null;
  return {
    asset_id: primary.sourceId,
    asset_revision: revision,
    kind: primary.mediaKind,
    bound: {
      subjectID: source.sourceId,
      versionID: source.sourceVersion,
      bindingID: primary.bindingId,
    },
  };
}

/** This is a display hint; server catalog validation still decides admission. */
export function unifiedSubjectModelIssue(
  references: ReferenceBinding[],
  model?: API.ModelResponse,
): string | null {
  if (!model) return null;
  const roles: unknown = model.current_version?.limits?.roles;
  for (const reference of references) {
    if (reference.source.kind !== "subject") continue;
    for (const media of reference.source.media) {
      if (!media.bindingId) continue;
      const role =
        reference.role === "subject"
          ? `reference_${media.mediaKind}`
          : reference.role;
      const rule: unknown =
        roles && typeof roles === "object"
          ? (roles as Record<string, unknown>)[role]
          : null;
      const types: unknown =
        rule && typeof rule === "object"
          ? (rule as Record<string, unknown>).types
          : null;
      if (
        !model.input_roles?.includes(role) ||
        !Array.isArray(types) ||
        !types.includes(media.mediaKind)
      ) {
        const kind =
          media.mediaKind === "audio"
            ? "音色"
            : media.mediaKind === "video"
              ? "视频"
              : "图片";
        return `当前模型未声明支持主体的${kind}参考，请更换支持该素材的模型。原主体引用已保留。`;
      }
    }
  }
  return null;
}

/** Mirrors the server's fixed version expansion, including every binding proof. */
export function unifiedSubjectBinding(
  subject: UnifiedSubject,
  role = "subject",
): ReferenceBinding {
  const media = [subject.primary, ...subject.references];
  if (subject.voice) media.push(subject.voice.media);
  return {
    id: crypto.randomUUID(),
    bindingGeneration: 1,
    mediaKind: subject.primary.kind,
    role,
    order: 0,
    source: {
      kind: "subject",
      sourceId: subject.id,
      sourceVersion: subject.version_id,
      description:
        subject.name + (subject.description ? `：${subject.description}` : ""),
      media: media.map((item) => ({
        sourceId: item.asset_id,
        sourceVersion: String(item.asset_revision),
        mediaKind: item.kind,
        bindingId: item.binding_id,
      })),
    },
  };
}
