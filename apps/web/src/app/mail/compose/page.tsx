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

  if (!loaded) return <PageLoader label="Loading composer" />;

  return (
    <div className="mx-auto max-w-5xl p-4 lg:p-6">
      <div className="qazera-panel overflow-hidden">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b-2 border-black bg-[#f2f2f2] px-5 py-4 lg:px-6">
          <div>
            <p className="qazera-label text-accent">Compose</p>
            <h1 className="mt-2 text-2xl font-black uppercase tracking-tight">New message</h1>
          </div>
          {savedAt && <span className="qazera-label">Draft saved {savedAt}</span>}
        </div>

        <div className="space-y-4 p-5 lg:p-6">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-end">
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
              <Button variant="secondary" onClick={() => setShowCc(true)}>
                Cc / Bcc
              </Button>
            )}
          </div>

          {showCc && (
            <div className="grid gap-4 lg:grid-cols-2">
              <Input label="Cc" value={cc} onChange={(e) => { setCc(e.target.value); markDirty(); }} />
              <Input label="Bcc" value={bcc} onChange={(e) => { setBcc(e.target.value); markDirty(); }} />
            </div>
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
            rows={14}
            label="Message"
            placeholder="Write your message..."
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              markDirty();
            }}
          />

          {uploads.length > 0 && (
            <ul className="space-y-3">
              {uploads.map((u) => (
                <li key={u.key} className="qazera-panel-muted flex flex-wrap items-center gap-3 p-4">
                  <span className="qazera-label">{u.name}</span>
                  {u.error && <span className="text-xs font-bold uppercase tracking-[0.16em] text-accent">{u.error}</span>}
                  {!u.error && !u.attachment && (
                    <span className="ml-auto flex items-center gap-3 text-xs font-bold uppercase tracking-[0.16em]">
                      <span className="h-2 w-28 overflow-hidden border border-black bg-white">
                        <span className="block h-full bg-accent" style={{ width: `${Math.round(u.progress * 100)}%` }} />
                      </span>
                      {Math.round(u.progress * 100)}%
                    </span>
                  )}
                  {u.attachment && (
                    <span className="ml-auto text-xs font-bold uppercase tracking-[0.16em]">
                      {formatBytes(u.attachment.size_bytes)}
                    </span>
                  )}
                  <button
                    className="ml-auto text-xs font-black uppercase tracking-[0.18em] text-black hover:text-accent"
                    onClick={() => setUploads((prev) => prev.filter((x) => x.key !== u.key))}
                    aria-label={`Remove ${u.name}`}
                  >
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2 border-t-2 border-black bg-[#f2f2f2] px-5 py-4 lg:px-6">
          <Button onClick={send} disabled={sending}>
            {sending ? "Sending..." : "Send"}
          </Button>
          <label className="inline-flex h-11 cursor-pointer items-center justify-center border-2 border-black bg-white px-4 text-xs font-bold uppercase tracking-[0.18em] transition-colors hover:bg-black hover:text-white">
            Attach
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
          <Button variant="secondary" onClick={async () => {
            const ok = await saveDraft();
            toast(ok ? "success" : "error", ok ? "Draft saved" : "Could not save draft");
          }}>
            Save draft
          </Button>
          <Button variant="secondary" onClick={discard} className="ml-auto">
            Discard
          </Button>
        </div>
      </div>
    </div>
  );
}

export default function ComposePage() {
  return (
    <Suspense fallback={<PageLoader label="Loading composer" />}>
      <ComposeForm />
    </Suspense>
  );
}
