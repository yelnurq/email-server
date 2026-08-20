"use client";

import { useEffect } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useMe } from "@/components/providers";
import { PageLoader, cx } from "@/components/ui";

const NAV = [
  { href: "/admin", label: "Dashboard" },
  { href: "/admin/organizations", label: "Organizations" },
  { href: "/admin/domains", label: "Domains" },
  { href: "/admin/users", label: "Users" },
  { href: "/admin/mailboxes", label: "Mailboxes" },
  { href: "/admin/aliases", label: "Aliases" },
  { href: "/admin/groups", label: "Groups" },
  { href: "/admin/api-keys", label: "API Keys" },
  { href: "/admin/smtp", label: "SMTP" },
  { href: "/admin/webhooks", label: "Webhooks" },
  { href: "/admin/security", label: "Security" },
  { href: "/admin/audit", label: "Audit" },
];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const me = useMe();
  const router = useRouter();
  const pathname = usePathname();
  const canAdmin = me.data?.permissions.includes("users.manage") ?? false;

  useEffect(() => {
    if (me.isLoading) return;
    if (!me.data) router.replace("/login");
    else if (!canAdmin) router.replace("/mail/inbox");
  }, [me.isLoading, me.data, canAdmin, router]);

  if (me.isLoading || !me.data || !canAdmin) {
    return (
      <main className="qazera-noise flex min-h-screen items-center justify-center bg-white">
        <PageLoader label="QazEra Control" />
      </main>
    );
  }

  return (
    <div className="qazera-noise flex min-h-screen flex-col bg-white">
      <header className="border-b-2 border-black bg-white">
        <div className="flex flex-col gap-4 px-4 py-4 lg:flex-row lg:items-center lg:px-6">
          <Link href="/admin" className="flex items-center gap-3">
            <span className="qazera-panel flex h-11 w-11 items-center justify-center bg-black text-sm font-black text-white">
              Q
            </span>
            <div>
              <p className="qazera-label text-accent">Control plane</p>
              <p className="mt-1 text-sm font-black uppercase tracking-[0.18em]">QazEra Admin</p>
            </div>
          </Link>

          <nav className="flex gap-2 overflow-x-auto lg:mx-6 lg:flex-wrap">
            {NAV.map((n) => (
              <Link
                key={n.href}
                href={n.href}
                className={cx(
                  "whitespace-nowrap border-2 border-black px-3 py-2 text-xs font-black uppercase tracking-[0.16em] transition-colors",
                  pathname === n.href ? "bg-black text-white" : "bg-white hover:bg-accent hover:text-white",
                )}
              >
                {n.label}
              </Link>
            ))}
          </nav>

          <Link
            href="/mail/inbox"
            className="ml-auto border-2 border-black bg-[#f2f2f2] px-4 py-3 text-xs font-black uppercase tracking-[0.18em] transition-colors hover:bg-black hover:text-white"
          >
            Webmail
          </Link>
        </div>
      </header>

      <main className="min-h-0 flex-1 overflow-y-auto p-4 lg:p-6">{children}</main>
    </div>
  );
}
