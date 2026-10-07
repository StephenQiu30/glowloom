import { AuthPage } from "@/components/preview/auth-pages";

export const metadata = { title: "登录 · 本地演示" };
export default function Page() {
  return <AuthPage mode="login" />;
}
