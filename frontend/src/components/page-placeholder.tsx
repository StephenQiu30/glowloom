import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type PagePlaceholderProps = {
  description: string;
  eyebrow: string;
  title: string;
};

export function PagePlaceholder({
  description,
  eyebrow,
  title,
}: PagePlaceholderProps) {
  return (
    <main
      id="main-content"
      className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-6xl items-center px-6 py-16"
    >
      <Card className="w-full">
        <CardHeader>
          <CardDescription>{eyebrow}</CardDescription>
          <CardTitle className="text-3xl sm:text-4xl">
            <h1>{title}</h1>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="max-w-2xl text-base leading-7 text-muted-foreground">
            {description}
          </p>
          <p className="text-sm font-medium">当前阶段：产品入口已建立</p>
        </CardContent>
      </Card>
    </main>
  );
}
