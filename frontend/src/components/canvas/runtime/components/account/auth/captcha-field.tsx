"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import Image from "next/image";
import { Keyboard, RefreshCw } from "lucide-react";
import * as api from "@/components/canvas/runtime/api/accountCaptchaController";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription } from "@/components/ui/field";
import { Slider } from "@/components/ui/slider";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Empty,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
} from "@/components/ui/empty";
import { Spinner } from "@/components/ui/spinner";
import { Separator } from "@/components/ui/separator";
import { Kbd } from "@/components/ui/kbd";
import { AccountError } from "../account-fields";

type CaptchaProps = {
  scene: "LOGIN" | "REGISTER";
  disabled: boolean;
  onVerified: (ticket: CanvasContract.CaptchaTicketVO) => Promise<void>;
};

type Phase =
  "loading" | "ready" | "verifying" | "complete" | "expired" | "error";

function isChallenge(
  value: CanvasContract.CaptchaChallengeVO,
): value is Required<CanvasContract.CaptchaChallengeVO> {
  return Boolean(
    value.challengeId &&
    value.backgroundImage &&
    value.templateImage &&
    typeof value.backgroundWidth === "number" &&
    value.backgroundWidth > 0 &&
    typeof value.backgroundHeight === "number" &&
    value.backgroundHeight > 0 &&
    typeof value.templateWidth === "number" &&
    value.templateWidth > 0 &&
    typeof value.templateHeight === "number" &&
    value.templateHeight > 0 &&
    value.templateWidth < value.backgroundWidth &&
    value.expiresAt &&
    Number.isFinite(Number(value.expiresAt)) &&
    Number(value.expiresAt) > Date.now(),
  );
}

