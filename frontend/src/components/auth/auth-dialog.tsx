"use client";

import { useRef, useState, type RefObject } from "react";
import Image from "next/image";
import { BrandMark } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AuthView } from "./auth-view";

export type AuthMode = "login" | "register";

type AuthDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAuthenticated: (session: API.EmailSessionView) => void;
  initialMode?: AuthMode;
  fallbackFocusRef?: RefObject<HTMLElement | null>;
};

export function AuthDialog(props: AuthDialogProps) {
  return props.open ? <OpenAuthDialog {...props} /> : null;
}

function OpenAuthDialog({
  open,
  onOpenChange,
  onAuthenticated,
  initialMode = "login",
  fallbackFocusRef,
}: AuthDialogProps) {
  const [mode, setMode] = useState<AuthMode>(initialMode);
  const [email, setEmail] = useState("");
  const [pending, setPending] = useState(false);
  const content = useRef<HTMLDivElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  function changeMode(next: AuthMode, nextEmail = email) {
    if (pending) return;
    setEmail(nextEmail);
    setMode(next);
    requestAnimationFrame(() => {
      content.current?.querySelector<HTMLInputElement>("input")?.focus();
    });
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        aria-describedby={undefined}
        ref={content}
        className="auth-dialog"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          returnFocus.current =
            document.activeElement instanceof HTMLElement
              ? document.activeElement
              : null;
          content.current?.querySelector<HTMLInputElement>("input")?.focus();
        }}
        onCloseAutoFocus={(event) => {
          const target =
            returnFocus.current?.isConnected &&
            returnFocus.current !== document.body
              ? returnFocus.current
              : fallbackFocusRef?.current;
          if (target?.isConnected) {
            event.preventDefault();
            target.focus();
          }
        }}
      >
        <div className="auth-dialog-visual" aria-hidden="true">
          <Image
            src="/design/reference/alpine-road.png"
            alt=""
            fill
            sizes="(min-width: 960px) 1210px, 0px"
          />
        </div>
        <div className="auth-dialog-panel">
          <div className="auth-dialog-brand" aria-hidden="true">
            <BrandMark />
            <span>织光</span>
          </div>
          <DialogHeader>
            <DialogTitle size="auth">
              {mode === "login" ? "欢迎登录织光" : "创建织光账号"}
            </DialogTitle>
          </DialogHeader>
          <div className="auth-dialog-body">
            <AuthView
              key={mode}
              embedded
              mode={mode}
              initialEmail={email}
              onEmailChange={setEmail}
              onLoginRequested={(value) => changeMode("login", value)}
              onPendingChange={setPending}
              onAuthenticated={onAuthenticated}
            />
          </div>
          <div className="auth-dialog-switch">
            <span>{mode === "login" ? "还没有账号？" : "已有账号？"}</span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={pending}
              onClick={() =>
                changeMode(mode === "login" ? "register" : "login")
              }
            >
              {mode === "login" ? "创建账号" : "登录"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
