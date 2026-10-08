"use client";

import Link from "next/link";
import { type LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export function IconButton({
  icon: Icon,
  label,
  onClick,
  href,
  ...props
}: {
  icon: LucideIcon;
  label: string;
  onClick?: () => void;
  href?: string;
} & Pick<
  React.ComponentProps<typeof Button>,
  "variant" | "size" | "disabled"
>) {
  const content = href ? (
    <Button variant="ghost" size="icon" aria-label={label} asChild {...props}>
      <Link href={href}>
        <Icon data-icon="inline-start" />
      </Link>
    </Button>
  ) : (
    <Button
      variant="ghost"
      size="icon"
      aria-label={label}
      onClick={onClick}
      {...props}
    >
      <Icon data-icon="inline-start" />
    </Button>
  );
  return (
    <Tooltip>
      <TooltipTrigger asChild>{content}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
