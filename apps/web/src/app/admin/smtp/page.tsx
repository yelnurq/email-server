"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Mailbox } from "@/lib/api";
import { Badge, Button, ConfirmDialog, EmptyState, PageLoader, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";

type Credential = {
  id: string;
  username: string;
  mailbox_address: string;
  last_used_at: string | null;
  status: string;
};

type CreatedCredential = Credential & {
  password: string;
  smtp_login: string;
  mailbox: string;
};

function CredentialStatus({ status }: { status: string }) {
  switch (status) {
    case "active":
      return <Badge tone="success">Active</Badge>;
    case "revoked":
      return <Badge tone="danger">Revoked</Badge>;
    case "disabled":
      return <Badge tone="neutral">Disabled</Badge>;
    default:
      return <Badge tone="accent">{status}</Badge>;
  }
}

function StatCard({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded-[10px] border border-border bg-surface-elevated p-4">
      <p className="text-[11px] uppercase tracking-[0.08em] text-muted-foreground">{label}</p>
      <p className="mt-2 text-2xl font-semibold text-foreground">{value}</p>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">{hint}</p>
    </div>
  );
}

function formatLastUsed(value: string | null) {
  if (!value) return "Never";
  return new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export default function SmtpPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [mailboxId, setMailboxId] = useState("");
  const [created, setCreated] = useState<CreatedCredential | null>(null);
  const [confirmRevoke, setConfirmRevoke] = useState<Credential | null>(null);

  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
  });
  const creds = useQuery({
    queryKey: ["admin", "smtp-creds"],
    queryFn: () => api.get<{ smtp_credentials: Credential[] }>("/api/v1/smtp-credentials"),
  });

  const mailboxList = mailboxes.data?.mailboxes ?? [];
  const credentialList = creds.data?.smtp_credentials ?? [];
  const activeCredentials = credentialList.filter((c) => c.status === "active").length;
  const linkedMailboxes = new Set(credentialList.map((c) => c.mailbox_address)).size;
  const selectedMailbox = mailboxList.find((m) => m.id === mailboxId) ?? null;

  useEffect(() => {
    if (!mailboxId && mailboxList.length > 0) {
      setMailboxId(mailboxList[0].id);
    }
  }, [mailboxId, mailboxList]);

  const create = useMutation({
    mutationFn: () => api.post<CreatedCredential>("/api/v1/smtp-credentials", { mailbox_id: mailboxId }),
    onSuccess: (res) => {
      setCreated(res);
      qc.invalidateQueries({ queryKey: ["admin", "smtp-creds"] });
      toast("success", "SMTP credential created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create credential"),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/smtp-credentials/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "smtp-creds"] });
      setConfirmRevoke(null);
      toast("success", "SMTP credential revoked");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not revoke credential"),
  });

  const canCreate = mailboxList.length > 0 && mailboxId.length > 0;

  return (
    <div className="space-y-6">
      <section className="mb-8 space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="max-w-3xl">
            <h1 className="page-title">SMTP Credentials</h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              This page issues app passwords for mailboxes. Use the generated username and password in mail clients,
              scripts, or integrations that need SMTP submission.
            </p>
          </div>
          <Link
            href="/admin/mailboxes"
            className="inline-flex h-8 items-center gap-2 rounded-[7px] border border-border bg-surface-elevated px-3 text-[13px] font-medium text-foreground transition-colors hover:bg-muted"
          >
            <Icon name="mail" className="h-4 w-4" />
            Manage mailboxes
          </Link>
        </div>

        <div className="grid gap-3 sm:grid-cols-3">
          <StatCard
            label="Total credentials"
            value={String(credentialList.length)}
            hint="All SMTP app passwords currently stored in the tenant."
          />
          <StatCard
            label="Active"
            value={String(activeCredentials)}
            hint="Credentials that are still allowed to authenticate."
          />
          <StatCard
            label="Linked mailboxes"
            value={String(linkedMailboxes)}
            hint="How many mailboxes currently have at least one credential."
          />
        </div>
      </section>

      <section className="qazera-panel p-6">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <p className="text-base font-semibold text-foreground">New credential</p>
            <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
              Choose a mailbox, create a credential, and store the secret immediately. The password is shown only once.
            </p>
          </div>
          <Badge tone="accent">Password shown once</Badge>
        </div>

        <form
          className="mt-5 flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (canCreate) create.mutate();
          }}
        >
          <label className="block min-w-72 flex-1">
            <span className="mb-2 block text-xs font-medium text-muted-foreground">Mailbox</span>
            <select
              value={mailboxId}
              onChange={(e) => setMailboxId(e.target.value)}
              className="w-full rounded-[7px] border border-border-strong bg-surface-elevated px-3 py-2 text-[13px] text-foreground outline-none transition-[border-color,box-shadow] duration-100 focus:border-primary focus:ring-2 focus:ring-primary/15"
            >
              {mailboxList.length === 0 ? (
                <option value="">No mailboxes available</option>
              ) : (
                mailboxList.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.address}
                  </option>
                ))
              )}
            </select>
            {selectedMailbox && (
              <span className="mt-2 block text-xs text-muted-foreground">
                Selected mailbox: <span className="font-mono text-foreground">{selectedMailbox.address}</span>
              </span>
            )}
          </label>
          <Button type="submit" disabled={create.isPending || !canCreate}>
            {create.isPending ? "Creating..." : "Create credential"}
          </Button>
        </form>

        {mailboxList.length === 0 && (
          <div className="mt-5">
            <EmptyState
              title="No mailboxes yet"
              hint="Create at least one mailbox before generating SMTP credentials."
              action={
                <Link href="/admin/mailboxes" className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline">
                  Go to mailboxes
                  <Icon name="chevron-right" className="h-3 w-3" />
                </Link>
              }
            />
          </div>
        )}
      </section>

      {created && (
        <section className="qazera-panel border-danger bg-surface-elevated p-6">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p className="text-base font-semibold text-foreground">Secret shown once</p>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                Save these values now. The password cannot be retrieved again after closing this box.
              </p>
            </div>
            <Badge tone="danger">Store securely</Badge>
          </div>

          <dl className="mt-5 grid gap-3 sm:grid-cols-2">
            <div className="rounded-[8px] border border-border bg-background p-3">
              <dt className="text-[11px] uppercase tracking-[0.08em] text-muted-foreground">SMTP login</dt>
              <dd className="mt-1 font-mono text-sm text-foreground">{created.smtp_login}</dd>
            </div>
            <div className="rounded-[8px] border border-border bg-background p-3">
              <dt className="text-[11px] uppercase tracking-[0.08em] text-muted-foreground">Username</dt>
              <dd className="mt-1 font-mono text-sm text-foreground">{created.username}</dd>
            </div>
            <div className="rounded-[8px] border border-border bg-background p-3 sm:col-span-2">
              <dt className="text-[11px] uppercase tracking-[0.08em] text-muted-foreground">Password</dt>
              <dd className="mt-1 break-all font-mono text-sm text-foreground">{created.password}</dd>
            </div>
            <div className="rounded-[8px] border border-border bg-background p-3 sm:col-span-2">
              <dt className="text-[11px] uppercase tracking-[0.08em] text-muted-foreground">Mailbox</dt>
              <dd className="mt-1 font-mono text-sm text-foreground">{created.mailbox}</dd>
            </div>
          </dl>

          <div className="mt-4 flex flex-wrap gap-2">
            <Button variant="secondary" onClick={() => setCreated(null)}>Done</Button>
          </div>
        </section>
      )}

      {creds.isLoading && <PageLoader label="Loading SMTP credentials" />}
      {creds.isSuccess && credentialList.length === 0 && <EmptyState title="No SMTP credentials yet" hint="Create the first credential for a mailbox." />}
      {creds.isSuccess && credentialList.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">Username</th>
                <th className="px-4 py-3 font-medium">Mailbox</th>
                <th className="px-4 py-3 font-medium">Last used</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {credentialList.map((c) => (
                <tr key={c.id}>
                  <td className="px-4 py-3">
                    <div className="font-mono text-xs text-foreground">{c.username}</div>
                    <div className="mt-1 text-[11px] text-muted-foreground">Use this with the generated password</div>
                  </td>
                  <td className="px-4 py-3 text-foreground">{c.mailbox_address}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{formatLastUsed(c.last_used_at)}</td>
                  <td className="px-4 py-3">
                    <CredentialStatus status={c.status} />
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button
                      variant="ghost"
                      onClick={() => setConfirmRevoke(c)}
                      disabled={c.status === "revoked" || revoke.isPending}
                    >
                      Revoke
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmDialog
        open={confirmRevoke !== null}
        title="Revoke SMTP credential?"
        body={
          confirmRevoke
            ? `${confirmRevoke.username} for ${confirmRevoke.mailbox_address} will stop working immediately for SMTP submission.`
            : undefined
        }
        confirmLabel={revoke.isPending ? "Revoking..." : "Revoke credential"}
        danger
        onConfirm={() => {
          if (confirmRevoke) revoke.mutate(confirmRevoke.id);
        }}
        onCancel={() => setConfirmRevoke(null)}
      />
    </div>
  );
}
