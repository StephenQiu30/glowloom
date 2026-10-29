"use client";

import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";

function timestamp(value: number) {
  const ms = Math.max(0, Math.floor(value));
  return `${String(Math.floor(ms / 3600000)).padStart(2, "0")}:${String(Math.floor(ms / 60000) % 60).padStart(2, "0")}:${String(Math.floor(ms / 1000) % 60).padStart(2, "0")},${String(ms % 1000).padStart(3, "0")}`;
}

export function subtitleSRT(subtitle: API.AudioSubtitle) {
  return (subtitle.sentences ?? [])
    .map(
      (sentence, index) =>
        `${index + 1}\n${timestamp(sentence.start_time ?? 0)} --> ${timestamp(sentence.end_time ?? 0)}\n${sentence.text ?? ""}`,
    )
    .join("\n\n");
}

/** The server returns reviewed, persisted output subtitles after completion. */
export function AudioSubtitles({ subtitle }: { subtitle: API.AudioSubtitle }) {
  function download() {
    const url = URL.createObjectURL(
      new Blob([subtitleSRT(subtitle)], {
        type: "application/x-subrip;charset=utf-8",
      }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = "配音字幕.srt";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <Collapsible className="space-y-2">
      <div className="flex items-center gap-2">
        <CollapsibleTrigger asChild>
          <Button variant="ghost">查看字幕</Button>
        </CollapsibleTrigger>
        <Button
          variant="ghost"
          onClick={download}
          disabled={!subtitle.sentences?.length}
        >
          下载字幕
        </Button>
      </div>
      <CollapsibleContent className="max-h-72 overflow-auto rounded-lg border p-3">
        {(subtitle.sentences ?? []).map((sentence, index) => (
          <div key={index} className="mb-3 space-y-1">
            <p className="text-xs text-muted-foreground">
              {timestamp(sentence.start_time ?? 0)} —{" "}
              {timestamp(sentence.end_time ?? 0)}
            </p>
            <p className="text-sm whitespace-pre-wrap">{sentence.text}</p>
            {!!sentence.words?.length && (
              <div className="flex flex-wrap gap-2" aria-label="字级时间戳">
                {sentence.words.map((word, i) => (
                  <span key={i} className="text-xs text-muted-foreground">
                    {word.text} {timestamp(word.start_time ?? 0)}–
                    {timestamp(word.end_time ?? 0)}
                  </span>
                ))}
              </div>
            )}
          </div>
        ))}
        {!subtitle.sentences?.length && (
          <p className="text-sm whitespace-pre-wrap">
            {subtitle.text || "供应商未返回字幕分句。"}
          </p>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
