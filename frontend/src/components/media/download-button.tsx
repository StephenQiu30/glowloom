"use client";
import { FeedbackToast } from "@/components/feedback/feedback-toast";

import { useRef, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";

/** The caller supplies a generated API function; this component only saves its result. */
export function DownloadButton({
  load,
  filename,
  children,
}: {
  load: () => Promise<Blob>;
  filename: string;
  children: ReactNode;
}) {
  const busy = useRef(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  async function download() {
    if (busy.current) return;
    busy.current = true;
    setPending(true);
    setError("");
    try {
      const blob = await load();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.append(link);
      try {
        link.click();
      } finally {
        link.remove();
        setTimeout(() => URL.revokeObjectURL(url), 0);
      }
    } catch {
      setError("下载失败，请重试。");
    } finally {
      busy.current = false;
      setPending(false);
    }
  }
  return (
    <span className="inline-flex flex-col gap-1">
      <Button
        variant="outline"
        disabled={pending}
        onClick={() => void download()}
      >
        {pending ? "正在下载…" : children}
      </Button>
      {error && <FeedbackToast message={error} />}
    </span>
  );
}
