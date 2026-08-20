"use client";

import { use, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import DOMPurify from "dompurify";
import { API_URL, api, formatBytes, formatDate, type MessageDetail } from "@/lib/api";
import { Button, ConfirmDialog, ErrorState, PageLoader, Textarea, cx, useToast } from "@/components/ui";

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

  const sanitizedHTML = useMemo(() => {
    const html = msg.data?.body_html;
    if (!html) return "";
    return DOMPurify.sanitize(html, {
      FORBID_TAGS: ["style", "form", "input", "button", "iframe", "object", "embed"],
      FORBID_ATTR: ["srcset"],
      ALLOW_UNKNOWN_PROTOCOLS: false,
    });
  }, [msg.data?.body_html]);

  if (msg.isLoading) return <PageLoader label="Opening message" />;
  if (msg.isError || !msg.data) return <ErrorState message="Message not found" onRetry={() => msg.refetch()} />;

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
    <div className="mx-auto max-w-6xl p-4 lg:p-6">
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <Button variant="secondary" onClick={() => router.back()}>
          Back
        </Button>
        <div className="ml-auto flex flex-wrap gap-2">
          <Button variant="secondary" onClick={() => setReplyOpen("reply")}>Reply</Button>
          {cc.length > 0 && <Button variant="secondary" onClick={() => setReplyOpen("reply_all")}>Reply all</Button>}
          <Link
            href={`/mail/compose?forward=${m.id}`}
            className="inline-flex h-11 items-center justify-center border-2 border-black bg-white px-4 text-xs font-bold uppercase tracking-[0.18em] transition-colors hover:bg-black hover:text-white"
          >
            Forward
          </Link>
          <Button
            variant="secondary"
            onClick={() =>
              act(
                () => api.patch(`/api/v1/mail/messages/${m.id}`, { is_starred: !m.is_starred }),
                m.is_starred ? "Unstarred" : "Starred",
              )
            }
          >
            {m.is_starred ? "Unstar" : "Star"}
          </Button>
          <Button
            variant="secondary"
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
            variant="secondary"
            onClick={() => {
              if (m.folder === "trash") setConfirmDelete(true);
              else {
                act(async () => {
                  await api.delete(`/api/v1/mail/messages/${m.id}`);
                  router.back();
                }, "Moved to trash");
              }
            }}
          >
            Delete
          </Button>
        </div>
      </div>

      <article className="qazera-panel overflow-hidden">
        <header className="border-b-2 border-black bg-[#f2f2f2] p-5 lg:p-6">
          <div className="flex flex-wrap items-start gap-4">
            <div className="min-w-0 flex-1">
              <p className="qazera-label text-accent">Message</p>
              <h1 className="mt-3 text-2xl font-black uppercase tracking-tight lg:text-4xl">
                {m.subject || "(no subject)"}
              </h1>
            </div>
            <span className="qazera-label">{formatDate(m.date)}</span>
          </div>

          <div className="mt-5 grid gap-3 text-sm lg:grid-cols-2">
            <div className="qazera-panel bg-white p-4">
              <p className="qazera-label text-accent">From</p>
              <p className="mt-2 text-sm font-bold">{m.from_display || m.from}</p>
              <p className="mt-1 text-xs text-neutral-500">&lt;{m.from}&gt;</p>
            </div>
            <div className="qazera-panel bg-white p-4">
              <p className="qazera-label text-accent">Recipients</p>
              <p className="mt-2 text-sm leading-6">
                <span className="font-bold">To:</span> {to.join(", ") || "—"}
              </p>
              {cc.length > 0 && <p className="mt-1 text-sm leading-6"><span className="font-bold">Cc:</span> {cc.join(", ")}</p>}
              {bcc.length > 0 && <p className="mt-1 text-sm leading-6"><span className="font-bold">Bcc:</span> {bcc.join(", ")}</p>}
            </div>
          </div>
        </header>

        {m.body_text ? (
          <div className="p-5 text-sm leading-7 whitespace-pre-wrap lg:p-6">{m.body_text}</div>
        ) : sanitizedHTML ? (
          <div className="prose max-w-none p-5 lg:p-6" dangerouslySetInnerHTML={{ __html: sanitizedHTML }} />
        ) : (
          <div className="p-5 text-sm text-neutral-500 lg:p-6">(empty message)</div>
        )}

        {m.attachments.length > 0 && (
          <footer className="border-t-2 border-black bg-[#f2f2f2] p-5 lg:p-6">
            <p className="qazera-label text-accent">Attachments ({m.attachments.length})</p>
            <ul className="mt-4 flex flex-wrap gap-3">
              {m.attachments.map((a) => (
                <li key={a.id}>
                  <a
                    href={`${API_URL}/api/v1/mail/attachments/${a.id}`}
                    className="inline-flex items-center gap-3 border-2 border-black bg-white px-4 py-3 text-xs font-bold uppercase tracking-[0.16em] transition-colors hover:bg-black hover:text-white"
                  >
                    <span>{a.filename}</span>
                    <span className="text-[10px] font-medium tracking-[0.12em]">{formatBytes(a.size_bytes)}</span>
                  </a>
                </li>
              ))}
            </ul>
          </footer>
        )}
      </article>

      {m.thread.length > 1 && (
        <section className="mt-6">
          <p className="qazera-label text-accent">Conversation ({m.thread.length})</p>
          <ul className="mt-3 space-y-2">
            {m.thread.map((t) => (
              <li key={t.id}>
                <Link
                  href={`/mail/message/${t.id}`}
                  className={cx(
                    "flex flex-wrap items-center gap-3 border-2 border-black bg-white px-4 py-3 text-sm transition-colors hover:bg-[#f2f2f2]",
                    t.id === m.id && "bg-black text-white",
                  )}
                >
                  <span className="w-full truncate font-bold lg:w-56">{t.from}</span>
                  <span className="min-w-0 flex-1 truncate">{t.subject || "(no subject)"}</span>
                  <span className="qazera-label">{formatDate(t.date)}</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      {replyOpen && (
        <section className="mt-6 qazera-panel p-5 lg:p-6">
          <p className="qazera-label text-accent">
            {replyOpen === "reply_all" ? `Reply to ${[m.from, ...cc].join(", ")}` : `Reply to ${m.from}`}
          </p>
          <div className="mt-4">
            <Textarea
              rows={7}
              value={replyText}
              onChange={(e) => setReplyText(e.target.value)}
              placeholder="Write your reply..."
              autoFocus
            />
          </div>
          <div className="mt-4 flex flex-wrap gap-2">
            <Button disabled={sending || !replyText.trim()} onClick={() => sendReply(replyOpen === "reply_all")}>
              {sending ? "Sending..." : "Send"}
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
