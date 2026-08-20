"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatDate } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type QuarantineItem = {
  id: string;
  message_id: string;
  from: string;
  subject: string;
  recipient_address: string;
  reason: string;
  signals: string[];
  risk_score: number;
  status: string;
  created_at: string;
};

type SenderBlock = {
  id: string;
  pattern: string;
  kind: string;
  reason: string;
  created_at: string;
};

export default function SecurityPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [blockPattern, setBlockPattern] = useState("");
  const [blockReason, setBlockReason] = useState("");

  const quarantine = useQuery({
    queryKey: ["admin", "quarantine"],
    queryFn: () => api.get<{ quarantine: QuarantineItem[] }>("/api/v1/quarantine"),
    refetchInterval: 10_000,
  });
  const blocks = useQuery({
    queryKey: ["admin", "security-blocks"],
    queryFn: () => api.get<{ blocks: SenderBlock[] }>("/api/v1/security/blocks"),
  });

  const act = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "release" | "delete" }) =>
      api.post(`/api/v1/quarantine/${id}/${action}`),
    onSuccess: (_d, v) => {
      qc.invalidateQueries({ queryKey: ["admin", "quarantine"] });
      toast("success", v.action === "release" ? "Released to inbox" : "Deleted");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Action failed"),
  });

  const addBlock = useMutation({
    mutationFn: () =>
      api.post("/api/v1/security/blocks", { pattern: blockPattern, reason: blockReason }),
    onSuccess: () => {
      setBlockPattern("");
      setBlockReason("");
      qc.invalidateQueries({ queryKey: ["admin", "security-blocks"] });
      toast("success", "Sender blocked");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not add block"),
  });

  const removeBlock = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/security/blocks/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "security-blocks"] });
      toast("success", "Block removed");
    },
  });

  const blockSender = useMutation({
    mutationFn: (address: string) =>
      api.post("/api/v1/security/blocks", { pattern: address, reason: "blocked from quarantine" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "security-blocks"] });
      toast("success", "Sender blocked");
    },
  });

  const pending = (quarantine.data?.quarantine ?? []).filter((q) => q.status === "pending");
  const resolved = (quarantine.data?.quarantine ?? []).filter((q) => q.status !== "pending");

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <section>
        <h1 className="mb-1 text-lg font-semibold">Security Center</h1>
        <p className="mb-4 text-xs text-neutral-400">
          Quarantined mail is held here and never reaches the recipient until released.
          Risk bands: 0–40 allow · 41–60 spam folder · 61+ quarantine.
        </p>

        <h2 className="mb-2 text-sm font-semibold">Quarantine ({pending.length} pending)</h2>
        {quarantine.isLoading && <PageLoader />}
        {quarantine.isSuccess && pending.length === 0 && (
          <EmptyState title="Quarantine is empty" hint="Nothing is being held right now." />
        )}
        {pending.length > 0 && (
          <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                  <th className="px-4 py-2.5 font-medium">From</th>
                  <th className="px-4 py-2.5 font-medium">Subject</th>
                  <th className="px-4 py-2.5 font-medium">To</th>
                  <th className="px-4 py-2.5 font-medium">Risk</th>
                  <th className="px-4 py-2.5 font-medium">Signals</th>
                  <th className="px-4 py-2.5 font-medium">When</th>
                  <th className="px-4 py-2.5 font-medium"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
                {pending.map((q) => (
                  <tr key={q.id}>
                    <td className="px-4 py-2.5">{q.from}</td>
                    <td className="max-w-56 truncate px-4 py-2.5">{q.subject}</td>
                    <td className="px-4 py-2.5 text-neutral-500">{q.recipient_address}</td>
                    <td className="px-4 py-2.5">
                      <span className="rounded-full bg-red-50 px-2 py-0.5 text-xs font-semibold text-red-600 dark:bg-red-950 dark:text-red-300">
                        {q.risk_score}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-xs text-neutral-500">{q.signals.join(", ")}</td>
                    <td className="px-4 py-2.5 text-xs text-neutral-500">{formatDate(q.created_at)}</td>
                    <td className="whitespace-nowrap px-4 py-2.5 text-right">
                      <Button variant="ghost" onClick={() => act.mutate({ id: q.id, action: "release" })}>
                        Release
                      </Button>
                      <Button variant="ghost" onClick={() => act.mutate({ id: q.id, action: "delete" })}>
                        Delete
                      </Button>
                      <Button variant="ghost" onClick={() => blockSender.mutate(q.from)}>
                        Block sender
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {resolved.length > 0 && (
          <p className="mt-2 text-xs text-neutral-400">{resolved.length} resolved item(s) in history.</p>
        )}
      </section>

      <section>
        <h2 className="mb-2 text-sm font-semibold">Sender blocks</h2>
        <form
          className="mb-3 flex flex-wrap items-end gap-2 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
          onSubmit={(e) => {
            e.preventDefault();
            if (blockPattern.trim()) addBlock.mutate();
          }}
        >
          <div className="w-64">
            <Input
              label="Address or domain"
              placeholder="spammer@evil.test or evil.test"
              value={blockPattern}
              onChange={(e) => setBlockPattern(e.target.value)}
            />
          </div>
          <div className="min-w-48 flex-1">
            <Input label="Reason" value={blockReason} onChange={(e) => setBlockReason(e.target.value)} />
          </div>
          <Button type="submit" disabled={addBlock.isPending || !blockPattern.trim()}>
            Block
          </Button>
        </form>
        {blocks.isSuccess && blocks.data.blocks.length === 0 && (
          <p className="text-sm text-neutral-400">No blocked senders.</p>
        )}
        {blocks.isSuccess && blocks.data.blocks.length > 0 && (
          <ul className="space-y-1">
            {blocks.data.blocks.map((b) => (
              <li
                key={b.id}
                className="flex items-center gap-3 rounded-xl border border-neutral-200 bg-white px-4 py-2 text-sm dark:border-neutral-800 dark:bg-neutral-900"
              >
                <span className="font-mono">{b.pattern}</span>
                <span className="rounded-full bg-neutral-100 px-2 py-0.5 text-xs text-neutral-500 dark:bg-neutral-800">
                  {b.kind}
                </span>
                {b.reason && <span className="text-xs text-neutral-400">{b.reason}</span>}
                <Button variant="ghost" className="ml-auto" onClick={() => removeBlock.mutate(b.id)}>
                  Unblock
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
