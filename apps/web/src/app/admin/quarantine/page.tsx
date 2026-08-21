"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatDate, type QuarantineItem } from "@/lib/api";
import {
  Badge, Button, ConfirmDialog, Drawer, EmptyState, ErrorState,
  ListSkeleton, useToast,
} from "@/components/ui";
import { Icon } from "@/components/icons";

function ReasonBadge({ reason }: { reason: string }) {
  const tone = reason === "malware" ? "danger" : reason === "spam" ? "warning" : "neutral";
  return <Badge tone={tone as "danger" | "warning" | "neutral"}>{reason || "policy"}</Badge>;
}

function StatusBadge({ status }: { status: string }) {
  switch (status) {
    case "released": return <Badge tone="success">Released</Badge>;
    case "deleted": return <Badge tone="neutral">Deleted</Badge>;
    case "pending_scan": return <Badge tone="accent">Pending scan</Badge>;
    default: return <Badge tone="warning">Held</Badge>;
  }
}

export default function QuarantinePage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [selected, setSelected] = useState<QuarantineItem | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const q = useQuery({
    queryKey: ["admin", "quarantine"],
    queryFn: () => api.get<{ quarantine: QuarantineItem[] }>("/api/v1/quarantine"),
    refetchInterval: 10000,
  });

  const release = useMutation({
    mutationFn: (id: string) => api.post(`/api/v1/quarantine/${id}/release`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "quarantine"] });
      setSelected(null);
      toast("success", "Message released to the recipient's inbox");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Release failed"),
  });

  const del = useMutation({
    mutationFn: (id: string) => api.post(`/api/v1/quarantine/${id}/delete`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "quarantine"] });
      setSelected(null);
      setDeleteId(null);
      toast("success", "Message deleted");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Delete failed"),
  });

  const items = q.data?.quarantine ?? [];
  const pending = items.filter((i) => i.status === "pending" || i.status === "pending_scan");

  return (
    <div className="space-y-6">
      <section className="mb-2">
        <h1 className="page-title">Quarantine</h1>
        <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
          Messages withheld by security policy. Releasing delivers exactly one copy to the recipient
          and keeps the original verdict on record; both actions are audited.
        </p>
      </section>

      {q.isLoading && <ListSkeleton rows={6} />}
      {q.isError && <ErrorState message="Could not load quarantine." onRetry={() => q.refetch()} />}
      {q.isSuccess && items.length === 0 && (
        <EmptyState icon="shield-check" title="Nothing in quarantine" hint="No messages are currently held by security policy." />
      )}

      {q.isSuccess && items.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">When</th>
                <th className="px-4 py-3 font-medium">Sender</th>
                <th className="px-4 py-3 font-medium">Recipient</th>
                <th className="px-4 py-3 font-medium">Subject</th>
                <th className="px-4 py-3 font-medium">Risk</th>
                <th className="px-4 py-3 font-medium">Reason</th>
                <th className="px-4 py-3 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {items.map((it) => (
                <tr key={it.id} className="cursor-pointer" onClick={() => setSelected(it)}>
                  <td className="px-4 py-3 whitespace-nowrap text-muted-foreground">{formatDate(it.created_at)}</td>
                  <td className="px-4 py-3">{it.from}</td>
                  <td className="px-4 py-3 text-muted-foreground">{it.recipient_address}</td>
                  <td className="px-4 py-3 max-w-64 truncate font-medium">{it.subject || "(no subject)"}</td>
                  <td className="px-4 py-3 tabular-nums">{it.risk_score}</td>
                  <td className="px-4 py-3"><ReasonBadge reason={it.reason} /></td>
                  <td className="px-4 py-3"><StatusBadge status={it.status} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {items.length > 0 && (
        <p className="text-xs text-muted-foreground">{pending.length} held · {items.length - pending.length} resolved</p>
      )}

      <Drawer open={!!selected} onClose={() => setSelected(null)} title="Quarantined message">
        {selected && (
          <div className="space-y-5 p-5">
            <div className="flex items-center gap-2">
              <ReasonBadge reason={selected.reason} />
              <StatusBadge status={selected.status} />
              <span className="text-xs text-muted-foreground">risk {selected.risk_score}</span>
            </div>

            {/* Safe preview: headers only, no HTML body render, no remote
                resources — the quarantined content is never executed (§54). */}
            <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-[13px]">
              <dt className="text-muted-foreground">Subject</dt>
              <dd className="font-medium">{selected.subject || "(no subject)"}</dd>
              <dt className="text-muted-foreground">From</dt>
              <dd>{selected.from}</dd>
              <dt className="text-muted-foreground">To</dt>
              <dd>{selected.recipient_address}</dd>
              <dt className="text-muted-foreground">Received</dt>
              <dd>{formatDate(selected.created_at)}</dd>
            </dl>

            <div>
              <p className="mb-2 text-xs font-medium uppercase tracking-[.06em] text-faint">Security signals</p>
              <div className="flex flex-wrap gap-1.5">
                {selected.signals.length === 0
                  ? <span className="text-sm text-muted-foreground">—</span>
                  : selected.signals.map((s, i) => (
                      <span key={i} className="rounded-full border border-border bg-background px-2 py-0.5 font-mono text-[11px] text-muted-foreground">{s}</span>
                    ))}
              </div>
            </div>

            <p className="rounded-[7px] border border-border bg-background px-3 py-2 text-xs leading-5 text-muted-foreground">
              <Icon name="shield" className="mr-1 inline h-3.5 w-3.5" />
              Content is shown as metadata only. The message body and any attachments are never
              rendered or executed from this view.
            </p>

            {(selected.status === "pending" || selected.status === "pending_scan") && (
              <div className="flex gap-2 border-t border-border pt-4">
                <Button onClick={() => release.mutate(selected.id)} disabled={release.isPending}>
                  <Icon name="check" className="h-3.5 w-3.5" /> Release to inbox
                </Button>
                <Button variant="danger" onClick={() => setDeleteId(selected.id)}>
                  <Icon name="trash" className="h-3.5 w-3.5" /> Delete
                </Button>
              </div>
            )}
          </div>
        )}
      </Drawer>

      <ConfirmDialog
        open={!!deleteId}
        title="Delete this message?"
        body="The quarantined message is permanently deleted and never delivered. This is audited and cannot be undone."
        confirmLabel="Delete permanently"
        danger
        onConfirm={() => deleteId && del.mutate(deleteId)}
        onCancel={() => setDeleteId(null)}
      />
    </div>
  );
}
