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
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="qazera-panel newsprint-texture p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">11. Dispatch</p>
        <h1 className="mt-4 font-serif text-5xl leading-[0.95] tracking-tighter text-[#111111] lg:text-7xl">
          Webhooks
        </h1>
        <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
          Webhooks carry events out of the system like press wires. They are signed, explicit, and
          observable.
        </p>
      </section>

      <section className="qazera-panel p-6">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">New webhook</p>
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
                <label key={ev} className="flex items-center gap-2 border border-[#111111] bg-[#e5e5e0] px-3 py-2 font-mono text-xs uppercase tracking-[0.18em]">
                  <input
                    type="checkbox"
                    className="h-4 w-4 accent-[#111111]"
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
            <div key={h.id} className="border border-[#111111] bg-[#f9f9f7]">
              <div className="flex flex-wrap items-center gap-3 border-b border-[#111111] p-4">
                <div className="min-w-0 flex-1">
                  <p className="truncate font-mono text-xs uppercase tracking-[0.3em]">{h.url}</p>
                  <p className="mt-1 text-xs text-[#525252] font-mono">{h.events.join(", ")}</p>
                </div>
                <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-xs uppercase tracking-[0.2em]">
                  {h.status}
                </span>
                <Button variant="ghost" onClick={() => remove.mutate(h.id)}>Remove</Button>
              </div>
              <div className="p-4">
                <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Deliveries</p>
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
    return <p className="mt-3 text-sm text-[#525252] font-body">No deliveries yet.</p>;
  }

  return (
    <div className="mt-3 overflow-x-auto border border-[#111111] bg-[#f9f9f7]">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-[#111111] text-left font-mono uppercase tracking-[0.3em] text-[#525252]">
            <th className="px-3 py-2 font-medium">Event</th>
            <th className="px-3 py-2 font-medium">Status</th>
            <th className="px-3 py-2 font-medium">Attempts</th>
            <th className="px-3 py-2 font-medium">Last result</th>
            <th className="px-3 py-2 font-medium">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-[#111111]">
          {deliveries.data?.deliveries.map((d) => (
            <tr key={d.id}>
              <td className="px-3 py-2 font-mono">{d.event_type}</td>
              <td className="px-3 py-2">
                <span className="border border-[#111111] bg-[#e5e5e0] px-2 py-1 font-mono text-[10px] uppercase tracking-[0.2em]">
                  {d.status}
                </span>
              </td>
              <td className="px-3 py-2">{d.attempts}</td>
              <td className="px-3 py-2 text-[#525252]">{d.last_result}</td>
              <td className="px-3 py-2 text-[#525252]">{formatDate(d.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
