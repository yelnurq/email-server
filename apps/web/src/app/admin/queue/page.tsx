"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatBytes, formatDate, type QueueMessage, type QueueView } from "@/lib/api";
import {
  Badge, Button, ConfirmDialog, Drawer, EmptyState, ErrorState,
  ListSkeleton, useToast,
} from "@/components/ui";
import { Icon } from "@/components/icons";

function StateBadge({ status }: { status: string }) {
  switch (status) {
    case "temp_fail":
      return <Badge tone="warning">Deferred</Badge>;
    case "perm_fail":
      return <Badge tone="danger">Failed</Badge>;
    case "completed":
      return <Badge tone="success">Delivered</Badge>;
    default:
      return <Badge tone="accent">Scheduled</Badge>;
  }
}

// Stat is one figure in the summary strip.
function Stat({ label, value, tone }: { label: string; value: React.ReactNode; tone?: string }) {
  return (
    <div className="qazera-panel px-4 py-3">
      <p className="text-[11px] font-medium uppercase tracking-[.06em] text-faint">{label}</p>
      <p className={`mt-1 text-xl font-semibold tabular-nums ${tone ?? "text-foreground"}`}>{value}</p>
    </div>
  );
}

export default function QueuePage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [selected, setSelected] = useState<QueueMessage | null>(null);
  const [cancelId, setCancelId] = useState<string | null>(null);

  const queue = useQuery({
    queryKey: ["admin", "queue"],
    queryFn: () => api.get<QueueView>("/api/v1/admin/queue"),
    refetchInterval: 5000,
  });

  const retry = useMutation({
    mutationFn: (id: string) => api.post(`/api/v1/admin/queue/${id}/retry`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "queue"] });
      toast("success", "Message rescheduled for immediate delivery");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Retry failed"),
  });

  const cancel = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/admin/queue/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "queue"] });
      setSelected(null);
      setCancelId(null);
      toast("success", "Message removed from the queue");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Cancel failed"),
  });

  const s = queue.data?.summary;

  return (
    <div className="space-y-6">
      <section className="mb-2 flex items-start justify-between gap-4">
        <div>
          <h1 className="page-title">Queue</h1>
          <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
            Outbound messages waiting in the mail core. Deferred messages retry automatically on
            the mail core&apos;s schedule; the verbatim remote reply explains each hold.
          </p>
        </div>
        <Button variant="secondary" size="sm" onClick={() => queue.refetch()}>
          <Icon name="refresh-cw" className="h-3.5 w-3.5" /> Refresh
        </Button>
      </section>

      {s && (
        <section className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Stat label="In queue" value={s.total} />
          <Stat label="Deferred" value={s.deferred} tone={s.deferred > 0 ? "text-warning" : undefined} />
          <Stat label="Retrying" value={s.retrying} />
          <Stat label="Next retry" value={s.next_retry ? formatDate(s.next_retry) : "—"} />
        </section>
      )}

      {queue.isLoading && <ListSkeleton rows={6} />}
      {queue.isError && <ErrorState message="Could not load the queue." onRetry={() => queue.refetch()} />}
      {queue.isSuccess && queue.data.messages.length === 0 && (
        <EmptyState icon="check-circle" title="Queue is empty" hint="No outbound messages are waiting for delivery." />
      )}

      {queue.isSuccess && queue.data.messages.length > 0 && (
        <>
          {s && s.listed < s.total && (
            <p className="text-xs text-muted-foreground">
              Showing the {s.listed} most recent of {s.total} queued messages.
            </p>
          )}
          <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border text-left text-xs">
                  <th className="px-4 py-3 font-medium">Sender</th>
                  <th className="px-4 py-3 font-medium">Recipient</th>
                  <th className="px-4 py-3 font-medium">State</th>
                  <th className="px-4 py-3 font-medium">Attempts</th>
                  <th className="px-4 py-3 font-medium">Next retry</th>
                  <th className="px-4 py-3 font-medium">Age</th>
                  <th className="px-4 py-3" />
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {queue.data.messages.map((m) => {
                  const r = m.recipients[0];
                  return (
                    <tr
                      key={m.id}
                      className="cursor-pointer"
                      onClick={() => setSelected(m)}
                    >
                      <td className="px-4 py-3 font-medium">{m.return_path || "—"}</td>
                      <td className="px-4 py-3">
                        {r?.address}
                        {m.recipients.length > 1 && (
                          <span className="ml-1 text-xs text-muted-foreground">+{m.recipients.length - 1}</span>
                        )}
                      </td>
                      <td className="px-4 py-3">{r ? <StateBadge status={r.status} /> : "—"}</td>
                      <td className="px-4 py-3 tabular-nums text-muted-foreground">{r?.retry_num ?? 0}</td>
                      <td className="px-4 py-3 text-muted-foreground">{r?.next_retry ? formatDate(r.next_retry) : "—"}</td>
                      <td className="px-4 py-3 text-muted-foreground">{formatDate(m.created)}</td>
                      <td className="px-4 py-3 text-right" onClick={(e) => e.stopPropagation()}>
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost" size="sm"
                            onClick={() => retry.mutate(m.id)}
                            disabled={retry.isPending}
                          >
                            <Icon name="play" className="h-3.5 w-3.5" /> Retry
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => setCancelId(m.id)}>
                            <Icon name="x" className="h-3.5 w-3.5" /> Cancel
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      <Drawer open={!!selected} onClose={() => setSelected(null)} title="Queued message">
        {selected && (
          <div className="space-y-5 p-5">
            <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-[13px]">
              <dt className="text-muted-foreground">Queue ID</dt>
              <dd className="font-mono text-xs">{selected.id}</dd>
              <dt className="text-muted-foreground">Sender</dt>
              <dd>{selected.return_path || "—"}</dd>
              <dt className="text-muted-foreground">Created</dt>
              <dd>{formatDate(selected.created)}</dd>
              <dt className="text-muted-foreground">Size</dt>
              <dd>{formatBytes(selected.size)}</dd>
            </dl>

            <div>
              <p className="mb-2 text-xs font-medium uppercase tracking-[.06em] text-faint">Recipients</p>
              <div className="space-y-3">
                {selected.recipients.map((r, i) => (
                  <div key={i} className="rounded-[9px] border border-border bg-background p-3">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium">{r.address}</span>
                      <StateBadge status={r.status} />
                    </div>
                    <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs text-muted-foreground">
                      <dt>Queue</dt><dd>{r.queue}</dd>
                      <dt>Attempts</dt><dd className="tabular-nums">{r.retry_num}</dd>
                      {r.next_retry && (<><dt>Next retry</dt><dd>{formatDate(r.next_retry)}</dd></>)}
                      {r.expires && (<><dt>Expires</dt><dd>{formatDate(r.expires)}</dd></>)}
                    </dl>
                    {r.status_detail && (
                      <p className="mt-2 rounded-[7px] bg-muted px-2.5 py-2 font-mono text-[11px] leading-4 text-foreground">
                        {r.status_detail}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            </div>

            <div className="flex gap-2 border-t border-border pt-4">
              <Button onClick={() => retry.mutate(selected.id)} disabled={retry.isPending}>
                <Icon name="play" className="h-3.5 w-3.5" /> Retry now
              </Button>
              <Button variant="danger" onClick={() => setCancelId(selected.id)}>
                <Icon name="x" className="h-3.5 w-3.5" /> Cancel delivery
              </Button>
            </div>
          </div>
        )}
      </Drawer>

      <ConfirmDialog
        open={!!cancelId}
        title="Cancel this delivery?"
        body="The message is removed from the outbound queue permanently and will not be delivered. This cannot be undone."
        confirmLabel="Cancel delivery"
        danger
        onConfirm={() => cancelId && cancel.mutate(cancelId)}
        onCancel={() => setCancelId(null)}
      />
    </div>
  );
}
