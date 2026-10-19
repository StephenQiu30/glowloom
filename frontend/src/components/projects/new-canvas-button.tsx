"use client";

import type { ComponentProps } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useProjectCreation } from "./project-creation";

export function NewCanvasButton({
  children,
  onClick,
  disabled,
  ...props
}: ComponentProps<typeof Button>) {
  const creation = useProjectCreation();
  return (
    <Button
      {...props}
      disabled={disabled || creation.disabled}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented) void creation.start();
      }}
    >
      {children ?? (
        <>
          <Plus />
          新建画布
        </>
      )}
    </Button>
  );
}
