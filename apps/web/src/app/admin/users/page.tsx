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
    <div className="mx-auto max-w-5xl">
      <h1 className="mb-4 text-lg font-semibold">Users</h1>

      <form
        className="mb-4 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <Input
            label="Email (login)"
            type="email"
            placeholder="user1@company.test"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
          <Input
            label="Display name"
            placeholder="User One"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
          />
          <Input
            label="Password (min 10 chars)"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={10}
          />
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">Organization</span>
            <select
              value={orgId}
              onChange={(e) => setOrgId(e.target.value)}
              className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-900"
            >
              {(orgs.data?.organizations ?? []).map((o) => (
                <option key={o.id} value={o.id}>{o.name}</option>
              ))}
            </select>
          </label>
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">Role</span>
            <select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm dark:border-neutral-700 dark:bg-neutral-900"
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
            <span className="mb-1 block text-xs font-medium text-neutral-600 dark:text-neutral-400">Mailbox</span>
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                className="h-4 w-4 accent-indigo-600"
                checked={withMailbox}
                onChange={(e) => setWithMailbox(e.target.checked)}
                id="with-mailbox"
              />
              <select
                value={domainId}
                onChange={(e) => setDomainId(e.target.value)}
                disabled={!withMailbox}
                className="w-full rounded-lg border border-neutral-300 bg-white px-3 py-1.5 text-sm disabled:opacity-50 dark:border-neutral-700 dark:bg-neutral-900"
              >
                {verifiedDomains.map((d) => (
                  <option key={d.id} value={d.id}>@{d.name}</option>
                ))}
              </select>
            </div>
          </label>
        </div>
        <div className="mt-3">
          <Button type="submit" disabled={create.isPending || !email || password.length < 10}>
            {create.isPending ? "Creating…" : "Create user"}
          </Button>
        </div>
      </form>

      {users.isLoading && <PageLoader />}
      {users.isSuccess && users.data.users.length === 0 && <EmptyState title="No users yet" />}
      {users.isSuccess && users.data.users.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Email</th>
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Mailbox</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {users.data.users.map((u) => (
                <tr key={u.id}>
                  <td className="px-4 py-2.5 font-medium">{u.email}</td>
                  <td className="px-4 py-2.5 text-neutral-500">{u.display_name || "—"}</td>
                  <td className="px-4 py-2.5 text-neutral-500">{u.mailbox_address || "—"}</td>
                  <td className="px-4 py-2.5">
                    <span
                      className={
                        u.status === "active"
                          ? "rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                          : "rounded-full bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-neutral-800"
                      }
                    >
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
