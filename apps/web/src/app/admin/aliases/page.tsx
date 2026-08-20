"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Domain, type Mailbox } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type Alias = {
  id: string;
  organization_id: string;
  address: string;
  status: string;
  targets: string[];
  created_at: string;
};

export default function AliasesPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [localPart, setLocalPart] = useState("");
  const [domainId, setDomainId] = useState("");
  const [targetIds, setTargetIds] = useState<string[]>([]);

  const domains = useQuery({
    queryKey: ["admin", "domains"],
    queryFn: () => api.get<{ domains: Domain[] }>("/api/v1/domains"),
  });
  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
  });
  const aliases = useQuery({
    queryKey: ["admin", "aliases"],
    queryFn: () => api.get<{ aliases: Alias[] }>("/api/v1/aliases"),
  });

  const verifiedDomains = (domains.data?.domains ?? []).filter((d) => d.status === "verified");

  const create = useMutation({
    mutationFn: () =>
      api.post("/api/v1/aliases", {
        domain_id: domainId || verifiedDomains[0]?.id,
        local_part: localPart,
        target_mailbox_ids: targetIds,
      }),
    onSuccess: () => {
      setLocalPart("");
      setTargetIds([]);
      qc.invalidateQueries({ queryKey: ["admin", "aliases"] });
      toast("success", "Alias created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create alias"),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/aliases/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "aliases"] });
      toast("success", "Alias deleted");
    },
    onError: () => toast("error", "Could not delete alias"),
  });

  const toggle = useMutation({
    mutationFn: (a: Alias) =>
      api.patch(`/api/v1/aliases/${a.id}`, {
        status: a.status === "active" ? "inactive" : "active",
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "aliases"] }),
  });

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">04. Desk</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Aliases
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          Aliases redirect mail with the logic of a newsroom switchboard. They keep public-facing
          addresses stable while routing content to the right mailbox.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">New alias</p>
        <form
          className="mt-4 space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (localPart.trim() && targetIds.length > 0) create.mutate();
          }}
        >
          <div className="flex flex-wrap items-end gap-3">
            <div className="min-w-56 flex-1">
              <Input
                label="Local part"
                placeholder="support"
                value={localPart}
                onChange={(e) => setLocalPart(e.target.value)}
              />
            </div>
            <label className="block min-w-52">
              <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
                Domain
              </span>
              <select
                value={domainId}
                onChange={(e) => setDomainId(e.target.value)}
                className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-mono text-sm text-[#111111] outline-none"
              >
                {verifiedDomains.map((d) => (
                  <option key={d.id} value={d.id}>
                    @{d.name}
                  </option>
                ))}
              </select>
            </label>
            <Button type="submit" disabled={create.isPending || !localPart.trim() || targetIds.length === 0}>
              {create.isPending ? "Creating..." : "Create alias"}
            </Button>
          </div>

          <div>
            <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
              Delivers to
            </span>
            <div className="flex flex-wrap gap-2">
              {(mailboxes.data?.mailboxes ?? []).map((m) => (
                <label
                  key={m.id}
                  className="flex items-center gap-2 border border-[#111111] bg-[#e5e5e0] px-3 py-2 font-mono text-xs uppercase tracking-[0.18em]"
                >
                  <input
                    type="checkbox"
                    className="h-4 w-4 accent-[#111111]"
                    checked={targetIds.includes(m.id)}
                    onChange={(e) =>
                      setTargetIds((prev) =>
                        e.target.checked ? [...prev, m.id] : prev.filter((x) => x !== m.id),
                      )
                    }
                  />
                  {m.address}
                </label>
              ))}
            </div>
          </div>
        </form>
      </section>

      {aliases.isLoading && <PageLoader label="Loading aliases" />}
      {aliases.isSuccess && aliases.data.aliases.length === 0 && (
        <EmptyState title="No aliases yet" hint="Create the first routing rule for the paper desk." />
      )}
      {aliases.isSuccess && aliases.data.aliases.length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Alias</th>
                <th className="px-4 py-3 font-medium">Delivers to</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {aliases.data.aliases.map((a) => (
                <tr key={a.id}>
                  <td className="px-4 py-3 font-semibold">{a.address}</td>
                  <td className="px-4 py-3 text-[#525252]">{a.targets.join(", ")}</td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => toggle.mutate(a)}
                      className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]"
                      title="Toggle status"
                    >
                      {a.status}
                    </button>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="ghost" onClick={() => remove.mutate(a.id)}>
                      Delete
                    </Button>
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
