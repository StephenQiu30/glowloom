import { Card, CardContent, CardHeader } from "@/components/ui/card";

export default function Loading() {
  return (
    <main className="mx-auto flex min-h-screen max-w-5xl items-center px-6 py-16">
      <Card className="w-full animate-pulse">
        <CardHeader>
          <div className="h-4 w-20 rounded bg-muted" />
          <div className="h-9 w-64 rounded bg-muted" />
        </CardHeader>
        <CardContent>
          <div className="h-5 max-w-xl rounded bg-muted" />
        </CardContent>
      </Card>
    </main>
  );
}
