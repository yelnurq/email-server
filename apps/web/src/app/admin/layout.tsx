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
      <main className="flex min-h-screen items-center justify-center">
        <PageLoader />
      </main>
    );
  }

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center gap-4 border-b border-neutral-200 bg-white px-4 py-2.5 dark:border-neutral-800 dark:bg-neutral-900">
        <Link href="/admin" className="flex items-center gap-2">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-neutral-900 text-sm font-bold text-white dark:bg-neutral-700">
            M
          </span>
          <span className="text-sm font-semibold">Admin Portal</span>
        </Link>
        <nav className="hidden gap-1 sm:flex">
          {NAV.map((n) => (
            <Link
              key={n.href}
              href={n.href}
              className={cx(
                "rounded-lg px-3 py-1.5 text-sm transition-colors",
                pathname === n.href
                  ? "bg-neutral-100 font-semibold dark:bg-neutral-800"
                  : "text-neutral-600 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800",
              )}
            >
              {n.label}
            </Link>
          ))}
        </nav>
        <Link
          href="/mail/inbox"
          className="ml-auto rounded-lg px-3 py-1.5 text-sm font-medium text-indigo-600 hover:bg-indigo-50 dark:text-indigo-400 dark:hover:bg-indigo-950"
        >
          ← Webmail
        </Link>
      </header>
      <nav className="flex gap-1 overflow-x-auto border-b border-neutral-200 bg-white px-2 py-1 sm:hidden dark:border-neutral-800 dark:bg-neutral-900">
        {NAV.map((n) => (
          <Link
            key={n.href}
            href={n.href}
            className={cx(
              "whitespace-nowrap rounded-lg px-3 py-1 text-sm",
              pathname === n.href ? "bg-neutral-100 font-semibold dark:bg-neutral-800" : "text-neutral-600",
            )}
          >
            {n.label}
          </Link>
        ))}
      </nav>
      <main className="min-h-0 flex-1 overflow-y-auto p-4">{children}</main>
    </div>
  );
}
