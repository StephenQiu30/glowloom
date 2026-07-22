import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function Home() {
  return (
    <main className="mx-auto flex min-h-screen max-w-5xl items-center px-6 py-16">
      <Card className="w-full">
        <CardHeader>
          <CardDescription>Thief</CardDescription>
          <CardTitle className="text-3xl">AI 内容创作平台</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <p className="max-w-2xl text-muted-foreground">
            汇集优质提示词与示例素材，为后续创作流程提供可复用的灵感。
          </p>
          <Button disabled>内容能力建设中</Button>
        </CardContent>
      </Card>
    </main>
  );
}
