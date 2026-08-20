"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type MailSummary } from "@/lib/api";
import { LocaleSwitcher, useI18n, useMe } from "@/components/providers";
import { Avatar, Kbd, Menu, PageLoader, cx } from "@/components/ui";
import { Icon } from "@/components/icons";
import { ComposeProvider, useCompose } from "@/components/compose";
import { CommandPalette } from "@/components/command-palette";
import { NotificationsBell } from "@/components/notifications-bell";

const FOLDER_META: Record<string, { icon: string; order: number }> = {
  inbox: { icon: "inbox", order: 0 },
  sent: { icon: "send", order: 1 },
  drafts: { icon: "file-text", order: 2 },
  spam: { icon: "alert-octagon", order: 3 },
  trash: { icon: "trash", order: 4 },
};

function NavItem({
  href, icon, label, active, badge, onClick,
}: {
  href: string;
  icon: string;
  label: string;
  active: boolean;
  badge?: React.ReactNode;
  onClick?: () => void;
}) {
  return (
    <Link
      href={href}
      onClick={onClick}
      className={cx(
        "group flex h-8 items-center gap-2.5 rounded-[7px] px-2.5 text-[13px] transition-colors duration-100",
        active
          ? "bg-graphite font-medium text-graphite-foreground"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      <Icon name={icon} className={cx("h-4 w-4 shrink-0", active ? "opacity-90" : "opacity-70 group-hover:opacity-90")} />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {badge}
    </Link>
  );
}

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <p className="mb-1 mt-4 px-2.5 text-[10.5px] font-medium uppercase tracking-[.08em] text-faint">{children}</p>;
}

function MailShell({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const qc = useQueryClient();
  const me = useMe();
  const { t } = useI18n();
  const { openCompose } = useCompose();
  const [search, setSearch] = useState(searchParams.get("q") ?? "");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  // Deliberately not gated on `me`: both requests carry the same session
  // cookie, so waiting for /me first only serialises two round trips. If the
  // session is missing this 401s and is discarded by the redirect below.
  const summary = useQuery({
    queryKey: ["mail", "summary"],
    queryFn: () => api.get<MailSummary>("/api/v1/mail/summary"),
    refetchInterval: 15_000,
  });

  useEffect(() => { if (!me.isLoading && !me.data) router.replace("/login"); }, [me.isLoading, me.data, router]);

  // Global shortcuts: "/" → focus search, "C" → compose (Ctrl/Cmd+K opens
  // the command palette, handled by <CommandPalette />).
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      const editing = target && (target.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName));
      if (editing || e.ctrlKey || e.metaKey || e.altKey) return;
      if (e.key === "/") {
        e.preventDefault();
        searchRef.current?.focus();
        searchRef.current?.select();
      }
      if (e.key.toLowerCase() === "c") {
        e.preventDefault();
        openCompose();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [openCompose]);

  if (me.isLoading || !me.data) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background">
        <PageLoader label="Loading QazEra" />
      </main>
    );
  }

  const isAdmin = me.data.permissions.includes("users.manage");
  const folders = (summary.data?.folders ?? [])
    .filter((f) => FOLDER_META[f.type])
    .sort((a, b) => FOLDER_META[a.type].order - FOLDER_META[b.type].order);

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

  const displayName = me.data.display_name || me.data.email;

  return (
    <div className="flex h-screen w-full overflow-hidden bg-background text-foreground">
      {sidebarOpen && (
        <button className="fixed inset-0 z-30 bg-graphite/40 lg:hidden" aria-label="Close navigation" onClick={() => setSidebarOpen(false)} />
      )}

      <aside
        className={cx(
          "fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r border-border bg-surface transition-transform duration-200 lg:static lg:translate-x-0",
          sidebarOpen ? "translate-x-0" : "-translate-x-full",
        )}
      >
        <div className="flex h-12 shrink-0 items-center gap-2.5 px-4">
          <span className="grid h-6.5 w-6.5 place-items-center rounded-[6px] bg-graphite text-xs font-semibold text-graphite-foreground">Q</span>
          <span className="text-[13px] font-semibold tracking-tight">QazEra Mail</span>
        </div>

        <div className="px-3 pb-1 pt-1">
          <button
            type="button"
            onClick={() => { setSidebarOpen(false); openCompose(); }}
            className="flex h-8 w-full items-center justify-center gap-2 rounded-[7px] bg-graphite text-[13px] font-medium text-graphite-foreground transition-colors hover:bg-graphite/90"
          >
            <Icon name="pen" className="h-3.5 w-3.5" /> {t("compose")}
          </button>
        </div>

        <nav className="min-h-0 flex-1 overflow-y-auto px-3 pb-3">
          <SectionLabel>{t("mail")}</SectionLabel>
          <div className="space-y-px">
            {folders.map((folder) => {
              const href = `/mail/${folder.type}`;
              const active = pathname === href || (folder.type === "inbox" && pathname === "/mail/search");
              const count = folder.type === "drafts" ? folder.total : folder.unread;
              return (
                <NavItem
                  key={folder.id}
                  href={href}
                  icon={FOLDER_META[folder.type].icon}
                  label={t(folder.type)}
                  active={active}
                  onClick={() => setSidebarOpen(false)}
                  badge={count > 0 ? (
                    <span className={cx(
                      "rounded-full px-1.5 text-[11px] font-semibold leading-4",
                      active ? "bg-white/15 text-graphite-foreground" : folder.type === "inbox" ? "bg-primary/12 text-primary" : "text-faint",
                    )}>
                      {count > 999 ? "999+" : count}
                    </span>
                  ) : undefined}
                />
              );
            })}
          </div>

          <SectionLabel>{t("workspace")}</SectionLabel>
          <div className="space-y-px">
            {me.data.permissions.includes("messages.send") && (
              <NavItem href="/mail/messages" icon="message-circle" label={t("messages")} active={pathname === "/mail/messages"} onClick={() => setSidebarOpen(false)} />
            )}
            {me.data.permissions.includes("official.read") && (
              <NavItem href="/mail/official" icon="megaphone" label={t("official")} active={pathname === "/mail/official"} onClick={() => setSidebarOpen(false)} />
            )}
            {me.data.permissions.includes("tasks.manage.self") && (
              <NavItem href="/mail/my-list" icon="check-circle" label={t("tasksReminders")} active={pathname === "/mail/my-list"} onClick={() => setSidebarOpen(false)} />
            )}
            {me.data.permissions.includes("departments.read") && (
              <NavItem href="/mail/contacts" icon="users" label={t("contacts")} active={pathname === "/mail/contacts"} onClick={() => setSidebarOpen(false)} />
            )}
          </div>

          {isAdmin && (
            <>
              <SectionLabel>{t("administration")}</SectionLabel>
              <NavItem href="/admin" icon="layout-grid" label={t("adminConsole")} active={false} onClick={() => setSidebarOpen(false)} />
            </>
          )}
        </nav>

        <div className="shrink-0 border-t border-border p-3">
          <NavItem href="/mail/settings" icon="settings" label={t("settings")} active={pathname === "/mail/settings"} onClick={() => setSidebarOpen(false)} />
          <Menu
            align="start"
            width="w-56"
            trigger={
              <button type="button" className="mt-1.5 flex w-full items-center gap-2.5 rounded-[8px] p-1.5 text-left transition-colors hover:bg-muted">
                <Avatar name={displayName} size="lg" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] font-medium leading-4">{displayName}</span>
                  <span className="block truncate text-[11px] text-muted-foreground">{summary.data?.mailbox.address || me.data.email}</span>
                </span>
                <Icon name="chevron-down" className="h-3.5 w-3.5 shrink-0 text-faint" />
              </button>
            }
            items={[
              { type: "label", label: isAdmin ? "Administrator" : "Member" },
              { label: t("settings"), icon: "settings", onSelect: () => router.push("/mail/settings") },
              ...(isAdmin ? [{ label: t("adminConsole"), icon: "layout-grid", onSelect: () => router.push("/admin") }] : []),
              { type: "separator" as const },
              { label: t("signOut"), icon: "log-out", danger: true, onSelect: () => void logout() },
            ]}
          />
          <div className="mt-2 flex items-center px-1 lg:hidden"><LocaleSwitcher /></div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-surface-elevated px-3 lg:px-4">
          <button
            className="grid h-8 w-8 place-items-center rounded-[7px] text-muted-foreground hover:bg-muted hover:text-foreground lg:hidden"
            onClick={() => setSidebarOpen(true)}
            aria-label="Open navigation"
          >
            <Icon name="menu" />
          </button>

          <form onSubmit={submitSearch} className="relative w-full max-w-lg">
            <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
            <input
              ref={searchRef}
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("searchMessages")}
              className="h-8 w-full rounded-[7px] border border-transparent bg-background pl-8 pr-14 text-[13px] outline-none transition-[border-color,box-shadow] placeholder:text-faint focus:border-primary focus:bg-surface-elevated focus:ring-2 focus:ring-primary/15"
            />
            <span className="pointer-events-none absolute right-2 top-1/2 flex -translate-y-1/2 items-center gap-1"><Kbd>/</Kbd></span>
          </form>

          <div className="ml-auto flex items-center gap-1">
            <button
              type="button"
              onClick={() => openCompose()}
              className="hidden h-8 items-center gap-1.5 rounded-[7px] px-2.5 text-[13px] font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground md:flex"
              title="New message (C)"
            >
              <Icon name="pen" className="h-3.5 w-3.5" /> New
            </button>
            <NotificationsBell />
            <span className="hidden md:block"><LocaleSwitcher /></span>
            {isAdmin && (
              <Link href="/admin" className="hidden h-8 items-center gap-1.5 rounded-[7px] border border-border-strong bg-surface-elevated px-2.5 text-xs font-medium transition-colors hover:bg-background sm:flex">
                <Icon name="shield" className="h-3.5 w-3.5 text-muted-foreground" /> {t("admin")}
              </Link>
            )}
            <Menu
              trigger={
                <button type="button" className="ml-1 grid place-items-center rounded-full transition-opacity hover:opacity-80" aria-label="Account">
                  <Avatar name={displayName} size="md" />
                </button>
              }
              items={[
                { type: "label", label: displayName },
                { label: t("settings"), icon: "settings", onSelect: () => router.push("/mail/settings") },
                ...(isAdmin ? [{ label: t("adminConsole"), icon: "layout-grid", onSelect: () => router.push("/admin") }] : []),
                { type: "separator" as const },
                { label: t("signOut"), icon: "log-out", danger: true, onSelect: () => void logout() },
              ]}
            />
          </div>
        </header>

        <main className="min-h-0 min-w-0 flex-1 overflow-hidden bg-background">{children}</main>
      </div>
      <CommandPalette />
    </div>
  );
}

export default function MailLayout({ children }: { children: React.ReactNode }) {
  return (
    <ComposeProvider>
      <Suspense
        fallback={
          <main className="flex min-h-screen items-center justify-center bg-background">
            <PageLoader label="Loading QazEra" />
          </main>
        }
      >
        <MailShell>{children}</MailShell>
      </Suspense>
    </ComposeProvider>
  );
}
