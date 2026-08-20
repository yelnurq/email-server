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
  const title = folder === "search" ? `Search / ${q || "ALL"}` : folder.toUpperCase();

  return (
    <div className="min-h-[calc(100vh-89px)] bg-[#f9f9f7]">
      <section className="border-b border-[#111111] bg-[#e5e5e0] px-4 py-5 lg:px-6">
        <div className="flex flex-wrap items-end gap-4">
          <div>
            <p className="font-mono text-[10px] uppercase tracking-[0.3em] text-[#cc0000]">
              Mailbox
            </p>
            <h1 className="mt-3 font-serif text-4xl leading-none tracking-tight text-[#111111] lg:text-5xl">
              {title}
            </h1>
          </div>
          <p className="ml-auto font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
            {total} messages
          </p>

          {selected.size > 0 && (
            <div className="flex flex-wrap gap-2">
              <span className="border border-[#111111] bg-[#f9f9f7] px-3 py-2 font-mono text-xs uppercase tracking-[0.25em]">
                {selected.size} selected
              </span>
              <Button variant="secondary" onClick={() => bulk("read")}>Read</Button>
              <Button variant="secondary" onClick={() => bulk("unread")}>Unread</Button>
              <Button variant="secondary" onClick={() => bulk("star")}>Star</Button>
              <Button variant="secondary" onClick={() => bulk(folder === "trash" ? "delete" : "trash")}>
                {folder === "trash" ? "Delete forever" : "Delete"}
              </Button>
            </div>
          )}

          {selected.size === 0 && pages > 1 && (
            <div className="ml-auto flex items-center gap-2">
              <Button variant="secondary" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
                Prev
              </Button>
              <span className="font-mono text-xs uppercase tracking-[0.3em] text-[#525252]">
                {page + 1} / {pages}
              </span>
              <Button variant="secondary" disabled={page >= pages - 1} onClick={() => setPage((p) => p + 1)}>
                Next
              </Button>
            </div>
          )}
        </div>
      </section>

      {list.isLoading && <PageLoader label="Loading mail" />}
      {list.isError && <ErrorState message="Could not load messages" onRetry={() => list.refetch()} />}
      {list.isSuccess && items.length === 0 && (
        <div className="p-4 lg:p-6">
          <EmptyState
            title={folder === "search" ? "No results" : "Nothing here"}
            hint={folder === "inbox" ? "Messages sent to you will appear here." : undefined}
          />
        </div>
      )}

      <ul className="divide-y divide-[#111111]">
        {items.map((m) => (
          <li key={m.id} className="bg-[#f9f9f7] transition-colors hover:bg-[#f2f2f2]">
            <div className="flex items-start gap-3 px-4 py-4 lg:px-6">
              <input
                type="checkbox"
                className="mt-1 h-4 w-4 accent-[#111111]"
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
                className={cx(
                  "mt-1 text-sm font-black uppercase tracking-[0.18em]",
                  m.is_starred ? "text-[#cc0000]" : "text-[#a3a3a3] hover:text-[#111111]",
                )}
                title={m.is_starred ? "Unstar" : "Star"}
                onClick={async () => {
                  await api.patch(`/api/v1/mail/messages/${m.id}`, { is_starred: !m.is_starred });
                  qc.invalidateQueries({ queryKey: ["mail", "list"] });
                }}
              >
                *
              </button>

              <Link
                href={folder === "drafts" ? `/mail/compose?draft=${m.id}` : `/mail/message/${m.id}`}
                className="flex min-w-0 flex-1 flex-col gap-2 lg:flex-row lg:items-baseline lg:gap-4"
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
                <span className={cx("w-full shrink-0 truncate font-body text-base lg:w-56", m.is_read ? "text-[#525252]" : "font-semibold text-[#111111]")}>
                  {m.from_display || m.from}
                </span>
                <span className={cx("truncate font-serif text-lg tracking-tight", m.is_read ? "text-[#525252]" : "text-[#111111]")}>
                  {m.subject || "(no subject)"}
                </span>
                <span className="hidden truncate font-body text-sm text-[#737373] lg:inline">— {m.snippet}</span>
                <span className="shrink-0 font-mono text-[10px] uppercase tracking-[0.25em] text-[#737373] lg:ml-auto">
                  {m.has_attachments && <span className="mr-2">ATT</span>}
                  {formatDate(m.date)}
                </span>
              </Link>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}
