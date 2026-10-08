import { Circle, TriangleAlert, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";

export function StatusBadge({ value }: { value: string }) {
  return (
    <Badge
      variant={
        value === "停用"
          ? "muted"
          : /失败|不可用|停用|错误|失效/.test(value)
            ? "destructive"
            : /待|锁定|改密|未发布/.test(value)
              ? "warning"
              : "muted"
      }
    >
      {value === "停用" ? (
        <Circle />
      ) : /失败|不可用|停用/.test(value) ? (
        <X />
      ) : /待|改密/.test(value) ? (
        <TriangleAlert />
      ) : (
        <Circle fill={/启用|正常/.test(value) ? "currentColor" : "none"} />
      )}
      {value}
    </Badge>
  );
}
