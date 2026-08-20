"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useMe } from "@/components/providers";
import { PageLoader } from "@/components/ui";

export default function Home() {
  const router = useRouter();
  const me = useMe();

  useEffect(() => {
    if (me.isLoading) return;
    router.replace(me.data ? "/mail/inbox" : "/login");
  }, [me.isLoading, me.data, router]);

  return (
    <main className="flex min-h-screen items-center justify-center">
      <PageLoader label="Mail Platform" />
    </main>
  );
}
