"use client";

// Logs & Audit Center: every recorded platform event — authentication, mail,
// administration, security — searchable and filterable, with a detail drawer
// that shows old → new values and raw JSON metadata.

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Badge, Button, Drawer, EmptyState, ErrorState, IconButton, ListSkeleton, Segmented, cx, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";

type AuditEntry = {
  id: number;
  actor_email?: string;
  action: string;
  resource_type?: string;
  resource_id?: string;
  detail: Record<string, unknown> | null;
  ip?: string;
  created_at: string;
};

const LIMIT = 50;

const CATEGORIES: Array<{ value: string; label: string }> = [
  { value: "", label: "All categories" },
  { value: "auth.", label: "Authentication" },
  { value: "user.", label: "Users" },
  { value: "department.", label: "Departments" },
  { value: "bulk_email.", label: "Broadcast" },
  { value: "official.", label: "Official messages" },
  { value: "task.", label: "Tasks" },
  { value: "security.", label: "Security" },
  { value: "quarantine.", label: "Quarantine" },
  { value: "domain.", label: "Domains" },
  { value: "mailbox.", label: "Mailboxes" },
  { value: "alias.", label: "Aliases" },
  { value: "group.", label: "Groups" },
  { value: "apikey.", label: "API keys" },
  { value: "smtpcred.", label: "SMTP credentials" },
  { value: "webhook.", label: "Webhooks" },
  { value: "organization.", label: "Organizations" },
];

const ACTION_TEXT: Record<string, string> = {
  "auth.login": "signed in",
  "auth.login_failed": "failed to sign in",
  "auth.logout": "signed out",
  "auth.password_change": "changed their password",
  "user.create": "created user",
  "user.update": "updated user",
  "department.create": "created department",
  "department.update": "updated department",
  "department.delete": "deleted department",
  "department.members_update": "changed department members",
  "bulk_email.create": "created a broadcast campaign",
  "bulk_email.send": "started a broadcast",
  "official.create": "published an official message",
  "task.assign": "assigned a task",
  "security.block_sender": "blocked a sender",
  "security.unblock_sender": "unblocked a sender",
  "quarantine.release": "released a message from quarantine",
  "quarantine.delete": "deleted a quarantined message",
  "domain.create": "added domain",
  "mailbox.create": "provisioned mailbox",
  "alias.create": "created alias",
  "alias.delete": "deleted alias",
  "alias.status": "changed alias status",
  "group.create": "created group",
  "group.delete": "deleted group",
  "group.members": "changed group members",
  "apikey.create": "created an API key",
  "apikey.revoke": "revoked an API key",
  "smtpcred.create": "created an SMTP credential",
  "smtpcred.revoke": "revoked an SMTP credential",
  "webhook.create": "created a webhook",
  "webhook.delete": "deleted a webhook",
  "webhook.retry": "retried a webhook delivery",
  "organization.create": "created organization",
};

function severityOf(action: string): "info" | "warning" {
  if (action === "auth.login_failed") return "warning";
  if (/\.(delete|revoke)$/.test(action) || action === "security.block_sender") return "warning";
  return "info";
}

function humanize(entry: AuditEntry): string {
  const base = ACTION_TEXT[entry.action] ?? entry.action.replace(".", ": ").replaceAll("_", " ");
  const detail = entry.detail ?? {};
  const target =
    (typeof detail.email === "string" && detail.email) ||
    (typeof detail.name === "string" && detail.name) ||
    (typeof detail.address === "string" && detail.address) ||
    (typeof detail.pattern === "string" && detail.pattern) ||
    (typeof detail.subject === "string" && detail.subject) ||
    "";
  return target ? `${base} “${target}”` : base;
}

function whenParts(iso: string): { date: string; time: string } {
  const d = new Date(iso.replace(" ", "T").replace("+00", "Z"));
  if (isNaN(d.getTime())) return { date: iso, time: "" };
  return {
    date: d.toLocaleDateString([], { year: "numeric", month: "short", day: "numeric" }),
    time: d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
  };
}

function ChangeList({ changes }: { changes: Record<string, { old?: string; new?: string }> }) {
  return (
    <div className="space-y-1.5">
      {Object.entries(changes).map(([field, value]) => (
        <div key={field} className="flex flex-wrap items-center gap-2 text-[13px]">
          <span className="font-medium">{field.replaceAll("_", " ")}</span>
          <span className="code-value rounded bg-danger/8 px-1.5 py-0.5 text-danger line-through decoration-danger/50">
            {value.old || "—"}
          </span>
          <Icon name="chevron-right" className="h-3 w-3 text-faint" />
          <span className="code-value rounded bg-success/10 px-1.5 py-0.5 text-success">{value.new || "—"}</span>
        </div>
      ))}
    </div>
  );
}

