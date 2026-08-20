"use client";

// Command palette (Ctrl/Cmd+K): navigation, actions and people search in one
// keyboard-first surface. People results come from the live directory.

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { api, type DirectoryUser } from "@/lib/api";
import { useMe } from "@/components/providers";
import { Avatar, Kbd, cx } from "@/components/ui";
import { Icon } from "@/components/icons";
import { useCompose } from "@/components/compose";

type Command = {
  id: string;
  label: string;
  hint?: string;
  icon: string;
  keywords?: string;
  section: string;
  run: () => void;
};

export function CommandPalette() {
  const router = useRouter();
  const me = useMe();
  const { openCompose } = useCompose();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        e.stopPropagation();
        setOpen((v) => !v);
        setQuery("");
        setIndex(0);
      }
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", handler, true);
    return () => document.removeEventListener("keydown", handler, true);
  }, []);

  useEffect(() => {
    if (open) setTimeout(() => inputRef.current?.focus(), 10);
  }, [open]);

  const can = (p: string) => me.data?.permissions.includes(p) ?? false;
  const isAdmin = can("users.manage");

  const people = useQuery({
    queryKey: ["directory", "palette", query],
    queryFn: () => api.get<{ users: DirectoryUser[] }>(`/api/v1/directory/users?limit=6&q=${encodeURIComponent(query.trim())}`),
    enabled: open && can("departments.read") && query.trim().length >= 2,
  });

  const go = (href: string) => () => { setOpen(false); router.push(href); };

  const commands = useMemo<Command[]>(() => {
    const base: Command[] = [
      { id: "compose", label: "New message", hint: "C", icon: "pen", keywords: "compose write email new", section: "Actions", run: () => { setOpen(false); openCompose(); } },
      ...(query.trim()
        ? [{ id: "search-mail", label: `Search mail for “${query.trim()}”`, icon: "search", section: "Actions", run: go(`/mail/search?q=${encodeURIComponent(query.trim())}`) }]
        : []),
      { id: "inbox", label: "Go to Inbox", icon: "inbox", keywords: "mail folder", section: "Mail", run: go("/mail/inbox") },
      { id: "sent", label: "Go to Sent", icon: "send", keywords: "mail folder", section: "Mail", run: go("/mail/sent") },
      { id: "drafts", label: "Go to Drafts", icon: "file-text", keywords: "mail folder", section: "Mail", run: go("/mail/drafts") },
      { id: "spam", label: "Go to Spam", icon: "alert-octagon", keywords: "mail folder junk", section: "Mail", run: go("/mail/spam") },
      { id: "trash", label: "Go to Trash", icon: "trash", keywords: "mail folder deleted", section: "Mail", run: go("/mail/trash") },
      ...(can("messages.send") ? [{ id: "messages", label: "Open Messages", icon: "message-circle", keywords: "chat conversation", section: "Workspace", run: go("/mail/messages") }] : []),
      ...(can("tasks.manage.self") ? [{ id: "tasks", label: "Open Tasks & Reminders", icon: "check-circle", keywords: "reminder todo", section: "Workspace", run: go("/mail/my-list") }] : []),
      ...(can("departments.read") ? [{ id: "contacts", label: "Open Contacts", icon: "users", keywords: "people directory", section: "Workspace", run: go("/mail/contacts") }] : []),
      ...(can("official.read") ? [{ id: "official", label: "Open Official messages", icon: "megaphone", keywords: "announcements", section: "Workspace", run: go("/mail/official") }] : []),
      { id: "settings", label: "Open Settings", icon: "settings", keywords: "account profile password", section: "Workspace", run: go("/mail/settings") },
      ...(isAdmin
        ? [
            { id: "admin", label: "Admin · Overview", icon: "layout-grid", keywords: "console dashboard", section: "Admin", run: go("/admin") },
            { id: "admin-users", label: "Admin · Users", icon: "users", keywords: "team accounts", section: "Admin", run: go("/admin/users") },
            ...(can("audit.read") ? [{ id: "admin-audit", label: "Admin · Logs & Audit", icon: "scroll-text", keywords: "events history", section: "Admin", run: go("/admin/audit") }] : []),
            ...(can("bulk_email.view_analytics") ? [{ id: "admin-broadcast", label: "Admin · Broadcast", icon: "megaphone", keywords: "bulk mass email", section: "Admin", run: go("/admin/bulk-email") }] : []),
            ...(can("security.manage") ? [{ id: "admin-security", label: "Admin · Security Center", icon: "shield", keywords: "quarantine blocks", section: "Admin", run: go("/admin/security") }] : []),
            ...(can("departments.manage") ? [{ id: "admin-departments", label: "Admin · Departments", icon: "building", section: "Admin", run: go("/admin/departments") }] : []),
          ]
        : []),
    ];
    const q = query.trim().toLowerCase();
    if (!q) return base;
    return base.filter((c) => `${c.label} ${c.keywords ?? ""}`.toLowerCase().includes(q) || c.id === "search-mail");
  }, [query, isAdmin, me.data, openCompose]); // eslint-disable-line react-hooks/exhaustive-deps

  const peopleItems: Command[] = (query.trim().length >= 2 ? people.data?.users ?? [] : []).map((u) => ({
    id: `person-${u.id}`,
    label: u.display_name || u.email,
    hint: u.mailbox_address,
    icon: "user",
    section: "People",
    run: () => { setOpen(false); openCompose({ to: u.mailbox_address }); },
  }));

  const all = [...commands, ...peopleItems];
  const active = Math.min(index, Math.max(0, all.length - 1));

  // Reset the highlighted row whenever the query changes (render-time reset).
  const [prevQuery, setPrevQuery] = useState(query);
  if (query !== prevQuery) {
    setPrevQuery(query);
    setIndex(0);
  }
  useEffect(() => {
    listRef.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: "nearest" });
  }, [active]);

  if (!open) return null;

  let lastSection = "";

  return (
    <div className="anim-fade fixed inset-0 z-[65] bg-graphite/40 p-4 pt-[12vh]" onClick={() => setOpen(false)} role="dialog" aria-modal="true" aria-label="Command palette">
      <div
        className="anim-rise mx-auto w-full max-w-lg overflow-hidden rounded-xl border border-border bg-surface-elevated shadow-[var(--shadow-window)]"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-2.5 border-b border-border px-3.5">
          <Icon name="search" className="h-4 w-4 shrink-0 text-faint" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") { e.preventDefault(); setIndex((i) => Math.min(i + 1, all.length - 1)); }
              if (e.key === "ArrowUp") { e.preventDefault(); setIndex((i) => Math.max(i - 1, 0)); }
              if (e.key === "Enter" && all[active]) { e.preventDefault(); all[active].run(); }
            }}
            placeholder="Search actions, pages, people…"
            className="h-11 w-full bg-transparent text-sm outline-none placeholder:text-faint"
          />
          <Kbd>Esc</Kbd>
        </div>
        <div ref={listRef} className="max-h-[50vh] overflow-y-auto p-1.5">
          {all.length === 0 && <p className="p-6 text-center text-[13px] text-muted-foreground">Nothing found.</p>}
          {all.map((c, i) => {
            const showSection = c.section !== lastSection;
            lastSection = c.section;
            return (
              <div key={c.id}>
                {showSection && <p className="px-2.5 pb-1 pt-2 text-[10.5px] font-medium uppercase tracking-[.06em] text-faint">{c.section}</p>}
                <button
                  type="button"
                  data-active={i === active}
                  className={cx(
                    "flex w-full items-center gap-2.5 rounded-[7px] px-2.5 py-2 text-left text-[13px]",
                    i === active ? "bg-graphite text-graphite-foreground" : "text-foreground hover:bg-muted",
                  )}
                  onMouseEnter={() => setIndex(i)}
                  onClick={() => c.run()}
                >
                  {c.id.startsWith("person-") ? (
                    <Avatar name={c.label} size="sm" />
                  ) : (
                    <Icon name={c.icon} className={cx("h-4 w-4 shrink-0", i === active ? "opacity-90" : "opacity-60")} />
                  )}
                  <span className="min-w-0 flex-1 truncate">{c.label}</span>
                  {c.hint && <span className={cx("shrink-0 text-[11px]", i === active ? "text-graphite-foreground/60" : "text-faint")}>{c.hint}</span>}
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
