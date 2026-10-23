import { Hand, MousePointer2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

// Adapted from BeefTV's CanvasModeMenu; Radix owns focus and keyboard behavior.
export function CanvasModeMenu({
  value,
  onChange,
}: {
  value: "select" | "pan";
  onChange: (value: "select" | "pan") => void;
}) {
  const Icon = value === "pan" ? Hand : MousePointer2;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-lg"
          aria-label={`画布操作模式：${value === "pan" ? "抓手" : "选择"}`}
          title="操作模式（V / H）"
          data-canvas-mode-trigger
        >
          <Icon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side="top"
        align="center"
        sideOffset={12}
        data-canvas-no-zoom
        className="canvas-mode-menu w-56 max-w-[calc(100vw-2rem)] p-2 md:w-48"
      >
        <DropdownMenuRadioGroup
          value={value}
          onValueChange={(next) => {
            if (next === "select" || next === "pan") onChange(next);
          }}
          aria-label="画布操作模式"
        >
          <DropdownMenuRadioItem
            value="select"
            className="h-11 gap-2 whitespace-nowrap md:h-10"
          >
            <MousePointer2 />
            <span className="flex-1">选择</span>
            <DropdownMenuShortcut>V</DropdownMenuShortcut>
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem
            value="pan"
            className="h-11 gap-2 whitespace-nowrap md:h-10"
          >
            <Hand />
            <span className="flex-1">抓手</span>
            <DropdownMenuShortcut>H</DropdownMenuShortcut>
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
