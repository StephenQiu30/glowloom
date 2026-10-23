export type ModelCapability = "image" | "video" | "text" | "audio";
export type ReasoningEffort = "auto" | "low" | "medium" | "high" | "xhigh";

export type ChannelModel = {
  name: string;
  capability: ModelCapability;
};

export type ModelChannel = {
  id: string;
  name: string;
  models: ChannelModel[];
};

export type AiConfig = {
  channels: ModelChannel[];
  model: string;
  imageModel: string;
  videoModel: string;
  textModel: string;
  audioModel: string;
  audioVoice: string;
  audioFormat: string;
  audioSpeed: string;
  audioInstructions: string;
  videoSeconds: string;
  vquality: string;
  videoGenerateAudio: string;
  videoWatermark: string;
  videoMode: string;
  systemPrompt: string;
  reasoningEffort: ReasoningEffort;
  models: string[];
  quality: string;
  size: string;
  background: string;
  count: string;
  canvasImageCount: string;
};

export const defaultConfig: AiConfig = {
  channels: [],
  model: "",
  imageModel: "",
  videoModel: "",
  textModel: "",
  audioModel: "",
  audioVoice: "alloy",
  audioFormat: "mp3",
  audioSpeed: "1",
  audioInstructions: "",
  videoSeconds: "6",
  vquality: "720",
  videoGenerateAudio: "true",
  videoWatermark: "false",
  videoMode: "frames",
  systemPrompt: "",
  reasoningEffort: "auto",
  models: [],
  quality: "auto",
  size: "1:1",
  background: "",
  count: "1",
  canvasImageCount: "3",
};
