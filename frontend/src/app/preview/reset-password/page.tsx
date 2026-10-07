import { AuthPage } from "@/components/preview/auth-pages";

export const metadata = { title: "设置新密码 · 本地演示" };
export default function Page() {
  return <AuthPage mode="reset-password" />;
}
