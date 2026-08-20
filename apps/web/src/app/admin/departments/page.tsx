"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type AdminUser, type Department } from "@/lib/api";
import { Button, ConfirmDialog, EmptyState, Input, PageLoader, Textarea, useToast } from "@/components/ui";
import { useI18n } from "@/components/providers";

export default function DepartmentsPage() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [managerId, setManagerId] = useState("");
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [editManagerId, setEditManagerId] = useState("");
  const [memberIds, setMemberIds] = useState<string[]>([]);
  const [deleteTarget, setDeleteTarget] = useState<Department | null>(null);

  const departments = useQuery({
    queryKey: ["departments"],
    queryFn: () => api.get<{ departments: Department[] }>("/api/v1/departments"),
  });
  const users = useQuery({
    queryKey: ["admin", "users"],
    queryFn: () => api.get<{ users: AdminUser[] }>("/api/v1/users"),
  });

  const refresh = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ["departments"] }),
      qc.invalidateQueries({ queryKey: ["admin", "users"] }),
    ]);
  };

  const create = useMutation({
    mutationFn: () => api.post("/api/v1/departments", { name, description, manager_user_id: managerId }),
    onSuccess: async () => {
      setName(""); setDescription(""); setManagerId("");
      await refresh(); toast("success", t("departmentCreated"));
    },
    onError: (error) => toast("error", error instanceof ApiError ? error.message : t("operationFailed")),
  });

  const save = useMutation({
    mutationFn: async () => {
      if (!editingId) return;
      await api.patch(`/api/v1/departments/${editingId}`, {
        name: editName, description: editDescription, manager_user_id: editManagerId,
      });
      await api.put(`/api/v1/departments/${editingId}/members`, { user_ids: memberIds });
    },
    onSuccess: async () => { setEditingId(null); await refresh(); toast("success", t("departmentSaved")); },
    onError: (error) => toast("error", error instanceof ApiError ? error.message : t("operationFailed")),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/departments/${id}`),
    onSuccess: async () => { setDeleteTarget(null); await refresh(); toast("success", t("departmentDeleted")); },
    onError: (error) => toast("error", error instanceof ApiError ? error.message : t("operationFailed")),
  });

  function startEditing(department: Department) {
    setEditingId(department.id);
    setEditName(department.name);
    setEditDescription(department.description);
    setEditManagerId(department.manager_user_id ?? "");
    setMemberIds((users.data?.users ?? []).filter((user) => user.department_id === department.id).map((user) => user.id));
  }

  const allUsers = users.data?.users ?? [];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="page-title">{t("departments")}</h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">{t("departmentsHint")}</p>
      </header>

      <section className="glass-panel rounded-xl p-5 sm:p-6">
        <h2 className="text-base font-semibold">{t("createDepartment")}</h2>
        <form className="mt-4 grid gap-4 lg:grid-cols-[1fr_1.4fr_1fr_auto] lg:items-end" onSubmit={(event) => { event.preventDefault(); create.mutate(); }}>
          <Input label={t("departmentName")} value={name} onChange={(event) => setName(event.target.value)} maxLength={120} required />
          <Input label={t("description")} value={description} onChange={(event) => setDescription(event.target.value)} maxLength={1000} />
          <label className="block">
            <span className="mb-1.5 block text-[13px] font-medium">{t("manager")}</span>
            <select className="h-10 w-full rounded-lg border border-border-strong bg-background px-3 text-sm" value={managerId} onChange={(event) => setManagerId(event.target.value)}>
              <option value="">{t("notAssigned")}</option>
              {allUsers.map((user) => <option key={user.id} value={user.id}>{user.display_name || user.email}</option>)}
            </select>
          </label>
          <Button type="submit" disabled={!name.trim() || create.isPending}>{create.isPending ? t("creating") : t("create")}</Button>
        </form>
      </section>

      {(departments.isLoading || users.isLoading) && <PageLoader label={t("loadingDepartments")} />}
      {departments.isSuccess && departments.data.departments.length === 0 && <EmptyState title={t("noDepartments")} hint={t("noDepartmentsHint")} />}

      <div className="grid gap-4 xl:grid-cols-2">
        {(departments.data?.departments ?? []).map((department) => {
          const members = allUsers.filter((user) => user.department_id === department.id);
          const editing = editingId === department.id;
          return (
            <section key={department.id} className="glass-panel rounded-xl p-5">
              {!editing ? (
                <>
                  <div className="flex items-start justify-between gap-4">
                    <div><h2 className="text-lg font-semibold">{department.name}</h2><p className="mt-1 text-sm text-muted-foreground">{department.description || t("noDescription")}</p></div>
                    <span className="rounded-full bg-primary/10 px-2.5 py-1 text-xs font-semibold text-primary">{department.employee_count} {t("people")}</span>
                  </div>
                  <div className="mt-4 border-t border-border pt-4">
                    <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{t("manager")}</p>
                    <p className="mt-1 text-sm">{department.manager_name || t("notAssigned")}{department.manager_email ? ` · ${department.manager_email}` : ""}</p>
                    <p className="mt-4 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{t("members")}</p>
                    <div className="mt-2 flex flex-wrap gap-2">
                      {members.length === 0 && <span className="text-sm text-muted-foreground">{t("noMembers")}</span>}
                      {members.slice(0, 8).map((user) => <span key={user.id} className="rounded-full border border-border bg-surface-elevated px-2.5 py-1 text-xs">{user.display_name || user.email}</span>)}
                      {members.length > 8 && <span className="px-2 py-1 text-xs text-muted-foreground">+{members.length - 8}</span>}
                    </div>
                  </div>
                  <div className="mt-5 flex gap-2"><Button variant="secondary" onClick={() => startEditing(department)}>{t("manage")}</Button><Button variant="ghost" onClick={() => setDeleteTarget(department)}>{t("delete")}</Button></div>
                </>
              ) : (
                <div className="space-y-4">
                  <Input label={t("departmentName")} value={editName} onChange={(event) => setEditName(event.target.value)} />
                  <Textarea label={t("description")} rows={3} value={editDescription} onChange={(event) => setEditDescription(event.target.value)} />
                  <label className="block"><span className="mb-1.5 block text-[13px] font-medium">{t("manager")}</span><select className="h-10 w-full rounded-lg border border-border-strong bg-background px-3 text-sm" value={editManagerId} onChange={(event) => setEditManagerId(event.target.value)}><option value="">{t("notAssigned")}</option>{allUsers.map((user) => <option key={user.id} value={user.id}>{user.display_name || user.email}</option>)}</select></label>
                  <div><div className="mb-2 flex items-center justify-between"><span className="text-[13px] font-medium">{t("members")}</span><button type="button" className="text-xs font-medium text-primary" onClick={() => setMemberIds(memberIds.length === allUsers.length ? [] : allUsers.map((user) => user.id))}>{memberIds.length === allUsers.length ? t("clearAll") : t("selectAll")}</button></div><div className="max-h-64 space-y-1 overflow-y-auto rounded-lg border border-border bg-background p-2">{allUsers.map((user) => <label key={user.id} className="flex cursor-pointer items-center gap-3 rounded-lg px-2.5 py-2 hover:bg-muted"><input type="checkbox" className="h-4 w-4 accent-primary" checked={memberIds.includes(user.id)} onChange={() => setMemberIds((current) => current.includes(user.id) ? current.filter((id) => id !== user.id) : [...current, user.id])} /><span className="min-w-0"><span className="block truncate text-sm font-medium">{user.display_name || user.email}</span><span className="block truncate text-xs text-muted-foreground">{user.email}{user.department_name ? ` · ${user.department_name}` : ""}</span></span></label>)}</div></div>
                  <div className="flex gap-2"><Button onClick={() => save.mutate()} disabled={!editName.trim() || save.isPending}>{save.isPending ? t("saving") : t("save")}</Button><Button variant="secondary" onClick={() => setEditingId(null)}>{t("cancel")}</Button></div>
                </div>
              )}
            </section>
          );
        })}
      </div>

      <ConfirmDialog open={Boolean(deleteTarget)} title={t("deleteDepartment")} body={t("deleteDepartmentHint")} confirmLabel={t("delete")} danger onCancel={() => setDeleteTarget(null)} onConfirm={() => deleteTarget && remove.mutate(deleteTarget.id)} />
    </div>
  );
}
