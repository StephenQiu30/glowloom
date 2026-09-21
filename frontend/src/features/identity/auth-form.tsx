"use client";

import { AlertCircle, ArrowRight } from "lucide-react";
import { Spinner } from "@/components/ui/spinner";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState, useSyncExternalStore } from "react";

import { RegistrationForm } from "@/features/identity/registration-form";
import { BasicLayout } from "@/components/layout/basic-layout";
import { LayoutContainer } from "@/components/layout/layout-container";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { setAccessToken } from "@/lib/auth-session";
import { appApiErrorMessage } from "@/lib/server-state";
import { useLoginMutation } from "@/features/identity/endpoints";

type AuthMode = "login" | "register";

function subscribeToHydration(): () => void {
  return () => undefined;
}

function clientIsHydrated(): boolean {
  return true;
}

function serverIsHydrated(): boolean {
  return false;
}

export function AuthForm({ mode }: { mode: AuthMode }) {
  const router = useRouter();
  const [login, loginState] = useLoginMutation();
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const hydrated = useSyncExternalStore(subscribeToHydration, clientIsHydrated, serverIsHydrated);
  const isRegister = mode === "register";
  const submitting = loginState.isLoading;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setErrorMessage(null);
    const values = new FormData(event.currentTarget);
    const email = String(values.get("email") ?? "").trim();
    const password = String(values.get("password") ?? "");

    try {
      const response = await login({ email, password }).unwrap();
      setAccessToken(response.access_token);
      router.replace("/projects");
    } catch (error: unknown) {
      setErrorMessage(appApiErrorMessage(error));
    }
  }

  return (
    <BasicLayout
      authState="anonymous"
      artwork={
        <>
          <Image
            alt="她从画中来项目画面"
            className="object-cover opacity-70 grayscale"
            fill
            priority
            sizes="55vw"
            src="/assets/lanverse-studio/painting-girl-cover.png"
            unoptimized
          />
          <div className="absolute inset-0 bg-linear-to-t from-artwork via-artwork/10 to-transparent" />
          <div className="absolute right-12 bottom-12 left-12 pt-8 text-artwork-foreground">
            <p className="font-mono text-xs text-artwork-foreground/60">
              视觉概念示例 · 她从画中来
            </p>
            <blockquote className="mt-4 max-w-xl text-3xl leading-11 font-medium tracking-[-0.03em]">
              “从已确认的事实继续，而不是从头重来。”
            </blockquote>
            <div className="mt-6 flex gap-5 text-sm text-artwork-foreground/70">
              <span>16 集</span>
              <span>水墨幻想</span>
              <span>9:16</span>
            </div>
          </div>
        </>
      }
    >
      <LayoutContainer>
        <div className="mx-auto w-full max-w-md py-12">
          <p className="text-sm font-medium">AI 竖屏短剧生产系统</p>
          <h1 className="mt-5 text-4xl font-semibold tracking-[-0.045em]">
            {isRegister ? "创建账号" : "登录 Lanverse"}
          </h1>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">
            {isRegister
              ? "创建你的创作空间，开始第一部可追溯的 AI 漫剧。"
              : "从已确认的剧本、资产与分镜继续制作。"}
          </p>

          {isRegister ? (
            <RegistrationForm hydrated={hydrated} />
          ) : (
            <form onSubmit={handleSubmit}>
              <FieldGroup className="mt-8 grid gap-5">
                <Field data-disabled={!hydrated || submitting}>
                  <FieldLabel htmlFor="email">邮箱</FieldLabel>
                  <Input
                    autoComplete="email"
                    disabled={!hydrated || submitting}
                    id="email"
                    name="email"
                    placeholder="creator@example.com"
                    required
                    type="email"
                  />
                </Field>
                <Field data-disabled={!hydrated || submitting}>
                  <div className="flex items-center justify-between">
                    <FieldLabel htmlFor="password">密码</FieldLabel>
                    <span className="text-xs text-muted-foreground">使用你的账户密码</span>
                  </div>
                  <Input
                    autoComplete="current-password"
                    disabled={!hydrated || submitting}
                    id="password"
                    name="password"
                    placeholder="输入账户密码"
                    required
                    type="password"
                  />
                </Field>
                {errorMessage ? (
                  <Alert variant="destructive">
                    <AlertCircle aria-hidden="true" />
                    <AlertTitle>登录失败</AlertTitle>
                    <AlertDescription>{errorMessage}</AlertDescription>
                  </Alert>
                ) : null}
                <Button size="lg" disabled={!hydrated || submitting} type="submit">
                  {submitting ? <Spinner data-icon="inline-start" aria-hidden="true" /> : null}
                  登录
                  <ArrowRight data-icon="inline-start" aria-hidden="true" />
                </Button>
              </FieldGroup>
            </form>
          )}
          <p className="mt-6 text-center text-sm text-muted-foreground">
            {isRegister ? "已有账号？" : "还没有账号？"}
            <Link
              className="ml-1 font-medium text-foreground underline-offset-4 hover:underline"
              href={isRegister ? "/login" : "/register"}
            >
              {isRegister ? "直接登录" : "创建账号"}
            </Link>
          </p>
        </div>
      </LayoutContainer>
    </BasicLayout>
  );
}
