"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Organization } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

export default function OrganizationsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");

  const orgs = useQuery({
    queryKey: ["admin", "organizations"],
    queryFn: () => api.get<{ organizations: Organization[] }>("/api/v1/organizations"),
  });

  const create = useMutation({
    mutationFn: () => api.post("/api/v1/organizations", { name }),
    onSuccess: () => {
      setName("");
      qc.invalidateQueries({ queryKey: ["admin", "organizations"] });
      toast("success", "Organization created");
    },
    onError: (e) =>
      toast("error", e instanceof ApiError ? e.message : "Could not create organization"),
  });

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">02. Method</p>
        <div className="mt-4 grid gap-6 lg:grid-cols-12">
          <div className="lg:col-span-8">
            <h1 className="font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
              Organizations
            </h1>
            <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
              Organizations anchor the tenant structure. Each one is a distinct editorial house
              with its own domains, users, and message flows.
            </p>
          </div>
          <div className="border border-[#111111] bg-[#e5e5e0] p-4 lg:col-span-4">
            <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
              Created today
            </p>
            <p className="mt-3 font-serif text-4xl text-[#111111]">
              {orgs.data?.organizations.length ?? 0}
            </p>
            <p className="mt-2 text-sm text-[#525252] font-body">
              Active organizations visible in the control plane.
            </p>
          </div>
        </div>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">New entry</p>
        <form
          className="mt-4 flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) create.mutate();
          }}
        >
          <div className="min-w-64 flex-1">
            <Input label="Name" placeholder="Acme Inc" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <Button type="submit" disabled={create.isPending || !name.trim()}>
            {create.isPending ? "Creating..." : "Create"}
          </Button>
        </form>
      </section>

      {orgs.isLoading && <PageLoader label="Loading organizations" />}
      {orgs.isSuccess && orgs.data.organizations.length === 0 && (
        <EmptyState title="No organizations yet" hint="Create the first tenant container to continue." />
      )}
      {orgs.isSuccess && orgs.data.organizations.length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Slug</th>
                <th className="px-4 py-3 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {orgs.data.organizations.map((o) => (
                <tr key={o.id}>
                  <td className="px-4 py-3 font-semibold text-[#111111]">{o.name}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{o.slug}</td>
                  <td className="px-4 py-3">
                    <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">
                      {o.status}
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
