import { AuthEntry } from "@/components/auth/auth-entry";

export const metadata = { title: "登录" };
export default function Page() {
  return <AuthEntry mode="login" />;
}
