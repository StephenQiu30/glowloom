import Link from "next/link";
import type { ComponentProps } from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "cn";
import { Button } from "@/components/ui/button";

type IconActionProps = Omit<
  ComponentProps<typeof Button>,
  "children" | "size" | "asChild"
> & {
  icon: LucideIcon;
  label: string;
  href?: string;
};

/** Keep icon controls named when used as links, form buttons, or Radix triggers. */
export function IconAction({
  icon: Icon,
  label,
  href,
  type = "button",
  variant = "ghost",
  className,
  ...props
}: IconActionProps) {
  const contents = <Icon aria-hidden="true" />;
  return (
    <Button
      {...props}
      type={href ? undefined : type}
      asChild={Boolean(href)}
      size="icon"
      variant={variant}
      className={cn("shrink-0 max-md:size-11", className)}
      aria-label={label}
      title={label}
    >
      {href ? <Link href={href}>{contents}</Link> : contents}
    </Button>
  );
}
