"use client";

import { ArrowRight, Check, Sparkles } from "lucide-react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";

import { StudioBrand } from "@/components/studio/studio-shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type AuthMode = "login" | "register";

export function AuthForm({ mode }: { mode: AuthMode }) {
  const router = useRouter();
  const [submitting, setSubmitting] = useState(false);
  const isRegister = mode === "register";

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    window.setTimeout(() => router.push("/"), 450);
  }

  return (
    <main className="grid min-h-screen bg-[#fbfcfd] lg:grid-cols-[minmax(420px,0.85fr)_1.15fr]">
      <section className="flex min-h-screen flex-col px-6 py-7 md:px-12 lg:px-16">
        <StudioBrand />
        <div className="my-auto w-full max-w-md self-center py-12">
          <div className="inline-flex items-center gap-2 rounded-full bg-cyan-50 px-3 py-1.5 text-xs font-medium text-[#087f91]"><Sparkles className="size-3.5" aria-hidden="true" />专为 AI 漫剧创作者设计</div>
          <h1 className="mt-5 text-3xl font-semibold tracking-[-0.035em]">{isRegister ? "创建账号" : "登录 Lanverse"}</h1>
          <p className="mt-2 text-sm leading-6 text-slate-500">{isRegister ? "创建你的创作空间，开始第一部 AI 漫剧。" : "回到你的剧本、资产与分镜工作台。"}</p>

          <form className="mt-8 grid gap-5" onSubmit={handleSubmit}>
            {isRegister ? <div className="grid gap-2"><Label htmlFor="displayName">显示名称</Label><Input id="displayName" name="displayName" placeholder="你的创作署名" required /></div> : null}
            <div className="grid gap-2"><Label htmlFor="email">邮箱</Label><Input id="email" name="email" defaultValue="creator@lanverse.ai" type="email" required /></div>
            <div className="grid gap-2"><div className="flex items-center justify-between"><Label htmlFor="password">密码</Label>{!isRegister ? <button className="text-xs text-[#078fa5]" type="button">忘记密码？</button> : null}</div><Input id="password" name="password" defaultValue="lanverse-demo" type="password" required />{isRegister ? <p className="text-xs text-slate-400">至少 12 个字符，建议包含数字与符号。</p> : null}</div>
            {isRegister ? <label className="flex items-start gap-2 text-sm leading-5 text-slate-500"><input className="mt-1 accent-[#079db3]" defaultChecked type="checkbox" />我已阅读并同意服务协议与隐私政策</label> : null}
            <Button className="mt-1 h-11 bg-[#079db3] text-white hover:bg-[#078da0]" disabled={submitting} type="submit">{submitting ? <Check aria-hidden="true" /> : null}{isRegister ? "注册并开始创作" : "登录"}<ArrowRight aria-hidden="true" /></Button>
          </form>
          <p className="mt-6 text-center text-sm text-slate-500">{isRegister ? "已有账号？" : "还没有账号？"}<Link className="ml-1 font-medium text-[#078fa5] hover:underline" href={isRegister ? "/login" : "/register"}>{isRegister ? "直接登录" : "创建账号"}</Link></p>
        </div>
        <p className="text-xs text-slate-400">© 2026 Lanverse · Mock 创作环境</p>
      </section>

      <aside className="relative hidden min-h-screen overflow-hidden bg-slate-900 lg:block">
        <Image alt="她从画中来项目画面" fill priority sizes="55vw" src="/assets/lanverse-studio/painting-girl-cover.png" className="object-cover opacity-80" />
        <div className="absolute inset-0 bg-slate-950/20" />
        <div className="absolute right-12 bottom-12 left-12 rounded-3xl border border-white/20 bg-white/10 p-8 text-white backdrop-blur-xl">
          <p className="text-sm text-white/65">正在制作 · 她从画中来</p>
          <blockquote className="mt-4 max-w-xl text-2xl leading-10 font-medium">“角色一旦被锁定，每一个镜头都能延续同一种灵魂。”</blockquote>
          <div className="mt-6 flex gap-5 text-sm text-white/70"><span>16 集</span><span>水墨幻想</span><span>9:16</span></div>
        </div>
      </aside>
    </main>
  );
}
