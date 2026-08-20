"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Domain, type AdminUser } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type Group = {
  id: string;
  organization_id: string;
  address: string;
  name: string;
  policy: string;
  members: string[];
};

export default function GroupsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [localPart, setLocalPart] = useState("");
  const [domainId, setDomainId] = useState("");
  const [name, setName] = useState("");
  const [memberIds, setMemberIds] = useState<string[]>([]);

  const domains = useQuery({
    queryKey: ["admin", "domains"],
    queryFn: () => api.get<{ domains: Domain[] }>("/api/v1/domains"),
  });
  const users = useQuery({
    queryKey: ["admin", "users"],
    queryFn: () => api.get<{ users: AdminUser[] }>("/api/v1/users"),
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
        member_user_ids: memberIds,
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
      toast("success", "Group removed");
    },
  });

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">10. Collective</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Groups
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          Groups collect multiple readers under one public address, with delivery policy and
          membership laid out explicitly.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">New group</p>
        <form
          className="mt-4 space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (localPart.trim() && memberIds.length > 0) create.mutate();
          }}
        >
          <div className="grid gap-4 md:grid-cols-3">
            <Input label="Local part" placeholder="team" value={localPart} onChange={(e) => setLocalPart(e.target.value)} />
            <Input label="Display name" placeholder="Team" value={name} onChange={(e) => setName(e.target.value)} />
            <label className="block">
              <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">Domain</span>
              <select
                value={domainId}
                onChange={(e) => setDomainId(e.target.value)}
                className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-mono text-sm text-[#111111] outline-none"
              >
                {verifiedDomains.map((d) => (
                  <option key={d.id} value={d.id}>@{d.name}</option>
                ))}
              </select>
            </label>
          </div>

          <div>
            <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">Members</span>
            <div className="flex flex-wrap gap-2">
              {(users.data?.users ?? []).map((u) => (
                <label
                  key={u.id}
                  className="flex items-center gap-2 border border-[#111111] bg-[#e5e5e0] px-3 py-2 font-mono text-xs uppercase tracking-[0.18em]"
                >
                  <input
                    type="checkbox"
                    className="h-4 w-4 accent-[#111111]"
                    checked={memberIds.includes(u.id)}
                    onChange={(e) =>
                      setMemberIds((prev) =>
                        e.target.checked ? [...prev, u.id] : prev.filter((x) => x !== u.id),
                      )
                    }
                  />
                  {u.email}
                </label>
              ))}
            </div>
          </div>

          <Button type="submit" disabled={create.isPending || !localPart.trim() || memberIds.length === 0}>
            Create group
          </Button>
        </form>
      </section>

      {groups.isLoading && <PageLoader label="Loading groups" />}
      {groups.isSuccess && groups.data.groups.length === 0 && (
        <EmptyState title="No groups yet" hint="Create a collective address for shared mail." />
      )}
      {groups.isSuccess && groups.data.groups.length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Group</th>
                <th className="px-4 py-3 font-medium">Members</th>
                <th className="px-4 py-3 font-medium">Policy</th>
                <th className="px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {groups.data.groups.map((g) => (
                <tr key={g.id}>
                  <td className="px-4 py-3 font-semibold">
                    {g.address}
                    {g.name && <span className="ml-2 font-mono text-xs text-[#525252]">{g.name}</span>}
                  </td>
                  <td className="px-4 py-3 text-[#525252]">{g.members.join(", ")}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{g.policy}</td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="ghost" onClick={() => remove.mutate(g.id)}>
                      Remove
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
