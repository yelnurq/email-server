"use client";

import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  api, formatBytes, formatDate,
  type MessageTrace, type TraceListItem,
} from "@/lib/api";
import { Badge, Button, EmptyState, Drawer, ErrorState, Input, ListSkeleton, PageLoader } from "@/components/ui";
import { Icon } from "@/components/icons";

const STATUS_TONE: Record<string, "success" | "warning" | "danger" | "neutral"> = {
  delivered: "success",
  partially_delivered: "warning",
  quarantined: "warning",
  failed: "danger",
  accepted: "neutral",
  processing: "neutral",
};

// Human labels for real pipeline events; unknown types fall back to the raw
// event name so nothing is hidden.
const EVENT_LABELS: Record<string, string> = {
  "email.accepted": "Accepted for delivery",
  "email.delivered_local": "Delivered to mailbox",
  "email.failed": "Delivery failed",
  "email.quarantined": "Held in quarantine",
};

function StatusBadge({ status }: { status: string }) {
  return <Badge tone={STATUS_TONE[status] ?? "neutral"}>{status.replace("_", " ")}</Badge>;
}

function TraceDrawer({ publicID, onClose }: { publicID: string; onClose: () => void }) {
  const trace = useQuery({
    queryKey: ["admin", "trace", publicID],
    queryFn: () => api.get<MessageTrace>(`/api/v1/admin/messages/${publicID}/trace`),
  });

  return (
    <Drawer open onClose={onClose} title={<span className="font-mono text-xs">{publicID}</span>} width="max-w-2xl">
      <div className="space-y-5 p-4">
        {trace.isLoading && <PageLoader label="Loading trace" />}
        {trace.isError && <ErrorState message="Could not load the message trace." onRetry={() => trace.refetch()} />}
        {trace.isSuccess && (
          <>
            <section>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold">{trace.data.subject || "(no subject)"}</p>
                  <p className="mt-0.5 text-xs text-muted-foreground">
                    {trace.data.from_display ? `${trace.data.from_display} · ` : ""}{trace.data.from}
                  </p>
                </div>
                <StatusBadge status={trace.data.status} />
              </div>
              <p className="mt-2 font-mono text-[11px] text-faint">
                {formatBytes(trace.data.size_bytes)}
                {trace.data.has_attachments ? " · attachments" : ""} · accepted {trace.data.created_at}
              </p>
            </section>

            <section>
              <p className="mb-2 text-xs font-medium uppercase tracking-[.08em] text-faint">Recipients</p>
              <div className="overflow-hidden rounded-[8px] border border-border">
                <table className="w-full text-xs">
                  <tbody className="divide-y divide-border">
                    {trace.data.recipients.map((r, i) => (
                      <tr key={i}>
                        <td className="px-3 py-2 font-mono uppercase text-faint">{r.kind}</td>
                        <td className="px-3 py-2 font-medium">{r.address}</td>
                        <td className="px-3 py-2"><StatusBadge status={r.status} /></td>
                        <td className="px-3 py-2 text-muted-foreground">{r.error || (r.mailbox ? `→ ${r.mailbox}` : "")}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>

            <section>
              <p className="mb-2 text-xs font-medium uppercase tracking-[.08em] text-faint">Timeline</p>
              <ol className="space-y-0">
                {trace.data.events.map((e, i) => (
                  <li key={i} className="relative flex gap-3 pb-4 last:pb-0">
                    {i < trace.data.events.length - 1 && (
                      <span className="absolute left-[5px] top-4 h-full w-px bg-border" aria-hidden />
                    )}
                    <span className="relative mt-1.5 h-[11px] w-[11px] shrink-0 rounded-full border-2 border-graphite bg-surface-elevated" aria-hidden />
                    <div className="min-w-0">
                      <p className="text-[13px] font-medium leading-5">{EVENT_LABELS[e.type] ?? e.type}</p>
                      <p className="font-mono text-[11px] text-faint">{e.created_at}</p>
                      {Object.keys(e.detail ?? {}).length > 0 && (
                        <p className="mt-0.5 break-all font-mono text-[11px] text-muted-foreground">
                          {JSON.stringify(e.detail)}
                        </p>
                      )}
                    </div>
                  </li>
                ))}
              </ol>
            </section>
          </>
        )}
      </div>
    </Drawer>
  );
}

function TraceContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [q, setQ] = useState(searchParams.get("q") ?? "");
  const [query, setQuery] = useState(q);
  const [status, setStatus] = useState("");
  const selected = searchParams.get("m");

  const list = useQuery({
    queryKey: ["admin", "trace-list", query, status],
    queryFn: () => {
      const params = new URLSearchParams({ limit: "50" });
      if (query) params.set("q", query);
      if (status) params.set("status", status);
      return api.get<{ messages: TraceListItem[]; total: number }>(`/api/v1/admin/messages?${params}`);
    },
  });

  const select = (id: string | null) => {
    const params = new URLSearchParams(searchParams.toString());
    if (id) params.set("m", id); else params.delete("m");
    router.replace(`/admin/trace?${params.toString()}`);
  };

  return (
    <div className="space-y-6">
      <section className="mb-8">
        <h1 className="page-title">Message Trace</h1>
        <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
          Follow any accepted message through the pipeline: recipients, routing outcome and the
          recorded event timeline. Search by message ID, sender, recipient or subject.
        </p>
      </section>

      <form
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => { e.preventDefault(); setQuery(q.trim()); }}
      >
        <div className="min-w-72 flex-1">
          <Input
            label="Search"
            placeholder="msg_…, user@company.test or subject"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <label className="block min-w-44">
          <span className="mb-2 block text-xs font-medium text-muted-foreground">Status</span>
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value)}
            className="w-full border-b-2 border-border bg-transparent px-3 py-2 font-mono text-sm text-foreground outline-none"
          >
            <option value="">All</option>
            {["accepted", "processing", "delivered", "partially_delivered", "failed", "quarantined"].map((s) => (
              <option key={s} value={s}>{s.replace("_", " ")}</option>
            ))}
          </select>
        </label>
        <Button type="submit" disabled={list.isFetching}>
          <Icon name="search" className="mr-1.5 h-3.5 w-3.5" /> Search
        </Button>
      </form>

      {list.isLoading && <ListSkeleton rows={8} />}
      {list.isError && <ErrorState message="Could not load messages." onRetry={() => list.refetch()} />}
      {list.isSuccess && list.data.messages.length === 0 && (
        <EmptyState title="No messages match" hint="Try a different message ID, address or status filter." />
      )}
      {list.isSuccess && list.data.messages.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">Message</th>
                <th className="px-4 py-3 font-medium">From</th>
                <th className="px-4 py-3 font-medium">Recipients</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Accepted</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {list.data.messages.map((m) => (
                <tr
                  key={m.message_id}
                  className="cursor-pointer transition-colors hover:bg-muted/60"
                  onClick={() => select(m.message_id)}
                >
                  <td className="max-w-sm px-4 py-3">
                    <p className="truncate font-semibold">{m.subject || "(no subject)"}</p>
                    <p className="truncate font-mono text-[11px] text-faint">{m.message_id}</p>
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">{m.from}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{m.recipients}</td>
                  <td className="px-4 py-3"><StatusBadge status={m.status} /></td>
                  <td className="px-4 py-3 text-xs text-muted-foreground">{formatDate(m.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="border-t border-border px-4 py-2 text-[11px] text-faint">
            {list.data.total} message{list.data.total === 1 ? "" : "s"} · showing latest {list.data.messages.length}
          </p>
        </div>
      )}

      {selected && <TraceDrawer publicID={selected} onClose={() => select(null)} />}
    </div>
  );
}

export default function TracePage() {
  return (
    <Suspense fallback={<PageLoader label="Loading Message Trace" />}>
      <TraceContent />
    </Suspense>
  );
}
