"use client";

import { useQuery } from "@tanstack/react-query";
import { api, type InfraReport } from "@/lib/api";
import { Badge, Button, ErrorState, ListSkeleton } from "@/components/ui";
import { Icon } from "@/components/icons";

const COMPONENT_META: Record<string, { label: string; description: string }> = {
  postgres: { label: "PostgreSQL", description: "Control-plane database: users, mail metadata, audit" },
  redis: { label: "Redis", description: "Sessions and rate limiting" },
  nats: { label: "NATS JetStream", description: "Mail event queue between API and workers" },
  minio: { label: "MinIO", description: "Attachment object storage" },
  stalwart: { label: "Stalwart", description: "Mail core: SMTP submission, IMAP and JMAP" },
  worker: { label: "Delivery worker", description: "Routes accepted mail into mailboxes" },
};

function StatusBadge({ status }: { status: string }) {
  if (status === "ok") return <Badge tone="success">Operational</Badge>;
  if (status === "disabled") return <Badge tone="neutral">Not configured</Badge>;
  return <Badge tone="danger">Unavailable</Badge>;
}

export default function InfrastructurePage() {
  const report = useQuery({
    queryKey: ["admin", "infrastructure"],
    queryFn: () => api.get<InfraReport>("/api/v1/system/infrastructure"),
    refetchInterval: 15_000,
  });

  const problems = (report.data?.components ?? []).filter((c) => c.status === "unavailable");

  return (
    <div className="space-y-6">
      <section className="mb-8 flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="page-title">Infrastructure</h1>
          <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
            Live state of every platform component. Checks run on the backend and are cached for a
            few seconds; timestamps are UTC.
          </p>
        </div>
        <Button variant="ghost" onClick={() => report.refetch()} disabled={report.isFetching}>
          <Icon name="refresh-cw" className="mr-1.5 h-3.5 w-3.5" />
          {report.isFetching ? "Checking…" : "Check now"}
        </Button>
      </section>

      {problems.length > 0 && (
        <section className="rounded-[10px] border border-danger/30 bg-danger/5 px-4 py-3 text-sm">
          <p className="font-semibold text-danger">Needs attention</p>
          <ul className="mt-1 space-y-0.5 text-muted-foreground">
            {problems.map((c) => (
              <li key={c.name}>
                {COMPONENT_META[c.name]?.label ?? c.name}: {c.detail || "did not respond to the health check"}
              </li>
            ))}
          </ul>
        </section>
      )}

      {report.isLoading && <ListSkeleton rows={6} />}
      {report.isError && (
        <ErrorState message="Could not load infrastructure state." onRetry={() => report.refetch()} />
      )}
      {report.isSuccess && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">Component</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Latency</th>
                <th className="px-4 py-3 font-medium">Version</th>
                <th className="px-4 py-3 font-medium">Detail</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {report.data.components.map((c) => {
                const meta = COMPONENT_META[c.name] ?? { label: c.name, description: "" };
                return (
                  <tr key={c.name}>
                    <td className="px-4 py-3">
                      <p className="font-semibold">{meta.label}</p>
                      <p className="text-xs text-muted-foreground">{meta.description}</p>
                    </td>
                    <td className="px-4 py-3"><StatusBadge status={c.status} /></td>
                    <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                      {c.status === "ok" ? `${c.latency_ms} ms` : "—"}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{c.version || "—"}</td>
                    <td className="max-w-md px-4 py-3 text-xs text-muted-foreground">
                      <span className="line-clamp-2">{c.detail || "—"}</span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <p className="border-t border-border px-4 py-2 text-[11px] text-faint">
            Last checked {report.data.checked_at}
          </p>
        </div>
      )}
    </div>
  );
}
