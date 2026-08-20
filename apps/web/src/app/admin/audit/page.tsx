"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, formatDate } from "@/lib/api";
import { Button, PageLoader } from "@/components/ui";

type AuditEntry = {
  id: string;
  actor: string;
  action: string;
  resource: string;
  target: string;
  metadata: string;
  created_at: string;
};

export default function AuditPage() {
  const [page, setPage] = useState(0);
  const limit = 50;

  const audit = useQuery({
    queryKey: ["admin", "audit", page],
    queryFn: () =>
      api.get<{ audit: AuditEntry[]; total: number }>(`/api/v1/audit?limit=${limit}&offset=${page * limit}`),
    refetchInterval: 15_000,
  });

  const total = audit.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / limit));

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">12. Record</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Audit Log
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          The audit record is the publication ledger. It captures who did what, when, and on which
          resource.
        </p>
      </section>

      <section className="flex flex-wrap items-center gap-3">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#525252]">
          {total} entries
        </p>
        <div className="ml-auto flex items-center gap-2">
          <Button variant="secondary" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
            Prev
          </Button>
          <span className="font-mono text-xs uppercase tracking-[0.3em] text-[#525252]">
            {page + 1} / {pages}
          </span>
          <Button variant="secondary" disabled={page >= pages - 1} onClick={() => setPage((p) => p + 1)}>
            Next
          </Button>
        </div>
      </section>

      {audit.isLoading && <PageLoader label="Loading audit log" />}
      {audit.isSuccess && (audit.data.audit ?? []).length === 0 && (
        <p className="text-sm text-[#525252] font-body">No audit entries yet.</p>
      )}
      {audit.isSuccess && (audit.data.audit ?? []).length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em] text-[#525252]">
                <th className="px-4 py-3 font-medium">When</th>
                <th className="px-4 py-3 font-medium">Actor</th>
                <th className="px-4 py-3 font-medium">Action</th>
                <th className="px-4 py-3 font-medium">Resource</th>
                <th className="px-4 py-3 font-medium">Target</th>
                <th className="px-4 py-3 font-medium">Metadata</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {(audit.data.audit ?? []).map((e) => (
                <tr key={e.id}>
                  <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-[#525252]">
                    {formatDate(e.created_at)}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs">{e.actor}</td>
                  <td className="px-4 py-3 text-sm font-semibold">{e.action}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{e.resource}</td>
                  <td className="max-w-56 truncate px-4 py-3 font-mono text-xs text-[#525252]">{e.target}</td>
                  <td className="max-w-72 truncate px-4 py-3 font-mono text-xs text-[#525252]">
                    {e.metadata}
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
