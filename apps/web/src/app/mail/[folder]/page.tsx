"use client";

// Mail workspace: Sidebar → Message list → Reading pane. Selecting a message
// keeps you inside the workspace; the URL carries ?m=<id> for deep linking.

import { Suspense, use, useMemo, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, api, formatDate, type MessageList, type MessageListItem } from "@/lib/api";
import { Button, EmptyState, ErrorState, IconButton, ListSkeleton, Segmented, cx, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";
import { MessagePane } from "@/components/message-pane";
import { useCompose } from "@/components/compose";
import { useI18n } from "@/components/providers";

const VALID = new Set(["inbox", "sent", "drafts", "spam", "trash", "search"]);
const PAGE_SIZE = 50;

function MessageRow({
  m,
  folder,
  active,
  checked,
  onCheck,
  onOpen,
  onStar,
  onDelete,
  onToggleRead,
}: {
  m: MessageListItem;
  folder: string;
  active: boolean;
  checked: boolean;
  onCheck: (checked: boolean) => void;
  onOpen: () => void;
  onStar: () => void;
  onDelete: () => void;
  onToggleRead: () => void;
}) {
  return (
    <div
      className={cx(
        "group relative cursor-pointer select-none border-l-2 px-3 py-2 transition-colors duration-100",
        active
          ? "border-graphite bg-muted/70"
          : m.is_read
            ? "border-transparent hover:bg-muted/50"
            : "border-primary bg-surface-elevated hover:bg-muted/50",
      )}
      onClick={onOpen}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => { if (e.key === "Enter") onOpen(); }}
    >
      <div className="flex items-center gap-2">
        <span
          className={cx("shrink-0 transition-opacity", checked ? "opacity-100" : "opacity-0 group-hover:opacity-100")}
          onClick={(e) => e.stopPropagation()}
        >
          <input
            type="checkbox"
            className="block h-3.5 w-3.5 accent-graphite"
            checked={checked}
            onChange={(e) => onCheck(e.target.checked)}
            aria-label="Select message"
          />
        </span>
        <button
          type="button"
          className={cx(
            "shrink-0 transition-colors",
            m.is_starred ? "text-warning" : "text-border-strong hover:text-warning",
          )}
          title={m.is_starred ? "Unstar" : "Star"}
          onClick={(e) => { e.stopPropagation(); onStar(); }}
        >
          <Icon name="star" className="h-3.5 w-3.5" strokeWidth={m.is_starred ? 0 : 1.7} />
        </button>
        <span className={cx("min-w-0 flex-1 truncate text-[13px]", m.is_read ? "text-muted-foreground" : "font-semibold text-foreground")}>
          {m.from_display || m.from || "(unknown)"}
        </span>
        <span className={cx("shrink-0 font-mono text-[11px]", m.is_read ? "text-faint" : "font-medium text-primary")}>
          {formatDate(m.date)}
        </span>
      </div>
      <div className="mt-0.5 flex items-baseline gap-1.5 pl-[42px]">
        <span className={cx("min-w-0 shrink-0 truncate text-[13px] leading-5", m.is_read ? "text-foreground/80" : "font-medium text-foreground")}>
          {m.subject || "(no subject)"}
        </span>
        {m.snippet && <span className="min-w-0 flex-1 truncate text-xs leading-5 text-faint">— {m.snippet}</span>}
        {m.has_attachments && <Icon name="paperclip" className="h-3 w-3 shrink-0 self-center text-faint" />}
      </div>

      {/* Hover actions */}
      <div
        className="absolute right-2 top-1/2 hidden -translate-y-1/2 items-center gap-0.5 rounded-[7px] border border-border bg-surface-elevated p-0.5 shadow-sm group-hover:flex"
        onClick={(e) => e.stopPropagation()}
      >
        <IconButton size="sm" label={m.is_read ? "Mark unread" : "Mark read"} icon={m.is_read ? "mail" : "mail-open"} onClick={onToggleRead} />
        <IconButton size="sm" label={folder === "trash" ? "Delete forever" : "Delete"} icon="trash" onClick={onDelete} />
      </div>
    </div>
  );
}

