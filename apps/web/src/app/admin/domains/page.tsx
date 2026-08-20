"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Domain, type Organization } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

export default function DomainsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const [orgId, setOrgId] = useState("");

  const orgs = useQuery({
    queryKey: ["admin", "organizations"],
    queryFn: () => api.get<{ organizations: Organization[] }>("/api/v1/organizations"),
  });
  const domains = useQuery({
    queryKey: ["admin", "domains"],
    queryFn: () => api.get<{ domains: Domain[] }>("/api/v1/domains"),
  });

  const orgName = (id: string) =>
    orgs.data?.organizations.find((o) => o.id === id)?.name ?? id.slice(0, 8);

  const create = useMutation({
    mutationFn: () =>
      api.post("/api/v1/domains", {
        name,
        organization_id: orgId || orgs.data?.organizations[0]?.id,
        verification_mode: "development",
      }),
    onSuccess: () => {
      setName("");
      qc.invalidateQueries({ queryKey: ["admin", "domains"] });
      toast("success", "Domain added");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not add domain"),
  });

  return (
    <div className="mx-auto max-w-4xl">
      <h1 className="mb-4 text-lg font-semibold">Domains</h1>

      <form
        className="mb-4 flex flex-wrap items-end gap-2 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create.mutate();
        }}
      >
        <div className="min-w-48 flex-1">
          <Input
            label="Domain name"
            placeholder="company.test"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <label className="block">
          <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">
            Organization
          </span>
          <select
            value={orgId}
            onChange={(e) => setOrgId(e.target.value)}
            className="rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-900"
          >
            {(orgs.data?.organizations ?? []).map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
        </label>
        <Button type="submit" disabled={create.isPending || !name.trim()}>
          {create.isPending ? "Adding…" : "Add development domain"}
        </Button>
      </form>
      <p className="mb-4 text-xs text-neutral-400">
        Development domains (e.g. <code>company.test</code>) are verified immediately and work only
        inside this local platform. DNS-verified production domains arrive in a later phase.
      </p>

      {domains.isLoading && <PageLoader />}
      {domains.isSuccess && domains.data.domains.length === 0 && <EmptyState title="No domains yet" />}
      {domains.isSuccess && domains.data.domains.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Domain</th>
                <th className="px-4 py-2.5 font-medium">Organization</th>
                <th className="px-4 py-2.5 font-medium">Mode</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {domains.data.domains.map((d) => (
                <tr key={d.id}>
                  <td className="px-4 py-2.5 font-medium">{d.name}</td>
                  <td className="px-4 py-2.5 text-neutral-500">{orgName(d.organization_id)}</td>
                  <td className="px-4 py-2.5 text-neutral-500">{d.verification_mode}</td>
                  <td className="px-4 py-2.5">
                    <span
                      className={
                        d.status === "verified"
                          ? "rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                          : "rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-950 dark:text-amber-300"
                      }
                    >
                      {d.status}
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
