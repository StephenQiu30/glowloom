import { expect, it } from "vitest";
import { modelsForCreation } from "@/components/generation/creation-contract";
import { referenceMode } from "@/components/generation/reference-mode";
import { editorIssues } from "@/components/generation/editor-capabilities";
import { emptyPromptDraft } from "@/components/generation/prompt-document";

const catalog = [
  "text2audio",
  "omni_audio",
  "music_song",
  "avatar",
  "motion",
  "text2video",
].map((mode) => ({ id: mode, current_version: { id: mode, modes: [mode] } }));
it("相同媒体类型的语音和音乐、视频和数字人不会混入对方模型选择", () => {
  expect(modelsForCreation(catalog, "audio").map((m) => m.id)).toEqual([
    "text2audio",
    "omni_audio",
  ]);
  expect(modelsForCreation(catalog, "music").map((m) => m.id)).toEqual([
    "music_song",
  ]);
  expect(modelsForCreation(catalog, "video").map((m) => m.id)).toEqual([
    "text2video",
  ]);
  expect(modelsForCreation(catalog, "avatar").map((m) => m.id)).toEqual([
    "avatar",
  ]);
  expect(modelsForCreation(catalog, "motion").map((m) => m.id)).toEqual([
    "motion",
  ]);
});
it("必填素材未添加时保留动作模式，并显示素材问题而不是要求提示词", () => {
  const draft = {
    ...emptyPromptDraft(),
    modelVersion: "motion",
    mode: "motion",
  };
  const model: API.ModelResponse = {
    status: "active",
    provider_status: "active",
    credential_present: true,
    input_roles: ["character_image", "driving_video"],
    current_version: {
      id: "motion",
      modes: ["motion"],
      param_schema: [],
      limits: {
        max_outputs: 1,
        roles: {
          character_image: { types: ["image"], min_count: 1, max_count: 1 },
          driving_video: { types: ["video"], min_count: 1, max_count: 1 },
        },
      },
    },
  };
  expect(referenceMode(draft, model)).toBe("motion");
  expect(editorIssues(draft, model)).toEqual([
    "请添加角色图片。",
    "请添加动作视频。",
  ]);
});
it("纯音乐模式不会被无素材模式推断重置为歌曲", () => {
  const draft = {
    ...emptyPromptDraft(),
    modelVersion: "music",
    mode: "music_instrumental",
  };
  expect(
    referenceMode(draft, {
      current_version: {
        id: "music",
        modes: ["music_song", "music_instrumental"],
      },
    }),
  ).toBe("music_instrumental");
});
