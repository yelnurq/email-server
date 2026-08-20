"use client";

// Legacy deep link: /mail/compose opens the floating composer over the inbox.

import { Suspense, useEffect, useRef } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { PageLoader } from "@/components/ui";
import { useCompose } from "@/components/compose";

function ComposeRedirect() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { openCompose } = useCompose();
  const opened = useRef(false);

  useEffect(() => {
    if (opened.current) return;
    opened.current = true;
    openCompose({
      draftId: searchParams.get("draft") ?? undefined,
      forwardFrom: searchParams.get("forward") ?? undefined,
      to: searchParams.get("to") ?? undefined,
    });
    router.replace("/mail/inbox");
  }, [openCompose, router, searchParams]);

  return <PageLoader label="Opening composer" />;
}

export default function ComposePage() {
  return (
    <Suspense fallback={<PageLoader label="Opening composer" />}>
      <ComposeRedirect />
    </Suspense>
  );
}
