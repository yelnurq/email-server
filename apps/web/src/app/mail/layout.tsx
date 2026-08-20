"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type MailSummary } from "@/lib/api";
import { useMe } from "@/components/providers";
import { Button, PageLoader, cx } from "@/components/ui";
import { Suspense } from "react";

const FOLDER_ICONS: Record<string, string> = {
  inbox: "📥",
  sent: "📤",
  drafts: "📝",
  spam: "⚠️",
  trash: "🗑️",
};

function MailShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const qc = useQueryClient();
  const me = useMe();
  const [search, setSearch] = useState(searchParams.get("q") ?? "");
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const summary = useQuery({
    queryKey: ["mail", "summary"],
    queryFn: () => api.get<MailSummary>("/api/v1/mail/summary"),
    enabled: !!me.data,
    refetchInterval: 15_000,
  });

  useEffect(() => {
    if (!me.isLoading && !me.data) router.replace("/login");
  }, [me.isLoading, me.data, router]);

  if (me.isLoading || !me.data) {
    return (
      <main className="flex min-h-screen items-center justify-center">
        <PageLoader />
      </main>
    );
  }

  const isAdmin = me.data.permissions.includes("users.manage");

  async function logout() {
    await api.post("/api/v1/auth/logout").catch(() => {});
    qc.clear();
    router.replace("/login");
  }

  function submitSearch(e: React.FormEvent) {
    e.preventDefault();
    const q = search.trim();
    router.push(q ? `/mail/search?q=${encodeURIComponent(q)}` : "/mail/inbox");
  }

  return (
    <div className="flex h-screen flex-col">
      <header className="flex items-center gap-3 border-b border-neutral-200 bg-white px-4 py-2.5 dark:border-neutral-800 dark:bg-neutral-900">
        <button
          className="rounded-md p-1.5 text-neutral-500 hover:bg-neutral-100 md:hidden dark:hover:bg-neutral-800"
          onClick={() => setSidebarOpen((v) => !v)}
          aria-label="Toggle sidebar"
        >
          ☰
        </button>
        <Link href="/mail/inbox" className="flex items-center gap-2">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-600 text-sm font-bold text-white">
            M
          </span>
          <span className="hidden text-sm font-semibold sm:block">Mail Platform</span>
        </Link>
        <form onSubmit={submitSearch} className="mx-auto w-full max-w-xl">
          <input
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search mail…"
            className="w-full rounded-full border border-neutral-200 bg-neutral-100 px-4 py-1.5 text-sm focus:border-indigo-500 focus:bg-white focus:outline-none focus:ring-1 focus:ring-indigo-500 dark:border-neutral-700 dark:bg-neutral-800 dark:focus:bg-neutral-900"
          />
        </form>
        <div className="ml-auto flex items-center gap-2">
          {isAdmin && (
            <Link
              href="/admin"
              className="hidden rounded-lg px-3 py-1.5 text-sm font-medium text-neutral-600 hover:bg-neutral-100 sm:block dark:text-neutral-300 dark:hover:bg-neutral-800"
            >
              Admin
            </Link>
          )}
          <div className="hidden text-right sm:block">
            <p className="text-xs font-medium">{me.data.display_name || me.data.email}</p>
            <p className="text-xs text-neutral-400">{summary.data?.mailbox.address}</p>
          </div>
          <Button variant="ghost" onClick={logout} title="Sign out">
            Sign out
          </Button>
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <aside
          className={cx(
            "w-52 shrink-0 border-r border-neutral-200 bg-white p-3 dark:border-neutral-800 dark:bg-neutral-900",
            "max-md:absolute max-md:inset-y-0 max-md:top-[49px] max-md:z-40 max-md:shadow-xl",
            !sidebarOpen && "max-md:hidden",
          )}
        >
          <Link
            href="/mail/compose"
            className="mb-4 flex w-full items-center justify-center gap-2 rounded-xl bg-indigo-600 px-4 py-2.5 text-sm font-medium text-white shadow-sm transition-colors hover:bg-indigo-500"
            onClick={() => setSidebarOpen(false)}
          >
            ✏️ Compose
          </Link>
          <nav className="space-y-0.5">
            {(summary.data?.folders ?? [])
              .filter((f) => f.type !== "custom")
              .map((f) => {
                const href = `/mail/${f.type}`;
                const active = pathname === href;
                return (
                  <Link
                    key={f.id}
                    href={href}
                    onClick={() => setSidebarOpen(false)}
                    className={cx(
                      "flex items-center justify-between rounded-lg px-3 py-1.5 text-sm transition-colors",
                      active
                        ? "bg-indigo-50 font-semibold text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300"
                        : "text-neutral-700 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800",
                    )}
                  >
                    <span className="flex items-center gap-2">
                      <span aria-hidden>{FOLDER_ICONS[f.type] ?? "📁"}</span>
                      {f.name}
                    </span>
                    {f.type === "inbox" && f.unread > 0 && (
                      <span className="rounded-full bg-indigo-600 px-1.5 py-0.5 text-[10px] font-semibold text-white">
                        {f.unread}
                      </span>
                    )}
                  </Link>
                );
              })}
          </nav>
        </aside>

        <main className="min-w-0 flex-1 overflow-y-auto">{children}</main>
      </div>
    </div>
  );
}

export default function MailLayout({ children }: { children: React.ReactNode }) {
  return (
    <Suspense
      fallback={
        <main className="flex min-h-screen items-center justify-center">
          <PageLoader />
        </main>
      }
    >
      <MailShell>{children}</MailShell>
    </Suspense>
  );
}
