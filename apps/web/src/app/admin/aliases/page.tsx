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
    <div className="mx-auto max-w-4xl">
      <h1 className="mb-4 text-lg font-semibold">Aliases</h1>

      <form
        className="mb-4 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          if (localPart.trim() && targetIds.length > 0) create.mutate();
        }}
      >
        <div className="flex flex-wrap items-end gap-2">
          <div className="w-40">
            <Input
              label="Local part"
              placeholder="support"
              value={localPart}
              onChange={(e) => setLocalPart(e.target.value)}
            />
          </div>
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">Domain</span>
            <select
              value={domainId}
              onChange={(e) => setDomainId(e.target.value)}
              className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            >
              {verifiedDomains.map((d) => (
                <option key={d.id} value={d.id}>@{d.name}</option>
              ))}
            </select>
          </label>
          <Button type="submit" disabled={create.isPending || !localPart.trim() || targetIds.length === 0}>
            {create.isPending ? "Creating…" : "Create alias"}
          </Button>
        </div>
        <div className="mt-3">
          <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">
            Delivers to
          </span>
          <div className="flex flex-wrap gap-2">
            {(mailboxes.data?.mailboxes ?? []).map((m) => (
              <label
                key={m.id}
                className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2 py-1 text-xs dark:border-neutral-700"
              >
                <input
                  type="checkbox"
                  className="h-3.5 w-3.5 accent-indigo-600"
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

      {aliases.isLoading && <PageLoader />}
      {aliases.isSuccess && aliases.data.aliases.length === 0 && <EmptyState title="No aliases yet" />}
      {aliases.isSuccess && aliases.data.aliases.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Alias</th>
                <th className="px-4 py-2.5 font-medium">Delivers to</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
                <th className="px-4 py-2.5 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {aliases.data.aliases.map((a) => (
                <tr key={a.id}>
                  <td className="px-4 py-2.5 font-medium">{a.address}</td>
                  <td className="px-4 py-2.5 text-neutral-500">{a.targets.join(", ")}</td>
                  <td className="px-4 py-2.5">
                    <button
                      onClick={() => toggle.mutate(a)}
                      className={
                        a.status === "active"
                          ? "rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                          : "rounded-full bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-neutral-800"
                      }
                      title="Toggle status"
                    >
                      {a.status}
                    </button>
                  </td>
                  <td className="px-4 py-2.5 text-right">
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
