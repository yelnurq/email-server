"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { api, type AdminUser, type Domain, type Mailbox, type Organization } from "@/lib/api";
import { PageLoader } from "@/components/ui";

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
    return <PageLoader />;
  }

  const cards = [
    { label: "Organizations", value: orgs.data?.organizations.length ?? 0, href: "/admin/organizations" },
    { label: "Domains", value: domains.data?.domains.length ?? 0, href: "/admin/domains" },
    { label: "Users", value: users.data?.users.length ?? 0, href: "/admin/users" },
    { label: "Mailboxes", value: mailboxes.data?.mailboxes.length ?? 0, href: "/admin/mailboxes" },
  ];

  return (
    <div className="mx-auto max-w-5xl">
      <h1 className="mb-4 text-lg font-semibold">Dashboard</h1>
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        {cards.map((c) => (
          <Link
            key={c.label}
            href={c.href}
            className="rounded-2xl border border-neutral-200 bg-white p-5 shadow-sm transition-colors hover:border-indigo-300 dark:border-neutral-800 dark:bg-neutral-900"
          >
            <p className="text-3xl font-semibold">{c.value}</p>
            <p className="mt-1 text-sm text-neutral-500">{c.label}</p>
          </Link>
        ))}
      </div>
      <div className="mt-6 rounded-2xl border border-neutral-200 bg-white p-5 text-sm text-neutral-600 shadow-sm dark:border-neutral-800 dark:bg-neutral-900 dark:text-neutral-300">
        <h2 className="mb-2 font-semibold text-neutral-900 dark:text-neutral-100">Onboarding flow</h2>
        <ol className="list-inside list-decimal space-y-1">
          <li>Create an <Link className="text-indigo-600 hover:underline" href="/admin/organizations">organization</Link></li>
          <li>Add a development <Link className="text-indigo-600 hover:underline" href="/admin/domains">domain</Link> (e.g. company.test)</li>
          <li>Create <Link className="text-indigo-600 hover:underline" href="/admin/users">users</Link> with mailboxes</li>
          <li>Users sign in and exchange mail locally</li>
        </ol>
      </div>
    </div>
  );
}
