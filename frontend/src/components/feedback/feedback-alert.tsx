import type { ReactNode } from "react";
import { Info, CircleX } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

export function FeedbackAlert({
  title,
  description,
  actions,
  tone = "info",
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  tone?: "info" | "error";
  className?: string;
}) {
  const Icon = tone === "error" ? CircleX : Info;
  return (
    <Alert role={tone === "error" ? "alert" : "status"} className={className}>
      <Icon data-tone={tone} />
      <AlertTitle>{title}</AlertTitle>
      {description ? (
        <AlertDescription className="col-span-full">
          {description}
        </AlertDescription>
      ) : null}
      {actions ? (
        <div className="col-span-full flex flex-wrap gap-2">{actions}</div>
      ) : null}
    </Alert>
  );
}
