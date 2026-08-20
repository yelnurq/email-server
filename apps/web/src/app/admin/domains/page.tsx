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

  const orgName = (id: string) => orgs.data?.organizations.find((o) => o.id === id)?.name ?? id.slice(0, 8);

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
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">03. Registry</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Domains
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          Domains determine where mail belongs and how the platform reasons about identity. In
          this system, verification is deliberately explicit.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">New domain</p>
        <form
          className="mt-4 flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <div className="min-w-64 flex-1">
            <Input label="Domain name" placeholder="company.test" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <label className="block min-w-52">
            <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
              Organization
            </span>
            <select
              value={orgId}
              onChange={(e) => setOrgId(e.target.value)}
              className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-mono text-sm text-[#111111] outline-none"
            >
              {(orgs.data?.organizations ?? []).map((o) => (
                <option key={o.id} value={o.id}>
                  {o.name}
                </option>
              ))}
            </select>
          </label>
          <Button type="submit" disabled={create.isPending || !name.trim()}>
            {create.isPending ? "Adding..." : "Add development domain"}
          </Button>
        </form>
        <p className="mt-4 text-sm leading-6 text-[#525252] font-body">
          Development domains such as <code className="font-mono">company.test</code> are
          verified immediately and work only inside this local platform.
        </p>
      </section>

      {domains.isLoading && <PageLoader label="Loading domains" />}
      {domains.isSuccess && domains.data.domains.length === 0 && (
        <EmptyState title="No domains yet" hint="Add the first local development domain to continue." />
      )}
      {domains.isSuccess && domains.data.domains.length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Domain</th>
                <th className="px-4 py-3 font-medium">Organization</th>
                <th className="px-4 py-3 font-medium">Mode</th>
                <th className="px-4 py-3 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {domains.data.domains.map((d) => (
                <tr key={d.id}>
                  <td className="px-4 py-3 font-semibold">{d.name}</td>
                  <td className="px-4 py-3 text-[#525252]">{orgName(d.organization_id)}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{d.verification_mode}</td>
                  <td className="px-4 py-3">
                    <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">
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
