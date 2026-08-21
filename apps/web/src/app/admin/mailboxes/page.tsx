"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatBytes, type Mailbox } from "@/lib/api";
import { Badge, ConfirmDialog, EmptyState, ListSkeleton, Menu, useToast } from "@/components/ui";

function ProvisioningBadge({ m }: { m: Mailbox }) {
  switch (m.provisioning_status) {
    case "active":
      return <Badge tone="success">Provisioned</Badge>;
    case "skipped":
      return <Badge tone="neutral">No mail core</Badge>;
    case "pending":
    case "provisioning":
      return <Badge tone="accent">Provisioning…</Badge>;
    case "failed":
      return <span title={m.provisioning_error}><Badge tone="danger">Failed</Badge></span>;
    default:
      return <Badge tone="neutral">Pending</Badge>;
  }
}

export default function MailboxesPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [confirmDisable, setConfirmDisable] = useState<Mailbox | null>(null);

  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
    // Provisioning runs asynchronously: poll while any mailbox is in flight.
    refetchInterval: (query) =>
      query.state.data?.mailboxes.some((m) => m.provisioning_status === "pending" || m.provisioning_status === "provisioning")
        ? 3000
        : false,
  });

  const refresh = () => qc.invalidateQueries({ queryKey: ["admin", "mailboxes"] });
  const onApiError = (e: unknown, fallback: string) =>
    toast("error", e instanceof ApiError ? e.message : fallback);

  const patch = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Record<string, unknown> }) =>
      api.patch(`/api/v1/mailboxes/${id}`, body),
    onSuccess: () => { refresh(); toast("success", "Mailbox updated"); },
    onError: (e) => onApiError(e, "Could not update the mailbox"),
  });

  const provision = useMutation({
    mutationFn: (id: string) => api.post<{ provisioning_status: string }>(`/api/v1/mailboxes/${id}/provision`),
    onSuccess: (res) => {
      refresh();
      if (res.provisioning_status === "active") toast("success", "Mailbox provisioned in the mail core");
      else toast("error", "Provisioning did not complete");
    },
    onError: (e) => onApiError(e, "Provisioning request failed"),
  });

  return (
    <div className="space-y-6">
      <section className="mb-8">
        <h1 className="page-title">Mailboxes</h1>
        <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
          Every mailbox is an account in the mail core: webmail, SMTP submission and IMAP all
          resolve to it. Disabling a mailbox revokes its mail-client passwords immediately.
        </p>
      </section>

      {mailboxes.isLoading && <ListSkeleton rows={6} />}
      {mailboxes.isSuccess && mailboxes.data.mailboxes.length === 0 && (
        <EmptyState title="No mailboxes yet" hint="Create a user with a mailbox first." />
      )}
      {mailboxes.isSuccess && mailboxes.data.mailboxes.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">Address</th>
                <th className="px-4 py-3 font-medium">Owner</th>
                <th className="px-4 py-3 font-medium">Usage</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Mail core</th>
                <th className="px-4 py-3" aria-label="Actions" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {mailboxes.data.mailboxes.map((m) => (
                <tr key={m.id}>
                  <td className="px-4 py-3 font-semibold">{m.address}</td>
                  <td className="px-4 py-3 text-muted-foreground">{m.user_email || "shared"}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                    {formatBytes(m.used_bytes)} / {formatBytes(m.quota_bytes)}
                  </td>
                  <td className="px-4 py-3">
                    <Badge tone={m.status === "active" ? "success" : "danger"}>{m.status}</Badge>
                  </td>
                  <td className="px-4 py-3"><ProvisioningBadge m={m} /></td>
                  <td className="px-4 py-3 text-right">
                    <Menu
                      trigger={
                        <button
                          type="button"
                          className="rounded-[6px] px-2 py-1 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
                          aria-label={`Actions for ${m.address}`}
                        >
                          Actions
                        </button>
                      }
                      items={[
                        ...(m.status === "active"
                          ? [{ label: "Disable mailbox", icon: "alert-octagon", danger: true, onSelect: () => setConfirmDisable(m) }]
                          : [{ label: "Enable mailbox", icon: "check-circle", onSelect: () => patch.mutate({ id: m.id, body: { status: "active" } }) }]),
                        ...(m.provisioning_status !== "active" && m.provisioning_status !== "skipped"
                          ? [{ label: "Retry provisioning", icon: "refresh-cw", onSelect: () => provision.mutate(m.id) }]
                          : []),
                        {
                          label: "Quota: 1 GB", icon: "database",
                          onSelect: () => patch.mutate({ id: m.id, body: { quota_bytes: 1073741824 } }),
                        },
                        {
                          label: "Quota: 5 GB", icon: "database",
                          onSelect: () => patch.mutate({ id: m.id, body: { quota_bytes: 5368709120 } }),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmDialog
        open={confirmDisable !== null}
        title="Disable this mailbox?"
        body={
          confirmDisable
            ? `${confirmDisable.address} will stop receiving mail and every issued SMTP credential for it will be revoked. Mail clients signed in with those credentials will be disconnected.`
            : ""
        }
        confirmLabel="Disable mailbox"
        danger
        onConfirm={() => {
          if (confirmDisable) patch.mutate({ id: confirmDisable.id, body: { status: "disabled" } });
          setConfirmDisable(null);
        }}
        onCancel={() => setConfirmDisable(null)}
      />
    </div>
  );
}
