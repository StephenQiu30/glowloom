"use client";
import { useState } from "react";
import Link from "next/link";
import {
  UserRound,
  ShieldCheck,
  SlidersHorizontal,
  Upload,
} from "lucide-react";
import { ProductShell } from "@/components/layout/product-shell";
import { useSession } from "@/components/auth/session";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ThemePreference } from "@/components/controls/theme-controls";
import { UnavailableDialog } from "@/components/availability/unavailable";
function Settings() {
  const session = useSession();
  const [tab, setTab] = useState("profile");
  const [feature, setFeature] = useState<string | null>(null);
  return (
    <>
      <h1 className="text-2xl">账号设置</h1>
      <div className="settings-layout">
        <nav className="settings-nav" aria-label="账号设置">
          {[
            { id: "profile", label: "个人资料", icon: UserRound },
            { id: "security", label: "密码与会话", icon: ShieldCheck },
            { id: "preferences", label: "创作偏好", icon: SlidersHorizontal },
          ].map(({ id, label, icon: Icon }) => (
            <Button
              key={id}
              variant="ghost"
              aria-current={id === tab ? "page" : undefined}
              onClick={() => setTab(id)}
            >
              <Icon />
              {label}
            </Button>
          ))}
        </nav>
        <section className="settings-card">
          {tab === "profile" ? (
            <>
              <h2>个人资料</h2>
              <p>管理你的头像与创作者名称。</p>
              <div className="mt-6 flex items-center gap-4">
                <span className="grid size-14 place-items-center rounded-full bg-accent text-xl">
                  {Array.from(session.actor.display_name)[0]}
                </span>
                <Button
                  variant="outline"
                  onClick={() => setFeature("头像更新")}
                >
                  <Upload />
                  更换头像
                </Button>
              </div>
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  setFeature("资料修改");
                }}
              >
                <label className="form-label">
                  显示名
                  <Input value={session.actor.display_name} readOnly />
                </label>
                <label className="form-label">
                  登录名
                  <Input value={session.actor.login_name} readOnly />
                </label>
                <p>登录名创建后不可修改。资料修改暂未开放。</p>
              </form>
            </>
          ) : tab === "security" ? (
            <>
              <h2>密码与会话</h2>
              <p>保护你的账号与创作内容。</p>
              <div className="my-8 flex items-center justify-between border-b pb-6">
                <div>
                  <h3>登录密码</h3>
                  <p>修改密码会退出其他设备的会话。</p>
                </div>
                <Button variant="outline" asChild>
                  <Link href="/reset-password">修改密码</Link>
                </Button>
              </div>
              <h3 className="mb-4">当前设备</h3>
              <div className="rounded-lg border p-4">
                <p>当前会话 · {session.persistent ? "保持登录" : "临时登录"}</p>
                <p>
                  最近活跃：
                  {new Date(session.last_active_at).toLocaleString("zh-CN")}
                </p>
                <p>
                  会话到期：
                  {new Date(session.absolute_expires_at).toLocaleString(
                    "zh-CN",
                  )}
                </p>
              </div>
              <p className="mt-5">其他设备会话管理暂未开放。</p>
            </>
          ) : (
            <>
              <h2>创作偏好</h2>
              <p>外观设置保存在此浏览器中。</p>
              <div className="mt-8">
                <h3 className="mb-4">外观</h3>
                <ThemePreference />
              </div>
            </>
          )}
        </section>
      </div>
      <UnavailableDialog feature={feature} onClose={() => setFeature(null)} />
    </>
  );
}
export function AccountPage() {
  return (
    <ProductShell screen="account">
      <Settings />
    </ProductShell>
  );
}
