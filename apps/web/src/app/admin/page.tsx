"use client";

// Admin overview: real operational data only — no decorative charts.
// Counts, live audit activity, subsystem health and security signals.

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { api, type AdminUser, type Department, type Domain, type InfraReport, type Mailbox } from "@/lib/api";
import { useMe } from "@/components/providers";
import { Badge, Button, Skeleton, cx } from "@/components/ui";
import { Icon } from "@/components/icons";

type AuditEntry = {
  id: number;
  actor_email?: string;
  action: string;
  resource_type?: string;
  detail: Record<string, unknown> | null;
  created_at: string;
};

const ACTION_TEXT: Record<string, string> = {
  "auth.login": "signed in",
  "auth.login_failed": "failed to sign in",
  "auth.logout": "signed out",
  "auth.password_change": "changed password",
  "user.create": "created user",
  "user.update": "updated user",
  "department.create": "created department",
  "department.members_update": "changed department members",
  "bulk_email.send": "started a broadcast",
  "security.block_sender": "blocked a sender",
  "quarantine.release": "released from quarantine",
};

function timeAgo(iso: string): string {
  const d = new Date(iso.replace(" ", "T").replace("+00", "Z"));
  const mins = Math.max(0, Math.floor((Date.now() - d.getTime()) / 60_000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function StatCell({ label, value, href, sub }: { label: string; value: React.ReactNode; href: string; sub?: string }) {
  return (
    <Link href={href} className="group flex-1 border-l border-border px-4 py-3 first:border-l-0 hover:bg-background">
      <p className="text-[11.5px] font-medium text-muted-foreground">{label}</p>
      <div className="mt-1 text-xl font-semibold tracking-[-.01em] tabular-nums group-hover:text-primary">{value}</div>
      {sub && <p className="mt-0.5 text-[11px] text-faint">{sub}</p>}
    </Link>
  );
}

export default function AdminDashboard() {
  const me = useMe();
  const can = (p: string) => me.data?.permissions.includes(p) ?? false;

  const users = useQuery({ queryKey: ["admin", "users"], queryFn: () => api.get<{ users: AdminUser[] }>("/api/v1/users"), enabled: can("users.manage") });
  const departments = useQuery({ queryKey: ["departments"], queryFn: () => api.get<{ departments: Department[] }>("/api/v1/departments"), enabled: can("departments.read") });
  const domains = useQuery({ queryKey: ["admin", "domains"], queryFn: () => api.get<{ domains: Domain[] }>("/api/v1/domains"), enabled: can("domains.manage") });
  const mailboxes = useQuery({ queryKey: ["admin", "mailboxes"], queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"), enabled: can("mailboxes.manage") });
  const quarantine = useQuery({ queryKey: ["admin", "quarantine"], queryFn: () => api.get<{ quarantine: unknown[] }>("/api/v1/quarantine"), enabled: can("security.manage") });
  const blocks = useQuery({ queryKey: ["admin", "blocks"], queryFn: () => api.get<{ blocks: unknown[] }>("/api/v1/security/blocks"), enabled: can("security.manage") });

  const activity = useQuery({
    queryKey: ["admin", "audit", "recent"],
    queryFn: () => api.get<{ entries: AuditEntry[]; total: number }>("/api/v1/audit?limit=9"),
    enabled: can("audit.read"),
    refetchInterval: 15_000,
  });
  const failedLogins = useQuery({
    queryKey: ["admin", "audit", "failed-logins"],
    queryFn: () => {
      const from = new Date(Date.now() - 24 * 3600 * 1000).toISOString();
      return api.get<{ entries: AuditEntry[]; total: number }>(`/api/v1/audit?action=auth.login_failed&from=${encodeURIComponent(from)}&limit=1`);
    },
    enabled: can("audit.read"),
    refetchInterval: 60_000,
  });
  const infra = useQuery({
    queryKey: ["admin", "infrastructure"],
    queryFn: () => api.get<InfraReport>("/api/v1/system/infrastructure"),
    enabled: can("users.manage"),
    refetchInterval: 30_000,
    retry: false,
  });

  const userList = users.data?.users ?? [];
  const activeUsers = userList.filter((u) => u.status === "active").length;
  const domainList = domains.data?.domains ?? [];
  const verifiedDomains = domainList.filter((d) => d.status === "verified").length;
  const quarantineCount = quarantine.data?.quarantine?.length ?? 0;
  const failedCount = failedLogins.data?.total ?? 0;

  const HEALTH_LABELS: Record<string, { label: string; icon: string }> = {
    postgres: { label: "Database", icon: "database" },
    redis: { label: "Cache / rate limiting", icon: "zap" },
    nats: { label: "Event bus", icon: "activity" },
    minio: { label: "Attachment storage", icon: "server" },
    stalwart: { label: "Mail core", icon: "mail" },
    worker: { label: "Delivery worker", icon: "send" },
  };
  const infraDown = (infra.data?.components ?? []).filter((c) => c.status === "unavailable");
  const infraOK = infra.isSuccess && infraDown.length === 0;

  // Needs attention: only real, current signals.
  const attention: Array<{ text: string; href: string }> = [
    ...infraDown.map((c) => ({
      text: `${HEALTH_LABELS[c.name]?.label ?? c.name} is unavailable`,
      href: "/admin/infrastructure",
    })),
    ...domainList
      .filter((d) => d.provisioning_status === "failed")
      .map((d) => ({ text: `Domain ${d.name}: mail-core provisioning failed`, href: "/admin/domains" })),
    ...(mailboxes.data?.mailboxes ?? [])
      .filter((m) => m.provisioning_status === "failed")
      .map((m) => ({ text: `Mailbox ${m.address}: mail-core provisioning failed`, href: "/admin/mailboxes" })),
    ...(quarantineCount > 0
      ? [{ text: `${quarantineCount} message${quarantineCount > 1 ? "s" : ""} held in quarantine`, href: "/admin/security" }]
      : []),
  ];

  return (
    <div>
      <header className="page-header">
        <div>
          <h1 className="page-title">Overview</h1>
          <p className="page-description">Current state of your organization, mail infrastructure and security.</p>
        </div>
        <div className="flex items-center gap-2">
          {can("users.manage") && <Link href="/admin/users"><Button variant="secondary" size="sm"><Icon name="plus" className="h-3.5 w-3.5" /> New user</Button></Link>}
          {can("bulk_email.create") && <Link href="/admin/bulk-email"><Button size="sm"><Icon name="megaphone" className="h-3.5 w-3.5" /> Broadcast</Button></Link>}
        </div>
      </header>

      {/* Stat strip */}
      <section className="flex flex-wrap overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
        {can("users.manage") && (
          <StatCell
            label="Users"
            value={users.isLoading ? <Skeleton className="h-6 w-10" /> : userList.length}
            sub={users.isSuccess ? `${activeUsers} active · ${userList.length - activeUsers} disabled` : undefined}
            href="/admin/users"
          />
        )}
        {can("departments.read") && (
          <StatCell
            label="Departments"
            value={departments.isLoading ? <Skeleton className="h-6 w-10" /> : departments.data?.departments.length ?? 0}
            sub={departments.isSuccess ? `${departments.data.departments.reduce((n, d) => n + d.employee_count, 0)} employees assigned` : undefined}
            href="/admin/departments"
          />
        )}
        {can("mailboxes.manage") && (
          <StatCell
            label="Mailboxes"
            value={mailboxes.isLoading ? <Skeleton className="h-6 w-10" /> : mailboxes.data?.mailboxes.length ?? 0}
            href="/admin/mailboxes"
          />
        )}
        {can("domains.manage") && (
          <StatCell
            label="Domains"
            value={domains.isLoading ? <Skeleton className="h-6 w-10" /> : domainList.length}
            sub={domains.isSuccess ? `${verifiedDomains} verified` : undefined}
            href="/admin/domains"
          />
        )}
        {can("security.manage") && (
          <StatCell
            label="Quarantine"
            value={quarantine.isLoading ? <Skeleton className="h-6 w-10" /> : quarantineCount}
            sub="pending review"
            href="/admin/security"
          />
        )}
        {can("audit.read") && (
          <StatCell
            label="Failed logins"
            value={failedLogins.isLoading ? <Skeleton className="h-6 w-10" /> : failedCount}
            sub="last 24 hours"
            href="/admin/audit"
          />
        )}
      </section>

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
        {/* Recent activity — live audit feed */}
        {can("audit.read") && (
          <section className="rounded-[10px] border border-border bg-surface-elevated">
            <header className="flex items-center justify-between border-b border-border px-4 py-2.5">
              <h2 className="text-[13px] font-semibold">Recent activity</h2>
              <Link href="/admin/audit" className="text-xs font-medium text-primary hover:underline">Open Logs & Audit</Link>
            </header>
            {activity.isLoading && (
              <div className="space-y-3 p-4">{Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className="h-4" />)}</div>
            )}
            {activity.isSuccess && (activity.data.entries ?? []).length === 0 && (
              <p className="p-6 text-center text-[13px] text-muted-foreground">No recorded events yet.</p>
            )}
            <ul className="divide-y divide-border/70">
              {(activity.data?.entries ?? []).map((e) => {
                const warning = e.action === "auth.login_failed" || /\.(delete|revoke)$/.test(e.action) || e.action === "security.block_sender";
                const target = (e.detail && typeof e.detail.email === "string" && e.detail.email) || (e.detail && typeof e.detail.name === "string" && e.detail.name) || "";
                return (
                  <li key={e.id} className="flex items-center gap-3 px-4 py-2">
                    <span className={cx("h-1.5 w-1.5 shrink-0 rounded-full", warning ? "bg-warning" : "bg-border-strong")} />
                    <p className="min-w-0 flex-1 truncate text-[13px]">
                      <span className="font-medium">{e.actor_email || "system"}</span>{" "}
                      <span className="text-muted-foreground">{ACTION_TEXT[e.action] ?? e.action}</span>
                      {target && <span className="text-muted-foreground"> “{target}”</span>}
                    </p>
                    <span className="shrink-0 text-[11px] text-faint">{timeAgo(e.created_at)}</span>
                  </li>
                );
              })}
            </ul>
          </section>
        )}

        <div className="space-y-4">
          {/* Needs attention — real current signals only */}
          {attention.length > 0 && (
            <section className="rounded-[10px] border border-warning/30 bg-warning/5">
              <header className="border-b border-warning/20 px-4 py-2.5">
                <h2 className="text-[13px] font-semibold text-warning">Needs attention</h2>
              </header>
              <ul className="divide-y divide-border/50 text-[13px]">
                {attention.slice(0, 6).map((a, i) => (
                  <li key={i}>
                    <Link href={a.href} className="flex items-center gap-2.5 px-4 py-2 hover:bg-background/60">
                      <Icon name="alert-triangle" className="h-3.5 w-3.5 shrink-0 text-warning" />
                      <span className="min-w-0 flex-1 truncate">{a.text}</span>
                      <Icon name="chevron-right" className="h-3 w-3 shrink-0 text-faint" />
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          )}

          {/* System health — cached backend checks incl. mail core + worker */}
          <section className="rounded-[10px] border border-border bg-surface-elevated">
            <header className="flex items-center justify-between border-b border-border px-4 py-2.5">
              <h2 className="text-[13px] font-semibold">System health</h2>
              {infra.isSuccess && (
                <Badge tone={infraOK ? "success" : "warning"}>{infraOK ? "Operational" : "Degraded"}</Badge>
              )}
              {infra.isError && <Badge tone="danger">Unreachable</Badge>}
            </header>
            <ul className="divide-y divide-border/70">
              {Object.entries(HEALTH_LABELS).map(([key, meta]) => {
                const c = infra.data?.components.find((x) => x.name === key);
                return (
                  <li key={key} className="flex items-center gap-3 px-4 py-2 text-[13px]">
                    <Icon name={meta.icon} className="h-3.5 w-3.5 text-muted-foreground" />
                    <span className="flex-1">{meta.label}</span>
                    {infra.isLoading ? (
                      <Skeleton className="h-3 w-12" />
                    ) : c?.status === "ok" ? (
                      <span className="inline-flex items-center gap-1.5 text-xs text-success"><span className="h-1.5 w-1.5 rounded-full bg-success" /> OK</span>
                    ) : c?.status === "disabled" ? (
                      <span className="text-xs text-faint">off</span>
                    ) : c ? (
                      <span className="inline-flex items-center gap-1.5 text-xs text-danger" title={c.detail}><span className="h-1.5 w-1.5 rounded-full bg-danger" /> Down</span>
                    ) : (
                      <span className="text-xs text-faint">unknown</span>
                    )}
                  </li>
                );
              })}
            </ul>
            <footer className="border-t border-border px-4 py-2">
              <Link href="/admin/infrastructure" className="text-xs font-medium text-primary hover:underline">Infrastructure details</Link>
            </footer>
          </section>

          {/* Security snapshot — real counters */}
          {can("security.manage") && (
            <section className="rounded-[10px] border border-border bg-surface-elevated">
              <header className="flex items-center justify-between border-b border-border px-4 py-2.5">
                <h2 className="text-[13px] font-semibold">Security</h2>
                <Link href="/admin/security" className="text-xs font-medium text-primary hover:underline">Security Center</Link>
              </header>
              <ul className="divide-y divide-border/70 text-[13px]">
                <li className="flex items-center justify-between px-4 py-2">
                  <span className="flex items-center gap-2.5"><Icon name="shield" className="h-3.5 w-3.5 text-muted-foreground" /> Messages in quarantine</span>
                  <span className={cx("font-semibold tabular-nums", quarantineCount > 0 && "text-warning")}>{quarantineCount}</span>
                </li>
                <li className="flex items-center justify-between px-4 py-2">
                  <span className="flex items-center gap-2.5"><Icon name="alert-octagon" className="h-3.5 w-3.5 text-muted-foreground" /> Blocked senders</span>
                  <span className="font-semibold tabular-nums">{blocks.data?.blocks?.length ?? 0}</span>
                </li>
                {can("audit.read") && (
                  <li className="flex items-center justify-between px-4 py-2">
                    <span className="flex items-center gap-2.5"><Icon name="alert-triangle" className="h-3.5 w-3.5 text-muted-foreground" /> Failed logins (24h)</span>
                    <span className={cx("font-semibold tabular-nums", failedCount > 0 && "text-warning")}>{failedCount}</span>
                  </li>
                )}
              </ul>
            </section>
          )}

          {/* Quick access */}
          <section className="rounded-[10px] border border-border bg-surface-elevated">
            <header className="border-b border-border px-4 py-2.5">
              <h2 className="text-[13px] font-semibold">Quick access</h2>
            </header>
            <ul className="divide-y divide-border/70">
              {[
                can("users.manage") && { href: "/admin/users", icon: "users", label: "Manage users", note: "Accounts, roles, departments" },
                can("bulk_email.view_analytics") && { href: "/admin/bulk-email", icon: "megaphone", label: "Broadcast", note: "Mass email to departments" },
                can("audit.read") && { href: "/admin/audit", icon: "scroll-text", label: "Logs & Audit", note: "Full event history" },
                can("apikeys.manage") && { href: "/admin/api-keys", icon: "key", label: "API keys", note: "Programmatic access" },
              ]
                .filter((x): x is { href: string; icon: string; label: string; note: string } => Boolean(x))
                .map((item) => (
                  <li key={item.href}>
                    <Link href={item.href} className="flex items-center gap-3 px-4 py-2.5 transition-colors hover:bg-background">
                      <Icon name={item.icon} className="h-4 w-4 text-muted-foreground" />
                      <span className="min-w-0 flex-1">
                        <span className="block text-[13px] font-medium leading-4">{item.label}</span>
                        <span className="block truncate text-[11px] text-muted-foreground">{item.note}</span>
                      </span>
                      <Icon name="chevron-right" className="h-3.5 w-3.5 text-faint" />
                    </Link>
                  </li>
                ))}
            </ul>
          </section>
        </div>
      </div>
    </div>
  );
}
