import { Card, CardContent, CardHeader } from "@/components/ui/card";

export default function Loading() {
  return (
    <main
      id="main-content"
      aria-busy="true"
      aria-live="polite"
      className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-5xl items-center px-6 py-16"
    >
      <p className="sr-only">页面加载中</p>
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
