"use client";

// Account settings: identity, interface language and password. Only real,
// working settings — the platform ships a single theme by design.

import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { api, ApiError, type MailClientConfig, type MailSummary } from "@/lib/api";
import { LocaleSwitcher, useI18n, useMe } from "@/components/providers";
import { Avatar, Badge, Button, Input, useToast } from "@/components/ui";

export default function SettingsPage() {
  const me = useMe();
  const { t } = useI18n();
  const toast = useToast();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");

  const change = useMutation({
    mutationFn: () => api.post("/api/v1/me/password", { current_password: current, new_password: next }),
    onSuccess: () => {
      setCurrent(""); setNext(""); setConfirm("");
      toast("success", "Password changed. Other sessions were signed out.");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not change password"),
  });

  const isAdmin = me.data?.permissions.includes("users.manage");
  const displayName = me.data?.display_name || me.data?.email || "";

  const clientConfig = useQuery({
    queryKey: ["mail", "client-config"],
    queryFn: () => api.get<MailClientConfig>("/api/v1/mail/client-config"),
    staleTime: 5 * 60_000,
  });
  const summary = useQuery({
    queryKey: ["mail", "summary"],
    queryFn: () => api.get<MailSummary>("/api/v1/mail/summary"),
    staleTime: 60_000,
  });

  return (
    <div className="h-full w-full overflow-y-auto">
      <div className="mx-auto max-w-2xl p-4 lg:p-6">
        <header className="page-header">
          <div>
            <h1 className="page-title">{t("settings")}</h1>
            <p className="page-description">Your account, interface language and security.</p>
          </div>
        </header>

        {/* Profile */}
        <section className="overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
          <header className="border-b border-border bg-background/60 px-4 py-2.5">
            <h2 className="text-[13px] font-semibold">Profile</h2>
          </header>
          <div className="flex items-center gap-4 px-4 py-4">
            <Avatar name={displayName} size="lg" className="h-12 w-12 text-sm" />
            <div className="min-w-0 flex-1">
              <p className="flex flex-wrap items-center gap-2 text-sm font-semibold">
                {displayName}
                <Badge tone={isAdmin ? "graphite" : "neutral"}>{isAdmin ? "Administrator" : "Member"}</Badge>
              </p>
              <p className="mt-0.5 truncate text-[13px] text-muted-foreground">{me.data?.email}</p>
            </div>
          </div>
          <dl className="grid gap-px border-t border-border bg-border sm:grid-cols-3">
            <div className="bg-surface-elevated px-4 py-3">
              <dt className="text-[11px] font-medium text-muted-foreground">Department</dt>
              <dd className="mt-0.5 text-[13px]">{me.data?.department_name || "—"}</dd>
            </div>
            <div className="bg-surface-elevated px-4 py-3">
              <dt className="text-[11px] font-medium text-muted-foreground">Roles</dt>
              <dd className="mt-0.5 flex flex-wrap gap-1 text-[13px]">
                {(me.data?.roles ?? []).map((r) => <Badge key={r.role + (r.scope_id ?? "")} tone="neutral">{r.role}</Badge>)}
              </dd>
            </div>
            <div className="bg-surface-elevated px-4 py-3">
              <dt className="text-[11px] font-medium text-muted-foreground">Account status</dt>
              <dd className="mt-0.5"><Badge tone="success">active</Badge></dd>
            </div>
          </dl>
        </section>

        {/* Language */}
        <section className="mt-4 overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
          <header className="border-b border-border bg-background/60 px-4 py-2.5">
            <h2 className="text-[13px] font-semibold">Interface language</h2>
          </header>
          <div className="flex flex-wrap items-center justify-between gap-3 px-4 py-3.5">
            <p className="text-[13px] text-muted-foreground">Қазақша, Русский or English.</p>
            <LocaleSwitcher />
          </div>
        </section>

        {/* Mail clients (IMAP/SMTP) — shown only when the mail core is on */}
        {clientConfig.data?.enabled && (
          <section className="mt-4 overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
            <header className="border-b border-border bg-background/60 px-4 py-2.5">
              <h2 className="text-[13px] font-semibold">Mail clients (IMAP / SMTP)</h2>
            </header>
            <div className="px-4 py-4">
              <p className="text-[13px] text-muted-foreground">
                Use these settings in Outlook, Thunderbird or a phone mail app. Your login is your
                mailbox address; the password is an SMTP credential issued by your administrator —
                your account password does not work for mail clients.
              </p>
              <dl className="mt-3 grid gap-px rounded-[8px] border border-border bg-border sm:grid-cols-2">
                <div className="bg-surface-elevated px-4 py-3">
                  <dt className="text-[11px] font-medium text-muted-foreground">Incoming — IMAP</dt>
                  <dd className="mt-0.5 font-mono text-[13px]">
                    {clientConfig.data.imap.host}:{clientConfig.data.imap.port}
                  </dd>
                  <dd className="text-[11px] text-faint">{clientConfig.data.imap.encryption}</dd>
                </div>
                <div className="bg-surface-elevated px-4 py-3">
                  <dt className="text-[11px] font-medium text-muted-foreground">Outgoing — SMTP</dt>
                  <dd className="mt-0.5 font-mono text-[13px]">
                    {clientConfig.data.smtp.host}:{clientConfig.data.smtp.port}
                  </dd>
                  <dd className="text-[11px] text-faint">{clientConfig.data.smtp.encryption}</dd>
                </div>
                <div className="bg-surface-elevated px-4 py-3 sm:col-span-2">
                  <dt className="text-[11px] font-medium text-muted-foreground">Username</dt>
                  <dd className="mt-0.5 font-mono text-[13px]">{summary.data?.mailbox.address || me.data?.email}</dd>
                </div>
              </dl>
            </div>
          </section>
        )}

        {/* Password */}
        <section className="mt-4 overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
          <header className="border-b border-border bg-background/60 px-4 py-2.5">
            <h2 className="text-[13px] font-semibold">Change password</h2>
          </header>
          <form
            className="max-w-sm space-y-4 px-4 py-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (next !== confirm) {
                toast("error", "New passwords do not match");
                return;
              }
              change.mutate();
            }}
          >
            <Input label="Current password" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required />
            <Input
              label="New password (min 10 characters)"
              type="password"
              autoComplete="new-password"
              value={next}
              onChange={(e) => setNext(e.target.value)}
              required
              minLength={10}
              error={confirm && next !== confirm ? "Passwords do not match" : undefined}
            />
            <Input label="Repeat new password" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
            <p className="text-xs text-faint">After the change, all other sessions are signed out.</p>
            <Button type="submit" disabled={change.isPending || !current || next.length < 10}>
              {change.isPending ? "Saving…" : "Change password"}
            </Button>
          </form>
        </section>
      </div>
    </div>
  );
}
