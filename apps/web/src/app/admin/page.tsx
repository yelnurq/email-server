"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { api, type AdminUser, type Domain, type Mailbox, type Organization } from "@/lib/api";
import { PageLoader, cx } from "@/components/ui";

export default function AdminDashboard() {
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
  const mailboxes = useQuery({
    queryKey: ["admin", "mailboxes"],
    queryFn: () => api.get<{ mailboxes: Mailbox[] }>("/api/v1/mailboxes"),
  });

  if (orgs.isLoading || domains.isLoading || users.isLoading || mailboxes.isLoading) {
    return <PageLoader label="Loading admin" />;
  }

  const cards = [
    { label: "Organizations", value: orgs.data?.organizations.length ?? 0, href: "/admin/organizations" },
    { label: "Domains", value: domains.data?.domains.length ?? 0, href: "/admin/domains" },
    { label: "Users", value: users.data?.users.length ?? 0, href: "/admin/users" },
    { label: "Mailboxes", value: mailboxes.data?.mailboxes.length ?? 0, href: "/admin/mailboxes" },
  ];

  const suite = [
    { href: "/admin/security", label: "Security", note: "Quarantine and blocks" },
    { href: "/admin/api-keys", label: "API Keys", note: "Programmatic access" },
    { href: "/admin/webhooks", label: "Webhooks", note: "Delivery events" },
    { href: "/admin/audit", label: "Audit", note: "Traceability layer" },
  ];

  return (
    <div className="mx-auto max-w-screen-xl space-y-6">
      <section className="newsprint-texture border border-[#111111] bg-[#f9f9f7] p-6 lg:p-8">
        <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Control Desk</p>
        <div className="mt-4 grid gap-6 lg:grid-cols-12">
          <div className="lg:col-span-8">
            <h1 className="font-serif text-[clamp(3rem,8vw,7rem)] leading-[0.9] tracking-tighter text-[#111111]">
              QazEra Control
            </h1>
            <p className="mt-5 max-w-3xl text-lg leading-8 text-[#111111] font-body text-justify">
              Administrative operations are laid out with the clarity of a front page: metrics,
              sections, and system responsibilities separated by visible editorial borders.
            </p>
          </div>
          <div className="border border-[#111111] bg-[#e5e5e0] p-4 lg:col-span-4">
            <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
              01. System
            </p>
            <ol className="mt-4 space-y-2 text-sm leading-6 font-body">
              <li>01. Create an organization.</li>
              <li>02. Add and verify a domain.</li>
              <li>03. Create users and mailboxes.</li>
              <li>04. Wire API keys, SMTP, and webhooks.</li>
            </ol>
          </div>
        </div>
      </section>

      <section className="grid gap-0 border border-[#111111] md:grid-cols-2 xl:grid-cols-4">
        {cards.map((c, idx) => (
          <Link
            key={c.label}
            href={c.href}
            className={cx(
              "border-b border-[#111111] p-5 transition-colors hover:bg-[#111111] hover:text-[#f9f9f7] md:border-b-0",
              idx < 3 && "md:border-r",
            )}
          >
            <p className="font-serif text-4xl tracking-tight">{String(c.value).padStart(2, "0")}</p>
            <p className="mt-4 font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000] hover:text-[#f9f9f7]">
              {c.label}
            </p>
          </Link>
        ))}
      </section>

      <section className="grid gap-0 border border-[#111111] lg:grid-cols-2">
        {suite.map((item, index) => (
          <Link
            key={item.href}
            href={item.href}
            className={cx(
              "border-b border-[#111111] bg-[#f9f9f7] p-5 transition-colors hover:bg-[#cc0000] hover:text-white lg:border-b-0",
              index % 2 === 0 && "lg:border-r",
            )}
          >
            <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000] hover:text-white">
              Section
            </p>
            <h2 className="mt-3 font-serif text-3xl tracking-tight">{item.label}</h2>
            <p className="mt-3 text-sm leading-6 font-body">{item.note}</p>
          </Link>
        ))}
      </section>
    </div>
  );
}
