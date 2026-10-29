import type { GenerationSeed } from "./generation-dialog";

const modes: Record<string, string[]> = {
  image: ["text2image", "image2image", "text_to_image"],
  video: ["text2video", "image2video", "omni_reference"],
  audio: ["text2audio", "omni_audio"],
  music: ["music_song", "music_instrumental"],
  avatar: ["avatar"],
  motion: ["motion"],
};

/** Media output alone cannot distinguish speech/music or video/avatar/motion. */
export function modelsForCreation(
  models: API.ModelResponse[],
  type?: GenerationSeed["creationType"],
) {
  if (!type) return models;
  return models.filter((model) =>
    model.current_version?.modes?.some((mode) => modes[type].includes(mode)),
  );
}

export function creationModeLabel(mode: string) {
  return (
    (
      {
        text2audio: "文本朗读",
        omni_audio: "全能配音",
        text2image: "文生图",
        image2image: "图生图",
        text2video: "文生视频",
        image2video: "首尾帧",
        omni_reference: "全能参考",
        music_song: "歌曲",
        music_instrumental: "纯音乐",
        avatar: "数字人",
        motion: "动作模仿",
      } as Record<string, string>
    )[mode] ?? mode
  );
}
