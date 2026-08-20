"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatDate, type Mailbox } from "@/lib/api";
import { Button, EmptyState, PageLoader, useToast } from "@/components/ui";

type SmtpCred = {
  id: string;
  organization_id: string;
  mailbox_address: string;
  username: string;
  status: string;
  last_used_at?: string;
  created_at: string;
};

type CreatedCred = { id: string; mailbox: string; username: string; password: string };

export default function SmtpPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [mailboxId, setMailboxId] = useState("");
  const [created, setCreated] = useState<CreatedCred | null>(null);

  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
  });
  const creds = useQuery({
    queryKey: ["admin", "smtp-credentials"],
    queryFn: () => api.get<{ smtp_credentials: SmtpCred[] }>("/api/v1/smtp-credentials"),
  });

  const create = useMutation({
    mutationFn: () =>
      api.post<CreatedCred>("/api/v1/smtp-credentials", {
        mailbox_id: mailboxId || mailboxes.data?.mailboxes[0]?.id,
      }),
    onSuccess: (c) => {
      setCreated(c);
      qc.invalidateQueries({ queryKey: ["admin", "smtp-credentials"] });
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create credential"),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/smtp-credentials/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "smtp-credentials"] });
      toast("success", "Credential revoked");
    },
  });

  return (
    <div className="mx-auto max-w-4xl">
      <h1 className="mb-1 text-lg font-semibold">SMTP Credentials</h1>
      <p className="mb-4 text-xs text-neutral-400">
        Credentials for SMTP submission (port 587). The SMTP endpoint itself ships with the mail-core
        integration phase; issue and manage credentials now.
      </p>

      <form
        className="mb-4 flex flex-wrap items-end gap-2 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">Mailbox</span>
          <select
            value={mailboxId}
            onChange={(e) => setMailboxId(e.target.value)}
            className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-900"
          >
            {(mailboxes.data?.mailboxes ?? []).map((m) => (
              <option key={m.id} value={m.id}>{m.address}</option>
            ))}
          </select>
        </label>
        <Button type="submit" disabled={create.isPending || (mailboxes.data?.mailboxes.length ?? 0) === 0}>
          {create.isPending ? "Issuing…" : "Issue credential"}
        </Button>
      </form>

      {created && (
        <div className="mb-4 rounded-2xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-700 dark:bg-amber-950">
          <p className="text-sm font-semibold text-amber-800 dark:text-amber-200">
            Copy the password now — it will not be shown again.
          </p>
          <dl className="mt-2 space-y-1 font-mono text-xs">
            <div className="flex gap-2"><dt className="w-20 text-neutral-500">username</dt><dd>{created.username}</dd></div>
            <div className="flex gap-2"><dt className="w-20 text-neutral-500">password</dt><dd>{created.password}</dd></div>
            <div className="flex gap-2"><dt className="w-20 text-neutral-500">mailbox</dt><dd>{created.mailbox}</dd></div>
          </dl>
          <div className="mt-2 flex gap-2">
            <Button
              variant="secondary"
              onClick={() => {
                navigator.clipboard.writeText(`${created.username}\n${created.password}`);
                toast("success", "Copied");
              }}
            >
              Copy
            </Button>
            <Button variant="ghost" onClick={() => setCreated(null)}>Done</Button>
          </div>
        </div>
      )}

      {creds.isLoading && <PageLoader />}
      {creds.isSuccess && creds.data.smtp_credentials.length === 0 && (
        <EmptyState title="No SMTP credentials yet" />
      )}
      {creds.isSuccess && creds.data.smtp_credentials.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Username</th>
                <th className="px-4 py-2.5 font-medium">Mailbox</th>
                <th className="px-4 py-2.5 font-medium">Last used</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
                <th className="px-4 py-2.5 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {creds.data.smtp_credentials.map((c) => (
                <tr key={c.id}>
                  <td className="px-4 py-2.5 font-mono text-xs">{c.username}</td>
                  <td className="px-4 py-2.5">{c.mailbox_address}</td>
                  <td className="px-4 py-2.5 text-xs text-neutral-500">
                    {c.last_used_at ? formatDate(c.last_used_at) : "never"}
                  </td>
                  <td className="px-4 py-2.5">
                    <span
                      className={
                        c.status === "active"
                          ? "rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                          : "rounded-full bg-red-50 px-2 py-0.5 text-xs font-medium text-red-600 dark:bg-red-950 dark:text-red-300"
                      }
                    >
                      {c.status}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    {c.status === "active" && (
                      <Button variant="ghost" onClick={() => revoke.mutate(c.id)}>
                        Revoke
                      </Button>
                    )}
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
