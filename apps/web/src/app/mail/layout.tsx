"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type MailSummary } from "@/lib/api";
import { useMe } from "@/components/providers";
import { Button, PageLoader, cx } from "@/components/ui";

const FOLDER_LABELS: Record<string, string> = {
  inbox: "IN",
  sent: "OUT",
  drafts: "DR",
  spam: "SP",
  trash: "TR",
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
      <main className="newsprint-texture flex min-h-screen items-center justify-center bg-[#f9f9f7]">
        <PageLoader label="QazEra Mail" />
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

  const folders = (summary.data?.folders ?? []).filter((f) => f.type !== "custom");

  return (
    <div className="newsprint-texture flex min-h-screen flex-col bg-[#f9f9f7] text-[#111111]">
      <header className="sticky top-0 z-40 border-b border-[#111111] bg-[#f9f9f7]/95 backdrop-blur">
        <div className="mx-auto flex w-full max-w-screen-xl flex-col gap-4 px-4 py-4 lg:flex-row lg:items-end lg:px-6">
          <div className="flex items-center gap-3">
            <button
              className="h-11 w-11 border border-[#111111] bg-transparent font-mono text-sm uppercase transition-colors hover:bg-[#111111] hover:text-[#f9f9f7] lg:hidden"
              onClick={() => setSidebarOpen((v) => !v)}
              aria-label="Toggle sidebar"
            >
              =
            </button>
            <Link href="/" className="flex items-center gap-3">
              <span className="flex h-11 w-11 items-center justify-center border border-[#111111] bg-[#111111] font-serif text-sm font-black text-[#f9f9f7]">
                Q
              </span>
              <div className="hidden lg:block">
                <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-[#cc0000]">
                  Vol. 1 | Inbox Edition
                </p>
                <span className="mt-2 block font-serif text-2xl tracking-tight">
                  QAZERA MAIL
                </span>
              </div>
            </Link>
          </div>

          <form onSubmit={submitSearch} className="w-full lg:mx-8 lg:max-w-2xl">
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search mail..."
              className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-body text-sm outline-none placeholder:text-[#737373] focus-visible:bg-[#f0f0f0]"
            />
          </form>

          <div className="flex items-center gap-2 lg:ml-auto">
            {isAdmin && (
              <Link
                href="/admin"
                className="border border-[#111111] bg-transparent px-3 py-2 font-mono text-xs uppercase tracking-[0.3em] transition-colors hover:bg-[#111111] hover:text-[#f9f9f7]"
              >
                Admin
              </Link>
            )}
            <div className="hidden text-right lg:block">
              <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-[#111111]">
                {me.data.display_name || me.data.email}
              </p>
              <p className="mt-1 font-body text-xs text-[#525252]">{summary.data?.mailbox.address}</p>
            </div>
            <Button variant="secondary" onClick={logout} title="Sign out">
              Sign out
            </Button>
          </div>
        </div>
      </header>

      <div className="mx-auto grid min-h-0 w-full max-w-screen-xl flex-1 lg:grid-cols-[17rem_minmax(0,1fr)]">
        <aside
          className={cx(
            "border-r border-[#111111] bg-[#e5e5e0] p-4 lg:sticky lg:top-[89px] lg:h-[calc(100vh-89px)] lg:overflow-y-auto",
            !sidebarOpen && "max-lg:hidden",
          )}
        >
          <p className="mb-3 font-mono text-[10px] uppercase tracking-[0.3em] text-[#cc0000]">
            Mailbox
          </p>
          <Link
            href="/mail/compose"
            className="mb-4 flex items-center justify-center gap-2 border border-[#111111] bg-[#111111] px-4 py-3 font-mono text-xs uppercase tracking-[0.25em] text-[#f9f9f7] transition-colors hover:bg-[#f9f9f7] hover:text-[#111111]"
            onClick={() => setSidebarOpen(false)}
          >
            Compose
          </Link>

          <div className="space-y-2">
            {folders.map((f) => {
              const href = `/mail/${f.type}`;
              const active = pathname === href;
              return (
                <Link
                  key={f.id}
                  href={href}
                  onClick={() => setSidebarOpen(false)}
                  className={cx(
                    "flex items-center justify-between gap-3 border border-[#111111] px-4 py-3 font-mono text-xs uppercase tracking-[0.25em] transition-colors",
                    active ? "bg-[#111111] text-[#f9f9f7]" : "bg-[#f9f9f7] text-[#111111] hover:bg-[#cc0000] hover:text-[#f9f9f7]",
                  )}
                >
                  <span className="flex items-center gap-3">
                    <span className="w-8">{FOLDER_LABELS[f.type] ?? "CM"}</span>
                    <span>{f.name}</span>
                  </span>
                  {f.type === "inbox" && f.unread > 0 && (
                    <span className="border border-current px-2 py-1 text-[10px]">{f.unread}</span>
                  )}
                </Link>
              );
            })}
          </div>

          <div className="mt-4 border-t border-[#111111] pt-4">
            <Link
              href="/mail/settings"
              onClick={() => setSidebarOpen(false)}
              className={cx(
                "flex items-center justify-between border border-[#111111] px-4 py-3 font-mono text-xs uppercase tracking-[0.25em] transition-colors",
                pathname === "/mail/settings" ? "bg-[#111111] text-[#f9f9f7]" : "bg-[#f9f9f7] hover:bg-[#cc0000] hover:text-[#f9f9f7]",
              )}
            >
              <span>Settings</span>
              <span>01</span>
            </Link>
          </div>
        </aside>

        <main className="min-w-0 bg-[#f9f9f7]">{children}</main>
      </div>
    </div>
  );
}

export default function MailLayout({ children }: { children: React.ReactNode }) {
  return (
    <Suspense
      fallback={
        <main className="newsprint-texture flex min-h-screen items-center justify-center bg-[#f9f9f7]">
          <PageLoader label="QazEra Mail" />
        </main>
      }
    >
      <MailShell>{children}</MailShell>
    </Suspense>
  );
}
