import { LoginForm } from "@/components/login-form";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type LoginPageProps = {
  searchParams: Promise<{ returnTo?: string | string[] }>;
};

export default async function LoginPage({ searchParams }: LoginPageProps) {
  const { returnTo: requestedReturnTo } = await searchParams;
  const returnTo = safeReturnTo(requestedReturnTo);

  return (
    <main
      id="main-content"
      className="mx-auto flex min-h-[calc(100vh-4rem)] max-w-md items-center px-6 py-16"
    >
      <Card className="w-full">
        <CardHeader>
          <CardDescription>账户</CardDescription>
          <CardTitle className="text-3xl">
            <h1>邀请制登录</h1>
          </CardTitle>
          <CardDescription>
            MVP 仅向已接受邀请的创作者开放。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <LoginForm returnTo={returnTo} />
        </CardContent>
      </Card>
    </main>
  );
}

function safeReturnTo(value: string | string[] | undefined) {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//")) {
    return "/create";
  }
  return value;
}