/** 一次明确提交意图的验证步骤；卸载／换题使旧请求失效。 */
export function CaptchaField({ scene, disabled, onVerified }: CaptchaProps) {
  const description = useId();
  const controller = useRef<AbortController | undefined>(undefined);
  const slider = useRef<HTMLDivElement>(null);
  const inputMethod = useRef("keyboard");
  const inFlight = useRef(false);
  const [challenge, setChallenge] =
    useState<Required<CanvasContract.CaptchaChallengeVO>>();
  const [position, setPosition] = useState(0);
  const [phase, setPhase] = useState<Phase>("loading");
  const [keyboardHelp, setKeyboardHelp] = useState(false);
  const [error, setError] = useState<Error>();

  const load = useCallback(async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    controller.current?.abort();
    const request = new AbortController();
    controller.current = request;
    setChallenge(undefined);
    setPhase("loading");
    setError(undefined);
    try {
      const result = unwrap(
        await api.captchaChallenge({ scene }, { signal: request.signal }),
      );
      if (request.signal.aborted) return;
      if (!isChallenge(result)) {
        throw new Error("验证服务返回无效数据，请重试");
      }
      setPosition(0);
      setChallenge(result);
      setPhase("ready");
    } catch (cause) {
      if (!request.signal.aborted) {
        setPhase("error");
        setError(
          cause instanceof Error ? cause : new Error("验证服务连接失败"),
        );
      }
    } finally {
      if (!request.signal.aborted) inFlight.current = false;
    }
  }, [scene]);

  useEffect(() => {
    const start = setTimeout(() => void load(), 0);
    return () => {
      clearTimeout(start);
      controller.current?.abort();
      inFlight.current = false;
    };
  }, [load]);

  useEffect(() => {
    if (!challenge?.expiresAt || phase === "complete") return;
    const timeout = setTimeout(
      () => {
        controller.current?.abort();
        inFlight.current = false;
        setChallenge(undefined);
        setPhase("expired");
      },
      Math.max(0, Number(challenge.expiresAt) - Date.now()),
    );
    return () => clearTimeout(timeout);
  }, [challenge?.expiresAt, phase]);

  async function verify(value: number) {
    if (inFlight.current || disabled || phase !== "ready" || !challenge) return;
    inFlight.current = true;
    controller.current?.abort();
    const request = new AbortController();
    controller.current = request;
    setPhase("verifying");
    setError(undefined);
    try {
      const ticket = unwrap(
        await api.captchaVerify(
          {
            scene,
            challengeId: challenge.challengeId,
            position:
              (value / 100) *
              (1 - challenge.templateWidth / challenge.backgroundWidth),
          },
          { signal: request.signal },
        ),
      );
      if (request.signal.aborted) return;
      if (
        !ticket.captchaToken ||
        !Number.isFinite(Number(ticket.expiresAt)) ||
        Number(ticket.expiresAt) <= Date.now()
      ) {
        throw new Error("验证凭据已失效，请重新验证");
      }
      setPhase("complete");
      await onVerified(ticket);
    } catch (cause) {
      if (!request.signal.aborted) {
        setChallenge(undefined);
        setPhase("error");
        setError(
          cause instanceof Error ? cause : new Error("验证服务连接失败"),
        );
      }
    } finally {
      if (!request.signal.aborted) inFlight.current = false;
    }
  }

  const busy =
    phase === "loading" ||
    phase === "verifying" ||
    phase === "complete" ||
    disabled;
  const ratio = challenge
    ? challenge.templateWidth / challenge.backgroundWidth
    : 0;
  const continuing = scene === "LOGIN" ? "登录" : "注册";
  const status =
    phase === "loading"
      ? "正在加载拼图…"
      : phase === "verifying"
        ? "正在验证…"
        : phase === "complete"
          ? `验证成功，正在${continuing}…`
          : phase === "expired"
            ? "拼图已过期，请换一张后重试。"
            : phase === "error"
              ? "请换一张后重试。"
              : `松开后自动验证，成功后继续${continuing}。`;

  return (
    <Field>
      <AccountError error={error} />
      {challenge ? (
        <div
          className="relative overflow-hidden rounded-md"
          style={{
            aspectRatio: `${challenge.backgroundWidth}/${challenge.backgroundHeight}`,
          }}
        >
          <Image
            src={challenge.backgroundImage}
            alt="拼图背景，请将滑块移到缺口处"
            fill
            unoptimized
            sizes="100vw"
          />
          <Image
            src={challenge.templateImage}
            alt=""
            unoptimized
            width={challenge.templateWidth}
            height={challenge.templateHeight}
            className="absolute top-0"
            style={{
              width: `${ratio * 100}%`,
              height: "100%",
              left: `${position * (1 - ratio)}%`,
            }}
          />
        </div>
      ) : phase === "loading" ? (
        <Skeleton className="aspect-video w-full" />
      ) : (
        <Empty className="aspect-video">
          <EmptyHeader>
            <EmptyTitle>请重新获取拼图</EmptyTitle>
            <EmptyDescription>可换一张重试，或返回修改输入。</EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      <div className="relative" ref={slider}>
        <Slider
          variant="verification"
          value={[position]}
          min={0}
          max={100}
          step={0.25}
          aria-label="向右拖动滑块完成拼图"
          aria-describedby={description}
          aria-valuetext={`${Math.round(position)}%`}
          disabled={busy || !challenge}
          onPointerDownCapture={() => {
            inputMethod.current = "pointer";
          }}
          onKeyDownCapture={(event) => {
            inputMethod.current = "keyboard";
            if (event.key === "Enter") {
              event.preventDefault();
              void verify(position);
            }
          }}
          onValueChange={([value]) => setPosition(value)}
          onValueCommit={([value]) => {
            if (inputMethod.current === "pointer") void verify(value);
          }}
        />
        <span
          className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-muted-foreground"
          aria-hidden="true"
        >
          {busy ? <Spinner /> : "向右拖动滑块"}
        </span>
      </div>
      <FieldDescription
        id={description}
        role="status"
        aria-live="polite"
        className="text-center"
      >
        {status}
      </FieldDescription>
      {keyboardHelp && (
        <FieldDescription className="text-center">
          <Kbd>←</Kbd> <Kbd>→</Kbd> 移动，<Kbd>Enter</Kbd> 确认
        </FieldDescription>
      )}
      <Separator />
      <div className="flex items-center justify-between gap-2">
        <Button
          type="button"
          variant="ghost"
          disabled={busy}
          onClick={() => void load()}
        >
          <RefreshCw aria-hidden="true" data-icon="inline-start" />
          {phase === "error" ? "重新加载" : "换一张"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          disabled={busy || !challenge}
          onClick={() => {
            setKeyboardHelp(true);
            slider.current
              ?.querySelector<HTMLElement>('[role="slider"]')
              ?.focus();
          }}
        >
          <Keyboard aria-hidden="true" data-icon="inline-start" />
          键盘操作
        </Button>
      </div>
    </Field>
  );
}
