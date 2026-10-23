"use client";

import { useMemo } from "react";
import { configActions } from "./app-store";
import { useAppDispatch, useAppSelector } from "./hooks";
import {
  defaultConfig,
  type AiConfig,
  type ModelCapability,
  type ModelChannel,
  type ChannelModel,
} from "@/components/canvas/runtime/types/generation-config";
export { defaultConfig } from "@/components/canvas/runtime/types/generation-config";
export type {
  AiConfig,
  ModelCapability,
  ModelChannel,
  ChannelModel,
  ReasoningEffort,
} from "@/components/canvas/runtime/types/generation-config";
const CHANNEL_MODEL_SEPARATOR = "::";

type ConfigStore = {
  config: AiConfig;
  setServiceConfig: (config: Partial<AiConfig>) => void;
  updateConfig: <K extends keyof AiConfig>(key: K, value: AiConfig[K]) => void;
  isAiConfigReady: (config: AiConfig, model: string) => boolean;
};

export function boolConfig(value: string, fallback: boolean) {
  return value ? value === "true" : fallback;
}

function findChannelModel(
  config: AiConfig,
  value: string,
): { channel: ModelChannel; model: ChannelModel } | null {
  const decoded = decodeChannelModel(value);
  const name = decoded?.model || value;
  const channel = decoded
    ? config.channels.find((item) => item.id === decoded.channelId)
    : config.channels.find((item) =>
        item.models.some((model) => model.name === name),
      );
  const model = channel?.models.find((item) => item.name === name);
  return channel && model ? { channel, model } : null;
}

export function modelCapabilityOf(
  config: AiConfig,
  value: string,
): ModelCapability | undefined {
  return findChannelModel(config, value)?.model.capability;
}

export function modelMatchesCapability(
  config: AiConfig,
  value: string,
  capability?: ModelCapability,
) {
  if (!capability) return true;
  return modelCapabilityOf(config, value) === capability;
}

export function resolveModelForCapability(
  config: AiConfig,
  currentModel: string | undefined,
  capability: ModelCapability,
) {
  const defaultModel =
    capability === "image"
      ? config.imageModel
      : capability === "video"
        ? config.videoModel
        : capability === "audio"
          ? config.audioModel
          : config.textModel;
  const fallbackModel =
    capability === "image"
      ? defaultConfig.imageModel
      : capability === "video"
        ? defaultConfig.videoModel
        : capability === "audio"
          ? defaultConfig.audioModel
          : defaultConfig.textModel;
  if (currentModel && modelMatchesCapability(config, currentModel, capability))
    return currentModel;
  if (defaultModel && modelMatchesCapability(config, defaultModel, capability))
    return defaultModel;
  return fallbackModel;
}

export function selectableModelsByCapability(
  config: AiConfig,
  capability?: ModelCapability,
) {
  if (!capability) return config.models;
  return config.channels.flatMap((channel) =>
    channel.models
      .filter((model) => model.capability === capability)
      .map((model) => encodeChannelModel(channel.id, model.name)),
  );
}

function isAiConfigReady() {
  return false;
}

export function useConfigStore<T>(selector: (state: ConfigStore) => T) {
  const dispatch = useAppDispatch();
  // Stable actions let components subscribe to a callback without subscribing to every parameter.
  const actions = useMemo(
    () => ({
      setServiceConfig: (config: Partial<AiConfig>) => {
        dispatch(
          configActions.patchConfig({
            ...config,
            ...(config.channels
              ? { models: modelOptionsFromChannels(config.channels) }
              : {}),
          }),
        );
      },
      updateConfig: <K extends keyof AiConfig>(key: K, value: AiConfig[K]) => {
        dispatch(configActions.patchConfig({ [key]: value }));
      },
      isAiConfigReady,
    }),
    [dispatch],
  );
  return useAppSelector((state) => selector({ ...state.config, ...actions }));
}
export function useEffectiveConfig() {
  return useConfigStore((state) => state.config);
}

export function encodeChannelModel(channelId: string, model: string) {
  return `${channelId}${CHANNEL_MODEL_SEPARATOR}${model.trim()}`;
}

export function decodeChannelModel(value: string) {
  const index = value.indexOf(CHANNEL_MODEL_SEPARATOR);
  if (index < 0) return null;
  return {
    channelId: value.slice(0, index),
    model: value.slice(index + CHANNEL_MODEL_SEPARATOR.length),
  };
}

export function modelOptionName(value: string) {
  return decodeChannelModel(value)?.model || value;
}

export function modelOptionLabel(config: AiConfig, value: string) {
  const decoded = decodeChannelModel(value);
  if (!decoded) return value;
  const channel = config.channels.find((item) => item.id === decoded.channelId);
  return channel ? `${decoded.model}（${channel.name}）` : decoded.model;
}

export function modelOptionsFromChannels(channels: ModelChannel[]) {
  return uniqueModelOptions(
    channels.flatMap((channel) =>
      channel.models.map((model) => encodeChannelModel(channel.id, model.name)),
    ),
  );
}

function uniqueModelOptions(models: string[]) {
  return Array.from(
    new Set(models.map((model) => model.trim()).filter(Boolean)),
  );
}
