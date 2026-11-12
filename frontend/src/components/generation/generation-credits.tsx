"use client";

import { useEffect, useMemo, useState } from "react";
import { CircleHelp, LoaderCircle, Sparkle } from "lucide-react";
import { InputGroupButton } from "@/components/ui/input-group";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { estimateGeneration } from "@/api/operations";
import { RequestError } from "@/lib/request";
import { draftRequest } from "./draft-save";
import type { PromptDraft } from "./prompt-document";

type Estimate = API.GenerationEstimate & {
  credits: string;
  kind: "estimate" | "maximum";
};
type CreditPreview =
  | { status: "ready"; estimate: Estimate }
  | { status: "loading" | "unavailable" | "incomplete" | "error" };

const units: Record<string, string> = {
  per_image: "张",
  per_second: "秒",
  per_request: "次",
  per_1k_tokens: "千 Token",
  per_1k_chars: "千字符",
};

export function useGenerationCredits({
  projectID,
  draft,
  enabled,
  regenerate,
}: {
  projectID: string;
  draft: PromptDraft;
  enabled: boolean;
  regenerate: boolean;
}): CreditPreview {
  const payload = useMemo(
    () =>
      enabled
        ? { draft: draftRequest(draft), force_regenerate: regenerate }
        : null,
    [draft, enabled, regenerate],
  );
  const [response, setResponse] = useState<{
    projectID: string;
    payload: API.GenerationEstimateRequest;
    value: CreditPreview;
  } | null>(null);

  useEffect(() => {
    if (payload === null) return;
    const controller = new AbortController();
    function publish(value: CreditPreview) {
      if (!controller.signal.aborted && payload !== null)
        setResponse({ projectID, payload, value });
    }
    const timer = setTimeout(() => {
      void estimateGeneration({ pid: projectID }, payload, {
        signal: controller.signal,
      }).then(
        (estimate) => {
          if (
            typeof estimate?.credits !== "string" ||
            !/^(0|[1-9]\d*)(\.\d{1,5})?$/.test(estimate.credits) ||
            (estimate.kind !== "estimate" && estimate.kind !== "maximum")
          ) {
            publish({ status: "error" });
            return;
          }
          publish({
            status: "ready",
            estimate: {
              ...estimate,
              credits: estimate.credits,
              kind: estimate.kind,
            },
          });
        },
        (error: unknown) => {
          const code = error instanceof RequestError ? error.code : "";
          publish({
            status:
              code === "commercial_price_unavailable" ||
              code === "model_unavailable"
                ? "unavailable"
                : code === "input_not_ready" || code === "reference_unavailable"
                  ? "incomplete"
                  : "error",
          });
        },
      );
    }, 450);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [payload, projectID]);

  if (payload === null) return { status: "incomplete" };
  return response?.projectID === projectID && response.payload === payload
    ? response.value
    : { status: "loading" };
}

export function GenerationCredits({ preview }: { preview: CreditPreview }) {
  if (preview.status === "loading")
    return (
      <span
        role="status"
        aria-label="正在估算积分"
        className="grid size-(--toolbar-control-size) place-items-center max-md:size-11"
      >
        <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
      </span>
    );

  const estimate = preview.status === "ready" ? preview.estimate : null;
  const description = estimate
    ? estimate.reused
      ? "复用已有结果 · 0 积分"
      : `${estimate.kind === "maximum" ? "最多占用" : "预计消耗"} ${estimate.credits} 积分`
    : preview.status === "unavailable"
      ? "暂无可用价目"
      : preview.status === "incomplete"
        ? "补全生成输入后估算"
        : "暂时无法估算";
  const basis = estimate?.basis && units[estimate.basis];
  return (
    <Popover>
      <PopoverTrigger asChild>
        <InputGroupButton
          variant="ghost"
          size="toolbar"
          aria-label={description}
          className="shrink-0 gap-1 px-1.5 text-muted-foreground tabular-nums max-md:h-11"
        >
          {estimate ? (
            <>
              <Sparkle aria-hidden="true" className="size-3.5" />
              <span>
                {estimate.kind === "maximum" && "≤"}
                {estimate.credits}
              </span>
            </>
          ) : (
            <CircleHelp aria-hidden="true" className="size-4" />
          )}
        </InputGroupButton>
      </PopoverTrigger>
      <PopoverContent
        side="top"
        align="end"
        sideOffset={8}
        data-canvas-no-zoom
        data-canvas-shortcuts-ignore
        className="w-fit max-w-[calc(100vw-32px)] gap-1 px-3 py-2 text-xs"
      >
        <span>{description}</span>
        {estimate?.quantity && basis && !estimate.reused && (
          <span className="text-muted-foreground">
            计量 {estimate.quantity} {basis}
          </span>
        )}
      </PopoverContent>
    </Popover>
  );
}
