"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Deliverability } from "@/lib/api";
import { EmptyState, ErrorState, ListSkeleton, Segmented } from "@/components/ui";
import { Icon } from "@/components/icons";

// Metric shows one headline count with its precise definition as a tooltip
// (§70: delivered vs relayed vs accepted are never conflated).
function Metric({ label, value, hint, tone }: { label: string; value: number; hint: string; tone?: string }) {
  return (
    <div className="qazera-panel px-4 py-3.5" title={hint}>
      <p className="flex items-center gap-1 text-[11px] font-medium uppercase tracking-[.06em] text-faint">
        {label}
        <Icon name="info" className="h-3 w-3 opacity-60" />
      </p>
      <p className={`mt-1 text-2xl font-semibold tabular-nums ${tone ?? "text-foreground"}`}>{value.toLocaleString()}</p>
    </div>
  );
}

const RANGES = [
  { value: "24h", label: "24h" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
];

export default function DeliverabilityPage() {
  const [range, setRange] = useState("7d");
  const q = useQuery({
    queryKey: ["admin", "deliverability", range],
    queryFn: () => api.get<Deliverability>(`/api/v1/admin/deliverability?range=${range}`),
  });
  const d = q.data;
  const def = (k: string) => d?.definitions?.[k] ?? "";

  // Chart scale over the accepted series.
  const peak = Math.max(1, ...(d?.series ?? []).map((s) => s.accepted));

  return (
    <div className="space-y-6">
      <section className="mb-2 flex items-start justify-between gap-4">
        <div>
          <h1 className="page-title">Deliverability</h1>
          <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
            Real delivery outcomes from the platform&apos;s own records. &ldquo;Relayed&rdquo; means handed to a
            remote server&apos;s queue — remote acceptance is not the same as a user receiving the mail.
          </p>
        </div>
        <Segmented value={range} onChange={setRange} options={RANGES} />
      </section>

      {q.isLoading && <ListSkeleton rows={6} />}
      {q.isError && <ErrorState message="Could not load deliverability data." onRetry={() => q.refetch()} />}

      {d && (
        <>
          <section className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:grid-cols-5">
            <Metric label="Accepted" value={d.totals.accepted} hint={def("accepted")} />
            <Metric label="Delivered" value={d.totals.delivered_local} hint={def("delivered_local")} tone="text-success" />
            <Metric label="Relayed" value={d.totals.relayed} hint={def("relayed")} />
            <Metric label="Failed" value={d.totals.failed} hint={def("failed")} tone={d.totals.failed > 0 ? "text-danger" : undefined} />
            <Metric label="Quarantined" value={d.totals.quarantined} hint={def("quarantined")} tone={d.totals.quarantined > 0 ? "text-warning" : undefined} />
          </section>

          {d.queue && (
            <p className="text-xs text-muted-foreground">
              Live queue: <span className="font-medium text-foreground">{d.queue.total}</span> waiting
              ({d.queue.deferred} deferred). Historical counts above; the queue is the current moment.
            </p>
          )}

          <section className="qazera-panel p-5">
            <p className="text-sm font-semibold">Mail flow over time</p>
            {d.series.length === 0 ? (
              <p className="mt-4 text-sm text-muted-foreground">No activity in this window.</p>
            ) : (
              <div className="mt-4 flex items-end gap-1.5" style={{ height: 140 }}>
                {d.series.map((s) => (
                  <div key={s.bucket} className="flex flex-1 flex-col items-center gap-1" title={`${s.bucket}\naccepted ${s.accepted} · delivered ${s.delivered_local} · relayed ${s.relayed} · failed ${s.failed}`}>
                    <div className="flex w-full flex-1 items-end">
                      <div className="flex w-full flex-col justify-end gap-px" style={{ height: "100%" }}>
                        <div className="w-full rounded-t-[3px] bg-graphite" style={{ height: `${(s.accepted / peak) * 100}%`, minHeight: s.accepted > 0 ? 2 : 0 }} />
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          <div className="grid gap-5 lg:grid-cols-2">
            <section className="qazera-panel p-5">
              <p className="text-sm font-semibold">Providers (remote mail)</p>
              <p className="mt-1 text-xs text-muted-foreground">Detected by recipient MX, not the address domain.</p>
              {!d.providers || d.providers.length === 0 ? (
                <EmptyState title="Not enough data" hint="Provider breakdown appears once mail is relayed to remote domains." />
              ) : (
                <table className="mt-3 w-full text-sm">
                  <thead>
                    <tr className="border-b border-border text-left text-xs">
                      <th className="py-2 font-medium">Provider</th>
                      <th className="py-2 font-medium">Domains</th>
                      <th className="py-2 font-medium">Relayed</th>
                      <th className="py-2 font-medium">Failed</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {d.providers.map((p) => (
                      <tr key={p.provider}>
                        <td className="py-2 font-medium">{p.provider}</td>
                        <td className="py-2 tabular-nums text-muted-foreground">{p.domains}</td>
                        <td className="py-2 tabular-nums">{p.relayed}</td>
                        <td className="py-2 tabular-nums text-muted-foreground">{p.failed}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </section>

            <section className="qazera-panel p-5">
              <p className="text-sm font-semibold">Top failure reasons</p>
              {!d.top_failures || d.top_failures.length === 0 ? (
                <EmptyState icon="check-circle" title="No failures" hint="No terminal delivery failures in this window." />
              ) : (
                <ul className="mt-3 space-y-2">
                  {d.top_failures.map((f, i) => (
                    <li key={i} className="flex items-start justify-between gap-3 text-sm">
                      <span className="text-muted-foreground">{f.error}</span>
                      <span className="shrink-0 tabular-nums font-medium">{f.count}</span>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </div>
        </>
      )}
    </div>
  );
}
