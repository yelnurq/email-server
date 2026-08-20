"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatDate } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type Webhook = {
  id: string;
  organization_id: string;
  url: string;
  events: string[];
  enabled: boolean;
  created_at: string;
};

type Delivery = {
  id: number;
  event_id: string;
  event_type: string;
  status: string;
  attempts: number;
  last_status_code?: number;
  last_error?: string;
  next_attempt_at: string;
  delivered_at?: string;
  created_at: string;
};

const ALL_EVENTS = ["email.accepted", "email.delivered_local", "email.failed"];

export default function WebhooksPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [url, setUrl] = useState("");
  const [selEvents, setSelEvents] = useState<string[]>(ALL_EVENTS);
  const [createdSecret, setCreatedSecret] = useState("");
  const [openLog, setOpenLog] = useState<string | null>(null);

  const hooks = useQuery({
    queryKey: ["admin", "webhooks"],
    queryFn: () => api.get<{ webhooks: Webhook[] }>("/api/v1/webhooks"),
  });

  const deliveries = useQuery({
    queryKey: ["admin", "webhook-deliveries", openLog],
    queryFn: () => api.get<{ deliveries: Delivery[] }>(`/api/v1/webhooks/${openLog}/deliveries`),
    enabled: !!openLog,
    refetchInterval: 5000,
  });

  const create = useMutation({
    mutationFn: () =>
      api.post<{ id: string; secret: string }>("/api/v1/webhooks", { url, events: selEvents }),
    onSuccess: (res) => {
      setUrl("");
      setCreatedSecret(res.secret);
      qc.invalidateQueries({ queryKey: ["admin", "webhooks"] });
      toast("success", "Webhook created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create webhook"),
  });

  const toggle = useMutation({
    mutationFn: (h: Webhook) => api.patch(`/api/v1/webhooks/${h.id}`, { enabled: !h.enabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "webhooks"] }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/webhooks/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "webhooks"] });
      toast("success", "Webhook deleted");
    },
  });

  const retry = useMutation({
    mutationFn: ({ hookID, deliveryID }: { hookID: string; deliveryID: number }) =>
      api.post(`/api/v1/webhooks/${hookID}/deliveries/${deliveryID}/retry`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "webhook-deliveries"] });
      toast("success", "Delivery rescheduled");
    },
  });

  return (
    <div className="mx-auto max-w-5xl">
      <h1 className="mb-4 text-lg font-semibold">Webhooks</h1>

      <form
        className="mb-4 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
        onSubmit={(e) => {
          e.preventDefault();
          if (url.trim() && selEvents.length > 0) create.mutate();
        }}
      >
        <div className="flex flex-wrap items-end gap-3">
          <div className="min-w-64 flex-1">
            <Input
              label="Endpoint URL"
              placeholder="https://example.com/hooks/mail"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
          </div>
          <div className="flex gap-3 pb-2">
            {ALL_EVENTS.map((ev) => (
              <label key={ev} className="flex items-center gap-1.5 text-xs text-neutral-600 dark:text-neutral-400">
                <input
                  type="checkbox"
                  className="h-3.5 w-3.5 accent-indigo-600"
                  checked={selEvents.includes(ev)}
                  onChange={() =>
                    setSelEvents((prev) =>
                      prev.includes(ev) ? prev.filter((x) => x !== ev) : [...prev, ev],
                    )
                  }
                />
                {ev}
              </label>
            ))}
          </div>
          <Button type="submit" disabled={create.isPending || !url.trim() || selEvents.length === 0}>
            {create.isPending ? "Creating…" : "Create webhook"}
          </Button>
        </div>
      </form>

      {createdSecret && (
        <div className="mb-4 rounded-2xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-700 dark:bg-amber-950">
          <p className="text-sm font-semibold text-amber-800 dark:text-amber-200">
            Signing secret — copy now, shown only once. Verify X-Mailplatform-Signature =
            v1=HMAC-SHA256(secret, timestamp + &quot;.&quot; + body).
          </p>
          <div className="mt-2 flex items-center gap-2">
            <code className="flex-1 overflow-x-auto rounded-lg bg-white px-3 py-2 font-mono text-xs dark:bg-neutral-900">
              {createdSecret}
            </code>
            <Button
              variant="secondary"
              onClick={() => {
                navigator.clipboard.writeText(createdSecret);
                toast("success", "Copied");
              }}
            >
              Copy
            </Button>
            <Button variant="ghost" onClick={() => setCreatedSecret("")}>
              Done
            </Button>
          </div>
        </div>
      )}

      {hooks.isLoading && <PageLoader />}
      {hooks.isSuccess && hooks.data.webhooks.length === 0 && (
        <EmptyState title="No webhooks yet" hint="Receive signed events about mail activity." />
      )}
      {hooks.isSuccess &&
        hooks.data.webhooks.map((h) => (
          <div
            key={h.id}
            className="mb-3 rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900"
          >
            <div className="flex flex-wrap items-center gap-3 p-4">
              <div className="min-w-0 flex-1">
                <p className="truncate font-mono text-sm">{h.url}</p>
                <p className="mt-0.5 text-xs text-neutral-400">{h.events.join(", ")}</p>
              </div>
              <button
                onClick={() => toggle.mutate(h)}
                className={
                  h.enabled
                    ? "rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
                    : "rounded-full bg-neutral-100 px-2 py-0.5 text-xs font-medium text-neutral-500 dark:bg-neutral-800"
                }
              >
                {h.enabled ? "enabled" : "disabled"}
              </button>
              <Button variant="ghost" onClick={() => setOpenLog(openLog === h.id ? null : h.id)}>
                {openLog === h.id ? "Hide log" : "Delivery log"}
              </Button>
              <Button variant="ghost" onClick={() => remove.mutate(h.id)}>
                Delete
              </Button>
            </div>
            {openLog === h.id && (
              <div className="border-t border-neutral-100 p-4 dark:border-neutral-800">
                {deliveries.isLoading && <PageLoader />}
                {deliveries.isSuccess && deliveries.data.deliveries.length === 0 && (
                  <p className="text-sm text-neutral-400">No deliveries yet.</p>
                )}
                {deliveries.isSuccess && deliveries.data.deliveries.length > 0 && (
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="text-left uppercase tracking-wide text-neutral-400">
                        <th className="py-1.5 pr-3 font-medium">Event</th>
                        <th className="py-1.5 pr-3 font-medium">Status</th>
                        <th className="py-1.5 pr-3 font-medium">Attempts</th>
                        <th className="py-1.5 pr-3 font-medium">Last result</th>
                        <th className="py-1.5 pr-3 font-medium">Created</th>
                        <th className="py-1.5 font-medium"></th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-neutral-100 dark:divide-neutral-800">
                      {deliveries.data.deliveries.map((d) => (
                        <tr key={d.id}>
                          <td className="py-1.5 pr-3 font-mono">{d.event_type}</td>
                          <td className="py-1.5 pr-3">
                            <span
                              className={
                                d.status === "delivered"
                                  ? "text-emerald-600"
                                  : d.status === "failed"
                                    ? "text-red-600"
                                    : "text-amber-600"
                              }
                            >
                              {d.status}
                            </span>
                          </td>
                          <td className="py-1.5 pr-3">{d.attempts}</td>
                          <td className="py-1.5 pr-3 text-neutral-500">
                            {d.last_status_code ?? d.last_error ?? "—"}
                          </td>
                          <td className="py-1.5 pr-3 text-neutral-500">{formatDate(d.created_at)}</td>
                          <td className="py-1.5 text-right">
                            {d.status !== "delivered" && (
                              <button
                                className="text-indigo-600 hover:underline"
                                onClick={() => retry.mutate({ hookID: h.id, deliveryID: d.id })}
                              >
                                Retry
                              </button>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}
              </div>
            )}
          </div>
        ))}
    </div>
  );
}