function FolderWorkspace({ folder }: { folder: string }) {
  const searchParams = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const { t } = useI18n();
  const { openCompose } = useCompose();
  const [page, setPage] = useState(0);
  const [filter, setFilter] = useState<"all" | "unread" | "starred">("all");
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const q = folder === "search" ? (searchParams.get("q") ?? "") : "";
  const activeId = searchParams.get("m");
  const effectiveFolder = folder === "search" ? "inbox" : folder;

  // Reset paging/selection when the list context changes (render-time reset,
  // the React-recommended alternative to a setState effect).
  const listKey = `${folder}|${q}|${filter}`;
  const [prevListKey, setPrevListKey] = useState(listKey);
  if (listKey !== prevListKey) {
    setPrevListKey(listKey);
    setPage(0);
    setSelected(new Set());
  }

  const list = useQuery({
    queryKey: ["mail", "list", folder, q, page, filter],
    queryFn: () =>
      api.get<MessageList>(
        `/api/v1/mail/messages?folder=${effectiveFolder}&limit=${PAGE_SIZE}&offset=${page * PAGE_SIZE}` +
          (q ? `&q=${encodeURIComponent(q)}` : "") +
          (filter === "unread" ? "&unread=1" : filter === "starred" ? "&starred=1" : ""),
      ),
    enabled: VALID.has(folder),
  });

  const items = useMemo(() => list.data?.messages ?? [], [list.data]);
  const total = list.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function select(id: string | null) {
    const params = new URLSearchParams(searchParams.toString());
    if (id) params.set("m", id);
    else params.delete("m");
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }

  function openMessage(m: MessageListItem) {
    if (folder === "drafts") {
      openCompose({ draftId: m.id });
      return;
    }
    select(m.id);
  }

  async function rowAction(fn: () => Promise<unknown>, message?: string) {
    try {
      await fn();
      qc.invalidateQueries({ queryKey: ["mail"] });
      if (message) toast("success", message);
    } catch {
      toast("error", "Action failed");
    }
  }

  async function bulk(action: "read" | "unread" | "star" | "spam" | "trash" | "delete") {
    const ids = [...selected];
    if (ids.length === 0) return;
    try {
      await Promise.all(
        ids.map((id) => {
          if (action === "read") return api.patch(`/api/v1/mail/messages/${id}`, { is_read: true });
          if (action === "unread") return api.patch(`/api/v1/mail/messages/${id}`, { is_read: false });
          if (action === "star") return api.patch(`/api/v1/mail/messages/${id}`, { is_starred: true });
          if (action === "spam") return api.patch(`/api/v1/mail/messages/${id}`, { folder: "spam" });
          return api.delete(`/api/v1/mail/messages/${id}`);
        }),
      );
      if (activeId && ids.includes(activeId) && (action === "trash" || action === "delete" || action === "spam")) select(null);
      setSelected(new Set());
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", `${ids.length} message${ids.length > 1 ? "s" : ""} updated`);
    } catch {
      toast("error", "Some actions failed");
    }
  }

  if (!VALID.has(folder)) {
    return <EmptyState title="Unknown folder" hint="Use the sidebar to open one of your mail folders." />;
  }

  const title = folder === "search" ? `Search: “${q || "all"}”` : t(folder);
  const allChecked = items.length > 0 && items.every((m) => selected.has(m.id));

  return (
    <div className="flex h-full min-w-0">
      {/* List pane */}
      <section
        className={cx(
          "flex h-full w-full min-w-0 flex-col border-r border-border bg-surface-elevated lg:w-[400px] lg:shrink-0 xl:w-[440px]",
          activeId && "hidden lg:flex",
        )}
      >
        {/* Toolbar */}
        <div className="flex h-11 shrink-0 items-center gap-1.5 border-b border-border px-3">
          <input
            type="checkbox"
            className="h-3.5 w-3.5 accent-graphite"
            checked={allChecked}
            onChange={(e) => setSelected(e.target.checked ? new Set(items.map((m) => m.id)) : new Set())}
            aria-label="Select all"
          />
          <h1 className="ml-1 min-w-0 truncate text-[13px] font-semibold capitalize">{title}</h1>
          <span className="text-[11px] text-faint">{total.toLocaleString()}</span>
          <span className="flex-1" />
          {folder !== "drafts" && folder !== "search" && (
            <Segmented
              value={filter}
              onChange={(v) => setFilter(v as typeof filter)}
              options={[
                { value: "all", label: "All" },
                { value: "unread", label: "Unread" },
                { value: "starred", label: "★" },
              ]}
            />
          )}
          <IconButton size="sm" label="Refresh" icon="refresh-cw" onClick={() => { qc.invalidateQueries({ queryKey: ["mail"] }); }} />
        </div>

        {/* Bulk toolbar */}
        {selected.size > 0 && (
          <div className="anim-rise flex shrink-0 flex-wrap items-center gap-1 border-b border-border bg-background px-3 py-1.5">
            <span className="mr-1 text-xs font-medium text-muted-foreground">{selected.size} selected</span>
            <Button size="sm" variant="ghost" onClick={() => bulk("read")}>Read</Button>
            <Button size="sm" variant="ghost" onClick={() => bulk("unread")}>Unread</Button>
            <Button size="sm" variant="ghost" onClick={() => bulk("star")}>Star</Button>
            {folder !== "spam" && folder !== "trash" && folder !== "drafts" && (
              <Button size="sm" variant="ghost" onClick={() => bulk("spam")}>Spam</Button>
            )}
            <Button size="sm" variant="ghost" className="text-danger hover:text-danger" onClick={() => bulk(folder === "trash" ? "delete" : "trash")}>
              {folder === "trash" ? "Delete forever" : "Delete"}
            </Button>
          </div>
        )}

        {/* Rows */}
        <div className="min-h-0 flex-1 overflow-y-auto">
          {list.isLoading && <ListSkeleton rows={10} />}
          {list.isError && (
            <ErrorState
              message={
                list.error instanceof ApiError && list.error.code === "MAIL_SERVICE_UNAVAILABLE"
                  ? "Mail service is temporarily unavailable. Your messages are safe; try again in a moment."
                  : "Could not load messages"
              }
              onRetry={() => list.refetch()}
            />
          )}
          {list.isSuccess && items.length === 0 && (
            <EmptyState
              icon={folder === "search" ? "search" : folder === "drafts" ? "file-text" : "inbox"}
              title={
                folder === "search" ? "No results"
                  : filter === "unread" ? "No unread messages"
                  : filter === "starred" ? "No starred messages"
                  : "Nothing here"
              }
              hint={
                folder === "inbox" && filter === "all" ? "Messages sent to your address will appear here."
                  : folder === "search" ? "Try different keywords, or search by sender address."
                  : folder === "drafts" ? "Messages you start writing are saved here automatically."
                  : undefined
              }
              action={folder === "inbox" && filter === "all" ? (
                <Button size="sm" variant="secondary" onClick={() => openCompose()}>
                  <Icon name="pen" className="h-3.5 w-3.5" /> Write a message
                </Button>
              ) : undefined}
            />
          )}
          <div className="divide-y divide-border/70">
            {items.map((m) => (
              <MessageRow
                key={m.id}
                m={m}
                folder={folder}
                active={m.id === activeId}
                checked={selected.has(m.id)}
                onCheck={(checked) => {
                  const next = new Set(selected);
                  if (checked) next.add(m.id); else next.delete(m.id);
                  setSelected(next);
                }}
                onOpen={() => openMessage(m)}
                onStar={() => rowAction(() => api.patch(`/api/v1/mail/messages/${m.id}`, { is_starred: !m.is_starred }))}
                onToggleRead={() => rowAction(() => api.patch(`/api/v1/mail/messages/${m.id}`, { is_read: !m.is_read }))}
                onDelete={() => {
                  if (m.id === activeId) select(null);
                  void rowAction(() => api.delete(`/api/v1/mail/messages/${m.id}`), folder === "trash" ? "Deleted" : "Moved to Trash");
                }}
              />
            ))}
          </div>
        </div>

        {/* Pagination */}
        {pages > 1 && (
          <div className="flex h-10 shrink-0 items-center justify-between border-t border-border px-3">
            <span className="text-[11px] text-muted-foreground">
              {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, total)} of {total.toLocaleString()}
            </span>
            <div className="flex items-center gap-0.5">
              <IconButton size="sm" label="Previous page" icon="chevron-left" disabled={page === 0} onClick={() => setPage((p) => p - 1)} />
              <IconButton size="sm" label="Next page" icon="chevron-right" disabled={page >= pages - 1} onClick={() => setPage((p) => p + 1)} />
            </div>
          </div>
        )}
      </section>

      {/* Reading pane */}
      <section className={cx("h-full min-w-0 flex-1", !activeId && "hidden lg:block")}>
        {activeId ? (
          <MessagePane id={activeId} onClose={() => select(null)} onOpenMessage={(id) => select(id)} />
        ) : (
          <div className="flex h-full items-center justify-center bg-background">
            <EmptyState
              icon="mail-open"
              title="Select a message to read"
              hint="Choose a message from the list. It opens here, so you never lose your place."
            />
          </div>
        )}
      </section>
    </div>
  );
}

export default function FolderPage({ params }: { params: Promise<{ folder: string }> }) {
  const { folder } = use(params);
  return (
    <Suspense fallback={<ListSkeleton rows={10} />}>
      <FolderWorkspace folder={folder} />
    </Suspense>
  );
}
