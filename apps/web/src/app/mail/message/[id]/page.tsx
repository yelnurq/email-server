"use client";

// Legacy deep link: /mail/message/<id> now opens inside the mail workspace.

import { use, useEffect } from "react";
import { useRouter } from "next/navigation";
import { PageLoader } from "@/components/ui";

export default function MessageRedirect({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  useEffect(() => {
    router.replace(`/mail/inbox?m=${encodeURIComponent(id)}`);
  }, [id, router]);
  return <PageLoader label="Opening message" />;
}
