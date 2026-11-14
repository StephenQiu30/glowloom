"use client";
import { useEffect, useRef, useState } from "react";
import { Volume2, LocateFixed, SkipBack, SkipForward } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  cueAt,
  rangeForChapter,
  type NarrationManifest,
} from "@/narration/manifest";

const base = process.env.NEXT_PUBLIC_WORKSPACE_BASE_PATH ?? "";
export function NarrationPlayer({
  documentId,
  revision,
  chapters,
}: {
  documentId: string;
  revision: string;
  chapters: { id: string; text: string; blocks: string[] }[];
}) {
  const [manifest, setManifest] = useState<NarrationManifest | null>(null);
  const [segment, setSegment] = useState(0);
  const [follow, setFollow] = useState(false);
  const [error, setError] = useState("");
  const audio = useRef<HTMLAudioElement>(null);
  const highlighted = useRef<HTMLElement | null>(null);
  const userPlay = useRef(false);
  const restoredTime = useRef(0);
  const lastSaved = useRef(0);
  const [chapter, setChapter] = useState("");
  const chapterRange =
    manifest && chapter
      ? rangeForChapter(
          manifest,
          chapters.find((c) => c.id === chapter)?.blocks ?? [],
        )
      : null;
  const rate = useRef(1);
  useEffect(() => {
    const controller = new AbortController();
    fetch(`${base}/narration/index.json`, { signal: controller.signal })
      .then((r) => (r.ok ? r.json() : {}))
      .then(async (index: Record<string, string>) => {
        if (!index[documentId]) return;
        const response = await fetch(`${base}/narration/${index[documentId]}`, {
          signal: controller.signal,
        });
        if (!response.ok) throw new Error("朗读资源加载失败");
        const m = (await response.json()) as NarrationManifest;
        if (m.revision !== revision)
          throw new Error("音频与当前文档版本不匹配");
        try {
          const saved = JSON.parse(
            localStorage.getItem(
              `narration:${documentId}:${revision}:${m.scope}:${m.configDigest}`,
            ) ?? "null",
          );
          if (
            saved &&
            Number.isInteger(saved.segment) &&
            saved.segment >= 0 &&
            saved.segment < m.segments.length
          ) {
            setSegment(saved.segment);
            restoredTime.current = saved.seconds;
          }
        } catch {
          /* Local resume is optional. */
        }
        setManifest(m);
      })
      .catch((e: Error) => {
        if (e.name !== "AbortError") setError(e.message);
      });
    return () => {
      controller.abort();
      highlighted.current?.removeAttribute("data-speaking");
    };
  }, [documentId, revision]);
  useEffect(() => {
    const media = audio.current;
    return () => media?.pause();
  }, [manifest]);
  useEffect(() => {
    const yieldControl = () => setFollow(false);
    const keyboard = (e: KeyboardEvent) => {
      if (
        ["PageDown", "PageUp", "Home", "End", "ArrowDown", "ArrowUp"].includes(
          e.key,
        )
      )
        yieldControl();
    };
    const selection = () => {
      if (window.getSelection()?.toString()) yieldControl();
    };
    const pointer = (e: PointerEvent) => {
      if (
        !(e.target instanceof Element) ||
        !e.target.closest(".narration-panel")
      )
        yieldControl();
    };
    window.addEventListener("wheel", yieldControl, { passive: true });
    window.addEventListener("touchmove", yieldControl, { passive: true });
    window.addEventListener("keydown", keyboard);
    document.addEventListener("selectionchange", selection);
    document.addEventListener("pointerdown", pointer);
    return () => {
      window.removeEventListener("wheel", yieldControl);
      window.removeEventListener("touchmove", yieldControl);
      window.removeEventListener("keydown", keyboard);
      document.removeEventListener("selectionchange", selection);
      document.removeEventListener("pointerdown", pointer);
    };
  }, []);
  if (!manifest)
    return error ? (
      <p className="subtle-note" role="status">
        {error}
      </p>
    ) : null;
  const current = manifest.segments[segment];
  const locate = () => {
    if (
      chapterRange &&
      segment === chapterRange.toSegment &&
      (audio.current?.currentTime ?? 0) * 1000 >= chapterRange.toMs
    ) {
      audio.current?.pause();
      userPlay.current = false;
    }
    if (Date.now() - lastSaved.current > 1500) {
      try {
        localStorage.setItem(
          `narration:${documentId}:${revision}:${manifest.scope}:${manifest.configDigest}`,
          JSON.stringify({ segment, seconds: audio.current?.currentTime ?? 0 }),
        );
        lastSaved.current = Date.now();
      } catch {
        /* Storage may be unavailable. */
      }
    }
    const cue = cueAt(current, audio.current?.currentTime ?? 0);
    highlighted.current?.removeAttribute("data-speaking");
    const el = cue ? document.getElementById(cue.blockId) : null;
    highlighted.current = el;
    el?.setAttribute("data-speaking", "true");
    if (follow && el) {
      const rect = el.getBoundingClientRect();
      if (rect.top < 90 || rect.bottom > innerHeight - 60)
        el.scrollIntoView({
          block: "center",
          behavior: window.matchMedia("(prefers-reduced-motion: reduce)")
            .matches
            ? "instant"
            : "smooth",
        });
    }
  };
  const skip = (index: number, autoplay?: boolean) => {
    if (index < 0 || index >= manifest.segments.length) return;
    userPlay.current = autoplay ?? !audio.current?.paused;
    setSegment(index);
    highlighted.current?.removeAttribute("data-speaking");
  };
  return (
    <section
      className="narration-panel"
      aria-label="文档朗读"
      data-pagefind-ignore
    >
      <div className="narration-toolbar">
        <Volume2 size={14} />
        <strong>文档朗读</strong>
        <span className="muted">
          {segment + 1} / {manifest.segments.length}　
          {manifest.precision === "none" ? "无定位时序" : "段落定位"}
        </span>
      </div>
      <audio
        ref={audio}
        src={`${base}/narration/${documentId}/${current.src}`}
        controls
        preload="metadata"
        onTimeUpdate={locate}
        onSeeked={locate}
        onEnded={() => {
          if (!chapterRange || segment < chapterRange.toSegment)
            skip(segment + 1, true);
        }}
        onError={() => setError("音频加载失败，请稍后重试。")}
        onLoadedMetadata={() => {
          if (audio.current) audio.current.playbackRate = rate.current;
          if (restoredTime.current && audio.current) {
            audio.current.currentTime = Math.min(
              restoredTime.current,
              audio.current.duration || 0,
            );
            restoredTime.current = 0;
          }
          if (userPlay.current)
            void audio.current?.play().catch(() => setError("请手动开始播放"));
          locate();
        }}
      />
      <div className="narration-toolbar">
        <label>
          朗读范围{" "}
          <select
            aria-label="朗读范围"
            value={chapter}
            onChange={(e) => {
              const id = e.target.value;
              setChapter(id);
              const range = id
                ? rangeForChapter(
                    manifest,
                    chapters.find((c) => c.id === id)?.blocks ?? [],
                  )
                : null;
              const index = range?.fromSegment ?? 0;
              const seconds = (range?.fromMs ?? 0) / 1000;
              if (index === segment && audio.current) {
                audio.current.currentTime = seconds;
                locate();
              } else {
                restoredTime.current = seconds;
                skip(index);
              }
            }}
          >
            <option value="">全文</option>
            {chapters
              .filter((c) => rangeForChapter(manifest, c.blocks))
              .map((c) => (
                <option key={c.id} value={c.id}>
                  {c.text}
                </option>
              ))}
          </select>
        </label>
        <Button
          variant="ghost"
          size="sm"
          aria-label="上一片段"
          disabled={!segment}
          onClick={() => skip(segment - 1)}
        >
          <SkipBack />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          aria-label="下一片段"
          disabled={segment === manifest.segments.length - 1}
          onClick={() => skip(segment + 1)}
        >
          <SkipForward />
        </Button>
        <label>
          播放速度{" "}
          <select
            aria-label="播放速度"
            defaultValue="1"
            onChange={(e) => {
              rate.current = Number(e.target.value);
              if (audio.current) audio.current.playbackRate = rate.current;
            }}
          >
            {[0.75, 1, 1.25, 1.5, 2].map((n) => (
              <option value={n} key={n}>
                {n}×
              </option>
            ))}
          </select>
        </label>
        <label>
          <input
            type="checkbox"
            checked={follow}
            onChange={(e) => setFollow(e.target.checked)}
          />{" "}
          跟随阅读
        </label>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            setFollow(true);
            highlighted.current?.scrollIntoView({ block: "center" });
          }}
        >
          <LocateFixed data-icon="inline-start" />
          回到朗读位置
        </Button>
      </div>
      {error && (
        <p role="status" className="subtle-note">
          {error}
        </p>
      )}
    </section>
  );
}
