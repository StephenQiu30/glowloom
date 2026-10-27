"use client";

import { useState, type ReactElement } from "react";
import { Button } from "@/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover";

/** Keep an unscoped creation action anchored to its own explanation. */
export function ProjectRequiredPopover({
  children,
  title,
  description,
  onChooseProject,
}: {
  children: ReactElement;
  title: string;
  description: string;
  onChooseProject: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        side="top"
        align="start"
        sideOffset={8}
        collisionPadding={16}
        className="max-h-(--radix-popover-content-available-height) w-80 max-w-[calc(100vw-32px)] overflow-y-auto p-4"
        aria-label={title}
      >
        <PopoverHeader>
          <PopoverTitle>{title}</PopoverTitle>
          <PopoverDescription>{description}</PopoverDescription>
        </PopoverHeader>
        <Button
          type="button"
          variant="secondary"
          className="max-md:min-h-11"
          onClick={() => {
            setOpen(false);
            onChooseProject();
          }}
        >
          选择项目
        </Button>
      </PopoverContent>
    </Popover>
  );
}
