"use client";

import dynamic from "next/dynamic";
import { useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
const SpeechComposer = dynamic(() =>
  import("./rich-generation").then((m) => m.RichGeneration),
);

/** Audio and video retain separate durable drafts, quotes and explicit confirmations. */
export function AvatarTextDrive({
  projectID,
  contextKey,
  disabled,
  onAdopt,
}: {
  projectID: string;
  contextKey: string;
  disabled: boolean;
  onAdopt: (id: string) => Promise<void>;
}) {
  const [open, setOpen] = useState(false),
    [draftID, setDraftID] = useState<string>(),
    [locked, setLocked] = useState(false),
    [closing, setClosing] = useState(false),
    [error, setError] = useState("");
  const guard = useRef<(() => Promise<boolean>) | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const key = `glowloom:avatar-speech:${contextKey}`;
  function show() {
    setError("");
    try {
      const saved = localStorage.getItem(key);
      setDraftID(saved && /^[0-9a-f-]{36}$/i.test(saved) ? saved : undefined);
    } catch {
      setError("本机暂时无法记录草稿位置，配音仍会保存到项目。");
    }
    setOpen(true);
  }
  async function close() {
    if (locked || closing) return;
    setClosing(true);
    try {
      if (!guard.current || (await guard.current())) setOpen(false);
      else setError("配音草稿尚未保存，请重试。");
    } finally {
      setClosing(false);
    }
  }
  return (
    <>
      <Button
        ref={trigger}
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={show}
      >
        文本驱动
      </Button>
      {open && (
        <Dialog
          open
          onOpenChange={(value) => {
            if (!value) void close();
          }}
        >
          <DialogContent
            className="max-h-[88dvh] overflow-y-auto sm:max-w-3xl"
            showCloseButton={!locked && !closing}
            onInteractOutside={(e) => e.preventDefault()}
            onCloseAutoFocus={(event) => {
              event.preventDefault();
              trigger.current?.focus();
            }}
          >
            <DialogTitle>文本驱动 · 制作配音</DialogTitle>
            <DialogDescription>
              输入台词、选择音色并生成配音。试听后使用生成结果，再确认数字人视频费用。
            </DialogDescription>
            {error && <p role="alert">{error}</p>}
            <SpeechComposer
              adoptLabel="使用这段配音"
              projectID={projectID}
              seed={{ creationType: "audio", capability: "audio", prompt: "" }}
              draftID={draftID}
              onLock={setLocked}
              onCloseGuard={(value) => {
                guard.current = value;
              }}
              onDraftReady={(id) => {
                try {
                  localStorage.setItem(key, id);
                } catch {
                  setError("配音已保存，本机无法记录恢复位置。");
                }
              }}
              onAdopt={async (id, kind) => {
                if (kind !== "audio") throw new Error("请选择配音的音频结果。");
                if (guard.current && !(await guard.current()))
                  throw new Error("请先保存当前配音草稿。");
                await onAdopt(id);
                setOpen(false);
              }}
            />
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
