import { Login } from '@/online/components/login';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card';
export const metadata = { title: '协作权限' };
export default function LoginPage() {
  return (
    <main
      id="main"
      className="mx-auto flex min-h-svh w-full max-w-lg items-center px-4 py-12"
    >
      <Card className="w-full">
        <CardHeader>
          <h1 className="text-xl font-semibold">验证协作权限</h1>
          <CardDescription>
            阅读和搜索无需验证。编辑与执行前，请验证本机权限。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Login />
        </CardContent>
      </Card>
    </main>
  );
}
