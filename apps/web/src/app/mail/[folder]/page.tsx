"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, formatDate, type MessageList } from "@/lib/api";
import { Button, EmptyState, ErrorState, PageLoader, cx, useToast } from "@/components/ui";

const VALID = new Set(["inbox", "sent", "drafts", "spam", "trash", "search"]);
const PAGE_SIZE = 50;

export default function FolderPage({ params }: { params: Promise<{ folder: string }> }) {
  const { folder } = use(params);
  const searchParams = useSearchParams();
  const qc = useQueryClient();
  const toast = useToast();
  const [page, setPage] = useState(0);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const q = folder === "search" ? (searchParams.get("q") ?? "") : "";
  const effectiveFolder = folder === "search" ? "inbox" : folder;

  const list = useQuery({
    queryKey: ["mail", "list", folder, q, page],
    queryFn: () =>
      api.get<MessageList>(
        `/api/v1/mail/messages?folder=${effectiveFolder}&limit=${PAGE_SIZE}&offset=${page * PAGE_SIZE}` +
          (q ? `&q=${encodeURIComponent(q)}` : ""),
      ),
    enabled: VALID.has(folder),
  });

  if (!VALID.has(folder)) {
    return <EmptyState title="Unknown folder" />;
  }

  async function bulk(action: "read" | "unread" | "star" | "trash" | "delete") {
    const ids = [...selected];
    if (ids.length === 0) return;
    try {
      await Promise.all(
        ids.map((id) => {
          if (action === "read") return api.patch(`/api/v1/mail/messages/${id}`, { is_read: true });
          if (action === "unread") return api.patch(`/api/v1/mail/messages/${id}`, { is_read: false });
          if (action === "star") return api.patch(`/api/v1/mail/messages/${id}`, { is_starred: true });
          return api.delete(`/api/v1/mail/messages/${id}`);
        }),
      );
      setSelected(new Set());
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", `${ids.length} message${ids.length > 1 ? "s" : ""} updated`);
    } catch {
      toast("error", "Some actions failed");
    }
  }

  const items = list.data?.messages ?? [];
  const total = list.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-neutral-200 bg-white px-4 py-2 dark:border-neutral-800 dark:bg-neutral-900">
        <h1 className="text-sm font-semibold capitalize">
          {folder === "search" ? `Search: “${q}”` : folder}
        </h1>
        <span className="text-xs text-neutral-400">{total} messages</span>
        {selected.size > 0 && (
          <div className="ml-auto flex items-center gap-1">
            <span className="mr-1 text-xs text-neutral-500">{selected.size} selected</span>
            <Button variant="ghost" onClick={() => bulk("read")}>Read</Button>
            <Button variant="ghost" onClick={() => bulk("unread")}>Unread</Button>
            <Button variant="ghost" onClick={() => bulk("star")}>Star</Button>
            <Button variant="ghost" onClick={() => bulk(folder === "trash" ? "delete" : "trash")}>
              {folder === "trash" ? "Delete forever" : "Delete"}
            </Button>
          </div>
        )}
        {selected.size === 0 && pages > 1 && (
          <div className="ml-auto flex items-center gap-2 text-xs text-neutral-500">
            <Button variant="ghost" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
              ←
            </Button>
            {page + 1} / {pages}
            <Button variant="ghost" disabled={page >= pages - 1} onClick={() => setPage((p) => p + 1)}>
              →
            </Button>
          </div>
        )}
      </div>

      {list.isLoading && <PageLoader />}
      {list.isError && <ErrorState message="Could not load messages" onRetry={() => list.refetch()} />}
      {list.isSuccess && items.length === 0 && (
        <EmptyState
          title={folder === "search" ? "No results" : "Nothing here"}
          hint={folder === "inbox" ? "Messages sent to you will appear here." : undefined}
        />
      )}

      <ul className="divide-y divide-neutral-100 dark:divide-neutral-800">
        {items.map((m) => (
          <li key={m.id} className="group flex items-center gap-3 bg-white px-4 py-2.5 transition-colors hover:bg-neutral-50 dark:bg-neutral-900 dark:hover:bg-neutral-800">
            <input
              type="checkbox"
              className="h-4 w-4 accent-indigo-600"
              checked={selected.has(m.id)}
              onChange={(e) => {
                const next = new Set(selected);
                if (e.target.checked) next.add(m.id);
                else next.delete(m.id);
                setSelected(next);
              }}
              aria-label="Select message"
            />
            <button
              className={cx("text-base", m.is_starred ? "text-amber-400" : "text-neutral-300 hover:text-amber-400")}
              title={m.is_starred ? "Unstar" : "Star"}
              onClick={async () => {
                await api.patch(`/api/v1/mail/messages/${m.id}`, { is_starred: !m.is_starred });
                qc.invalidateQueries({ queryKey: ["mail", "list"] });
              }}
            >
              ★
            </button>
            <Link
              href={folder === "drafts" ? `/mail/compose?draft=${m.id}` : `/mail/message/${m.id}`}
              className="flex min-w-0 flex-1 items-baseline gap-3"
              onClick={() => {
                if (!m.is_read) {
                  qc.setQueryData(["mail", "list", folder, q, page], (old: MessageList | undefined) =>
                    old
                      ? { ...old, messages: old.messages.map((x) => (x.id === m.id ? { ...x, is_read: true } : x)) }
                      : old,
                  );
                }
              }}
            >
              <span
                className={cx(
                  "w-44 shrink-0 truncate text-sm",
                  m.is_read ? "text-neutral-500" : "font-semibold",
                )}
              >
                {m.from_display || m.from}
              </span>
              <span className={cx("truncate text-sm", m.is_read ? "text-neutral-500" : "font-medium")}>
                {m.subject || "(no subject)"}
              </span>
              <span className="hidden truncate text-xs text-neutral-400 sm:inline">— {m.snippet}</span>
              <span className="ml-auto shrink-0 text-xs text-neutral-400">
                {m.has_attachments && <span className="mr-1">📎</span>}
                {formatDate(m.date)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
