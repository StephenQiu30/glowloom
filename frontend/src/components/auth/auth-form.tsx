"use client";

import { AlertCircle, ArrowRight, LoaderCircle } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState, useSyncExternalStore } from "react";

import { StudioBrand } from "@/components/studio/studio-brand";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { setAccessToken } from "@/lib/auth-session";
import {
  appApiErrorMessage,
  useLoginMutation,
  useRegisterMutation,
} from "@/lib/server-state";

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
  const [register, registerState] = useRegisterMutation();
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const hydrated = useSyncExternalStore(
    subscribeToHydration,
    clientIsHydrated,
    serverIsHydrated,
  );
  const isRegister = mode === "register";
  const submitting = loginState.isLoading || registerState.isLoading;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setErrorMessage(null);
    const values = new FormData(event.currentTarget);
    const email = String(values.get("email") ?? "").trim();
    const password = String(values.get("password") ?? "");

    try {
      const response = isRegister
        ? await register({
            display_name: String(values.get("displayName") ?? "").trim(),
            email,
            password,
          }).unwrap()
        : await login({ email, password }).unwrap();
      setAccessToken(response.access_token);
      router.replace("/projects");
    } catch (error: unknown) {
      setErrorMessage(appApiErrorMessage(error));
    }
  }

  return (
    <main className="grid min-h-screen bg-background lg:grid-cols-[minmax(420px,0.85fr)_1.15fr]">
      <section className="flex min-h-screen flex-col px-6 py-7 md:px-12 lg:px-16">
        <StudioBrand />
        <div className="my-auto w-full max-w-md self-center py-12">
          <p className="text-sm font-medium">AI 竖屏短剧生产系统</p>
          <h1 className="mt-5 text-4xl font-semibold tracking-[-0.045em]">{isRegister ? "创建账号" : "登录 Lanverse"}</h1>
          <p className="mt-3 text-sm leading-6 text-muted-foreground">{isRegister ? "创建你的创作空间，开始第一部可追溯的 AI 漫剧。" : "从已确认的剧本、资产与分镜继续制作。"}</p>

          <form className="mt-8 grid gap-5" onSubmit={handleSubmit}>
            {isRegister ? <div className="grid gap-2"><Label htmlFor="displayName">显示名称</Label><Input autoComplete="name" disabled={!hydrated || submitting} id="displayName" name="displayName" placeholder="你的创作署名" required /></div> : null}
            <div className="grid gap-2"><Label htmlFor="email">邮箱</Label><Input autoComplete="email" disabled={!hydrated || submitting} id="email" name="email" placeholder="creator@example.com" type="email" required /></div>
            <div className="grid gap-2"><div className="flex items-center justify-between"><Label htmlFor="password">密码</Label>{!isRegister ? <span className="text-xs text-slate-400">使用你的账户密码</span> : null}</div><Input autoComplete={isRegister ? "new-password" : "current-password"} disabled={!hydrated || submitting} id="password" minLength={isRegister ? 12 : undefined} name="password" placeholder="输入账户密码" type="password" required />{isRegister ? <p className="text-xs text-slate-400">至少 12 个字符，建议包含数字与符号。</p> : null}</div>
            {isRegister ? <label className="flex items-start gap-2 text-sm leading-5 text-muted-foreground"><input className="mt-1 accent-black" defaultChecked disabled={!hydrated || submitting} required type="checkbox" />我已阅读并同意服务协议与隐私政策</label> : null}
            {errorMessage ? <div className="flex items-start gap-2 border-y border-destructive/25 bg-destructive/5 px-3.5 py-3 text-sm text-destructive" role="alert"><AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" /><span>{errorMessage}</span></div> : null}
            <Button className="mt-1 h-11" disabled={!hydrated || submitting} type="submit">{submitting ? <LoaderCircle className="animate-spin" aria-hidden="true" /> : null}{isRegister ? "注册并开始创作" : "登录"}<ArrowRight aria-hidden="true" /></Button>
          </form>
          <p className="mt-6 text-center text-sm text-muted-foreground">{isRegister ? "已有账号？" : "还没有账号？"}<Link className="ml-1 font-medium text-foreground underline-offset-4 hover:underline" href={isRegister ? "/login" : "/register"}>{isRegister ? "直接登录" : "创建账号"}</Link></p>
        </div>
        <p className="text-xs text-slate-400">© 2026 Lanverse · 安全创作环境</p>
      </section>

      <aside className="relative hidden min-h-screen overflow-hidden bg-black lg:block">
        <Image
          alt="她从画中来项目画面"
          className="object-cover opacity-70 grayscale"
          fill
          priority
          sizes="55vw"
          src="/assets/lanverse-studio/painting-girl-cover.png"
          unoptimized
        />
        <div className="absolute inset-0 bg-linear-to-t from-black via-black/10 to-transparent" />
        <div className="absolute right-12 bottom-12 left-12 border-t border-white/30 pt-8 text-white">
          <p className="font-mono text-xs text-white/60">正在制作 · 她从画中来</p>
          <blockquote className="mt-4 max-w-xl text-3xl leading-11 font-medium tracking-[-0.03em]">“从已确认的事实继续，而不是从头重来。”</blockquote>
          <div className="mt-6 flex gap-5 text-sm text-white/70"><span>16 集</span><span>水墨幻想</span><span>9:16</span></div>
        </div>
      </aside>
    </main>
  );
}
