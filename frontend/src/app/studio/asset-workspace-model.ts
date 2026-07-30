import {
  Box,
  MapPin,
  Mic2,
  Palette,
  Shirt,
  Users,
  type LucideIcon,
} from "lucide-react";

export type AssetKind = API.AssetResponse["kind"];

export type AssetTypeConfig = {
  id: AssetKind;
  label: string;
  singular: string;
  icon: LucideIcon;
  mediaKind: "image" | "audio";
  mediaPurpose: API.AssetMediaReferenceRequest["purpose"];
  mediaOptional?: boolean;
};

export const assetTypes: AssetTypeConfig[] = [
  {
    id: "character",
    label: "角色",
    singular: "角色",
    icon: Users,
    mediaKind: "image",
    mediaPurpose: "portrait",
  },
  {
    id: "location",
    label: "场景",
    singular: "场景",
    icon: MapPin,
    mediaKind: "image",
    mediaPurpose: "environment",
  },
  {
    id: "prop",
    label: "道具",
    singular: "道具",
    icon: Box,
    mediaKind: "image",
    mediaPurpose: "object",
  },
  {
    id: "costume",
    label: "服装",
    singular: "服装",
    icon: Shirt,
    mediaKind: "image",
    mediaPurpose: "outfit",
  },
  {
    id: "voice",
    label: "声音",
    singular: "声音",
    icon: Mic2,
    mediaKind: "audio",
    mediaPurpose: "voice_sample",
  },
  {
    id: "visual_style",
    label: "风格",
    singular: "视觉风格",
    icon: Palette,
    mediaKind: "image",
    mediaPurpose: "style_reference",
    mediaOptional: true,
  },
];

export const selectClassName =
  "h-9 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm outline-none transition focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10";
export const textareaClassName =
  "min-h-24 w-full resize-y rounded-xl border border-slate-200 bg-white px-3 py-2.5 text-sm leading-6 outline-none transition focus:border-[#079db3] focus:ring-3 focus:ring-cyan-500/10";
export const dialogClassName =
  "fixed top-1/2 left-1/2 z-50 max-h-[88vh] w-[calc(100%-2rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl border border-slate-200 bg-white p-6 shadow-2xl shadow-slate-950/10";

export function typeConfig(kind: AssetKind): AssetTypeConfig {
  return assetTypes.find((item) => item.id === kind) ?? assetTypes[0];
}

export function splitValues(value: FormDataEntryValue | null): string[] {
  return String(value ?? "")
    .split(/[，,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function textValue(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

export function shortId(value: string): string {
  return value.slice(0, 8);
}

export function formatDate(value: string): string {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

export function buildSpec(
  kind: AssetKind,
  form: FormData,
): API.AssetVersionCreateRequest["spec"] {
  switch (kind) {
    case "character":
      return {
        kind,
        identity: textValue(form, "identity"),
        appearance: textValue(form, "appearance"),
        age_impression: textValue(form, "ageImpression"),
        temperament: splitValues(form.get("temperament")),
      };
    case "location":
      return {
        kind,
        spatial_description: textValue(form, "spatialDescription"),
        time_weather: textValue(form, "timeWeather"),
        visual_elements: splitValues(form.get("visualElements")),
        lighting: textValue(form, "lighting"),
      };
    case "prop":
      return {
        kind,
        appearance: textValue(form, "appearance"),
        material: textValue(form, "material"),
        usage_context: textValue(form, "usageContext"),
        holder_character_id: textValue(form, "relatedCharacter") || null,
      };
    case "costume":
      return {
        kind,
        appearance: textValue(form, "appearance"),
        material: textValue(form, "material"),
        usage_context: textValue(form, "usageContext"),
        wearer_character_id: textValue(form, "relatedCharacter") || null,
      };
    case "visual_style":
      return {
        kind,
        visual_language: textValue(form, "visualLanguage"),
        palette: textValue(form, "palette"),
        lighting_language: textValue(form, "lightingLanguage"),
        negative_constraints: splitValues(form.get("negativeConstraints")),
      };
    case "voice":
      return {
        kind,
        source_kind:
          (textValue(form, "sourceKind") as API.VoiceSpec["source_kind"]) || null,
        language: textValue(form, "language"),
        performance_traits: splitValues(form.get("performanceTraits")),
        allowed_usage: splitValues(form.get("allowedUsage")),
      };
  }
}
