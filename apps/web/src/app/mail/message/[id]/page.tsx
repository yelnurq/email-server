"use client";

import { use, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import DOMPurify from "dompurify";
import { API_URL, api, formatBytes, formatDate, type MessageDetail } from "@/lib/api";
import {
  Button,
  ConfirmDialog,
  ErrorState,
  PageLoader,
  Textarea,
  cx,
  useToast,
} from "@/components/ui";

export default function MessagePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const [replyOpen, setReplyOpen] = useState<null | "reply" | "reply_all">(null);
  const [replyText, setReplyText] = useState("");
  const [sending, setSending] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const msg = useQuery({
    queryKey: ["mail", "message", id],
    queryFn: () => api.get<MessageDetail>(`/api/v1/mail/messages/${id}`),
  });

  // HTML bodies are sanitized before rendering; scripts, event handlers and
  // dangerous URLs are stripped. Plain-text messages never touch innerHTML.
  const sanitizedHTML = useMemo(() => {
    const html = msg.data?.body_html;
    if (!html) return "";
    return DOMPurify.sanitize(html, {
      FORBID_TAGS: ["style", "form", "input", "button", "iframe", "object", "embed"],
      FORBID_ATTR: ["srcset"],
      ALLOW_UNKNOWN_PROTOCOLS: false,
    });
  }, [msg.data?.body_html]);

  if (msg.isLoading) return <PageLoader />;
  if (msg.isError || !msg.data)
    return <ErrorState message="Message not found" onRetry={() => msg.refetch()} />;
  const m = msg.data;

  const to = m.recipients.filter((r) => r.kind === "to").map((r) => r.address);
  const cc = m.recipients.filter((r) => r.kind === "cc").map((r) => r.address);
  const bcc = m.recipients.filter((r) => r.kind === "bcc").map((r) => r.address);

  async function act(fn: () => Promise<unknown>, message: string) {
    try {
      await fn();
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", message);
    } catch {
      toast("error", "Action failed");
    }
  }

  async function sendReply(all: boolean) {
    if (!replyText.trim()) return;
    setSending(true);
    try {
      const replyTo = [m.from];
      const replyCc = all ? cc : [];
      await api.post("/api/v1/mail/send", {
        to: replyTo,
        cc: replyCc,
        subject: m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
        text: replyText,
        in_reply_to: m.message_id,
      });
      setReplyOpen(null);
      setReplyText("");
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", "Reply sent");
    } catch {
      toast("error", "Could not send reply");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="mx-auto max-w-4xl p-4">
      <div className="mb-3 flex items-center gap-2">
        <Button variant="ghost" onClick={() => router.back()}>
          ← Back
        </Button>
        <div className="ml-auto flex items-center gap-1">
          <Button variant="ghost" onClick={() => setReplyOpen("reply")}>Reply</Button>
          {cc.length > 0 && (
            <Button variant="ghost" onClick={() => setReplyOpen("reply_all")}>Reply all</Button>
          )}
          <Link
            href={`/mail/compose?forward=${m.id}`}
            className="rounded-lg px-3 py-1.5 text-sm font-medium text-neutral-600 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800"
          >
            Forward
          </Link>
          <Button
            variant="ghost"
            onClick={() =>
              act(
                () => api.patch(`/api/v1/mail/messages/${m.id}`, { is_starred: !m.is_starred }),
                m.is_starred ? "Unstarred" : "Starred",
              )
            }
          >
            {m.is_starred ? "★ Unstar" : "☆ Star"}
          </Button>
          <Button
            variant="ghost"
            onClick={() =>
              act(async () => {
                await api.patch(`/api/v1/mail/messages/${m.id}`, { is_read: false });
                router.back();
              }, "Marked unread")
            }
          >
            Mark unread
          </Button>
          <Button
            variant="ghost"
            onClick={() => {
              if (m.folder === "trash") setConfirmDelete(true);
              else

                act(async () => {
                  await api.delete(`/api/v1/mail/messages/${m.id}`);
                  router.back();
                }, "Moved to Trash");
            }}
          >
            Delete
          </Button>
        </div>
      </div>

      <article className="rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
        <header className="border-b border-neutral-100 p-5 dark:border-neutral-800">
          <h1 className="text-lg font-semibold">{m.subject || "(no subject)"}</h1>
          <div className="mt-3 flex flex-wrap items-baseline gap-x-2 text-sm">
            <span className="font-medium">{m.from_display || m.from}</span>
            <span className="text-neutral-400">&lt;{m.from}&gt;</span>
            <span className="ml-auto text-xs text-neutral-400">{formatDate(m.date)}</span>
          </div>
          <div className="mt-1 space-y-0.5 text-xs text-neutral-500">
            <p>To: {to.join(", ") || "—"}</p>
            {cc.length > 0 && <p>Cc: {cc.join(", ")}</p>}
            {bcc.length > 0 && <p>Bcc: {bcc.join(", ")}</p>}
          </div>
        </header>
        {m.body_text ? (
          <div className="whitespace-pre-wrap p-5 text-sm leading-relaxed">{m.body_text}</div>
        ) : sanitizedHTML ? (
          <div
            className="prose prose-sm max-w-none p-5 dark:prose-invert"
            dangerouslySetInnerHTML={{ __html: sanitizedHTML }}
          />
        ) : (
          <div className="p-5 text-sm text-neutral-400">(empty message)</div>
        )}
        {m.attachments.length > 0 && (
          <footer className="border-t border-neutral-100 p-5 dark:border-neutral-800">
            <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-400">
              Attachments ({m.attachments.length})
            </h2>
            <ul className="flex flex-wrap gap-2">
              {m.attachments.map((a) => (
                <li key={a.id}>
                  <a
                    href={`${API_URL}/api/v1/mail/attachments/${a.id}`}
                    className="flex items-center gap-2 rounded-lg border border-neutral-200 px-3 py-2 text-sm hover:border-indigo-300 hover:bg-indigo-50 dark:border-neutral-700 dark:hover:bg-indigo-950"
                  >
                    <span aria-hidden>📎</span>
                    <span className="max-w-56 truncate">{a.filename}</span>
                    <span className="text-xs text-neutral-400">{formatBytes(a.size_bytes)}</span>
                  </a>
                </li>
              ))}
            </ul>
          </footer>
        )}
      </article>

      {m.thread.length > 1 && (
        <section className="mt-4">
          <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-neutral-400">
            Conversation ({m.thread.length})
          </h2>
          <ul className="space-y-1">
            {m.thread.map((t) => (
              <li key={t.id}>
                <Link
                  href={`/mail/message/${t.id}`}
                  className={cx(
                    "flex items-baseline gap-3 rounded-lg border border-neutral-200 bg-white px-3 py-2 text-sm hover:bg-neutral-50 dark:border-neutral-800 dark:bg-neutral-900 dark:hover:bg-neutral-800",
                    t.id === m.id && "ring-1 ring-indigo-400",
                  )}
                >
                  <span className="w-44 shrink-0 truncate text-neutral-600 dark:text-neutral-300">{t.from}</span>
                  <span className="truncate">{t.subject || "(no subject)"}</span>
                  <span className="ml-auto shrink-0 text-xs text-neutral-400">{formatDate(t.date)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      {replyOpen && (
        <section className="mt-4 rounded-2xl border border-neutral-200 bg-white p-4 shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
          <p className="mb-2 text-xs text-neutral-500">
            {replyOpen === "reply_all"
              ? `Reply to ${[m.from, ...cc].join(", ")}`
              : `Reply to ${m.from}`}
          </p>
          <Textarea
            rows={6}
            value={replyText}
            onChange={(e) => setReplyText(e.target.value)}
            placeholder="Write your reply…"
            autoFocus
          />
          <div className="mt-3 flex gap-2">
            <Button disabled={sending || !replyText.trim()} onClick={() => sendReply(replyOpen === "reply_all")}>
              {sending ? "Sending…" : "Send"}
            </Button>
            <Button variant="secondary" onClick={() => setReplyOpen(null)}>
              Discard
            </Button>
          </div>
        </section>
      )}

      <ConfirmDialog
        open={confirmDelete}
        title="Delete forever?"
        body="This message will be permanently removed from Trash."
        confirmLabel="Delete forever"
        danger
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() =>
          act(async () => {
            setConfirmDelete(false);
            await api.delete(`/api/v1/mail/messages/${m.id}`);
            router.back();
          }, "Deleted")
        }
      />
    </div>
  );
}
