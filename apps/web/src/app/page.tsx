"use client";

// No public landing page: "/" routes straight into the product.
// Unauthenticated → /login. Authenticated → mail workspace.

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useMe } from "@/components/providers";
import { PageLoader } from "@/components/ui";

export default function Home() {
  const me = useMe();
  const router = useRouter();

  useEffect(() => {
    if (me.isLoading) return;
    router.replace(me.data ? "/mail/inbox" : "/login");
  }, [me.isLoading, me.data, router]);

  return (
    <main className="flex min-h-screen items-center justify-center bg-background">
      <PageLoader label="Opening QazEra" />
    </main>
  );
}
