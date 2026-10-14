import { AuthEntry } from "@/components/auth/auth-entry";

export const metadata = { title: "注册" };
export default function Page() {
  return <AuthEntry mode="register" />;
}
