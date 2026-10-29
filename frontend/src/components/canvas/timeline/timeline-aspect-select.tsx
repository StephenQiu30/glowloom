"use client";

import { Fragment } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  isTimelineAspectRatio,
  timelineAspectRatios,
  type TimelineAspectRatio,
} from "./timeline-aspect-ratio";

function AspectIcon({ value }: { value: string }) {
  if (value === "auto")
    return (
      <svg
        viewBox="0 0 24 24"
        width="16"
        height="16"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
      >
        <rect x="3.75" y="3.75" width="16.5" height="16.5" rx="3" />
        <path d="M13 8h3v3M8 13v3h3" />
      </svg>
    );
  const [x, y] = value.split(":").map(Number);
  const ratio = x > 0 && y > 0 ? x / y : 1;
  const width = ratio >= 1 ? 12.36 : 12.36 * ratio;
  const height = ratio >= 1 ? 12.36 / ratio : 12.36;
  return (
    <svg
      viewBox="0 0 16 16"
      width="16"
      height="16"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.33"
      aria-hidden="true"
    >
      <rect
        x={(16 - width) / 2}
        y={(16 - height) / 2}
        width={width}
        height={height}
        rx="1.6"
      />
    </svg>
  );
}

/** The timeline host owns persistence and read-only authority. */
export function TimelineAspectSelect({
  value,
  onChange,
  readOnly = false,
}: {
  value: string;
  onChange: (value: TimelineAspectRatio) => void;
  readOnly?: boolean;
}) {
  const supported = isTimelineAspectRatio(value);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          aria-label="画面比例"
          aria-invalid={!supported}
          disabled={readOnly}
        >
          <AspectIcon value={value} />
          {supported ? (value === "auto" ? "自动" : value) : "不支持的比例"}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        className="w-[200px] p-1"
        align="end"
        side="top"
        data-canvas-no-zoom
        onMouseDown={(event) => event.stopPropagation()}
      >
        <DropdownMenuRadioGroup
          value={supported ? value : ""}
          onValueChange={(next) => {
            if (!readOnly && isTimelineAspectRatio(next)) onChange(next);
          }}
        >
          {timelineAspectRatios.map((ratio, index) => (
            <Fragment key={ratio}>
              {index === 1 && (
                <DropdownMenuSeparator className="mx-3 my-0 h-1 bg-transparent" />
              )}
              <DropdownMenuRadioItem
                value={ratio}
                className="h-9 gap-2 rounded-lg pr-8 pl-3 text-[13px] [&>[data-slot=dropdown-menu-radio-item-indicator]]:right-3 [&>[data-slot=dropdown-menu-radio-item-indicator]]:left-auto"
              >
                <AspectIcon value={ratio} />
                {ratio === "auto" ? "自动" : ratio}
              </DropdownMenuRadioItem>
            </Fragment>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
