"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";

export function LogoutButton() {
  const router = useRouter();
  const [error, setError] = useState("");

  async function logout() {
    setError("");
    const csrfToken = readCookie("thief_csrf");
    if (!csrfToken) {
      setError("无法验证注销请求。");
      return;
    }
    const response = await fetch("/api/session", {
      method: "DELETE",
      headers: { "X-CSRF-Token": csrfToken },
    });
    if (!response.ok) {
      setError("注销失败，请重试。");
      return;
    }
    router.replace("/");
    router.refresh();
  }

  return (
    <div className="flex items-center gap-2">
      {error ? (
        <span className="text-xs text-destructive" role="alert">
          {error}
        </span>
      ) : null}
      <Button onClick={logout} size="sm" variant="outline">
        退出
      </Button>
    </div>
  );
}

function readCookie(name: string) {
  const prefix = `${name}=`;
  const value = document.cookie
    .split("; ")
    .find((item) => item.startsWith(prefix))
    ?.slice(prefix.length);
  return value ? decodeURIComponent(value) : "";
}