export default function AuditPage() {
  const toast = useToast();
  const [page, setPage] = useState(0);
  const [category, setCategory] = useState("");
  const [severity, setSeverity] = useState("all");
  const [q, setQ] = useState("");
  const [qInput, setQInput] = useState("");
  const [actor, setActor] = useState("");
  const [actorInput, setActorInput] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [selected, setSelected] = useState<AuditEntry | null>(null);
  const [showRaw, setShowRaw] = useState(false);

  const params = useMemo(() => {
    const p = new URLSearchParams();
    p.set("limit", String(LIMIT));
    p.set("offset", String(page * LIMIT));
    if (category) p.set("action", category);
    if (q) p.set("q", q);
    if (actor) p.set("actor", actor);
    if (from) p.set("from", from);
    if (to) {
      // inclusive end-of-day
      const d = new Date(to); d.setDate(d.getDate() + 1);
      p.set("to", d.toISOString().slice(0, 10));
    }
    return p.toString();
  }, [page, category, q, actor, from, to]);

  const audit = useQuery({
    queryKey: ["admin", "audit", params],
    queryFn: () => api.get<{ entries: AuditEntry[]; total: number }>(`/api/v1/audit?${params}`),
    refetchInterval: 15_000,
  });

  const entries = (audit.data?.entries ?? []).filter((e) => severity === "all" || severityOf(e.action) === severity);
  const total = audit.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / LIMIT));
  const hasFilters = Boolean(category || q || actor || from || to || severity !== "all");

  function applySearch(e: React.FormEvent) {
    e.preventDefault();
    setQ(qInput.trim());
    setActor(actorInput.trim());
    setPage(0);
  }

  return (
    <div>
      <header className="page-header">
        <div>
          <h1 className="page-title">Logs & Audit</h1>
          <p className="page-description">
            Every recorded platform event: who did what, when, from which address — with old and new values for changes.
          </p>
        </div>
        <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-success/60" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-success" />
          </span>
          Live · refreshes every 15s
        </span>
      </header>

      {/* Filter bar */}
      <form onSubmit={applySearch} className="mb-4 flex flex-wrap items-end gap-2 rounded-[10px] border border-border bg-surface-elevated p-3">
        <div className="relative min-w-52 flex-1">
          <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
          <input
            value={qInput}
            onChange={(e) => setQInput(e.target.value)}
            placeholder="Search action, target, metadata…"
            className="h-8 w-full rounded-[7px] border border-border-strong bg-surface-elevated pl-8 pr-2.5 text-[13px] outline-none placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary/15"
          />
        </div>
        <select value={category} onChange={(e) => { setCategory(e.target.value); setPage(0); }} aria-label="Category">
          {CATEGORIES.map((c) => <option key={c.value} value={c.value}>{c.label}</option>)}
        </select>
        <input
          value={actorInput}
          onChange={(e) => setActorInput(e.target.value)}
          placeholder="Actor email"
          className="h-8 w-40 rounded-[7px] border border-border-strong bg-surface-elevated px-2.5 text-[13px] outline-none placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary/15"
          aria-label="Actor email"
        />
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          From
          <input type="date" value={from} onChange={(e) => { setFrom(e.target.value); setPage(0); }} className="h-8 rounded-[7px] border border-border-strong bg-surface-elevated px-2 text-xs outline-none focus:border-primary" />
        </label>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          To
          <input type="date" value={to} onChange={(e) => { setTo(e.target.value); setPage(0); }} className="h-8 rounded-[7px] border border-border-strong bg-surface-elevated px-2 text-xs outline-none focus:border-primary" />
        </label>
        <Segmented
          value={severity}
          onChange={(v) => { setSeverity(v); }}
          options={[{ value: "all", label: "All" }, { value: "info", label: "Info" }, { value: "warning", label: "Warning" }]}
        />
        <Button type="submit" variant="secondary" size="sm">Apply</Button>
        {hasFilters && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => { setCategory(""); setSeverity("all"); setQ(""); setQInput(""); setActor(""); setActorInput(""); setFrom(""); setTo(""); setPage(0); }}
          >
            Reset
          </Button>
        )}
      </form>

      {audit.isLoading && <div className="rounded-[10px] border border-border bg-surface-elevated"><ListSkeleton rows={8} /></div>}
      {audit.isError && <ErrorState message="Could not load the audit log" onRetry={() => audit.refetch()} />}

      {audit.isSuccess && entries.length === 0 && (
        <div className="rounded-[10px] border border-border bg-surface-elevated">
          <EmptyState
            icon="scroll-text"
            title={hasFilters ? "No events match these filters" : "No events recorded yet"}
            hint={hasFilters ? "Try a wider date range or fewer filters." : "Administrative and authentication activity will appear here as it happens."}
          />
        </div>
      )}

      {audit.isSuccess && entries.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full min-w-[760px] text-[13px]">
            <thead>
              <tr className="border-b border-border text-left">
                <th className="w-40 px-3">Time</th>
                <th className="w-52 px-3">Actor</th>
                <th className="px-3">Event</th>
                <th className="w-36 px-3">Resource</th>
                <th className="w-32 px-3">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {entries.map((e) => {
                const sev = severityOf(e.action);
                const when = whenParts(e.created_at);
                return (
                  <tr key={e.id} className="cursor-pointer" onClick={() => { setSelected(e); setShowRaw(false); }}>
                    <td className="whitespace-nowrap px-3">
                      <span className="code-value block text-muted-foreground">{when.time}</span>
                      <span className="block text-[11px] text-faint">{when.date}</span>
                    </td>
                    <td className="max-w-52 truncate px-3 font-medium">{e.actor_email || <span className="text-faint">system</span>}</td>
                    <td className="px-3">
                      <span className="flex items-center gap-2">
                        <span className={cx("h-1.5 w-1.5 shrink-0 rounded-full", sev === "warning" ? "bg-warning" : "bg-border-strong")} />
                        <span className="min-w-0 truncate">{humanize(e)}</span>
                      </span>
                    </td>
                    <td className="px-3"><Badge tone="neutral">{e.resource_type || "—"}</Badge></td>
                    <td className="code-value whitespace-nowrap px-3 text-muted-foreground">{e.ip || "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {audit.isSuccess && total > LIMIT && (
        <div className="mt-3 flex items-center justify-between">
          <span className="text-xs text-muted-foreground">
            {page * LIMIT + 1}–{Math.min((page + 1) * LIMIT, total)} of {total.toLocaleString()} events
          </span>
          <div className="flex items-center gap-1">
            <IconButton size="sm" label="Previous page" icon="chevron-left" disabled={page === 0} onClick={() => setPage((p) => p - 1)} />
            <span className="text-xs text-muted-foreground">{page + 1} / {pages}</span>
            <IconButton size="sm" label="Next page" icon="chevron-right" disabled={page >= pages - 1} onClick={() => setPage((p) => p + 1)} />
          </div>
        </div>
      )}

      {/* Detail drawer */}
      <Drawer
        open={!!selected}
        onClose={() => setSelected(null)}
        title={selected ? (
          <span className="flex items-center gap-2">
            <Badge tone={severityOf(selected.action) === "warning" ? "warning" : "neutral"}>
              {severityOf(selected.action) === "warning" ? "Warning" : "Info"}
            </Badge>
            <span className="code-value">{selected.action}</span>
          </span>
        ) : ""}
      >
        {selected && (
          <div className="space-y-5 p-4">
            <p className="text-sm leading-6">
              <span className="font-semibold">{selected.actor_email || "System"}</span> {humanize(selected)}.
            </p>

            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 rounded-[8px] border border-border bg-background px-3.5 py-3 text-[13px]">
              <dt className="text-muted-foreground">Event ID</dt><dd className="code-value">{selected.id}</dd>
              <dt className="text-muted-foreground">Time</dt><dd>{selected.created_at}</dd>
              <dt className="text-muted-foreground">Actor</dt><dd>{selected.actor_email || "system"}</dd>
              <dt className="text-muted-foreground">Action</dt><dd className="code-value">{selected.action}</dd>
              <dt className="text-muted-foreground">Resource</dt>
              <dd>{selected.resource_type || "—"}{selected.resource_id && <span className="code-value block break-all text-muted-foreground">{selected.resource_id}</span>}</dd>
              <dt className="text-muted-foreground">IP address</dt><dd className="code-value">{selected.ip || "—"}</dd>
            </dl>

            {selected.detail && typeof selected.detail.changes === "object" && selected.detail.changes !== null && (
              <section>
                <h3 className="mb-2 text-xs font-medium uppercase tracking-[.05em] text-muted-foreground">Changes</h3>
                <ChangeList changes={selected.detail.changes as Record<string, { old?: string; new?: string }>} />
              </section>
            )}

            {selected.detail && Object.keys(selected.detail).length > 0 && (
              <section>
                <div className="mb-2 flex items-center justify-between">
                  <h3 className="text-xs font-medium uppercase tracking-[.05em] text-muted-foreground">Metadata</h3>
                  <div className="flex items-center gap-1">
                    <Button variant="ghost" size="sm" onClick={() => setShowRaw((v) => !v)}>{showRaw ? "Formatted" : "Raw JSON"}</Button>
                    <IconButton
                      size="sm"
                      label="Copy JSON"
                      icon="file-text"
                      onClick={() => { void navigator.clipboard.writeText(JSON.stringify(selected.detail, null, 2)); toast("info", "Metadata copied"); }}
                    />
                  </div>
                </div>
                {showRaw ? (
                  <pre className="code-value overflow-x-auto rounded-[8px] border border-border bg-background p-3 leading-5">
                    {JSON.stringify(selected.detail, null, 2)}
                  </pre>
                ) : (
                  <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-[13px]">
                    {Object.entries(selected.detail)
                      .filter(([k]) => k !== "changes")
                      .map(([k, v]) => (
                        <div key={k} className="contents">
                          <dt className="text-muted-foreground">{k.replaceAll("_", " ")}</dt>
                          <dd className="min-w-0 break-all">{typeof v === "object" ? <span className="code-value">{JSON.stringify(v)}</span> : String(v)}</dd>
                        </div>
                      ))}
                  </dl>
                )}
              </section>
            )}
          </div>
        )}
      </Drawer>
    </div>
  );
}
