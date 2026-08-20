"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { LocaleSwitcher, useI18n, useMe } from "@/components/providers";
import { Avatar, Kbd, Menu, PageLoader, cx } from "@/components/ui";
import { Icon } from "@/components/icons";
import { ComposeProvider } from "@/components/compose";
import { CommandPalette } from "@/components/command-palette";

const GROUPS: Array<{ label: string; items: Array<{ href: string; label: string; icon: string; permission: string }> }> = [
  {
    label: "overview",
    items: [{ href: "/admin", label: "dashboard", icon: "layout-grid", permission: "" }],
  },
  {
    label: "organization",
    items: [
      { href: "/admin/users", label: "team", icon: "users", permission: "users.manage" },
      { href: "/admin/departments", label: "departments", icon: "building", permission: "departments.manage" },
      { href: "/admin/bulk-email", label: "Broadcast", icon: "megaphone", permission: "bulk_email.view_analytics" },
      { href: "/admin/organizations", label: "organizations", icon: "briefcase", permission: "organizations.manage" },
    ],
  },
  {
    label: "infrastructure",
    items: [
      { href: "/admin/domains", label: "domains", icon: "globe", permission: "domains.manage" },
      { href: "/admin/mailboxes", label: "mailboxes", icon: "mail", permission: "mailboxes.manage" },
      { href: "/admin/aliases", label: "aliases", icon: "at-sign", permission: "mailboxes.manage" },
      { href: "/admin/groups", label: "groups", icon: "users", permission: "mailboxes.manage" },
    ],
  },
  {
    label: "developer",
    items: [
      { href: "/admin/api-keys", label: "apiKeys", icon: "key", permission: "apikeys.manage" },
      { href: "/admin/smtp", label: "smtp", icon: "server", permission: "apikeys.manage" },
      { href: "/admin/webhooks", label: "webhooks", icon: "webhook", permission: "webhooks.manage" },
    ],
  },
  {
    label: "operations",
    items: [
      { href: "/admin/trace", label: "messageTrace", icon: "search", permission: "security.manage" },
      { href: "/admin/infrastructure", label: "infrastructureHealth", icon: "activity", permission: "users.manage" },
    ],
  },
  {
    label: "security",
    items: [
      { href: "/admin/security", label: "securityCenter", icon: "shield", permission: "security.manage" },
      { href: "/admin/audit", label: "Logs & Audit", icon: "scroll-text", permission: "audit.read" },
    ],
  },
];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const me = useMe();
  const router = useRouter();
  const pathname = usePathname();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const { t } = useI18n();
  const canAdmin = GROUPS.some((group) => group.items.some((item) => item.permission && me.data?.permissions.includes(item.permission)));

  useEffect(() => {
    if (me.isLoading) return;
    if (!me.data) router.replace("/login");
    else if (!canAdmin) router.replace("/mail/inbox");
  }, [me.isLoading, me.data, canAdmin, router]);

  if (me.isLoading || !me.data || !canAdmin) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background">
        <PageLoader label="Loading admin console" />
      </main>
    );
  }

  const visibleGroups = GROUPS
    .map((group) => ({ ...group, items: group.items.filter((item) => !item.permission || me.data.permissions.includes(item.permission)) }))
    .filter((group) => group.items.length > 0);
  const current = visibleGroups.flatMap((g) => g.items).find((item) => item.href === pathname);
  const displayName = me.data.display_name || me.data.email;

  async function logout() {
    await api.post("/api/v1/auth/logout").catch(() => {});
    qc.clear();
    router.replace("/login");
  }

  return (
    <ComposeProvider>
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      {open && <button className="fixed inset-0 z-30 bg-graphite/40 lg:hidden" onClick={() => setOpen(false)} aria-label="Close navigation" />}

      <aside className={cx(
        "fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r border-border bg-surface transition-transform duration-200 lg:static lg:translate-x-0",
        open ? "translate-x-0" : "-translate-x-full",
      )}>
        <Link href="/admin" className="flex h-12 shrink-0 items-center gap-2.5 px-4">
          <span className="grid h-6.5 w-6.5 place-items-center rounded-[6px] bg-graphite text-xs font-semibold text-graphite-foreground">Q</span>
          <span className="text-[13px] font-semibold tracking-tight">QazEra</span>
          <span className="rounded-full border border-border-strong px-1.5 py-px text-[10px] font-medium text-muted-foreground">Admin</span>
        </Link>

        <nav className="min-h-0 flex-1 overflow-y-auto px-3 pb-4">
          {visibleGroups.map((group) => (
            <div key={group.label} className="mt-3.5 first:mt-1">
              <p className="mb-1 px-2.5 text-[10.5px] font-medium uppercase tracking-[.08em] text-faint">{t(group.label) || group.label}</p>
              <div className="space-y-px">
                {group.items.map((item) => {
                  const active = pathname === item.href;
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      onClick={() => setOpen(false)}
                      className={cx(
                        "group flex h-8 items-center gap-2.5 rounded-[7px] px-2.5 text-[13px] transition-colors duration-100",
                        active ? "bg-graphite font-medium text-graphite-foreground" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                      )}
                    >
                      <Icon name={item.icon} className={cx("h-4 w-4 shrink-0", active ? "opacity-90" : "opacity-70 group-hover:opacity-90")} />
                      <span className="truncate">{t(item.label) || item.label}</span>
                    </Link>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>

        <div className="shrink-0 border-t border-border p-3">
          <Link href="/mail/inbox" className="flex h-8 items-center gap-2.5 rounded-[7px] px-2.5 text-[13px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground">
            <Icon name="corner-up-left" className="h-4 w-4 opacity-70" /> {t("backToMail")}
          </Link>
          <Menu
            align="start"
            width="w-56"
            trigger={
              <button type="button" className="mt-1.5 flex w-full items-center gap-2.5 rounded-[8px] p-1.5 text-left transition-colors hover:bg-muted">
                <Avatar name={displayName} size="lg" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] font-medium leading-4">{displayName}</span>
                  <span className="block truncate text-[11px] text-muted-foreground">Administrator</span>
                </span>
                <Icon name="chevron-down" className="h-3.5 w-3.5 shrink-0 text-faint" />
              </button>
            }
            items={[
              { label: t("settings"), icon: "settings", onSelect: () => router.push("/mail/settings") },
              { label: t("backToMail"), icon: "inbox", onSelect: () => router.push("/mail/inbox") },
              { type: "separator" as const },
              { label: t("signOut"), icon: "log-out", danger: true, onSelect: () => void logout() },
            ]}
          />
          <div className="mt-2 flex items-center px-1 lg:hidden"><LocaleSwitcher /></div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border bg-surface-elevated px-3 lg:px-5">
          <button className="grid h-8 w-8 place-items-center rounded-[7px] text-muted-foreground hover:bg-muted lg:hidden" onClick={() => setOpen(true)} aria-label="Open navigation">
            <Icon name="menu" />
          </button>
          <nav className="flex min-w-0 items-center gap-1.5 text-[13px]" aria-label="Breadcrumb">
            <Link href="/admin" className="text-muted-foreground hover:text-foreground">Admin</Link>
            {current && current.href !== "/admin" && (
              <>
                <Icon name="chevron-right" className="h-3 w-3 text-faint" />
                <span className="truncate font-medium">{t(current.label) || current.label}</span>
              </>
            )}
          </nav>
          <div className="ml-auto flex items-center gap-1.5">
            <span className="mr-1 hidden items-center gap-1.5 text-[11px] text-faint lg:flex">Quick actions <Kbd>Ctrl K</Kbd></span>
            <span className="hidden md:block"><LocaleSwitcher /></span>
            <Menu
              trigger={
                <button type="button" className="grid place-items-center rounded-full transition-opacity hover:opacity-80" aria-label="Account">
                  <Avatar name={displayName} size="md" />
                </button>
              }
              items={[
                { type: "label", label: displayName },
                { label: t("backToMail"), icon: "inbox", onSelect: () => router.push("/mail/inbox") },
                { type: "separator" as const },
                { label: t("signOut"), icon: "log-out", danger: true, onSelect: () => void logout() },
              ]}
            />
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto bg-background p-4 sm:p-5 lg:p-6">
          <div className="app-content">{children}</div>
        </main>
      </div>
      <CommandPalette />
    </div>
    </ComposeProvider>
  );
}
