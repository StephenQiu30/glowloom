"use client";

import { useRef, useState, type RefObject } from "react";
import { X } from "lucide-react";
import { BrandMark } from "@/components/brand/brand";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AuthView } from "./auth-view";

export type AuthMode = "login" | "register";

export function AuthDialog({
  open,
  onOpenChange,
  onAuthenticated,
  initialMode = "login",
  fallbackFocusRef,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onAuthenticated: (session: API.EmailSessionView) => void;
  initialMode?: AuthMode;
  fallbackFocusRef?: RefObject<HTMLElement | null>;
}) {
  const [mode, setMode] = useState<AuthMode>(initialMode);
  const [pending, setPending] = useState(false);
  const content = useRef<HTMLDivElement>(null);
  const returnFocus = useRef<HTMLElement | null>(null);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        ref={content}
        className="auth-dialog"
        showCloseButton={false}
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
        <div className="auth-brand-row">
          <BrandMark />
          <span>织光</span>
          <Button
            variant="ghost"
            size="icon"
            aria-label="关闭登录注册弹窗"
            onClick={() => onOpenChange(false)}
          >
            <X data-icon="inline-start" />
          </Button>
        </div>
        <DialogHeader>
          <DialogTitle>
            {mode === "login" ? "登录织光" : "创建织光账号"}
          </DialogTitle>
          <DialogDescription>
            自由探索灵感，登录后保存你的创作。
          </DialogDescription>
        </DialogHeader>
        <Tabs
          value={mode}
          onValueChange={(value) => {
            if (!pending && (value === "login" || value === "register"))
              setMode(value);
          }}
          className="auth-tabs"
        >
          <TabsList className="w-full" aria-label="登录或注册">
            <TabsTrigger value="login" disabled={pending}>
              登录
            </TabsTrigger>
            <TabsTrigger value="register" disabled={pending}>
              注册
            </TabsTrigger>
          </TabsList>
          {(["login", "register"] as const).map((value) => (
            <TabsContent key={value} value={value}>
              <AuthView
                embedded
                mode={value}
                onPendingChange={setPending}
                onAuthenticated={onAuthenticated}
                onContinueGuest={() => onOpenChange(false)}
              />
            </TabsContent>
          ))}
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
