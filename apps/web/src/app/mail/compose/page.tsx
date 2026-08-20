"use client";

import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import {
  api,
  ApiError,
  formatBytes,
  uploadAttachment,
  type Attachment,
  type MessageDetail,
} from "@/lib/api";
import { Button, Input, PageLoader, Textarea, useToast } from "@/components/ui";

type PendingUpload = {
  key: string;
  name: string;
  progress: number;
  attachment?: Attachment;
  error?: string;
};

function parseAddresses(s: string): string[] {
  return s
    .split(/[,;\s]+/)
    .map((x) => x.trim())
    .filter(Boolean);
}

function ComposeForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const qc = useQueryClient();
  const toast = useToast();

  const draftParam = searchParams.get("draft");
  const forwardParam = searchParams.get("forward");

  const [to, setTo] = useState("");
  const [cc, setCc] = useState("");
  const [bcc, setBcc] = useState("");
  const [showCc, setShowCc] = useState(false);
  const [subject, setSubject] = useState("");
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [uploads, setUploads] = useState<PendingUpload[]>([]);
  const [loaded, setLoaded] = useState(!draftParam && !forwardParam);
  const [draftId, setDraftId] = useState<string | null>(draftParam);
  const [savedAt, setSavedAt] = useState<string>("");
  const dirty = useRef(false);

  // Load an existing draft or the message being forwarded.
  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        if (draftParam) {
          const d = await api.get<MessageDetail>(`/api/v1/mail/messages/${draftParam}`);
          if (cancelled) return;
          setTo(d.recipients.filter((r) => r.kind === "to").map((r) => r.address).join(", "));
          setCc(d.recipients.filter((r) => r.kind === "cc").map((r) => r.address).join(", "));
          setBcc(d.recipients.filter((r) => r.kind === "bcc").map((r) => r.address).join(", "));
          setShowCc(d.recipients.some((r) => r.kind !== "to"));
          setSubject(d.subject);
          setText(d.body_text);
        } else if (forwardParam) {
          const d = await api.get<MessageDetail>(`/api/v1/mail/messages/${forwardParam}`);
          if (cancelled) return;
          setSubject(d.subject.startsWith("Fwd:") ? d.subject : `Fwd: ${d.subject}`);
          setText(
            `\n\n---------- Forwarded message ----------\nFrom: ${d.from}\nDate: ${d.date}\nSubject: ${d.subject}\n\n${d.body_text}`,
          );
        }
      } catch {
        toast("error", "Could not load message");
      } finally {
        if (!cancelled) setLoaded(true);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [draftParam, forwardParam, toast]);

  const saveDraft = useCallback(async () => {
    const body = {
      to: parseAddresses(to),
      cc: parseAddresses(cc),
      bcc: parseAddresses(bcc),
      subject,
      text,
    };
    try {
      if (draftId) {
        await api.put(`/api/v1/mail/drafts/${draftId}`, body);
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

  // Autosave every 5s while there are unsaved changes.
  useEffect(() => {
    const t = setInterval(() => {
      if (dirty.current && (to || subject || text)) void saveDraft();
    }, 5000);
    return () => clearInterval(t);
  }, [saveDraft, to, subject, text]);

  function markDirty() {
    dirty.current = true;
  }

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
      if (draftId && attachmentIds.length === 0) {
        // Persist latest content, then promote the draft.
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
        });
        if (draftId) {
          await api.delete(`/api/v1/mail/messages/${draftId}`).catch(() => {});
        }
      }
      qc.invalidateQueries({ queryKey: ["mail"] });
      toast("success", "Message sent");
      router.push("/mail/sent");
    } catch (err) {
      toast(
        "error",
        err instanceof ApiError && err.code === "INVALID_MESSAGE"
          ? err.message
          : "Could not send. Draft is preserved.",
      );
      // Keep content; try to snapshot it as a draft so nothing is lost.
      if (!draftId) void saveDraft();
    } finally {
      setSending(false);
    }
  }

  async function discard() {
    if (draftId) {
      await api.delete(`/api/v1/mail/messages/${draftId}`).catch(() => {});
      qc.invalidateQueries({ queryKey: ["mail"] });
    }
    router.back();
  }

  if (!loaded) return <PageLoader />;

  return (
    <div className="mx-auto max-w-3xl p-4">
      <div className="rounded-2xl border border-neutral-200 bg-white shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
        <div className="flex items-center justify-between border-b border-neutral-100 px-5 py-3 dark:border-neutral-800">
          <h1 className="text-sm font-semibold">New message</h1>
          {savedAt && <span className="text-xs text-neutral-400">Draft saved {savedAt}</span>}
        </div>
        <div className="space-y-3 p-5">
          <div className="flex items-end gap-2">
            <div className="flex-1">
              <Input
                label="To"
                placeholder="user2@company.test, user3@company.test"
                value={to}
                onChange={(e) => {
                  setTo(e.target.value);
                  markDirty();
                }}
                autoFocus
              />
            </div>
            {!showCc && (
              <Button variant="ghost" onClick={() => setShowCc(true)}>
                Cc/Bcc
              </Button>
            )}
          </div>
          {showCc && (
            <>
              <Input label="Cc" value={cc} onChange={(e) => { setCc(e.target.value); markDirty(); }} />
              <Input label="Bcc" value={bcc} onChange={(e) => { setBcc(e.target.value); markDirty(); }} />
            </>
          )}
          <Input
            label="Subject"
            value={subject}
            onChange={(e) => {
              setSubject(e.target.value);
              markDirty();
            }}
          />
          <Textarea
            rows={12}
            placeholder="Write your message…"
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              markDirty();
            }}
          />
          {uploads.length > 0 && (
            <ul className="space-y-1">
              {uploads.map((u) => (
                <li
                  key={u.key}
                  className="flex items-center gap-2 rounded-lg border border-neutral-200 px-3 py-1.5 text-xs dark:border-neutral-700"
                >
                  <span aria-hidden>📎</span>
                  <span className="truncate">{u.name}</span>
                  {u.error && <span className="text-red-600">{u.error}</span>}
                  {!u.error && !u.attachment && (
                    <span className="ml-auto flex items-center gap-2 text-neutral-400">
                      <span className="h-1.5 w-24 overflow-hidden rounded-full bg-neutral-200 dark:bg-neutral-700">
                        <span
                          className="block h-full bg-indigo-500 transition-all"
                          style={{ width: `${Math.round(u.progress * 100)}%` }}
                        />
                      </span>
                      {Math.round(u.progress * 100)}%
                    </span>
                  )}
                  {u.attachment && (
                    <span className="ml-auto text-neutral-400">
                      {formatBytes(u.attachment.size_bytes)}
                    </span>
                  )}
                  <button
                    className="text-neutral-400 hover:text-red-600"
                    onClick={() => setUploads((prev) => prev.filter((x) => x.key !== u.key))}
                    aria-label={`Remove ${u.name}`}
                  >
                    ✕
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="flex items-center gap-2 border-t border-neutral-100 px-5 py-3 dark:border-neutral-800">
          <Button onClick={send} disabled={sending}>
            {sending ? "Sending…" : "Send"}
          </Button>
          <label className="inline-flex cursor-pointer items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium text-neutral-600 hover:bg-neutral-100 dark:text-neutral-300 dark:hover:bg-neutral-800">
            📎 Attach
            <input
              type="file"
              multiple
              className="hidden"
              onChange={(e) => {
                void addFiles(e.target.files);
                e.target.value = "";
              }}
            />
          </label>
          <Button
            variant="secondary"
            onClick={async () => {
              const ok = await saveDraft();
              toast(ok ? "success" : "error", ok ? "Draft saved" : "Could not save draft");
            }}
          >
            Save draft
          </Button>
          <Button variant="ghost" onClick={discard} className="ml-auto">
            Discard
          </Button>
        </div>
      </div>
    </div>
  );
}

export default function ComposePage() {
  return (
    <Suspense fallback={<PageLoader />}>
      <ComposeForm />
    </Suspense>
  );
}
