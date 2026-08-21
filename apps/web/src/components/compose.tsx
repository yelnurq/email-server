"use client";

// Floating composer window (Gmail-style): opened from anywhere in the mail
// workspace via useCompose(). Supports drafts with autosave, reply/forward
// prefills, attachments with upload progress, minimize and safe discard.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  api,
  ApiError,
  formatBytes,
  uploadAttachment,
  type Attachment,
  type MessageDetail,
} from "@/lib/api";
import { Button, ConfirmDialog, IconButton, Spinner, cx, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";
import { RecipientSelector } from "@/components/recipient-selector";

export type ComposeOptions = {
  draftId?: string;
  forwardFrom?: string;
  to?: string;
  cc?: string;
  subject?: string;
  body?: string;
  inReplyTo?: string;
};

const ComposeCtx = createContext<{ openCompose: (opts?: ComposeOptions) => void }>({ openCompose: () => {} });

export function useCompose() {
  return useContext(ComposeCtx);
}

type PendingUpload = {
  key: string;
  name: string;
  progress: number;
  attachment?: Attachment;
  error?: string;
};

function parseAddresses(s: string): string[] {
  return s.split(/[,;\s]+/).map((x) => x.trim()).filter(Boolean);
}

export function ComposeProvider({ children }: { children: React.ReactNode }) {
  const [session, setSession] = useState<{ key: number; opts: ComposeOptions } | null>(null);
  const openCompose = useCallback((opts?: ComposeOptions) => {
    // A new key remounts the window so switching drafts resets state cleanly.
    setSession({ key: Date.now(), opts: opts ?? {} });
  }, []);

  return (
    <ComposeCtx.Provider value={{ openCompose }}>
      {children}
      {session && (
        <ComposerWindow key={session.key} opts={session.opts} onClose={() => setSession(null)} />
      )}
    </ComposeCtx.Provider>
  );
}

function ComposerWindow({ opts, onClose }: { opts: ComposeOptions; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();

  const [minimized, setMinimized] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [to, setTo] = useState(opts.to ?? "");
  const [cc, setCc] = useState(opts.cc ?? "");
  const [bcc, setBcc] = useState("");
  const [showCc, setShowCc] = useState(Boolean(opts.cc));
  const [subject, setSubject] = useState(opts.subject ?? "");
  const [text, setText] = useState(opts.body ?? "");
  const [inReplyTo] = useState(opts.inReplyTo ?? "");
  const [sending, setSending] = useState(false);
  const [uploads, setUploads] = useState<PendingUpload[]>([]);
  const [loaded, setLoaded] = useState(!opts.draftId && !opts.forwardFrom);
  const [draftId, setDraftId] = useState<string | null>(opts.draftId ?? null);
  const [savedAt, setSavedAt] = useState<string>("");
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const dirty = useRef(false);

  // Prefill from an existing draft or a forwarded message.
  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        if (opts.draftId) {
          const d = await api.get<MessageDetail>(`/api/v1/mail/messages/${opts.draftId}`);
          if (cancelled) return;
          setTo(d.recipients.filter((r) => r.kind === "to").map((r) => r.address).join(", "));
          setCc(d.recipients.filter((r) => r.kind === "cc").map((r) => r.address).join(", "));
          setBcc(d.recipients.filter((r) => r.kind === "bcc").map((r) => r.address).join(", "));
          setShowCc(d.recipients.some((r) => r.kind !== "to"));
          setSubject(d.subject);
          setText(d.body_text);
        } else if (opts.forwardFrom) {
          const d = await api.get<MessageDetail>(`/api/v1/mail/messages/${opts.forwardFrom}`);
          if (cancelled) return;
          setSubject(d.subject.startsWith("Fwd:") ? d.subject : `Fwd: ${d.subject}`);
          setText(`\n\n---------- Forwarded message ----------\nFrom: ${d.from}\nDate: ${d.date}\nSubject: ${d.subject}\n\n${d.body_text}`);
        }
      } catch {
        toast("error", "Could not load message");
      } finally {
        if (!cancelled) setLoaded(true);
      }
    }
    void load();
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const saveDraft = useCallback(async () => {
    const body = { to: parseAddresses(to), cc: parseAddresses(cc), bcc: parseAddresses(bcc), subject, text };
    try {
      // Drafts live in the mail store, which has no in-place update: each
      // save writes a new copy and returns its id, so always adopt it.
      if (draftId) {
        const res = await api.put<{ id: string }>(`/api/v1/mail/drafts/${draftId}`, body);
        if (res?.id) setDraftId(res.id);
      } else {
        const res = await api.post<{ id: string }>("/api/v1/mail/drafts", body);
        setDraftId(res.id);
      }
      dirty.current = false;
      setSavedAt(new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }));
      qc.invalidateQueries({ queryKey: ["mail"] });
      return true;
    } catch {
      return false;
    }
  }, [to, cc, bcc, subject, text, draftId, qc]);

  // Autosave every 5s while there is unsaved content.
  useEffect(() => {
    const timer = setInterval(() => {
      if (dirty.current && (to || subject || text)) void saveDraft();
    }, 5000);
    return () => clearInterval(timer);
  }, [saveDraft, to, subject, text]);

  // Escape minimizes (never destroys work).
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMinimized(true);
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, []);

  function markDirty() { dirty.current = true; }
  const hasContent = Boolean(to.trim() || subject.trim() || text.trim() || uploads.length);

  async function addFiles(files: FileList | null) {
    if (!files) return;
    for (const file of Array.from(files)) {
      if (file.size > 25 * 1024 * 1024) {
        toast("error", `${file.name}: file exceeds the 25 MB limit`);
        continue;
      }
      const key = `${file.name}-${Date.now()}-${Math.random()}`;
      setUploads((u) => [...u, { key, name: file.name, progress: 0 }]);
      try {
        const att = await uploadAttachment(file, (fraction) =>
          setUploads((u) => u.map((x) => (x.key === key ? { ...x, progress: fraction } : x))),
        );
        setUploads((u) => u.map((x) => (x.key === key ? { ...x, progress: 1, attachment: att } : x)));
      } catch {
        setUploads((u) => u.map((x) => (x.key === key ? { ...x, error: "Upload failed" } : x)));
        toast("error", `${file.name}: upload failed`);
      }
    }
  }

  async function send() {
    const toList = parseAddresses(to);
    if (toList.length === 0) {
      toast("error", "Add at least one recipient");
      return;
    }
    if (uploads.some((u) => !u.attachment && !u.error)) {
      toast("error", "Wait for attachments to finish uploading");
      return;
    }
    const attachmentIds = uploads.filter((u) => u.attachment).map((u) => u.attachment!.id);
    setSending(true);
    try {
      if (draftId && attachmentIds.length === 0 && !inReplyTo) {
        const ok = await saveDraft();
        if (!ok) throw new Error("draft save failed");
        await api.post(`/api/v1/mail/drafts/${draftId}/send`);
      } else {
        await api.post("/api/v1/mail/send", {
          to: toList,
          cc: parseAddresses(cc),
          bcc: parseAddresses(bcc),
          subject,
          text,
          attachment_ids: attachmentIds,
          ...(inReplyTo ? { in_reply_to: inReplyTo } : {}),
        });
        if (draftId) await api.delete(`/api/v1/mail/messages/${draftId}`).catch(() => {});
      }
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", "Message sent");
      onClose();
    } catch (err) {
      toast(
        "error",
        err instanceof ApiError && err.code === "INVALID_MESSAGE" ? err.message : "Could not send. Draft is preserved.",
      );
      if (!draftId) void saveDraft();
    } finally {
      setSending(false);
    }
  }

  // Closing keeps work: save silently as a draft when there is content.
  async function close() {
    if (hasContent && dirty.current) {
      const ok = await saveDraft();
      if (ok) toast("info", "Draft saved");
    }
    onClose();
  }

  async function discard() {
    if (draftId) {
      await api.delete(`/api/v1/mail/messages/${draftId}`).catch(() => {});
      qc.invalidateQueries({ queryKey: ["mail"] });
    }
    setConfirmDiscard(false);
    onClose();
  }

  if (minimized) {
    return (
      <div className="fixed bottom-0 right-4 z-[45] w-72 max-w-[calc(100vw-2rem)]">
        <button
          type="button"
          className="flex w-full items-center gap-2.5 rounded-t-[10px] border border-b-0 border-border bg-graphite px-3.5 py-2.5 text-left text-[13px] font-medium text-graphite-foreground shadow-[var(--shadow-window)]"
          onClick={() => setMinimized(false)}
        >
          <Icon name="pen" className="h-3.5 w-3.5 opacity-70" />
          <span className="min-w-0 flex-1 truncate">{subject || "New message"}</span>
          <span
            role="button"
            tabIndex={0}
            className="grid h-6 w-6 place-items-center rounded hover:bg-white/10"
            onClick={(e) => { e.stopPropagation(); void close(); }}
            onKeyDown={(e) => { if (e.key === "Enter") { e.stopPropagation(); void close(); } }}
            aria-label="Close composer"
          >
            <Icon name="x" className="h-3.5 w-3.5" />
          </span>
        </button>
      </div>
    );
  }

  return (
    <>
      <div
        role="dialog"
        aria-label="New message"
        className={cx(
          "fixed z-[45] flex flex-col overflow-hidden border border-border bg-surface-elevated shadow-[var(--shadow-window)]",
          expanded
            ? "inset-4 rounded-xl sm:inset-8"
            : "inset-x-2 bottom-0 top-14 rounded-t-[10px] sm:inset-x-auto sm:right-6 sm:top-auto sm:h-[560px] sm:max-h-[calc(100vh-5rem)] sm:w-[620px] sm:max-w-[calc(100vw-3rem)]",
        )}
      >
        <header className="flex h-10 shrink-0 items-center gap-1 bg-graphite pl-3.5 pr-2 text-graphite-foreground">
          <p className="min-w-0 flex-1 truncate text-[13px] font-medium">
            {subject || (inReplyTo ? "Reply" : "New message")}
          </p>
          {savedAt && <span className="mr-1 hidden text-[11px] text-graphite-foreground/60 sm:inline">Draft saved {savedAt}</span>}
          <button type="button" className="grid h-7 w-7 place-items-center rounded hover:bg-white/10" onClick={() => setMinimized(true)} aria-label="Minimize">
            <Icon name="minus" className="h-3.5 w-3.5" />
          </button>
          <button type="button" className="hidden h-7 w-7 place-items-center rounded hover:bg-white/10 sm:grid" onClick={() => setExpanded((v) => !v)} aria-label={expanded ? "Restore size" : "Expand"}>
            <Icon name="maximize" className="h-3.5 w-3.5" />
          </button>
          <button type="button" className="grid h-7 w-7 place-items-center rounded hover:bg-white/10" onClick={() => void close()} aria-label="Save and close">
            <Icon name="x" className="h-3.5 w-3.5" />
          </button>
        </header>

        {!loaded ? (
          <div className="flex flex-1 items-center justify-center gap-2 text-[13px] text-muted-foreground">
            <Spinner /> Loading…
          </div>
        ) : (
          <>
            <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
              <div className="flex items-start gap-2 border-b border-border px-4">
                <span className="pt-2.5 text-[13px] text-muted-foreground">To</span>
                <div className="min-w-0 flex-1">
                  <RecipientSelector value={to} onChange={(next) => { setTo(next); markDirty(); }} autoFocus={!opts.draftId} placeholder="" />
                </div>
                {!showCc && (
                  <button type="button" className="shrink-0 py-2.5 text-xs font-medium text-muted-foreground hover:text-foreground" onClick={() => setShowCc(true)}>
                    Cc / Bcc
                  </button>
                )}
              </div>
              {showCc && (
                <>
                  <div className="flex items-center gap-2 border-b border-border px-4">
                    <span className="text-[13px] text-muted-foreground">Cc</span>
                    <input value={cc} onChange={(e) => { setCc(e.target.value); markDirty(); }} className="h-9 min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-faint" placeholder="cc@company.kz" />
                  </div>
                  <div className="flex items-center gap-2 border-b border-border px-4">
                    <span className="text-[13px] text-muted-foreground">Bcc</span>
                    <input value={bcc} onChange={(e) => { setBcc(e.target.value); markDirty(); }} className="h-9 min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-faint" placeholder="bcc@company.kz" />
                  </div>
                </>
              )}
              <div className="border-b border-border px-4">
                <input
                  value={subject}
                  onChange={(e) => { setSubject(e.target.value); markDirty(); }}
                  className="h-9 w-full bg-transparent text-[13px] font-medium outline-none placeholder:font-normal placeholder:text-faint"
                  placeholder="Subject"
                  aria-label="Subject"
                />
              </div>
              <textarea
                value={text}
                onChange={(e) => { setText(e.target.value); markDirty(); }}
                className="min-h-40 w-full flex-1 resize-none bg-transparent px-4 py-3 text-[13.5px] leading-relaxed outline-none placeholder:text-faint"
                placeholder="Write your message…"
                aria-label="Message body"
              />
              {uploads.length > 0 && (
                <ul className="space-y-1.5 border-t border-border px-4 py-3">
                  {uploads.map((u) => (
                    <li key={u.key} className="flex items-center gap-2.5 rounded-[7px] border border-border bg-background px-2.5 py-1.5 text-xs">
                      <Icon name="paperclip" className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate font-medium">{u.name}</span>
                      {u.error && <span className="font-medium text-danger">{u.error}</span>}
                      {!u.error && !u.attachment && (
                        <span className="flex items-center gap-2 text-muted-foreground">
                          <span className="h-1 w-20 overflow-hidden rounded-full bg-muted">
                            <span className="block h-full rounded-full bg-primary transition-[width]" style={{ width: `${Math.round(u.progress * 100)}%` }} />
                          </span>
                          {Math.round(u.progress * 100)}%
                        </span>
                      )}
                      {u.attachment && <span className="text-muted-foreground">{formatBytes(u.attachment.size_bytes)}</span>}
                      <button
                        type="button"
                        className="grid h-5 w-5 shrink-0 place-items-center rounded text-muted-foreground hover:bg-muted hover:text-danger"
                        onClick={() => setUploads((prev) => prev.filter((x) => x.key !== u.key))}
                        aria-label={`Remove ${u.name}`}
                      >
                        <Icon name="x" className="h-3 w-3" />
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <footer className="flex shrink-0 items-center gap-1.5 border-t border-border bg-background/60 px-3 py-2.5">
              <Button onClick={send} disabled={sending} className="px-4">
                {sending ? <><Spinner className="h-3.5 w-3.5 border-white" /> Sending…</> : <>Send <Icon name="send" className="h-3.5 w-3.5" /></>}
              </Button>
              <label className="grid h-8 w-8 cursor-pointer place-items-center rounded-[7px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground" title="Attach files">
                <Icon name="paperclip" className="h-4 w-4" />
                <input type="file" multiple className="hidden" onChange={(e) => { void addFiles(e.target.files); e.target.value = ""; }} />
              </label>
              <Button
                variant="ghost"
                size="sm"
                onClick={async () => {
                  const ok = await saveDraft();
                  toast(ok ? "success" : "error", ok ? "Draft saved" : "Could not save draft");
                }}
              >
                Save draft
              </Button>
              <span className="flex-1" />
              <IconButton
                label="Discard"
                icon="trash"
                onClick={() => {
                  if (hasContent || draftId) setConfirmDiscard(true);
                  else onClose();
                }}
              />
            </footer>
          </>
        )}
      </div>

      <ConfirmDialog
        open={confirmDiscard}
        title="Discard this message?"
        body={draftId ? "The message and its saved draft will be deleted." : "The message you are writing will be lost."}
        confirmLabel="Discard"
        danger
        onCancel={() => setConfirmDiscard(false)}
        onConfirm={() => void discard()}
      />
    </>
  );
}
