import type { ComponentProps } from "react";
import { cn } from "cn";
import { UserRound } from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

export function AccountAvatar({
  className,
  ...props
}: ComponentProps<typeof Avatar>) {
  return (
    <Avatar
      aria-hidden="true"
      className={cn("[&_[data-slot=avatar-fallback]>svg]:size-1/2!", className)}
      {...props}
    >
      <AvatarFallback>
        <UserRound />
      </AvatarFallback>
    </Avatar>
  );
}
