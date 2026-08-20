"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Domain, type Organization } from "@/lib/api";
import { Badge, Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

// Mail-core provisioning state of a domain. "failed" keeps the error in a
// tooltip and offers a retry; "skipped" means no mail core is configured.
function ProvisioningBadge({ d, onRetry, retrying }: { d: Domain; onRetry: () => void; retrying: boolean }) {
  switch (d.provisioning_status) {
    case "active":
      return <Badge tone="success">Provisioned</Badge>;
    case "skipped":
      return <Badge tone="neutral">No mail core</Badge>;
    case "provisioning":
      return <Badge tone="accent">Provisioning…</Badge>;
    case "failed":
      return (
        <span className="inline-flex items-center gap-2" title={d.provisioning_error}>
          <Badge tone="danger">Failed</Badge>
          <button
            type="button"
            className="text-xs font-medium text-primary hover:underline disabled:opacity-50"
            onClick={onRetry}
            disabled={retrying}
          >
            {retrying ? "Retrying…" : "Retry"}
          </button>
        </span>
      );
    default:
      return <Badge tone="neutral">Pending</Badge>;
  }
}

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

  const provision = useMutation({
    mutationFn: (id: string) => api.post<{ provisioning_status: string }>(`/api/v1/domains/${id}/provision`),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ["admin", "domains"] });
      if (res.provisioning_status === "active") toast("success", "Domain provisioned in the mail core");
      else toast("error", "Provisioning did not complete; see the domain row for details");
    },
    onError: () => toast("error", "Provisioning request failed"),
  });

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
                <th className="px-4 py-3 font-medium">Verification</th>
                <th className="px-4 py-3 font-medium">Mail core</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {domains.data.domains.map((d) => (
                <tr key={d.id}>
                  <td className="px-4 py-3 font-semibold">{d.name}</td>
                  <td className="px-4 py-3 text-muted-foreground">{orgName(d.organization_id)}</td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{d.verification_mode}</td>
                  <td className="px-4 py-3">
                    <Badge tone={d.status === "verified" ? "success" : "neutral"}>{d.status}</Badge>
                  </td>
                  <td className="px-4 py-3">
                    <ProvisioningBadge
                      d={d}
                      onRetry={() => provision.mutate(d.id)}
                      retrying={provision.isPending && provision.variables === d.id}
                    />
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
