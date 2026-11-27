import type { ComponentProps } from "react";
import { IconAction } from "@/components/ui/icon-action";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export function AiServiceAction(props: ComponentProps<typeof IconAction>) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <IconAction {...props} />
      </TooltipTrigger>
      <TooltipContent>{props.label}</TooltipContent>
    </Tooltip>
  );
}
