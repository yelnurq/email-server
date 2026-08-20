"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { Button, ConfirmDialog, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type ApiKey = {
  id: string;
  name: string;
  prefix: string;
  scopes: string[];
  last_used_at: string | null;
  status: string;
};

export default function ApiKeysPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(["emails:write"]);
  const [created, setCreated] = useState<{ secret: string; prefix: string } | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<ApiKey | null>(null);

  const keys = useQuery({
    queryKey: ["admin", "api-keys"],
    queryFn: () => api.get<{ keys: ApiKey[] }>("/api/v1/api-keys"),
  });

  const create = useMutation({
    mutationFn: () => api.post<{ secret: string; prefix: string }>("/api/v1/api-keys", { name, scopes }),
    onSuccess: (res) => {
      setName("");
      setCreated(res);
      qc.invalidateQueries({ queryKey: ["admin", "api-keys"] });
      toast("success", "API key created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create key"),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/api-keys/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "api-keys"] });
      toast("success", "API key revoked");
      setRevokeTarget(null);
    },
    onError: () => toast("error", "Could not revoke key"),
  });

  const availableScopes = ["emails:write", "emails:read", "webhooks:write"];

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">08. Access</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          API Keys
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          API keys are the external circulation desk. They allow trusted systems to publish mail
          or inspect events without using a browser session.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">New key</p>
        <form
          className="mt-4 flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim() && scopes.length > 0) create.mutate();
          }}
        >
          <div className="min-w-64 flex-1">
            <Input label="Name" placeholder="CI pipeline" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-wrap gap-2">
            {availableScopes.map((scope) => (
              <label
                key={scope}
                className="flex items-center gap-2 border border-[#111111] bg-[#e5e5e0] px-3 py-2 font-mono text-xs uppercase tracking-[0.18em]"
              >
                <input
                  type="checkbox"
                  className="h-4 w-4 accent-[#111111]"
                  checked={scopes.includes(scope)}
                  onChange={(e) =>
                    setScopes((prev) =>
                      e.target.checked ? [...prev, scope] : prev.filter((x) => x !== scope),
                    )
                  }
                />
                {scope}
              </label>
            ))}
          </div>
          <Button type="submit" disabled={create.isPending || !name.trim() || scopes.length === 0}>
            Create key
          </Button>
        </form>
      </section>

      {created && (
        <section className="qazera-panel border-[#cc0000] bg-[#f9f9f7] p-6">
          <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">
            Secret shown once
          </p>
          <p className="mt-2 text-sm leading-6 font-body">
            Save this secret now. It will not be shown again.
          </p>
          <div className="mt-4 border border-[#111111] bg-white p-4 font-mono text-xs break-all">
            <div>prefix: {created.prefix}</div>
            <div>secret: {created.secret}</div>
          </div>
          <div className="mt-4 flex gap-2">
            <Button onClick={() => setCreated(null)}>Done</Button>
          </div>
        </section>
      )}

      {keys.isLoading && <PageLoader label="Loading API keys" />}
      {keys.isSuccess && keys.data.keys.length === 0 && (
        <EmptyState title="No API keys yet" hint="Create one to use the Email API." />
      )}
      {keys.isSuccess && keys.data.keys.length > 0 && (
        <div className="overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[#111111] text-left font-mono text-xs uppercase tracking-[0.3em]">
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Key</th>
                <th className="px-4 py-3 font-medium">Scopes</th>
                <th className="px-4 py-3 font-medium">Last used</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#111111]">
              {keys.data.keys.map((k) => (
                <tr key={k.id}>
                  <td className="px-4 py-3 font-semibold">{k.name}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{k.prefix}…</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{k.scopes.join(", ")}</td>
                  <td className="px-4 py-3 font-mono text-xs text-[#525252]">{k.last_used_at || "—"}</td>
                  <td className="px-4 py-3">
                    <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">
                      {k.status}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Button variant="ghost" onClick={() => setRevokeTarget(k)}>
                      Revoke
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmDialog
        open={!!revokeTarget}
        title="Revoke API key?"
        body="This will immediately disable access for systems using the key."
        confirmLabel="Revoke"
        danger
        onCancel={() => setRevokeTarget(null)}
        onConfirm={() => revokeTarget && revoke.mutate(revokeTarget.id)}
      />
    </div>
  );
}
