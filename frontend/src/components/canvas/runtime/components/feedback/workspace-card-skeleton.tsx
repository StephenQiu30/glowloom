import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

export function WorkspaceCardSkeleton({
  compact = false,
  showDate = false,
}: {
  compact?: boolean;
  showDate?: boolean;
}) {
  if (compact)
    return (
      <Card size="sm" className="home-asset" aria-hidden="true">
        <CardContent>
          <Skeleton className="home-asset-preview w-full" />
        </CardContent>
        <CardHeader className="min-w-0 flex-1 gap-2 px-0">
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-3 w-1/2" />
        </CardHeader>
      </Card>
    );
  return (
    <Card
      size="sm"
      className="gap-2 border-0 bg-transparent py-0 shadow-none ring-0"
      aria-hidden="true"
    >
      <Skeleton className="workspace-media" />
      <CardHeader className="gap-2 px-2">
        <Skeleton className="h-4 w-3/4" />
        {showDate && <Skeleton className="h-3 w-1/2" />}
      </CardHeader>
    </Card>
  );
}
