"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, formatDate } from "@/lib/api";
import { Button, EmptyState, PageLoader } from "@/components/ui";

type AuditEntry = {
  id: number;
  actor_email?: string;
  action: string;
  resource_type?: string;
  resource_id?: string;
  detail: Record<string, unknown>;
  ip?: string;
  created_at: string;
};

const PAGE_SIZE = 50;

export default function AuditPage() {
  const [page, setPage] = useState(0);
  const audit = useQuery({
    queryKey: ["admin", "audit", page],
    queryFn: () =>
      api.get<{ entries: AuditEntry[]; total: number }>(
        `/api/v1/audit?limit=${PAGE_SIZE}&offset=${page * PAGE_SIZE}`,
      ),
  });

  const pages = Math.max(1, Math.ceil((audit.data?.total ?? 0) / PAGE_SIZE));

  return (
    <div className="mx-auto max-w-5xl">
      <div className="mb-4 flex items-center gap-3">
        <h1 className="text-lg font-semibold">Audit Log</h1>
        <span className="text-xs text-neutral-400">{audit.data?.total ?? 0} entries</span>
        {pages > 1 && (
          <div className="ml-auto flex items-center gap-2 text-xs text-neutral-500">
            <Button variant="ghost" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>←</Button>
            {page + 1} / {pages}
            <Button variant="ghost" disabled={page >= pages - 1} onClick={() => setPage((p) => p + 1)}>→</Button>
          </div>
        )}
      </div>

      {audit.isLoading && <PageLoader />}
      {audit.isSuccess && audit.data.entries.length === 0 && <EmptyState title="No audit entries" />}
      {audit.isSuccess && audit.data.entries.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Time</th>
                <th className="px-4 py-2.5 font-medium">Actor</th>
                <th className="px-4 py-2.5 font-medium">Action</th>
                <th className="px-4 py-2.5 font-medium">Resource</th>
                <th className="px-4 py-2.5 font-medium">Detail</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {audit.data.entries.map((e) => (
                <tr key={e.id}>
                  <td className="whitespace-nowrap px-4 py-2 text-xs text-neutral-500">
                    {formatDate(e.created_at)}
                  </td>
                  <td className="px-4 py-2 text-xs">{e.actor_email || "system"}</td>
                  <td className="px-4 py-2">
                    <code className="rounded bg-neutral-100 px-1.5 py-0.5 text-xs dark:bg-neutral-800">
                      {e.action}
                    </code>
                  </td>
                  <td className="px-4 py-2 text-xs text-neutral-500">
                    {e.resource_type && `${e.resource_type}`}
                  </td>
                  <td className="max-w-64 truncate px-4 py-2 text-xs text-neutral-400">
                    {Object.keys(e.detail).length > 0 ? JSON.stringify(e.detail) : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
