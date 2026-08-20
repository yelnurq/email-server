"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type AdminUser, type Domain, type Organization } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

export default function UsersPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("member");
  const [orgId, setOrgId] = useState("");
  const [domainId, setDomainId] = useState("");
  const [withMailbox, setWithMailbox] = useState(true);

  const orgs = useQuery({
    queryKey: ["admin", "organizations"],
    queryFn: () => api.get<{ organizations: Organization[] }>("/api/v1/organizations"),
  });
  const domains = useQuery({
    queryKey: ["admin", "domains"],
    queryFn: () => api.get<{ domains: Domain[] }>("/api/v1/domains"),
  });
  const users = useQuery({
    queryKey: ["admin", "users"],
    queryFn: () => api.get<{ users: AdminUser[] }>("/api/v1/users"),
  });

  const verifiedDomains = (domains.data?.domains ?? []).filter((d) => d.status === "verified");

  const create = useMutation({
    mutationFn: () =>
      api.post("/api/v1/users", {
        email,
        display_name: displayName,
        password,
        role,
        organization_id: orgId || orgs.data?.organizations[0]?.id,
        ...(withMailbox && verifiedDomains.length > 0
          ? { mailbox_domain_id: domainId || verifiedDomains[0].id }
          : {}),
      }),
    onSuccess: () => {
      setEmail("");
      setDisplayName("");
      setPassword("");
      qc.invalidateQueries({ queryKey: ["admin"] });
      toast("success", "User created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create user"),
  });

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">05. Staff</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Users
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          Each user is an authenticated reader with an assigned role. Mailboxes may be created at
          the same time as the account, so editorial access and delivery are aligned.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Create user</p>
        <form
          className="mt-4 space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            <Input label="Email (login)" type="email" placeholder="user1@company.test" value={email} onChange={(e) => setEmail(e.target.value)} />
            <Input label="Display name" placeholder="User One" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
            <Input
              label="Password (min 10 chars)"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              minLength={10}
            />
            <label className="block">
              <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">Organization</span>
              <select
                value={orgId}
                onChange={(e) => setOrgId(e.target.value)}
                className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-mono text-sm text-[#111111] outline-none"
              >
                {(orgs.data?.organizations ?? []).map((o) => (
                  <option key={o.id} value={o.id}>{o.name}</option>
                ))}
              </select>
            </label>
            <label className="block">
              <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">Role</span>
              <select
                value={role}
                onChange={(e) => setRole(e.target.value)}
                className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-mono text-sm text-[#111111] outline-none"
              >
                <option value="member">Member</option>
                <option value="org_admin">Organization Admin</option>
                <option value="domain_admin">Domain Admin</option>
                <option value="developer">Developer</option>
                <option value="security_analyst">Security Analyst</option>
                <option value="auditor">Auditor</option>
              </select>
            </label>
            <label className="block">
              <span className="mb-2 block font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">Mailbox</span>
              <div className="flex items-center gap-3">
                <input
                  type="checkbox"
                  className="h-4 w-4 accent-[#111111]"
                  checked={withMailbox}
                  onChange={(e) => setWithMailbox(e.target.checked)}
                  id="with-mailbox"
                />
                <select
                  value={domainId}
                  onChange={(e) => setDomainId(e.target.value)}
                  disabled={!withMailbox}
                  className="w-full border-b-2 border-[#111111] bg-transparent px-3 py-2 font-mono text-sm text-[#111111] outline-none disabled:opacity-50"
                >
                  {verifiedDomains.map((d) => (
                    <option key={d.id} value={d.id}>@{d.name}</option>
                  ))}
                </select>
              </div>
            </label>
          </div>

          <Button type="submit" disabled={create.isPending || !email || password.length < 10}>
            {create.isPending ? "Creating..." : "Create user"}
          </Button>
        </form>
      </section>

      {users.isLoading && <PageLoader label="Loading users" />}
      {users.isSuccess && users.data.users.length === 0 && (
        <EmptyState title="No users yet" hint="Create the first reader to activate the system." />
      )}
      {users.isSuccess && users.data.users.length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Email</th>
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Mailbox</th>
                <th className="px-4 py-3 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {users.data.users.map((u) => (
                <tr key={u.id}>
                  <td className="px-4 py-3 font-semibold">{u.email}</td>
                  <td className="px-4 py-3 text-[#525252]">{u.display_name || "—"}</td>
                  <td className="px-4 py-3 text-[#525252]">{u.mailbox_address || "—"}</td>
                  <td className="px-4 py-3">
                    <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">
                      {u.status}
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
