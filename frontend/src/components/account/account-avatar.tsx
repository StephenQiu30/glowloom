import type { ComponentProps } from "react";
import { UserRound } from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

export function AccountAvatar(props: ComponentProps<typeof Avatar>) {
  return (
    <Avatar aria-hidden="true" {...props}>
      <AvatarFallback>
        <UserRound className="size-(--ui-icon-size)" />
      </AvatarFallback>
    </Avatar>
  );
}
