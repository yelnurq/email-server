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
    <div className="space-y-6">
      <section className="mb-8">
        <h1 className="page-title">
          Mailboxes
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
          Mailboxes are the receiving desks of the platform. They inherit folders automatically and
          show capacity at a glance.
        </p>
      </section>

      {mailboxes.isLoading && <PageLoader label="Loading mailboxes" />}
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
                    <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
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
