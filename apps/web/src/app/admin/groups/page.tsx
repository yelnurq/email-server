"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Domain, type Mailbox } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type Group = {
  id: string;
  organization_id: string;
  address: string;
  name: string;
  status: string;
  internal_only: boolean;
  members: string[];
  created_at: string;
};

export default function GroupsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [localPart, setLocalPart] = useState("");
  const [name, setName] = useState("");
  const [domainId, setDomainId] = useState("");
  const [internalOnly, setInternalOnly] = useState(false);
  const [memberIds, setMemberIds] = useState<string[]>([]);

  const domains = useQuery({
    queryKey: ["admin", "domains"],
    queryFn: () => api.get<{ domains: Domain[] }>("/api/v1/domains"),
  });
  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
  });
  const groups = useQuery({
    queryKey: ["admin", "groups"],
    queryFn: () => api.get<{ groups: Group[] }>("/api/v1/groups"),
  });

  const verifiedDomains = (domains.data?.domains ?? []).filter((d) => d.status === "verified");

  const create = useMutation({
    mutationFn: () =>
      api.post("/api/v1/groups", {
        domain_id: domainId || verifiedDomains[0]?.id,
        local_part: localPart,
        name,
        internal_only: internalOnly,
        member_mailbox_ids: memberIds,
      }),
    onSuccess: () => {
      setLocalPart("");
      setName("");
      setMemberIds([]);
      qc.invalidateQueries({ queryKey: ["admin", "groups"] });
      toast("success", "Group created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create group"),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/groups/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "groups"] });
      toast("success", "Group deleted");
    },
    onError: () => toast("error", "Could not delete group"),
  });

  return (
    <div className="mx-auto max-w-4xl">
      <h1 className="mb-4 text-lg font-semibold">Groups</h1>

      <form
        className="mb-4 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          if (localPart.trim() && memberIds.length > 0) create.mutate();
        }}
      >
        <div className="flex flex-wrap items-end gap-2">
          <div className="w-36">
            <Input label="Local part" placeholder="team" value={localPart} onChange={(e) => setLocalPart(e.target.value)} />
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
          <div className="w-40">
            <Input label="Display name" placeholder="Team" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <label className="flex items-center gap-1.5 pb-2 text-xs text-neutral-600 dark:text-neutral-400">
            <input
              type="checkbox"
              className="h-3.5 w-3.5 accent-indigo-600"
              checked={internalOnly}
              onChange={(e) => setInternalOnly(e.target.checked)}
            />
            Internal senders only
          </label>
          <Button type="submit" disabled={create.isPending || !localPart.trim() || memberIds.length === 0}>
            {create.isPending ? "Creating…" : "Create group"}
          </Button>
        </div>
        <div className="mt-3">
          <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">Members</span>
          <div className="flex flex-wrap gap-2">
            {(mailboxes.data?.mailboxes ?? []).map((m) => (
              <label
                key={m.id}
                className="flex items-center gap-1.5 rounded-lg border border-neutral-200 px-2 py-1 text-xs dark:border-neutral-700"
              >
                <input
                  type="checkbox"
                  className="h-3.5 w-3.5 accent-indigo-600"
                  checked={memberIds.includes(m.id)}
                  onChange={(e) =>
                    setMemberIds((prev) =>
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

      {groups.isLoading && <PageLoader />}
      {groups.isSuccess && groups.data.groups.length === 0 && <EmptyState title="No groups yet" />}
      {groups.isSuccess && groups.data.groups.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Group</th>
                <th className="px-4 py-2.5 font-medium">Members</th>
                <th className="px-4 py-2.5 font-medium">Policy</th>
                <th className="px-4 py-2.5 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {groups.data.groups.map((g) => (
                <tr key={g.id}>
                  <td className="px-4 py-2.5">
                    <span className="font-medium">{g.address}</span>
                    {g.name && <span className="ml-2 text-xs text-neutral-400">{g.name}</span>}
                  </td>
                  <td className="px-4 py-2.5 text-neutral-500">{g.members.join(", ")}</td>
                  <td className="px-4 py-2.5 text-xs text-neutral-500">
                    {g.internal_only ? "internal only" : "open"}
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <Button variant="ghost" onClick={() => remove.mutate(g.id)}>
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
