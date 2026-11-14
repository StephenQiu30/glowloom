import { Login } from '@/online/components/login';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card';
export const metadata = { title: '访问工作空间' };
export default function LoginPage() {
  return (
    <main
      id="main"
      className="mx-auto flex min-h-svh w-full max-w-lg items-center px-4 py-12"
    >
      <Card className="w-full">
        <CardHeader>
          <h1 className="text-xl font-semibold">访问工作空间</h1>
          <CardDescription>
            验证本机身份后，进入知识与研发协作。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Login />
        </CardContent>
      </Card>
    </main>
  );
}
