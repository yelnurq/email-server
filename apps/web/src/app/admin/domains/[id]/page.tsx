"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  api, ApiError, formatDate,
  type Domain, type DomainDns, type DnsCheck, type DkimKey,
} from "@/lib/api";
import {
  Badge, Button, ConfirmDialog, EmptyState, PageLoader, Tabs, useToast,
} from "@/components/ui";
import { Icon } from "@/components/icons";

const DNS_TONE: Record<string, "success" | "warning" | "danger" | "neutral" | "accent"> = {
  verified: "success", warning: "warning", invalid: "danger",
  missing: "danger", dns_error: "warning", pending: "neutral",
};

function CopyButton({ text }: { text: string }) {
  const toast = useToast();
  return (
    <button
      type="button"
      title="Copy"
      className="grid h-6 w-6 shrink-0 place-items-center rounded-[6px] text-muted-foreground hover:bg-muted hover:text-foreground"
      onClick={() => {
        navigator.clipboard?.writeText(text).then(
          () => toast("success", "Copied"),
          () => toast("error", "Copy failed"),
        );
      }}
    >
      <Icon name="copy" className="h-3.5 w-3.5" />
    </button>
  );
}

export default function DomainDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [tab, setTab] = useState("overview");

  const domain = useQuery({
    queryKey: ["admin", "domain", id],
    queryFn: async () => {
      const res = await api.get<{ domains: Domain[] }>("/api/v1/domains");
      return res.domains.find((d) => d.id === id) ?? null;
    },
  });

  if (domain.isLoading) return <PageLoader label="Loading domain" />;
  if (!domain.data) {
    return <EmptyState icon="globe" title="Domain not found" hint="It may have been removed." action={<Link href="/admin/domains" className="text-sm text-primary hover:underline">Back to domains</Link>} />;
  }
  const d = domain.data;

  return (
    <div className="space-y-6">
      <section>
        <Link href="/admin/domains" className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <Icon name="arrow-left" className="h-3.5 w-3.5" /> Domains
        </Link>
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <h1 className="page-title">{d.name}</h1>
          <Badge tone={d.status === "verified" ? "success" : "neutral"}>{d.status}</Badge>
          {d.project_name && <Badge tone="neutral">{d.project_name}</Badge>}
          <Link
            href={`/admin/domains/${id}/dns`}
            className="inline-flex h-8 items-center gap-1 rounded-[7px] border border-border-strong bg-surface-elevated px-3 text-[13px] font-medium text-foreground transition-colors hover:bg-muted"
          >
            <Icon name="globe" className="h-3.5 w-3.5" />
            DNS & DKIM
          </Link>
        </div>
      </section>

      <Tabs
        value={tab}
        onChange={setTab}
        tabs={[
          { value: "overview", label: "Overview" },
          { value: "dns", label: "DNS" },
          { value: "dkim", label: "DKIM" },
          { value: "deliverability", label: "Deliverability" },
          { value: "security", label: "Security" },
        ]}
      />

      {tab === "overview" && <OverviewTab d={d} />}
      {tab === "dns" && <DnsTab id={id} />}
      {tab === "dkim" && <DkimTab id={id} />}
      {tab === "deliverability" && <DomainDeliverabilityTab domain={d.name} />}
      {tab === "security" && <DomainSecurityTab domain={d.name} />}
    </div>
  );
}

function OverviewTab({ d }: { d: Domain }) {
  return (
    <section className="qazera-panel p-6">
      <dl className="grid grid-cols-[auto_1fr] gap-x-8 gap-y-3 text-sm">
        <dt className="text-muted-foreground">Status</dt>
        <dd><Badge tone={d.status === "verified" ? "success" : "neutral"}>{d.status}</Badge></dd>
        <dt className="text-muted-foreground">Verification mode</dt>
        <dd className="font-mono text-xs">{d.verification_mode}</dd>
        <dt className="text-muted-foreground">Project</dt>
        <dd>{d.project_name || "—"}</dd>
        <dt className="text-muted-foreground">Mail core</dt>
        <dd>{d.provisioning_status}</dd>
        <dt className="text-muted-foreground">Added</dt>
        <dd>{formatDate(d.created_at)}</dd>
      </dl>
    </section>
  );
}

