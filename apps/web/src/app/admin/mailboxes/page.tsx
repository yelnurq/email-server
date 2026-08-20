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
    <div className="mx-auto max-w-5xl">
      <h1 className="mb-1 text-lg font-semibold">Mailboxes</h1>
      <p className="mb-4 text-xs text-neutral-400">
        Mailboxes are provisioned from the Users page (or via API). Each mailbox gets Inbox, Sent,
        Drafts, Spam and Trash folders automatically.
      </p>

      {mailboxes.isLoading && <PageLoader />}
      {mailboxes.isSuccess && mailboxes.data.mailboxes.length === 0 && (
        <EmptyState title="No mailboxes yet" hint="Create a user with a mailbox first." />
      )}
      {mailboxes.isSuccess && mailboxes.data.mailboxes.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Address</th>
                <th className="px-4 py-2.5 font-medium">Owner</th>
                <th className="px-4 py-2.5 font-medium">Usage</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {mailboxes.data.mailboxes.map((m) => (
                <tr key={m.id}>
                  <td className="px-4 py-2.5 font-medium">{m.address}</td>
                  <td className="px-4 py-2.5 text-neutral-500">{m.user_email || "shared"}</td>
                  <td className="px-4 py-2.5 text-neutral-500">
                    {formatBytes(m.used_bytes)} / {formatBytes(m.quota_bytes)}
                  </td>
                  <td className="px-4 py-2.5">
                    <span
                      className={
                        m.status === "active"
                          ? "rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                          : "rounded-full bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-neutral-800"
                      }
                    >
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
