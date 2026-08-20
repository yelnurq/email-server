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
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">06. Security</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Security Center
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          Quarantine is the newsroom gatekeeper. It holds risky mail outside the inbox until a
          human authorizes release or deletion.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">
          Quarantine ({pending.length} pending)
        </p>
        {quarantine.isLoading && <PageLoader label="Loading quarantine" />}
        {quarantine.isSuccess && pending.length === 0 && (
          <EmptyState title="Quarantine is empty" hint="Nothing is being held right now." />
        )}
        {pending.length > 0 && (
          <div className="mt-4 overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                  <th className="px-4 py-3 font-medium">From</th>
                  <th className="px-4 py-3 font-medium">Subject</th>
                  <th className="px-4 py-3 font-medium">To</th>
                  <th className="px-4 py-3 font-medium">Risk</th>
                  <th className="px-4 py-3 font-medium">Signals</th>
                  <th className="px-4 py-3 font-medium">When</th>
                  <th className="px-4 py-3 font-medium"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#111111]">
                {pending.map((q) => (
                  <tr key={q.id}>
                    <td className="px-4 py-3 font-semibold">{q.from}</td>
                    <td className="max-w-56 truncate px-4 py-3">{q.subject}</td>
                    <td className="px-4 py-3 text-[#525252]">{q.recipient_address}</td>
                    <td className="px-4 py-3">
                      <span className="border border-[#111111] bg-[#cc0000] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em] text-white">
                        {q.risk_score}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-[#525252]">{q.signals.join(", ")}</td>
                    <td className="px-4 py-3 text-xs text-[#525252]">{formatDate(q.created_at)}</td>
                    <td className="whitespace-nowrap px-4 py-3 text-right">
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
          <p className="mt-3 text-xs font-mono uppercase tracking-[0.3em] text-[#525252]">
            {resolved.length} resolved item(s) in history.
          </p>
        )}
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Sender blocks</p>
        <form
          className="mt-4 flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (blockPattern.trim()) addBlock.mutate();
          }}
        >
          <div className="min-w-64 flex-1">
            <Input
              label="Address or domain"
              placeholder="spammer@evil.test or evil.test"
              value={blockPattern}
              onChange={(e) => setBlockPattern(e.target.value)}
            />
          </div>
          <div className="min-w-64 flex-1">
            <Input label="Reason" value={blockReason} onChange={(e) => setBlockReason(e.target.value)} />
          </div>
          <Button type="submit" disabled={addBlock.isPending || !blockPattern.trim()}>
            Block
          </Button>
        </form>

        {blocks.isSuccess && blocks.data.blocks.length === 0 && (
          <p className="mt-4 text-sm text-[#525252] font-body">No blocked senders.</p>
        )}
        {blocks.isSuccess && blocks.data.blocks.length > 0 && (
          <ul className="mt-4 space-y-2">
            {blocks.data.blocks.map((b) => (
              <li
                key={b.id}
                className="flex flex-wrap items-center gap-3 border border-[#111111] bg-[#f9f9f7] px-4 py-3 text-sm"
              >
                <span className="font-mono text-xs uppercase tracking-[0.3em]">{b.pattern}</span>
                <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">
                  {b.kind}
                </span>
                {b.reason && <span className="text-xs text-[#525252]">{b.reason}</span>}
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
