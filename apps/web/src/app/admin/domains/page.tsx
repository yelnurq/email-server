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
    <div className="space-y-6">
      <section className="mb-8">
        <h1 className="page-title">
          Domains
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
          Sending and receiving domains for your organization. A domain must be verified before mailboxes can be provisioned on it.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="text-base font-semibold text-foreground">New domain</p>
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
            <span className="mb-2 block text-xs font-medium text-muted-foreground">
              Organization
            </span>
            <select
              value={orgId}
              onChange={(e) => setOrgId(e.target.value)}
              className="w-full border-b-2 border-border bg-transparent px-3 py-2 font-mono text-sm text-foreground outline-none"
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
        <p className="mt-4 text-sm leading-6 text-muted-foreground font-body">
          Development domains such as <code className="font-mono">company.test</code> are
          verified immediately and work only inside this local platform.
        </p>
      </section>

      {domains.isLoading && <PageLoader label="Loading domains" />}
      {domains.isSuccess && domains.data.domains.length === 0 && (
        <EmptyState title="No domains yet" hint="Add the first local development domain to continue." />
      )}
      {domains.isSuccess && domains.data.domains.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">Domain</th>
                <th className="px-4 py-3 font-medium">Organization</th>
                <th className="px-4 py-3 font-medium">Mode</th>
                <th className="px-4 py-3 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {domains.data.domains.map((d) => (
                <tr key={d.id}>
                  <td className="px-4 py-3 font-semibold">{d.name}</td>
                  <td className="px-4 py-3 text-muted-foreground">{orgName(d.organization_id)}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{d.verification_mode}</td>
                  <td className="px-4 py-3">
                    <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
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
