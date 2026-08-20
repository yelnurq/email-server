"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Mailbox } from "@/lib/api";
import { Button, EmptyState, PageLoader, useToast } from "@/components/ui";

type Credential = {
  id: string;
  username: string;
  mailbox_address: string;
  last_used_at: string | null;
  status: string;
};

export default function SmtpPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [mailboxId, setMailboxId] = useState("");
  const [created, setCreated] = useState<{ username: string; password: string; mailbox: string } | null>(null);

  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
  });
  const creds = useQuery({
    queryKey: ["admin", "smtp-creds"],
    queryFn: () => api.get<{ creds: Credential[] }>("/api/v1/smtp-credentials"),
  });

  const create = useMutation({
    mutationFn: () => api.post<{ username: string; password: string; mailbox: string }>("/api/v1/smtp-credentials", { mailbox_id: mailboxId }),
    onSuccess: (res) => {
      setMailboxId("");
      setCreated(res);
      qc.invalidateQueries({ queryKey: ["admin", "smtp-creds"] });
      toast("success", "SMTP credential created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create credential"),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/smtp-credentials/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "smtp-creds"] });
      toast("success", "SMTP credential revoked");
    },
  });

  return (
    <div className="space-y-6">
      <section className="mb-8">
        <h1 className="page-title">
          SMTP Credentials
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
          SMTP credentials are the delivery press credentials. They allow automated mail systems to
          submit on behalf of a mailbox.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="text-base font-semibold text-foreground">New credential</p>
        <form
          className="mt-4 flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (mailboxId) create.mutate();
          }}
        >
          <label className="block min-w-72 flex-1">
            <span className="mb-2 block text-xs font-medium text-muted-foreground">
              Mailbox
            </span>
            <select
              value={mailboxId}
              onChange={(e) => setMailboxId(e.target.value)}
              className="w-full border-b-2 border-border bg-transparent px-3 py-2 font-mono text-sm text-foreground outline-none"
            >
              {(mailboxes.data?.mailboxes ?? []).map((m) => (
                <option key={m.id} value={m.id}>{m.address}</option>
              ))}
            </select>
          </label>
          <Button type="submit" disabled={create.isPending || (mailboxes.data?.mailboxes.length ?? 0) === 0}>
            Create credential
          </Button>
        </form>
      </section>

      {created && (
        <section className="qazera-panel border-danger bg-surface-elevated p-6">
          <p className="text-base font-semibold text-foreground">Secret shown once</p>
          <dl className="mt-4 space-y-2 font-mono text-xs">
            <div className="flex gap-2"><dt className="w-20 text-muted-foreground">username</dt><dd>{created.username}</dd></div>
            <div className="flex gap-2"><dt className="w-20 text-muted-foreground">password</dt><dd>{created.password}</dd></div>
            <div className="flex gap-2"><dt className="w-20 text-muted-foreground">mailbox</dt><dd>{created.mailbox}</dd></div>
          </dl>
          <div className="mt-4">
            <Button variant="secondary" onClick={() => setCreated(null)}>Done</Button>
          </div>
        </section>
      )}

      {creds.isLoading && <PageLoader label="Loading SMTP credentials" />}
      {creds.isSuccess && creds.data.creds.length === 0 && (
        <EmptyState title="No SMTP credentials yet" />
      )}
      {creds.isSuccess && creds.data.creds.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">Username</th>
                <th className="px-4 py-3 font-medium">Mailbox</th>
                <th className="px-4 py-3 font-medium">Last used</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {creds.data.creds.map((c) => (
                <tr key={c.id}>
                  <td className="px-4 py-3 font-mono text-xs">{c.username}</td>
                  <td className="px-4 py-3">{c.mailbox_address}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{c.last_used_at || "—"}</td>
                  <td className="px-4 py-3">
                    <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">{c.status}</span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="ghost" onClick={() => revoke.mutate(c.id)}>Revoke</Button>
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
