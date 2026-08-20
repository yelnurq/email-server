"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatDate } from "@/lib/api";
import { Button, ConfirmDialog, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type ApiKey = {
  id: string;
  organization_id: string;
  name: string;
  prefix: string;
  scopes: string[];
  status: string;
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
};

type CreatedKey = { id: string; name: string; prefix: string; scopes: string[]; secret: string };

export default function ApiKeysPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<string[]>(["emails.send", "emails.read"]);
  const [created, setCreated] = useState<CreatedKey | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<ApiKey | null>(null);

  const keys = useQuery({
    queryKey: ["admin", "api-keys"],
    queryFn: () => api.get<{ api_keys: ApiKey[] }>("/api/v1/api-keys"),
  });

  const create = useMutation({
    mutationFn: () => api.post<CreatedKey>("/api/v1/api-keys", { name, scopes }),
    onSuccess: (k) => {
      setName("");
      setCreated(k);
      qc.invalidateQueries({ queryKey: ["admin", "api-keys"] });
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create key"),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/api-keys/${id}`),
    onSuccess: () => {
      setRevokeTarget(null);
      qc.invalidateQueries({ queryKey: ["admin", "api-keys"] });
      toast("success", "Key revoked");
    },
    onError: () => toast("error", "Could not revoke key"),
  });

  function toggleScope(s: string) {
    setScopes((prev) => (prev.includes(s) ? prev.filter((x) => x !== s) : [...prev, s]));
  }

  return (
    <div className="mx-auto max-w-4xl">
      <h1 className="mb-4 text-lg font-semibold">API Keys</h1>

      <form
        className="mb-4 flex flex-wrap items-end gap-3 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim() && scopes.length > 0) create.mutate();
        }}
      >
        <div className="min-w-48 flex-1">
          <Input label="Name" placeholder="CI pipeline" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="flex gap-3 pb-2">
          {["emails.send", "emails.read"].map((s) => (
            <label key={s} className="flex items-center gap-1.5 text-xs text-neutral-600 dark:text-neutral-400">
              <input
                type="checkbox"
                className="h-3.5 w-3.5 accent-indigo-600"
                checked={scopes.includes(s)}
                onChange={() => toggleScope(s)}
              />
              {s}
            </label>
          ))}
        </div>
        <Button type="submit" disabled={create.isPending || !name.trim() || scopes.length === 0}>
          {create.isPending ? "Creating…" : "Create key"}
        </Button>
      </form>

      {created && (
        <div className="mb-4 rounded-2xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-700 dark:bg-amber-950">
          <p className="text-sm font-semibold text-amber-800 dark:text-amber-200">
            Copy this secret now — it will not be shown again.
          </p>
          <div className="mt-2 flex items-center gap-2">
            <code className="flex-1 overflow-x-auto rounded-lg bg-white px-3 py-2 font-mono text-xs dark:bg-neutral-900">
              {created.secret}
            </code>
            <Button
              variant="secondary"
              onClick={() => {
                navigator.clipboard.writeText(created.secret);
                toast("success", "Copied to clipboard");
              }}
            >
              Copy
            </Button>
            <Button variant="ghost" onClick={() => setCreated(null)}>
              Done
            </Button>
          </div>
        </div>
      )}

      {keys.isLoading && <PageLoader />}
      {keys.isSuccess && keys.data.api_keys.length === 0 && (
        <EmptyState title="No API keys yet" hint="Create a key to use POST /api/v1/emails." />
      )}
      {keys.isSuccess && keys.data.api_keys.length > 0 && (
        <div className="overflow-x-auto rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-100 text-left text-xs uppercase tracking-wide text-neutral-400 dark:border-neutral-800">
                <th className="px-4 py-2.5 font-medium">Name</th>
                <th className="px-4 py-2.5 font-medium">Key</th>
                <th className="px-4 py-2.5 font-medium">Scopes</th>
                <th className="px-4 py-2.5 font-medium">Last used</th>
                <th className="px-4 py-2.5 font-medium">Status</th>
                <th className="px-4 py-2.5 font-medium"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
              {keys.data.api_keys.map((k) => (
                <tr key={k.id}>
                  <td className="px-4 py-2.5 font-medium">{k.name}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-neutral-500">{k.prefix}…</td>
                  <td className="px-4 py-2.5 text-xs text-neutral-500">{k.scopes.join(", ")}</td>
                  <td className="px-4 py-2.5 text-xs text-neutral-500">
                    {k.last_used_at ? formatDate(k.last_used_at) : "never"}
                  </td>
                  <td className="px-4 py-2.5">
                    <span
                      className={
                        k.status === "active"
                          ? "rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                          : "rounded-full bg-red-50 px-2 py-0.5 text-xs font-medium text-red-600 dark:bg-red-950 dark:text-red-300"
                      }
                    >
                      {k.status}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    {k.status === "active" && (
                      <Button variant="ghost" onClick={() => setRevokeTarget(k)}>
                        Revoke
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <ConfirmDialog
        open={!!revokeTarget}
        title={`Revoke "${revokeTarget?.name}"?`}
        body="Applications using this key will immediately lose access. This cannot be undone."
        confirmLabel="Revoke"
        danger
        onCancel={() => setRevokeTarget(null)}
        onConfirm={() => revokeTarget && revoke.mutate(revokeTarget.id)}
      />
    </div>
  );
}
