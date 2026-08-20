"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, formatDate } from "@/lib/api";
import { Button, EmptyState, Input, PageLoader, useToast } from "@/components/ui";

type Webhook = {
  id: string;
  url: string;
  events: string[];
  status: string;
  secret_prefix: string;
};

type Delivery = {
  id: string;
  event_type: string;
  status: string;
  attempts: number;
  last_result: string;
  created_at: string;
};

export default function WebhooksPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<string[]>(["mail.sent"]);

  const hooks = useQuery({
    queryKey: ["admin", "webhooks"],
    queryFn: () => api.get<{ webhooks: Webhook[] }>("/api/v1/webhooks"),
  });

  const create = useMutation({
    mutationFn: () => api.post("/api/v1/webhooks", { url, events }),
    onSuccess: () => {
      setUrl("");
      qc.invalidateQueries({ queryKey: ["admin", "webhooks"] });
      toast("success", "Webhook created");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not create webhook"),
  });

  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/api/v1/webhooks/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "webhooks"] });
      toast("success", "Webhook removed");
    },
  });

  const eventOptions = ["mail.sent", "mail.delivered", "mail.bounced", "webhook.test"];

  return (
    <div className="space-y-6">
      <section className="mb-8">
        <h1 className="page-title">
          Webhooks
        </h1>
        <p className="mt-2 max-w-3xl text-sm leading-5 text-muted-foreground">
          Signed webhook deliveries push mail events to your systems, with per-delivery status and retries.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="text-base font-semibold text-foreground">New webhook</p>
        <form
          className="mt-4 space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (url.trim() && events.length > 0) create.mutate();
          }}
        >
          <div className="grid gap-4 lg:grid-cols-[1fr_auto]">
            <Input label="Delivery URL" placeholder="https://example.com/webhooks/qazera" value={url} onChange={(e) => setUrl(e.target.value)} />
            <div className="flex flex-wrap gap-2">
              {eventOptions.map((ev) => (
                <label key={ev} className="flex items-center gap-2 rounded-lg border border-border bg-surface px-3 py-2 font-mono text-xs">
                  <input
                    type="checkbox"
                    className="h-4 w-4 accent-primary"
                    checked={events.includes(ev)}
                    onChange={(e) =>
                      setEvents((prev) =>
                        e.target.checked ? [...prev, ev] : prev.filter((x) => x !== ev),
                      )
                    }
                  />
                  {ev}
                </label>
              ))}
            </div>
          </div>
          <Button type="submit" disabled={create.isPending || !url.trim() || events.length === 0}>
            Create webhook
          </Button>
        </form>
      </section>

      {hooks.isLoading && <PageLoader label="Loading webhooks" />}
      {hooks.isSuccess && hooks.data.webhooks.length === 0 && (
        <EmptyState title="No webhooks yet" hint="Create one to receive signed events." />
      )}
      {hooks.isSuccess && hooks.data.webhooks.length > 0 && (
        <div className="space-y-4">
          {hooks.data.webhooks.map((h) => (
            <div key={h.id} className="border border-border bg-surface-elevated">
              <div className="flex flex-wrap items-center gap-3 border-b border-border p-4">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-xs">{h.url}</p>
                  <p className="mt-1 text-xs text-muted-foreground font-mono">{h.events.join(", ")}</p>
                </div>
                <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
                  {h.status}
                </span>
                <Button variant="ghost" onClick={() => remove.mutate(h.id)}>Remove</Button>
              </div>
              <div className="p-4">
                <p className="text-base font-semibold text-foreground">Deliveries</p>
                <WebhookDeliveries webhookId={h.id} />
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function WebhookDeliveries({ webhookId }: { webhookId: string }) {
  const deliveries = useQuery({
    queryKey: ["admin", "webhooks", webhookId, "deliveries"],
    queryFn: () => api.get<{ deliveries: Delivery[] }>(`/api/v1/webhooks/${webhookId}/deliveries`),
    refetchInterval: 15_000,
  });

  if (deliveries.isLoading) return <PageLoader label="Loading deliveries" />;
  if (deliveries.isSuccess && deliveries.data.deliveries.length === 0) {
    return <p className="mt-3 text-sm text-muted-foreground font-body">No deliveries yet.</p>;
  }

  return (
    <div className="mt-3 overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-border text-left text-xs text-muted-foreground">
            <th className="px-3 py-2 font-medium">Event</th>
            <th className="px-3 py-2 font-medium">Status</th>
            <th className="px-3 py-2 font-medium">Attempts</th>
            <th className="px-3 py-2 font-medium">Last result</th>
            <th className="px-3 py-2 font-medium">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {deliveries.data?.deliveries.map((d) => (
            <tr key={d.id}>
              <td className="px-3 py-2 font-mono">{d.event_type}</td>
              <td className="px-3 py-2">
                <span className="border border-border bg-surface px-2 py-1 font-mono text-[10px]">
                  {d.status}
                </span>
              </td>
              <td className="px-3 py-2">{d.attempts}</td>
              <td className="px-3 py-2 text-muted-foreground">{d.last_result}</td>
              <td className="px-3 py-2 text-muted-foreground">{formatDate(d.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
