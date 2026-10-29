"use client";
import { useEffect, useRef, useState } from "react";
import {
  createSubjectVoiceJob,
  getSubjectVoiceCommand,
  getSubjectVoiceJob,
  listSubjectVoiceOptions,
} from "@/api/subjects";
import {
  getScopeGeneration,
  getSessionGeneration,
  RequestError,
} from "@/lib/request";
import type { SelectedSubjectMedia } from "./unified-contracts";
import {
  requireSubjectVoiceJob,
  requireSubjectVoiceOptions,
  subjectVoiceError,
  type SubjectVoiceInput,
  type SubjectVoiceJob,
  type SubjectVoiceOptions,
} from "./unified-voice-contracts";

type Phase =
  | "idle"
  | "options"
  | "submitting"
  | "waiting"
  | "ready"
  | "failed"
  | "unavailable"
  | "unknown"
  | "read_error"
  | "unrecorded"
  | "import_pending";
type Operation = {
  key: string;
  input: SubjectVoiceInput;
  modelVersion: string;
};
const isActive = (phase: Phase) =>
  [
    "options",
    "submitting",
    "waiting",
    "unknown",
    "read_error",
    "unrecorded",
    "import_pending",
  ].includes(phase);
const sample = "你好，很高兴与你一起创作。";
export function useSubjectVoiceJob({
  actorID,
  scopeID,
  onReady,
}: {
  actorID: string;
  scopeID: string;
  onReady: (name: string, media: SelectedSubjectMedia) => void;
}) {
  const [phase, setPhase] = useState<Phase>("idle");
  const [options, setOptions] = useState<SubjectVoiceOptions | null>(null);
  const [job, setJob] = useState<SubjectVoiceJob | null>(null);
  const [error, setError] = useState("");
  const phaseRef = useRef<Phase>("idle");
  const optionRef = useRef<SubjectVoiceOptions | null>(null);
  const operation = useRef<Operation | null>(null);
  const jobRef = useRef<SubjectVoiceJob | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const request = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  const generation = useRef(0);
  const attempt = useRef(0);
  const accepted = useRef("");
  const callback = useRef(onReady);
  useEffect(() => {
    callback.current = onReady;
  }, [onReady]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      request.current?.abort();
      if (timer.current) clearTimeout(timer.current);
    };
  }, [actorID, scopeID]);
  function change(value: Phase) {
    phaseRef.current = value;
    setPhase(value);
  }
  function stop() {
    ++generation.current;
    request.current?.abort();
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  }
  function flight() {
    stop();
    const token = generation.current,
      scope = getScopeGeneration(),
      session = getSessionGeneration(),
      controller = new AbortController();
    request.current = controller;
    return {
      controller,
      current: () =>
        mounted.current &&
        !controller.signal.aborted &&
        token === generation.current &&
        scope === getScopeGeneration() &&
        session === getSessionGeneration(),
    };
  }
  function accept(value: unknown, op: Operation) {
    const next = requireSubjectVoiceJob(
      value,
      actorID,
      scopeID,
      op.input,
      op.modelVersion,
      jobRef.current ?? undefined,
    );
    jobRef.current = next;
    setJob(next);
    setError("");
    if (next.status === "succeeded") {
      change("ready");
      if (accepted.current !== next.id) {
        accepted.current = next.id;
        callback.current(next.name, next.result!);
      }
    } else if (next.status === "failed") {
      change("failed");
      setError(subjectVoiceError(next.error_code));
    } else if (next.status === "unknown") {
      change("unknown");
      setError("音色生成结果尚未确认，请恢复读取；不会自动重新生成。");
    } else if (
      next.status === "ingesting" &&
      next.error_code === "media_import_pending"
    ) {
      change("import_pending");
      setError("音频已生成，保存尚未完成。重试保存音频不会重新合成。");
    } else {
      change("waiting");
      timer.current = setTimeout(() => void read(op), 1500);
    }
  }
  async function read(op: Operation) {
    if (!jobRef.current) return;
    const task = flight();
    try {
      const value = await getSubjectVoiceJob(
        { job_id: jobRef.current.id },
        { signal: task.controller.signal },
      );
      if (task.current()) accept(value, op);
    } catch (reason) {
      if (task.current()) {
        change("read_error");
        setError(
          reason instanceof RequestError
            ? subjectVoiceError(reason.code)
            : "暂时无法读取音色任务，请恢复读取。",
        );
      }
    }
  }
  async function submit(op: Operation) {
    const task = flight();
    change("submitting");
    setError("");
    try {
      const value = await createSubjectVoiceJob(op.input, {
        headers: { "Idempotency-Key": op.key },
        signal: task.controller.signal,
      });
      if (task.current()) accept(value, op);
    } catch (reason) {
      if (task.current()) {
        const rejected =
          reason instanceof RequestError &&
          reason.status >= 400 &&
          reason.status < 500;
        change(rejected ? "failed" : "unknown");
        setError(
          rejected
            ? subjectVoiceError(reason.code)
            : "音色任务提交结果尚未确认，请恢复读取。",
        );
      }
    }
  }
  async function generate(
    releaseID: string,
    voiceKey: string,
    name: string,
    text: string,
  ) {
    if (phaseRef.current !== "options") return;
    const model = optionRef.current?.models.find(
      (model) => model.release_id === releaseID,
    );
    if (
      !model ||
      !model.voices.some((voice) => voice.key === voiceKey) ||
      !name.trim() ||
      name.trim().length > 20 ||
      !text.trim() ||
      Array.from(text.trim()).length > 500
    ) {
      setError("请选择模型与音色，名称最多20字，试听文本最多500字。");
      return;
    }
    const op = {
      key: crypto.randomUUID(),
      modelVersion: model.model_version_id,
      input: {
        draft_intent_id: crypto.randomUUID(),
        release_id: releaseID,
        voice_key: voiceKey,
        name: name.trim(),
        text: text.trim(),
      },
    };
    operation.current = op;
    jobRef.current = null;
    setJob(null);
    await submit(op);
  }
  async function begin() {
    if (isActive(phaseRef.current)) return;
    const task = flight();
    change("options");
    setError("");
    try {
      const result = requireSubjectVoiceOptions(
        await listSubjectVoiceOptions({ signal: task.controller.signal }),
        actorID,
        scopeID,
      );
      if (!task.current()) return;
      optionRef.current = result;
      setOptions(result);
      if (!result.available) {
        change("unavailable");
        setError(subjectVoiceError(result.reason));
        return;
      }
      const model = result.models.length === 1 ? result.models[0] : null;
      if (model?.default_voice_key) {
        ++attempt.current;
        await generate(
          model.release_id,
          model.default_voice_key,
          `智能配音${attempt.current}`,
          sample,
        );
      } else {
        change("unavailable");
        setError("智能配音暂不可用，请稍后重试。");
      }
    } catch (reason) {
      if (task.current()) {
        change("unavailable");
        setError(
          reason instanceof RequestError
            ? subjectVoiceError(reason.code)
            : "暂时无法读取语音配置，请重试。",
        );
      }
    }
  }
  async function recover() {
    const op = operation.current;
    if (!op || !isActive(phaseRef.current)) return;
    if (jobRef.current) {
      await read(op);
      return;
    }
    const task = flight();
    change("submitting");
    setError("");
    try {
      const receipt = await getSubjectVoiceCommand(
        { key: op.key },
        { signal: task.controller.signal },
      );
      if (!task.current()) return;
      if (receipt.state === "recorded" && receipt.job) accept(receipt.job, op);
      else if (receipt.state === "unrecorded" && !receipt.job) {
        change("unrecorded");
        setError("暂未查到任务记录。可重试原请求，原意图和请求标识保持不变。");
      } else throw new RequestError("invalid_receipt");
    } catch (reason) {
      if (task.current()) {
        change("unknown");
        setError(
          reason instanceof RequestError
            ? subjectVoiceError(reason.code)
            : "暂时无法查证音色任务，请恢复读取。",
        );
      }
    }
  }
  async function retryOriginal() {
    if (
      (phaseRef.current === "unrecorded" ||
        (phaseRef.current === "import_pending" &&
          jobRef.current?.status === "ingesting" &&
          jobRef.current.error_code === "media_import_pending")) &&
      operation.current
    )
      await submit(operation.current);
  }
  function reset() {
    stop();
    operation.current = null;
    jobRef.current = null;
    optionRef.current = null;
    setJob(null);
    setOptions(null);
    setError("");
    change("idle");
  }
  return {
    phase,
    options,
    job,
    error,
    active: isActive(phase),
    begin,
    generate,
    recover,
    retryOriginal,
    reset,
  };
}
