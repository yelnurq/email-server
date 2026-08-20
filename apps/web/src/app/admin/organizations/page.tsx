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
    <div className="mx-auto max-w-4xl">
      <h1 className="mb-4 text-lg font-semibold">Organizations</h1>

      <form
        className="mb-4 flex items-end gap-2 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create.mutate();
        }}
      >
        <div className="flex-1">
          <Input label="Name" placeholder="Acme Inc" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <Button type="submit" disabled={create.isPending || !name.trim()}>
          {create.isPending ? "Creating…" : "Create"}
        </Button>
      </form>

      {orgs.isLoading && <PageLoader />}
      {orgs.isSuccess && orgs.data.organizations.length === 0 && (
        <EmptyState title="No organizations yet" />
      )}
      {orgs.isSuccess && orgs.data.organizations.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Slug</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {orgs.data.organizations.map((o) => (
                <tr key={o.id}>
                  <td className="px-4 py-2.5 font-medium">{o.name}</td>
                  <td className="px-4 py-2.5 text-neutral-500">{o.slug}</td>
                  <td className="px-4 py-2.5">
                    <span className="rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300">
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
