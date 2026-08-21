"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, type Organization, type Project } from "@/lib/api";
import { Badge, Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";

// ProjectsPanel lists and creates the projects of one organization (§79).
function ProjectsPanel({ org }: { org: Organization }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const projects = useQuery({
    queryKey: ["admin", "org", org.id, "projects"],
    queryFn: () => api.get<{ projects: Project[] }>(`/api/v1/organizations/${org.id}/projects`),
  });
  const create = useMutation({
    mutationFn: () => api.post(`/api/v1/organizations/${org.id}/projects`, { name }),
    onSuccess: () => {
      setName("");
      qc.invalidateQueries({ queryKey: ["admin", "org", org.id, "projects"] });
      toast("success", "Project created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create project"),
  });

  return (
    <div className="border-t border-border bg-background px-4 py-4">
      <form
        className="mb-3 flex flex-wrap items-end gap-2"
        onSubmit={(e) => { e.preventDefault(); if (name.trim()) create.mutate(); }}
      >
        <div className="min-w-56 flex-1">
          <Input label="New project" placeholder="Marketing" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <Button type="submit" size="sm" disabled={create.isPending || !name.trim()}>Add project</Button>
      </form>
      {projects.isLoading && <p className="text-xs text-muted-foreground">Loading projects…</p>}
      {projects.isSuccess && (
        <div className="flex flex-wrap gap-2">
          {projects.data.projects.map((p) => (
            <span key={p.id} className="inline-flex items-center gap-2 rounded-[8px] border border-border bg-surface-elevated px-2.5 py-1.5 text-[13px]">
              <Icon name="layers" className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="font-medium">{p.name}</span>
              <span className="text-xs text-muted-foreground">{p.domains} domain{p.domains === 1 ? "" : "s"}</span>
              {p.status !== "active" && <Badge tone="neutral">{p.status}</Badge>}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

export default function OrganizationsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);

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
    <div className="space-y-6">
      <section className="mb-8">
        <div className="mt-4 grid gap-6 lg:grid-cols-12">
          <div className="lg:col-span-8">
            <h1 className="page-title">
              Organizations
            </h1>
            <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
              Organizations are the top-level tenants of the platform. Users, domains and mailboxes always belong to exactly one organization.
            </p>
          </div>
          <div className="border border-border bg-surface p-4 lg:col-span-4">
            <p className="text-xs font-medium text-muted-foreground">
              Created today
            </p>
            <p className="mt-3 font-serif text-4xl text-foreground">
              {orgs.data?.organizations.length ?? 0}
            </p>
            <p className="mt-2 text-sm text-muted-foreground font-body">
              Active organizations visible in the control plane.
            </p>
          </div>
        </div>
      </section>

      <section className="qazera-panel p-6">
        <p className="text-base font-semibold text-foreground">New entry</p>
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
        <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left text-xs">
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Slug</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Projects</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {orgs.data.organizations.map((o) => (
                <>
                  <tr key={o.id} className="cursor-pointer" onClick={() => setExpanded(expanded === o.id ? null : o.id)}>
                    <td className="px-4 py-3 font-semibold text-foreground">{o.name}</td>
                    <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{o.slug}</td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
                        {o.status}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center gap-1 text-xs font-medium text-primary">
                        {expanded === o.id ? "Hide" : "Manage"} projects
                        <Icon name={expanded === o.id ? "chevron-up" : "chevron-down"} className="h-3 w-3" />
                      </span>
                    </td>
                  </tr>
                  {expanded === o.id && (
                    <tr key={`${o.id}-projects`}>
                      <td colSpan={4} className="p-0">
                        <ProjectsPanel org={o} />
                      </td>
                    </tr>
                  )}
                </>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
