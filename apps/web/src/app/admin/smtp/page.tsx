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
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">09. Transport</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          SMTP Credentials
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          SMTP credentials are the delivery press credentials. They allow automated mail systems to
          submit on behalf of a mailbox.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">New credential</p>
        <form
          className="mt-4 flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (mailboxId) create.mutate();
          }}
        >
          <label className="block min-w-72 flex-1">
            <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
              Mailbox
            </span>
            <select
              value={mailboxId}
              onChange={(e) => setMailboxId(e.target.value)}
              className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-mono text-sm text-[#111111] outline-none"
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
        <section className="qazera-panel border-[#cc0000] bg-[#f9f9f7] p-6">
          <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Secret shown once</p>
          <dl className="mt-4 space-y-2 font-mono text-xs">
            <div className="flex gap-2"><dt className="w-20 text-[#525252]">username</dt><dd>{created.username}</dd></div>
            <div className="flex gap-2"><dt className="w-20 text-[#525252]">password</dt><dd>{created.password}</dd></div>
            <div className="flex gap-2"><dt className="w-20 text-[#525252]">mailbox</dt><dd>{created.mailbox}</dd></div>
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
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Username</th>
                <th className="px-4 py-3 font-medium">Mailbox</th>
                <th className="px-4 py-3 font-medium">Last used</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {creds.data.creds.map((c) => (
                <tr key={c.id}>
                  <td className="px-4 py-3 font-mono text-xs">{c.username}</td>
                  <td className="px-4 py-3">{c.mailbox_address}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{c.last_used_at || "—"}</td>
                  <td className="px-4 py-3">
                    <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">{c.status}</span>
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
