"use client";

// Reading pane: renders one message inside the mail workspace (no page
// navigation). Reply / reply-all / forward, folder moves, reminders,
// technical details and the conversation thread all live here.

import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import DOMPurify from "dompurify";
import { API_URL, api, formatBytes, formatDate, type DeliveryEvents, type MailSummary, type MessageDetail, type MessageList } from "@/lib/api";
import { Avatar, Badge, Button, ConfirmDialog, ErrorState, IconButton, Menu, Skeleton, Spinner, cx, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";
import { useCompose } from "@/components/compose";

function fullDate(iso: string): string {
  const d = new Date(iso.replace(" ", "T").replace("+00", "Z"));
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString([], { year: "numeric", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function reminderDate(kind: "today" | "tomorrow" | "week"): string {
  const d = new Date();
  if (kind === "today") d.setHours(d.getHours() + 3);
  if (kind === "tomorrow") { d.setDate(d.getDate() + 1); d.setHours(9, 0, 0, 0); }
  if (kind === "week") { d.setDate(d.getDate() + 7); d.setHours(9, 0, 0, 0); }
  return d.toISOString();
}

// DeliveryStatus shows the real delivery outcome of a sent message: overall
// status, per-recipient results and the recorded event timeline. Rendered
// only for the sender's Sent copy; data comes from message_events.
function DeliveryStatus({ id }: { id: string }) {
  const [expanded, setExpanded] = useState(false);
  const events = useQuery({
    queryKey: ["mail", "events", id],
    queryFn: () => api.get<DeliveryEvents>(`/api/v1/mail/messages/${id}/events`),
    staleTime: 15_000,
  });
  if (!events.isSuccess) return null;

  const tone =
    events.data.status === "delivered" ? "success"
    : events.data.status === "failed" ? "danger"
    : events.data.status === "partially_delivered" || events.data.status === "quarantined" ? "warning"
    : "neutral";
  const failed = events.data.recipients.filter((r) => r.status === "failed");

  return (
    <div className="anim-rise mt-3 rounded-[8px] border border-border bg-background px-3.5 py-2.5 text-xs">
      <button
        type="button"
        className="flex w-full items-center gap-2 text-left"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
      >
        <Badge tone={tone}>{events.data.status.replace("_", " ")}</Badge>
        <span className="min-w-0 flex-1 truncate text-muted-foreground">
          {failed.length > 0
            ? `${failed.length} of ${events.data.recipients.length} recipient${events.data.recipients.length > 1 ? "s" : ""} failed`
            : `${events.data.recipients.length} recipient${events.data.recipients.length > 1 ? "s" : ""}`}
        </span>
        <Icon name="chevron-down" className={cx("h-3 w-3 shrink-0 transition-transform", expanded && "rotate-180")} />
      </button>
      {expanded && (
        <div className="mt-2.5 space-y-2 border-t border-border pt-2.5">
          {events.data.recipients.map((r) => (
            <p key={r.address} className="flex items-baseline gap-2">
              <span className="min-w-0 flex-1 truncate">{r.address}</span>
              <span className={cx("shrink-0 font-medium", r.status === "delivered" ? "text-success" : r.status === "failed" ? "text-danger" : "text-warning")}>
                {r.status}
              </span>
              {r.error && <span className="shrink-0 text-faint">{r.error}</span>}
            </p>
          ))}
          {events.data.events.length > 0 && (
            <div className="border-t border-border pt-2 text-faint">
              {events.data.events.map((e, i) => (
                <p key={i} className="font-mono text-[11px]">{formatDate(e.created_at)} · {e.type}</p>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function MessagePane({
  id,
  onClose,
  onOpenMessage,
}: {
  id: string;
  onClose: () => void;
  onOpenMessage: (id: string) => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const { openCompose } = useCompose();
  const [replyText, setReplyText] = useState("");
  const [replyMode, setReplyMode] = useState<"reply" | "reply_all">("reply");
  const [sending, setSending] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [showDetails, setShowDetails] = useState(false);
  const [threadOpen, setThreadOpen] = useState(true);

  const summary = useQuery({ queryKey: ["mail", "summary"], queryFn: () => api.get<MailSummary>("/api/v1/mail/summary") });
  const selfAddress = summary.data?.mailbox.address?.toLowerCase() ?? "";

  const msg = useQuery({
    queryKey: ["mail", "message", id],
    queryFn: () => api.get<MessageDetail>(`/api/v1/mail/messages/${id}`),
  });

  // Opening a message marks it read server-side: reflect that in the list
  // caches and folder counters without a refetch storm.
  useEffect(() => {
    if (!msg.data) return;
    qc.setQueriesData<MessageList>({ queryKey: ["mail", "list"] }, (old) =>
      old ? { ...old, messages: old.messages.map((x) => (x.id === id ? { ...x, is_read: true } : x)) } : old,
    );
    qc.invalidateQueries({ queryKey: ["mail", "summary"] });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [msg.data?.id]);

  // Reset transient UI state when switching to another message.
  const [prevId, setPrevId] = useState(id);
  if (id !== prevId) {
    setPrevId(id);
    setReplyText("");
    setShowDetails(false);
  }

  const sanitizedHTML = useMemo(() => {
    const html = msg.data?.body_html;
    if (!html) return "";
    return DOMPurify.sanitize(html, {
      FORBID_TAGS: ["style", "form", "input", "button", "iframe", "object", "embed"],
      FORBID_ATTR: ["srcset"],
      ALLOW_UNKNOWN_PROTOCOLS: false,
    });
  }, [msg.data?.body_html]);

  if (msg.isLoading) {
    return (
      <div className="flex h-full flex-col p-5">
        <div className="flex items-center gap-2">
          <Skeleton className="h-7 w-7 rounded-full" />
          <div className="flex-1 space-y-1.5"><Skeleton className="h-3.5 w-1/3" /><Skeleton className="h-3 w-1/4" /></div>
        </div>
        <Skeleton className="mt-6 h-4 w-2/3" />
        <div className="mt-6 space-y-2.5">
          {Array.from({ length: 6 }, (_, i) => <Skeleton key={i} className={i % 3 === 2 ? "h-3 w-2/3" : "h-3"} />)}
        </div>
      </div>
    );
  }
  if (msg.isError || !msg.data) {
    return <ErrorState message="Message not found" onRetry={() => msg.refetch()} />;
  }

  const m = msg.data;
  const to = m.recipients.filter((r) => r.kind === "to").map((r) => r.address);
  const cc = m.recipients.filter((r) => r.kind === "cc").map((r) => r.address);
  const bcc = m.recipients.filter((r) => r.kind === "bcc").map((r) => r.address);
  const replyAllTargets = [...new Set([m.from, ...to].map((a) => a.toLowerCase()))].filter((a) => a !== selfAddress);
  const canReplyAll = replyAllTargets.length > 1 || cc.length > 0;

  async function act(fn: () => Promise<unknown>, message: string, closeAfter = false) {
    try {
      await fn();
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", message);
      if (closeAfter) onClose();
    } catch {
      toast("error", "Action failed");
    }
  }

  async function sendReply() {
    if (!replyText.trim()) return;
    setSending(true);
    try {
      await api.post("/api/v1/mail/send", {
        to: replyMode === "reply_all" ? replyAllTargets : [m.from],
        cc: replyMode === "reply_all" ? cc.filter((a) => a.toLowerCase() !== selfAddress) : [],
        subject: m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
        text: replyText,
        in_reply_to: m.message_id,
      });
      setReplyText("");
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", "Reply sent");
    } catch {
      toast("error", "Could not send reply");
    } finally {
      setSending(false);
    }
  }

  function remind(kind: "today" | "tomorrow" | "week") {
    void act(
      () => api.post("/api/v1/tasks", {
        title: m.subject || "Email follow-up",
        description: `From: ${m.from}`,
        source_type: "email",
        source_id: m.id,
        reminder_at: reminderDate(kind),
      }),
      "Reminder created",
    );
  }

  return (
    <div className="flex h-full min-w-0 flex-col bg-surface-elevated">
      {/* Toolbar */}
      <div className="flex h-11 shrink-0 items-center gap-0.5 border-b border-border px-2">
        <IconButton label="Close" icon="x" onClick={onClose} className="lg:hidden" />
        <IconButton label="Reply" icon="reply" onClick={() => { setReplyMode("reply"); document.getElementById("quick-reply")?.focus(); }} />
        {canReplyAll && <IconButton label="Reply all" icon="reply-all" onClick={() => { setReplyMode("reply_all"); document.getElementById("quick-reply")?.focus(); }} />}
        <IconButton label="Forward" icon="forward" onClick={() => openCompose({ forwardFrom: m.id })} />
        <span className="mx-1 h-4 w-px bg-border" />
        <IconButton
          label={m.is_starred ? "Unstar" : "Star"}
          icon="star"
          className={m.is_starred ? "text-warning hover:text-warning" : undefined}
          onClick={() => act(() => api.patch(`/api/v1/mail/messages/${m.id}`, { is_starred: !m.is_starred }), m.is_starred ? "Unstarred" : "Starred")}
        />
        <IconButton
          label="Mark unread"
          icon="mail"
          onClick={() => act(async () => { await api.patch(`/api/v1/mail/messages/${m.id}`, { is_read: false }); }, "Marked unread", true)}
        />
        <Menu
          trigger={<IconButton label="Remind me" icon="clock" />}
          items={[
            { type: "label", label: "Remind me" },
            { label: "Later today (+3h)", icon: "clock", onSelect: () => remind("today") },
            { label: "Tomorrow 09:00", icon: "calendar", onSelect: () => remind("tomorrow") },
            { label: "Next week", icon: "calendar", onSelect: () => remind("week") },
          ]}
        />
        <IconButton
          label={m.folder === "trash" ? "Delete forever" : "Delete"}
          icon="trash"
          onClick={() => {
            if (m.folder === "trash") setConfirmDelete(true);
            else void act(async () => { await api.delete(`/api/v1/mail/messages/${m.id}`); }, "Moved to Trash", true);
          }}
        />
        <span className="flex-1" />
        <Menu
          trigger={<IconButton label="More actions" icon="more-horizontal" />}
          items={[
            ...(m.folder !== "spam" && m.folder !== "sent" && m.folder !== "drafts"
              ? [{ label: "Move to Spam", icon: "alert-octagon", onSelect: () => void act(async () => { await api.patch(`/api/v1/mail/messages/${m.id}`, { folder: "spam" }); }, "Moved to Spam", true) }]
              : []),
            ...(m.folder === "spam" || m.folder === "trash"
              ? [{ label: "Move to Inbox", icon: "inbox", onSelect: () => void act(async () => { await api.patch(`/api/v1/mail/messages/${m.id}`, { folder: "inbox" }); }, "Moved to Inbox", true) }]
              : []),
            { label: "Add to Tasks", icon: "check-circle", onSelect: () => void act(() => api.post("/api/v1/tasks", { title: m.subject || "Email follow-up", description: `From: ${m.from}`, source_type: "email", source_id: m.id }), "Added to Tasks & Reminders") },
            { label: "Copy sender address", icon: "at-sign", onSelect: () => { void navigator.clipboard.writeText(m.from); toast("info", "Address copied"); } },
            { type: "separator" },
            { label: showDetails ? "Hide details" : "View details", icon: "info", onSelect: () => setShowDetails((v) => !v) },
          ]}
        />
      </div>

      {/* Scrollable content */}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="px-5 pb-5 pt-4">
          <div className="flex items-start justify-between gap-3">
            <h1 className="min-w-0 text-[17px] font-semibold leading-6 tracking-[-.01em]">
              {m.subject || "(no subject)"}
            </h1>
            <div className="flex shrink-0 items-center gap-1.5 pt-0.5">
              {m.folder !== "inbox" && <Badge tone="neutral" className="capitalize">{m.folder}</Badge>}
              {m.is_starred && <Icon name="star" className="h-3.5 w-3.5 text-warning" />}
            </div>
          </div>

          <div className="mt-3.5 flex items-start gap-3">
            <Avatar name={m.from_display || m.from} size="lg" />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-baseline gap-x-2">
                <span className="text-[13px] font-semibold">{m.from_display || m.from}</span>
                <span className="truncate text-xs text-muted-foreground">&lt;{m.from}&gt;</span>
              </div>
              <button
                type="button"
                className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                onClick={() => setShowDetails((v) => !v)}
              >
                to {to.length ? to.join(", ") : "—"}{cc.length > 0 && `, cc ${cc.length}`}
                <Icon name="chevron-down" className={cx("h-3 w-3 transition-transform", showDetails && "rotate-180")} />
              </button>
            </div>
            <span className="shrink-0 pt-0.5 text-xs text-muted-foreground" title={m.date}>{fullDate(m.date)}</span>
          </div>

          {m.folder === "sent" && <DeliveryStatus id={m.id} />}

          {showDetails && (
            <dl className="anim-rise mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 rounded-[8px] border border-border bg-background px-3.5 py-3 text-xs">
              <dt className="text-muted-foreground">From</dt><dd className="min-w-0 break-all">{m.from}</dd>
              <dt className="text-muted-foreground">To</dt><dd className="min-w-0 break-all">{to.join(", ") || "—"}</dd>
              {cc.length > 0 && <><dt className="text-muted-foreground">Cc</dt><dd className="min-w-0 break-all">{cc.join(", ")}</dd></>}
              {bcc.length > 0 && <><dt className="text-muted-foreground">Bcc</dt><dd className="min-w-0 break-all">{bcc.join(", ")}</dd></>}
              <dt className="text-muted-foreground">Date</dt><dd>{m.date}</dd>
              <dt className="text-muted-foreground">Message ID</dt><dd className="code-value min-w-0 break-all">{m.message_id || "—"}</dd>
              <dt className="text-muted-foreground">Folder</dt><dd className="capitalize">{m.folder}</dd>
              {m.attachments.length > 0 && <><dt className="text-muted-foreground">Attachments</dt><dd>{m.attachments.length} · {formatBytes(m.attachments.reduce((n, a) => n + a.size_bytes, 0))}</dd></>}
            </dl>
          )}

          <div className="mt-5 border-t border-border pt-5">
            {m.body_text ? (
              <div className="mail-body whitespace-pre-wrap">{m.body_text}</div>
            ) : sanitizedHTML ? (
              <div className="mail-body" dangerouslySetInnerHTML={{ __html: sanitizedHTML }} />
            ) : (
              <p className="text-[13px] text-faint">(empty message)</p>
            )}
          </div>

          {m.attachments.length > 0 && (
            <div className="mt-6 border-t border-border pt-4">
              <p className="text-xs font-medium text-muted-foreground">
                {m.attachments.length} attachment{m.attachments.length > 1 ? "s" : ""}
              </p>
              <ul className="mt-2.5 flex flex-wrap gap-2">
                {m.attachments.map((a) => (
                  <li key={a.id}>
                    <a
                      // Attachments are MIME parts in the mail store; the
                      // backend streams them by blob id with the display
                      // metadata carried in the query.
                      href={`${API_URL}/api/v1/mail/blob/${a.id}?name=${encodeURIComponent(a.filename)}&type=${encodeURIComponent(a.content_type)}`}
                      className="group inline-flex items-center gap-2.5 rounded-[8px] border border-border bg-background px-3 py-2 transition-colors hover:border-border-strong hover:bg-surface-elevated"
                    >
                      <span className="grid h-7 w-7 place-items-center rounded-[6px] border border-border bg-surface-elevated text-muted-foreground">
                        <Icon name="paperclip" className="h-3.5 w-3.5" />
                      </span>
                      <span className="min-w-0">
                        <span className="block max-w-52 truncate text-xs font-medium">{a.filename}</span>
                        <span className="block text-[11px] text-muted-foreground">{formatBytes(a.size_bytes)}</span>
                      </span>
                      <Icon name="download" className="h-3.5 w-3.5 text-faint opacity-0 transition-opacity group-hover:opacity-100" />
                    </a>
                  </li>
                ))}
              </ul>
            </div>
          )}

          {m.thread.length > 1 && (
            <div className="mt-6 border-t border-border pt-4">
              <button
                type="button"
                className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground hover:text-foreground"
                onClick={() => setThreadOpen((v) => !v)}
              >
                <Icon name="chevron-down" className={cx("h-3 w-3 transition-transform", !threadOpen && "-rotate-90")} />
                Conversation · {m.thread.length} messages
              </button>
              {threadOpen && (
                <ul className="mt-2.5 space-y-1">
                  {m.thread.map((titem) => (
                    <li key={titem.id}>
                      <button
                        type="button"
                        onClick={() => titem.id !== m.id && onOpenMessage(titem.id)}
                        className={cx(
                          "flex w-full items-center gap-3 rounded-[7px] border px-3 py-2 text-left text-[13px] transition-colors",
                          titem.id === m.id
                            ? "border-border-strong bg-background"
                            : "border-transparent hover:bg-background",
                        )}
                      >
                        <Avatar name={titem.from} size="sm" />
                        <span className="w-40 shrink-0 truncate font-medium">{titem.from}</span>
                        <span className="min-w-0 flex-1 truncate text-muted-foreground">{titem.subject || "(no subject)"}</span>
                        <span className="shrink-0 text-[11px] text-faint">{formatDate(titem.date)}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Quick reply */}
      {m.folder !== "drafts" && (
        <div className="shrink-0 border-t border-border bg-background/60 p-3">
          <div className="rounded-[9px] border border-border bg-surface-elevated transition-[border-color,box-shadow] focus-within:border-primary focus-within:ring-2 focus-within:ring-primary/12">
            <div className="flex items-center gap-2 px-3 pt-2 text-xs text-muted-foreground">
              <Icon name={replyMode === "reply_all" ? "reply-all" : "reply"} className="h-3 w-3" />
              {replyMode === "reply_all" ? `Reply all — ${[...replyAllTargets, ...cc].length} recipients` : `Reply to ${m.from_display || m.from}`}
              {canReplyAll && (
                <button
                  type="button"
                  className="ml-auto font-medium text-primary hover:underline"
                  onClick={() => setReplyMode((v) => (v === "reply" ? "reply_all" : "reply"))}
                >
                  {replyMode === "reply" ? "Switch to reply all" : "Switch to reply"}
                </button>
              )}
            </div>
            <textarea
              id="quick-reply"
              rows={2}
              value={replyText}
              onChange={(e) => setReplyText(e.target.value)}
              onKeyDown={(e) => { if ((e.ctrlKey || e.metaKey) && e.key === "Enter") void sendReply(); }}
              placeholder="Write a reply… (Ctrl+Enter to send)"
              className="max-h-40 w-full resize-y bg-transparent px-3 py-2 text-[13px] leading-relaxed outline-none placeholder:text-faint"
            />
            <div className="flex items-center gap-2 px-2.5 pb-2">
              <Button size="sm" disabled={sending || !replyText.trim()} onClick={() => void sendReply()}>
                {sending ? <><Spinner className="h-3 w-3 border-white" /> Sending…</> : "Send"}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => openCompose({
                  to: (replyMode === "reply_all" ? replyAllTargets : [m.from]).join(", "),
                  cc: replyMode === "reply_all" ? cc.join(", ") : "",
                  subject: m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
                  body: replyText,
                  inReplyTo: m.message_id,
                })}
              >
                Open in composer
              </Button>
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirmDelete}
        title="Delete forever?"
        body="This message will be permanently removed from Trash. This cannot be undone."
        confirmLabel="Delete forever"
        danger
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() => void act(async () => { setConfirmDelete(false); await api.delete(`/api/v1/mail/messages/${m.id}`); }, "Deleted", true)}
      />
    </div>
  );
}
