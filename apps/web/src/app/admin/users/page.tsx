"use client";

// User management: search, filters, creation, per-user drawer with live
// activity from the audit log, and real account actions (rename, department,
// activate/deactivate) backed by PATCH /users/{id}.

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatDate, type AdminUser, type Department, type Domain, type Organization } from "@/lib/api";
import { useMe } from "@/components/providers";
import { Avatar, Badge, Button, ConfirmDialog, Drawer, EmptyState, ErrorState, IconButton, Input, ListSkeleton, Menu, Segmented, Tabs, cx, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";

type AuditEntry = {
  id: number;
  actor_email?: string;
  action: string;
  resource_type?: string;
  detail: Record<string, unknown> | null;
  ip?: string;
  created_at: string;
};

const ROLES = [
  { value: "member", label: "Member" },
  { value: "org_admin", label: "Organization Admin" },
  { value: "domain_admin", label: "Domain Admin" },
  { value: "developer", label: "Developer" },
  { value: "security_analyst", label: "Security Analyst" },
  { value: "auditor", label: "Auditor" },
];

function CreateUserDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState("member");
  const [orgId, setOrgId] = useState("");
  const [domainId, setDomainId] = useState("");
  const [departmentId, setDepartmentId] = useState("");
  const [withMailbox, setWithMailbox] = useState(true);

  const orgs = useQuery({ queryKey: ["admin", "organizations"], queryFn: () => api.get<{ organizations: Organization[] }>("/api/v1/organizations"), enabled: open });
  const domains = useQuery({ queryKey: ["admin", "domains"], queryFn: () => api.get<{ domains: Domain[] }>("/api/v1/domains"), enabled: open });
  const departments = useQuery({ queryKey: ["departments"], queryFn: () => api.get<{ departments: Department[] }>("/api/v1/departments"), enabled: open });
  const verifiedDomains = (domains.data?.domains ?? []).filter((d) => d.status === "verified");

  const create = useMutation({
    mutationFn: () =>
      api.post("/api/v1/users", {
        email,
        display_name: displayName,
        password,
        role,
        department_id: departmentId || undefined,
        organization_id: orgId || orgs.data?.organizations[0]?.id,
        ...(withMailbox && verifiedDomains.length > 0 ? { mailbox_domain_id: domainId || verifiedDomains[0].id } : {}),
      }),
    onSuccess: () => {
      setEmail(""); setDisplayName(""); setPassword("");
      qc.invalidateQueries({ queryKey: ["admin"] });
      toast("success", "User created");
      onClose();
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create user"),
  });

  return (
    <Drawer open={open} onClose={onClose} title="New user" width="max-w-md">
      <form className="space-y-4 p-4" onSubmit={(e) => { e.preventDefault(); create.mutate(); }}>
        <Input label="Email (login)" type="email" placeholder="user@company.kz" value={email} onChange={(e) => setEmail(e.target.value)} required />
        <Input label="Display name" placeholder="Full name" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
        <Input label="Password (min 10 characters)" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={10} required />
        <label className="block">
          <span className="mb-1 block text-xs font-medium">Organization</span>
          <select value={orgId} onChange={(e) => setOrgId(e.target.value)} className="w-full">
            {(orgs.data?.organizations ?? []).map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </select>
        </label>
        <label className="block">
          <span className="mb-1 block text-xs font-medium">Role</span>
          <select value={role} onChange={(e) => setRole(e.target.value)} className="w-full">
            {ROLES.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
          </select>
        </label>
        <label className="block">
          <span className="mb-1 block text-xs font-medium">Department</span>
          <select value={departmentId} onChange={(e) => setDepartmentId(e.target.value)} className="w-full">
            <option value="">No department</option>
            {(departments.data?.departments ?? []).map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </select>
        </label>
        <div className="rounded-[8px] border border-border bg-background p-3">
          <label className="flex items-center gap-2.5 text-[13px] font-medium">
            <input type="checkbox" className="h-3.5 w-3.5 accent-graphite" checked={withMailbox} onChange={(e) => setWithMailbox(e.target.checked)} />
            Provision a mailbox
          </label>
          {withMailbox && (
            <select value={domainId} onChange={(e) => setDomainId(e.target.value)} className="mt-2 w-full" disabled={verifiedDomains.length === 0}>
              {verifiedDomains.length === 0 && <option>No verified domains available</option>}
              {verifiedDomains.map((d) => <option key={d.id} value={d.id}>@{d.name}</option>)}
            </select>
          )}
        </div>
        <div className="flex justify-end gap-2 border-t border-border pt-4">
          <Button type="button" variant="secondary" onClick={onClose}>Cancel</Button>
          <Button type="submit" disabled={create.isPending || !email || password.length < 10}>
            {create.isPending ? "Creating…" : "Create user"}
          </Button>
        </div>
      </form>
    </Drawer>
  );
}

function UserDrawer({
  user, departments, onClose, selfEmail,
}: {
  user: AdminUser;
  departments: Department[];
  onClose: () => void;
  selfEmail: string;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const [tab, setTab] = useState("overview");
  const [name, setName] = useState(user.display_name);
  const [departmentId, setDepartmentId] = useState(user.department_id ?? "");
  const [confirmStatus, setConfirmStatus] = useState(false);
  const isSelf = user.email === selfEmail;

  const activity = useQuery({
    queryKey: ["admin", "audit", "user", user.email],
    queryFn: () => api.get<{ entries: AuditEntry[]; total: number }>(`/api/v1/audit?actor=${encodeURIComponent(user.email)}&limit=30`),
    enabled: tab === "activity",
  });

  const patch = useMutation({
    mutationFn: (body: Record<string, unknown>) => api.patch(`/api/v1/users/${user.id}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin"] });
      toast("success", "User updated");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Update failed"),
  });

  const changed = name !== user.display_name || departmentId !== (user.department_id ?? "");

  return (
    <Drawer
      open
      onClose={onClose}
      width="max-w-lg"
      title={
        <span className="flex items-center gap-2.5">
          <Avatar name={user.display_name || user.email} size="md" />
          <span className="min-w-0">
            <span className="block truncate leading-4">{user.display_name || user.email}</span>
            <span className="block text-[11px] font-normal text-muted-foreground">{user.email}</span>
          </span>
          <Badge tone={user.status === "active" ? "success" : "danger"} className="ml-1">{user.status}</Badge>
        </span>
      }
    >
      <Tabs
        className="px-4"
        value={tab}
        onChange={setTab}
        tabs={[{ value: "overview", label: "Overview" }, { value: "activity", label: "Activity" }]}
      />

      {tab === "overview" && (
        <div className="space-y-4 p-4">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 rounded-[8px] border border-border bg-background px-3.5 py-3 text-[13px]">
            <dt className="text-muted-foreground">Email</dt><dd className="break-all font-medium">{user.email}</dd>
            <dt className="text-muted-foreground">Mailbox</dt><dd className="break-all">{user.mailbox_address || "—"}</dd>
            <dt className="text-muted-foreground">Status</dt><dd className="capitalize">{user.status}</dd>
            <dt className="text-muted-foreground">Created</dt><dd>{formatDate(user.created_at)}</dd>
            <dt className="text-muted-foreground">User ID</dt><dd className="code-value break-all text-muted-foreground">{user.id}</dd>
          </dl>

          <Input label="Display name" value={name} onChange={(e) => setName(e.target.value)} />
          <label className="block">
            <span className="mb-1 block text-xs font-medium">Department</span>
            <select value={departmentId} onChange={(e) => setDepartmentId(e.target.value)} className="w-full">
              <option value="">No department</option>
              {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
            </select>
          </label>
          <div className="flex items-center justify-between border-t border-border pt-4">
            {!isSelf ? (
              <Button
                variant={user.status === "active" ? "secondary" : "primary"}
                size="sm"
                onClick={() => setConfirmStatus(true)}
              >
                {user.status === "active" ? "Deactivate account" : "Activate account"}
              </Button>
            ) : (
              <span className="text-xs text-faint">This is your account.</span>
            )}
            <Button
              size="sm"
              disabled={!changed || patch.isPending}
              onClick={() => patch.mutate({ display_name: name, department_id: departmentId })}
            >
              {patch.isPending ? "Saving…" : "Save changes"}
            </Button>
          </div>
        </div>
      )}

      {tab === "activity" && (
        <div className="p-4">
          {activity.isLoading && <ListSkeleton rows={6} />}
          {activity.isError && <ErrorState message="Could not load activity" onRetry={() => activity.refetch()} />}
          {activity.isSuccess && (activity.data.entries ?? []).length === 0 && (
            <EmptyState icon="activity" title="No recorded activity" hint="Sign-ins and administrative actions by this user will appear here." />
          )}
          <ul className="space-y-px">
            {(activity.data?.entries ?? []).map((e) => (
              <li key={e.id} className="flex items-center gap-3 rounded-[7px] px-2 py-1.5 text-[13px] hover:bg-background">
                <span className={cx("h-1.5 w-1.5 shrink-0 rounded-full", e.action === "auth.login_failed" ? "bg-warning" : "bg-border-strong")} />
                <span className="code-value min-w-0 flex-1 truncate">{e.action}</span>
                {e.ip && <span className="code-value shrink-0 text-faint">{e.ip}</span>}
                <span className="shrink-0 text-[11px] text-faint">{formatDate(e.created_at)}</span>
              </li>
            ))}
          </ul>
          {activity.isSuccess && (activity.data.total ?? 0) > 30 && (
            <p className="mt-3 text-center text-xs text-faint">Showing the 30 most recent events. Open Logs & Audit for the full history.</p>
          )}
        </div>
      )}

      <ConfirmDialog
        open={confirmStatus}
        title={user.status === "active" ? "Deactivate this account?" : "Activate this account?"}
        body={
          user.status === "active"
            ? `${user.email} will no longer be able to sign in, and all active sessions will be revoked immediately.`
            : `${user.email} will be able to sign in again.`
        }
        confirmLabel={user.status === "active" ? "Deactivate" : "Activate"}
        danger={user.status === "active"}
        onCancel={() => setConfirmStatus(false)}
        onConfirm={() => {
          setConfirmStatus(false);
          patch.mutate({ status: user.status === "active" ? "disabled" : "active" });
        }}
      />
    </Drawer>
  );
}

export default function UsersPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const me = useMe();
  const [search, setSearch] = useState("");
  const [departmentFilter, setDepartmentFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [confirmToggle, setConfirmToggle] = useState<AdminUser | null>(null);

  const users = useQuery({ queryKey: ["admin", "users"], queryFn: () => api.get<{ users: AdminUser[] }>("/api/v1/users") });
  const departments = useQuery({ queryKey: ["departments"], queryFn: () => api.get<{ departments: Department[] }>("/api/v1/departments") });

  const patch = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Record<string, unknown> }) => api.patch(`/api/v1/users/${id}`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin"] });
      toast("success", "User updated");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Update failed"),
  });

  const list = useMemo(() => {
    const all = users.data?.users ?? [];
    const q = search.trim().toLowerCase();
    return all.filter((u) => {
      if (q && !`${u.email} ${u.display_name} ${u.mailbox_address ?? ""}`.toLowerCase().includes(q)) return false;
      if (departmentFilter && u.department_id !== departmentFilter) return false;
      if (statusFilter !== "all" && u.status !== statusFilter) return false;
      return true;
    });
  }, [users.data, search, departmentFilter, statusFilter]);

  const selected = users.data?.users.find((u) => u.id === selectedId) ?? null;
  const selfEmail = me.data?.email ?? "";

  return (
    <div>
      <header className="page-header">
        <div>
          <h1 className="page-title">Users</h1>
          <p className="page-description">Accounts, roles, mailboxes and department membership across the organization.</p>
        </div>
        <Button onClick={() => setCreateOpen(true)}><Icon name="plus" className="h-3.5 w-3.5" /> New user</Button>
      </header>

      {/* Toolbar */}
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1 sm:max-w-xs">
          <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search name, email, mailbox…"
            className="h-8 w-full rounded-[7px] border border-border-strong bg-surface-elevated pl-8 pr-2.5 text-[13px] outline-none placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary/15"
          />
        </div>
        <select value={departmentFilter} onChange={(e) => setDepartmentFilter(e.target.value)} aria-label="Filter by department">
          <option value="">All departments</option>
          {(departments.data?.departments ?? []).map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
        </select>
        <Segmented
          value={statusFilter}
          onChange={setStatusFilter}
          options={[{ value: "all", label: "All" }, { value: "active", label: "Active" }, { value: "disabled", label: "Disabled" }]}
        />
        <span className="ml-auto text-xs text-muted-foreground">
          {users.isSuccess && `${list.length} of ${users.data.users.length} users`}
        </span>
      </div>

      {users.isLoading && <div className="rounded-[10px] border border-border bg-surface-elevated"><ListSkeleton rows={8} /></div>}
      {users.isError && <ErrorState message="Could not load users" onRetry={() => users.refetch()} />}
      {users.isSuccess && list.length === 0 && (
        <div className="rounded-[10px] border border-border bg-surface-elevated">
          <EmptyState
            icon="users"
            title={users.data.users.length === 0 ? "No users yet" : "No users match the filters"}
            hint={users.data.users.length === 0 ? "Create the first account to give your team access." : "Adjust the search or filters."}
            action={users.data.users.length === 0 ? <Button size="sm" onClick={() => setCreateOpen(true)}>New user</Button> : undefined}
          />
        </div>
      )}

      {users.isSuccess && list.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full min-w-[720px] text-[13px]">
            <thead>
              <tr className="border-b border-border text-left">
                <th className="px-3">User</th>
                <th className="px-3">Mailbox</th>
                <th className="px-3">Department</th>
                <th className="w-24 px-3">Status</th>
                <th className="w-28 px-3">Created</th>
                <th className="w-12 px-3" aria-label="Actions" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {list.map((u) => (
                <tr key={u.id} className="cursor-pointer" onClick={() => setSelectedId(u.id)}>
                  <td className="px-3">
                    <span className="flex items-center gap-2.5 py-1.5">
                      <Avatar name={u.display_name || u.email} size="md" />
                      <span className="min-w-0">
                        <span className="block truncate font-medium leading-4">{u.display_name || u.email}</span>
                        <span className="block truncate text-xs text-muted-foreground">{u.email}</span>
                      </span>
                      {u.email === selfEmail && <Badge tone="accent">you</Badge>}
                    </span>
                  </td>
                  <td className="max-w-48 truncate px-3 text-muted-foreground">{u.mailbox_address || "—"}</td>
                  <td className="px-3">{u.department_name ? <Badge tone="neutral">{u.department_name}</Badge> : <span className="text-faint">—</span>}</td>
                  <td className="px-3">
                    <Badge tone={u.status === "active" ? "success" : "danger"}>{u.status}</Badge>
                  </td>
                  <td className="code-value whitespace-nowrap px-3 text-muted-foreground">{formatDate(u.created_at)}</td>
                  <td className="px-3" onClick={(e) => e.stopPropagation()}>
                    <Menu
                      trigger={<IconButton size="sm" label="User actions" icon="more-horizontal" />}
                      items={[
                        { label: "View details", icon: "user", onSelect: () => setSelectedId(u.id) },
                        { label: "View activity", icon: "activity", onSelect: () => setSelectedId(u.id) },
                        { type: "separator" },
                        ...(u.email !== selfEmail
                          ? [{
                              label: u.status === "active" ? "Deactivate" : "Activate",
                              icon: u.status === "active" ? "x" : "check",
                              danger: u.status === "active",
                              onSelect: () => setConfirmToggle(u),
                            }]
                          : []),
                      ]}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <CreateUserDrawer open={createOpen} onClose={() => setCreateOpen(false)} />
      {selected && (
        <UserDrawer
          user={selected}
          departments={departments.data?.departments ?? []}
          selfEmail={selfEmail}
          onClose={() => setSelectedId(null)}
        />
      )}

      <ConfirmDialog
        open={!!confirmToggle}
        title={confirmToggle?.status === "active" ? "Deactivate this account?" : "Activate this account?"}
        body={
          confirmToggle?.status === "active"
            ? `${confirmToggle?.email} will no longer be able to sign in, and all active sessions will be revoked immediately.`
            : `${confirmToggle?.email} will be able to sign in again.`
        }
        confirmLabel={confirmToggle?.status === "active" ? "Deactivate" : "Activate"}
        danger={confirmToggle?.status === "active"}
        onCancel={() => setConfirmToggle(null)}
        onConfirm={() => {
          if (confirmToggle) patch.mutate({ id: confirmToggle.id, body: { status: confirmToggle.status === "active" ? "disabled" : "active" } });
          setConfirmToggle(null);
        }}
      />
    </div>
  );
}
