"use client";

// Contacts workspace: searchable directory grouped by department, with a
// profile drawer showing recent correspondence and one-click compose.

import { useDeferredValue, useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { api, formatDate, type Department, type DirectoryUser, type MessageList } from "@/lib/api";
import { Avatar, Badge, Button, Drawer, EmptyState, ErrorState, ListSkeleton, cx } from "@/components/ui";
import { Icon } from "@/components/icons";
import { useI18n, useMe } from "@/components/providers";
import { useCompose } from "@/components/compose";

function ContactDrawer({ person, onClose }: { person: DirectoryUser; onClose: () => void }) {
  const { openCompose } = useCompose();
  const me = useMe();
  const canMessage = me.data?.permissions.includes("messages.send");

  // Recent correspondence with this person, straight from mail search.
  const recent = useQuery({
    queryKey: ["mail", "contact-history", person.mailbox_address],
    queryFn: () => api.get<MessageList>(`/api/v1/mail/messages?folder=inbox&q=${encodeURIComponent(person.mailbox_address)}&limit=6`),
  });

  return (
    <Drawer
      open
      onClose={onClose}
      width="max-w-md"
      title={
        <span className="flex items-center gap-2.5">
          <Avatar name={person.display_name || person.email} size="md" />
          <span className="min-w-0">
            <span className="block truncate leading-4">{person.display_name || person.email}</span>
            <span className="block truncate text-[11px] font-normal text-muted-foreground">{person.mailbox_address}</span>
          </span>
        </span>
      }
    >
      <div className="space-y-5 p-4">
        <div className="flex gap-2">
          <Button size="sm" onClick={() => { onClose(); openCompose({ to: person.mailbox_address }); }}>
            <Icon name="pen" className="h-3.5 w-3.5" /> Write email
          </Button>
          {canMessage && (
            <Link href="/mail/messages">
              <Button size="sm" variant="secondary"><Icon name="message-circle" className="h-3.5 w-3.5" /> Message</Button>
            </Link>
          )}
          <Link href={`/mail/search?q=${encodeURIComponent(person.mailbox_address)}`} className="ml-auto">
            <Button size="sm" variant="ghost">All mail</Button>
          </Link>
        </div>

        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 rounded-[8px] border border-border bg-background px-3.5 py-3 text-[13px]">
          <dt className="text-muted-foreground">Email</dt><dd className="break-all font-medium">{person.email}</dd>
          <dt className="text-muted-foreground">Mailbox</dt><dd className="break-all">{person.mailbox_address}</dd>
          <dt className="text-muted-foreground">Department</dt>
          <dd>{person.department_name ? <Badge tone="neutral">{person.department_name}</Badge> : "—"}</dd>
          {person.is_online !== undefined && (
            <>
              <dt className="text-muted-foreground">Presence</dt>
              <dd className="flex items-center gap-1.5">
                <span className={cx("h-1.5 w-1.5 rounded-full", person.is_online ? "bg-success" : "bg-border-strong")} />
                {person.is_online ? "Online" : "Offline"}
              </dd>
            </>
          )}
        </dl>

        <section>
          <h3 className="mb-2 text-xs font-medium uppercase tracking-[.05em] text-muted-foreground">Recent messages from them</h3>
          {recent.isLoading && <ListSkeleton rows={3} />}
          {recent.isSuccess && recent.data.messages.length === 0 && (
            <p className="rounded-[8px] border border-dashed border-border-strong p-4 text-center text-xs text-muted-foreground">
              No messages from this contact in your inbox yet.
            </p>
          )}
          <ul className="space-y-1">
            {(recent.data?.messages ?? []).map((m) => (
              <li key={m.id}>
                <Link
                  href={`/mail/inbox?m=${m.id}`}
                  className="flex items-baseline gap-2 rounded-[7px] border border-transparent px-2.5 py-1.5 text-[13px] transition-colors hover:border-border hover:bg-background"
                >
                  <span className="min-w-0 flex-1 truncate font-medium">{m.subject || "(no subject)"}</span>
                  <span className="shrink-0 font-mono text-[11px] text-faint">{formatDate(m.date)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </Drawer>
  );
}

export default function ContactsPage() {
  const { t } = useI18n();
  const { openCompose } = useCompose();
  const [departmentId, setDepartmentId] = useState("");
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<DirectoryUser | null>(null);
  const deferredSearch = useDeferredValue(search);

  const departments = useQuery({ queryKey: ["departments"], queryFn: () => api.get<{ departments: Department[] }>("/api/v1/departments") });
  const contacts = useQuery({
    queryKey: ["directory", "contacts", departmentId, deferredSearch],
    queryFn: () => {
      const params = new URLSearchParams();
      params.set("limit", "500");
      if (departmentId) params.set("department_id", departmentId);
      if (deferredSearch.trim()) params.set("q", deferredSearch.trim());
      return api.get<{ users: DirectoryUser[] }>(`/api/v1/directory/users?${params}`);
    },
  });

  // Group alphabetically by first letter of display name.
  const groups = useMemo(() => {
    const users = [...(contacts.data?.users ?? [])].sort((a, b) =>
      (a.display_name || a.email).localeCompare(b.display_name || b.email),
    );
    const map = new Map<string, DirectoryUser[]>();
    for (const u of users) {
      const letter = (u.display_name || u.email).charAt(0).toUpperCase();
      if (!map.has(letter)) map.set(letter, []);
      map.get(letter)!.push(u);
    }
    return [...map.entries()];
  }, [contacts.data]);

  const totalShown = contacts.data?.users.length ?? 0;

  return (
    <div className="h-full w-full overflow-y-auto">
      <div className="mx-auto max-w-3xl p-4 lg:p-6">
        <header className="page-header">
          <div>
            <h1 className="page-title">{t("contacts")}</h1>
            <p className="page-description">{t("contactsHint")}</p>
          </div>
        </header>

        {/* Toolbar */}
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <div className="relative min-w-52 flex-1 sm:max-w-xs">
            <Icon name="search" className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-faint" />
            <input
              type="search"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={t("searchRecipients")}
              className="h-8 w-full rounded-[7px] border border-border-strong bg-surface-elevated pl-8 pr-2.5 text-[13px] outline-none placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary/15"
            />
          </div>
          <select value={departmentId} onChange={(event) => setDepartmentId(event.target.value)} aria-label={t("chooseDepartment")}>
            <option value="">{t("allDepartments")}</option>
            {(departments.data?.departments ?? []).map((d) => (
              <option key={d.id} value={d.id}>{d.name} · {d.employee_count}</option>
            ))}
          </select>
          <span className="ml-auto text-xs text-muted-foreground">{contacts.isSuccess && `${totalShown} people`}</span>
        </div>

        {contacts.isLoading && <div className="rounded-[10px] border border-border bg-surface-elevated"><ListSkeleton rows={8} /></div>}
        {contacts.isError && <ErrorState message="Could not load contacts" onRetry={() => contacts.refetch()} />}
        {contacts.isSuccess && totalShown === 0 && (
          <div className="rounded-[10px] border border-border bg-surface-elevated">
            <EmptyState icon="users" title={t("noPeopleFound")} hint="Try a different name, email or department." />
          </div>
        )}

        <div className="space-y-4">
          {groups.map(([letter, users]) => (
            <section key={letter} className="overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
              <header className="border-b border-border bg-background/60 px-4 py-1.5">
                <h2 className="text-xs font-semibold text-muted-foreground">{letter}</h2>
              </header>
              <ul className="divide-y divide-border/70">
                {users.map((person) => (
                  <li key={person.id}>
                    <div
                      className="group flex w-full cursor-pointer items-center gap-3 px-4 py-2 text-left transition-colors hover:bg-background"
                      role="button"
                      tabIndex={0}
                      onClick={() => setSelected(person)}
                      onKeyDown={(e) => { if (e.key === "Enter") setSelected(person); }}
                    >
                      <Avatar name={person.display_name || person.email} size="lg" />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-[13px] font-medium leading-4">{person.display_name || person.email}</span>
                        <span className="block truncate text-xs text-muted-foreground">{person.mailbox_address}</span>
                      </span>
                      {person.department_name && <Badge tone="neutral" className="hidden sm:inline-flex">{person.department_name}</Badge>}
                      <Button
                        size="sm"
                        variant="secondary"
                        className="opacity-0 transition-opacity group-hover:opacity-100"
                        onClick={(e) => { e.stopPropagation(); openCompose({ to: person.mailbox_address }); }}
                      >
                        <Icon name="pen" className="h-3 w-3" /> {t("write")}
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      </div>

      {selected && <ContactDrawer person={selected} onClose={() => setSelected(null)} />}
    </div>
  );
}