function DnsRow({ c }: { c: DnsCheck }) {
  return (
    <tr className="align-top">
      <td className="px-4 py-3 font-medium uppercase">{c.type}</td>
      <td className="px-4 py-3">
        <Badge tone={DNS_TONE[c.status] ?? "neutral"}>{c.status.replace("_", " ")}</Badge>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-start gap-1.5">
          <code className="block max-w-md break-all font-mono text-[11px] leading-4 text-muted-foreground">
            {c.host}
          </code>
          {c.host && <CopyButton text={c.host} />}
        </div>
      </td>
      <td className="px-4 py-3">
        <div className="flex items-start gap-1.5">
          <code className="block max-w-md break-all font-mono text-[11px] leading-4 text-foreground">
            {c.expected || "—"}
          </code>
          {c.expected && <CopyButton text={c.expected} />}
        </div>
        {c.detail && <p className="mt-1 max-w-md text-[11px] leading-4 text-muted-foreground">{c.detail}</p>}
        {c.detected && c.detected.length > 0 && (
          <p className="mt-1 max-w-md break-all font-mono text-[11px] leading-4 text-faint">detected: {c.detected.join(", ")}</p>
        )}
      </td>
    </tr>
  );
}

function DnsTab({ id }: { id: string }) {
  const qc = useQueryClient();
  const toast = useToast();
  const dns = useQuery({
    queryKey: ["admin", "domain", id, "dns"],
    queryFn: () => api.get<DomainDns>(`/api/v1/domains/${id}/dns`),
  });
  const recheck = useMutation({
    mutationFn: () => api.post<DomainDns>(`/api/v1/domains/${id}/dns/recheck`),
    onSuccess: (data) => {
      qc.setQueryData(["admin", "domain", id, "dns"], data);
      qc.invalidateQueries({ queryKey: ["admin", "domains"] });
      toast("success", "DNS rechecked");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Recheck failed"),
  });

  if (dns.isLoading) return <PageLoader label="Loading DNS" />;
  const data = dns.data;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {data?.checked_at ? <>Last checked {formatDate(data.checked_at)}</> : "Not checked yet"}
        </p>
        <Button onClick={() => recheck.mutate()} disabled={recheck.isPending}>
          <Icon name="refresh-cw" className={recheck.isPending ? "h-3.5 w-3.5 animate-spin" : "h-3.5 w-3.5"} />
          {recheck.isPending ? "Checking…" : "Recheck DNS"}
        </Button>
      </div>

      <div className="overflow-x-auto rounded-[10px] border border-border bg-surface-elevated">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border text-left text-xs">
              <th className="px-4 py-3 font-medium">Record</th>
              <th className="px-4 py-3 font-medium">Status</th>
              <th className="px-4 py-3 font-medium">Host</th>
              <th className="px-4 py-3 font-medium">Expected value</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {data?.records.map((c) => <DnsRow key={c.type} c={c} />)}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function DkimTab({ id }: { id: string }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [rotateOpen, setRotateOpen] = useState(false);
  const keys = useQuery({
    queryKey: ["admin", "domain", id, "dkim"],
    queryFn: () => api.get<{ domain: string; keys: DkimKey[] }>(`/api/v1/domains/${id}/dkim`),
  });
  const rotate = useMutation({
    mutationFn: () => api.post(`/api/v1/domains/${id}/dkim/rotate`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "domain", id, "dkim"] });
      setRotateOpen(false);
      toast("success", "DKIM key issued");
    },
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Rotation failed"),
  });

  if (keys.isLoading) return <PageLoader label="Loading DKIM keys" />;
  const list = keys.data?.keys ?? [];
  const hasActive = list.some((k) => k.status === "active");

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          Outbound mail from this domain is signed with the active key. Publish its TXT record so
          receivers can verify the signature.
        </p>
        <Button onClick={() => setRotateOpen(true)} disabled={rotate.isPending}>
          <Icon name="rotate-cw" className="h-3.5 w-3.5" /> {hasActive ? "Rotate key" : "Generate key"}
        </Button>
      </div>

      {list.length === 0 && (
        <EmptyState icon="key" title="No DKIM key yet" hint="Generate a key to start signing outbound mail." />
      )}

      {list.map((k) => (
        <section key={k.id} className="qazera-panel p-5">
          <div className="flex items-center gap-2">
            <span className="font-mono text-sm font-semibold">{k.selector}</span>
            <Badge tone={k.status === "active" ? "success" : k.status === "previous" ? "warning" : "neutral"}>{k.status}</Badge>
            <span className="text-xs uppercase text-muted-foreground">{k.algorithm}</span>
            {k.retire_after && <span className="ml-auto text-xs text-muted-foreground">retire after {formatDate(k.retire_after)}</span>}
          </div>
          <div className="mt-4 space-y-2 text-[13px]">
            <div>
              <p className="mb-1 text-xs font-medium uppercase tracking-[.06em] text-faint">Type</p>
              <code className="font-mono text-xs">TXT</code>
            </div>
            <div>
              <p className="mb-1 text-xs font-medium uppercase tracking-[.06em] text-faint">Host</p>
              <div className="flex items-start gap-1.5">
                <code className="block break-all rounded-[7px] bg-muted px-2.5 py-1.5 font-mono text-[11px] leading-4">{k.dns_host}</code>
                <CopyButton text={k.dns_host} />
              </div>
            </div>
            <div>
              <p className="mb-1 text-xs font-medium uppercase tracking-[.06em] text-faint">Value</p>
              <div className="flex items-start gap-1.5">
                <code className="block break-all rounded-[7px] bg-muted px-2.5 py-1.5 font-mono text-[11px] leading-4">{k.dns_value}</code>
                <CopyButton text={k.dns_value} />
              </div>
            </div>
          </div>
        </section>
      ))}

      <ConfirmDialog
        open={rotateOpen}
        title={hasActive ? "Rotate the DKIM key?" : "Generate a DKIM key?"}
        body={hasActive
          ? "A new key becomes active immediately and the mail core signs with it. The current key moves to 'previous' and its record must stay published until its retire-after date so in-flight mail still verifies."
          : "The mail core generates a signing key and starts signing outbound mail. Publish the TXT record it returns."}
        confirmLabel={hasActive ? "Rotate key" : "Generate key"}
        onConfirm={() => rotate.mutate()}
        onCancel={() => setRotateOpen(false)}
      />
    </div>
  );
}

function DomainDeliverabilityTab({ domain }: { domain: string }) {
  const q = useQuery({
    queryKey: ["admin", "deliverability", "7d", domain],
    queryFn: () => api.get<import("@/lib/api").Deliverability>(`/api/v1/admin/deliverability?range=7d&domain=${encodeURIComponent(domain)}`),
  });
  if (q.isLoading) return <PageLoader label="Loading" />;
  const t = q.data?.totals;
  if (!t) return <EmptyState title="No data" />;
  return (
    <section className="grid grid-cols-2 gap-3 sm:grid-cols-4">
      {[
        ["Accepted", t.accepted],
        ["Delivered", t.delivered_local],
        ["Relayed", t.relayed],
        ["Failed", t.failed],
      ].map(([label, value]) => (
        <div key={label} className="qazera-panel px-4 py-3">
          <p className="text-[11px] font-medium uppercase tracking-[.06em] text-faint">{label}</p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{value}</p>
        </div>
      ))}
    </section>
  );
}

function DomainSecurityTab({ domain }: { domain: string }) {
  return (
    <EmptyState
      icon="shield"
      title="Domain security"
      hint={`Recent spam, spoofing and malware events for ${domain} appear here once the Security Center feeds are wired per-domain.`}
    />
  );
}
