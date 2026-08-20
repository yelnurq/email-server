"use client";

import { useQuery } from "@tanstack/react-query";
import { api, type Mailbox } from "@/lib/api";
import { EmptyState, PageLoader } from "@/components/ui";

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

export default function MailboxesPage() {
  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
  });

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">07. Inventory</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Mailboxes
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          Mailboxes are the receiving desks of the platform. They inherit folders automatically and
          show capacity at a glance.
        </p>
      </section>

      {mailboxes.isLoading && <PageLoader label="Loading mailboxes" />}
      {mailboxes.isSuccess && mailboxes.data.mailboxes.length === 0 && (
        <EmptyState title="No mailboxes yet" hint="Create a user with a mailbox first." />
      )}
      {mailboxes.isSuccess && mailboxes.data.mailboxes.length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Address</th>
                <th className="px-4 py-3 font-medium">Owner</th>
                <th className="px-4 py-3 font-medium">Usage</th>
                <th className="px-4 py-3 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {mailboxes.data.mailboxes.map((m) => (
                <tr key={m.id}>
                  <td className="px-4 py-3 font-semibold">{m.address}</td>
                  <td className="px-4 py-3 text-[#525252]">{m.user_email || "shared"}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">
                    {formatBytes(m.used_bytes)} / {formatBytes(m.quota_bytes)}
                  </td>
                  <td className="px-4 py-3">
                    <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">
                      {m.status}
                    </span>
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
