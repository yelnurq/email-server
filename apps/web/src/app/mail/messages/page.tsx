"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  API_URL,
  api,
  ApiError,
  uploadAttachment,
  type Attachment,
  type DirectoryUser,
} from "@/lib/api";
import { Button, Input, PageLoader, useToast } from "@/components/ui";
import { useMe } from "@/components/providers";

type Conversation = {
  id: string;
  kind: "direct" | "group";
  title: string;
  last_message: string;
  updated_at: string;
  unread: number;
};
type ChatMessage = {
  id: string;
  conversation_id: string;
  sender_user_id: string;
  sender_name: string;
  reply_to_id?: string;
  body: string;
  edited_at?: string;
  deleted_at?: string;
  created_at: string;
  attachments?: Attachment[];
};

export default function MessagesPage() {
  const me = useMe(),
    qc = useQueryClient(),
    toast = useToast();
  const [active, setActive] = useState(""),
    [body, setBody] = useState(""),
    [search, setSearch] = useState(""),
    [newChat, setNewChat] = useState(false),
    [selectedUsers, setSelectedUsers] = useState<string[]>([]),
    [title, setTitle] = useState(""),
    [replyTo, setReplyTo] = useState<ChatMessage | null>(null);
  const [attachments, setAttachments] = useState<Attachment[]>([]),
    [uploading, setUploading] = useState(false);
  const conversations = useQuery({
    queryKey: ["chat", "conversations"],
    queryFn: () =>
      api.get<{ conversations: Conversation[] }>("/api/v1/chat/conversations"),
  });
  const resolvedActive =
    active || conversations.data?.conversations[0]?.id || "";
  const messages = useQuery({
    queryKey: ["chat", "messages", resolvedActive],
    queryFn: () =>
      api.get<{ messages: ChatMessage[] }>(
        `/api/v1/chat/conversations/${resolvedActive}/messages`,
      ),
    enabled: Boolean(resolvedActive),
  });
  const directory = useQuery({
    queryKey: ["directory", "chat", search],
    queryFn: () =>
      api.get<{ users: DirectoryUser[] }>(
        `/api/v1/directory/users?limit=50&q=${encodeURIComponent(search)}`,
      ),
    enabled: newChat,
  });
  const current = conversations.data?.conversations.find(
    (c) => c.id === resolvedActive,
  );
  useEffect(() => {
    if (!resolvedActive) return;
    api
      .post(`/api/v1/chat/conversations/${resolvedActive}/read`)
      .then(() => qc.invalidateQueries({ queryKey: ["chat", "conversations"] }))
      .catch(() => {});
  }, [resolvedActive, qc]);
  useEffect(() => {
    if (!me.data) return;
    const path = `${API_URL}/api/v1/realtime`,
      base = API_URL.startsWith("http")
        ? new URL(path)
        : new URL(path, window.location.href);
    base.protocol = base.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(base);
    ws.onmessage = (event) => {
      try {
        const p = JSON.parse(event.data) as {
          type: string;
          message?: ChatMessage;
          conversation_id?: string;
          message_id?: string;
          body?: string;
        };
        if (p.type === "message.created" && p.message)
          qc.setQueryData<{ messages: ChatMessage[] }>(
            ["chat", "messages", p.message.conversation_id],
            (old) =>
              old
                ? {
                    messages: old.messages.some((m) => m.id === p.message!.id)
                      ? old.messages
                      : [...old.messages, p.message!],
                  }
                : old,
          );
        else if (p.type === "message.updated")
          qc.setQueryData<{ messages: ChatMessage[] }>(
            ["chat", "messages", p.conversation_id],
            (old) =>
              old
                ? {
                    messages: old.messages.map((m) =>
                      m.id === p.message_id
                        ? {
                            ...m,
                            body: p.body ?? m.body,
                            edited_at: new Date().toISOString(),
                          }
                        : m,
                    ),
                  }
                : old,
          );
        else if (p.type === "message.deleted")
          qc.setQueryData<{ messages: ChatMessage[] }>(
            ["chat", "messages", p.conversation_id],
            (old) =>
              old
                ? {
                    messages: old.messages.map((m) =>
                      m.id === p.message_id
                        ? {
                            ...m,
                            body: "",
                            deleted_at: new Date().toISOString(),
                          }
                        : m,
                    ),
                  }
                : old,
          );
        qc.invalidateQueries({ queryKey: ["chat", "conversations"] });
      } catch {}
    };
    return () => ws.close();
  }, [me.data, qc]);
  const send = useMutation({
    mutationFn: () =>
      api.post<ChatMessage>(
        `/api/v1/chat/conversations/${resolvedActive}/messages`,
        {
          body,
          reply_to_id: replyTo?.id,
          attachment_ids: attachments.map((item) => item.id),
        },
      ),
    onSuccess: () => {
      setBody("");
      setAttachments([]);
      setReplyTo(null);
      qc.invalidateQueries({ queryKey: ["chat"] });
    },
    onError: (e) =>
      toast("error", e instanceof ApiError ? e.message : "Could not send"),
  });
  const create = useMutation({
    mutationFn: () =>
      api.post<{ id: string }>("/api/v1/chat/conversations", {
        kind: selectedUsers.length === 1 ? "direct" : "group",
        title,
        user_ids: selectedUsers,
      }),
    onSuccess: async (result) => {
      setNewChat(false);
      setSelectedUsers([]);
      setTitle("");
      await qc.invalidateQueries({ queryKey: ["chat", "conversations"] });
      setActive(result.id);
    },
    onError: (e) =>
      toast(
        "error",
        e instanceof ApiError ? e.message : "Could not create chat",
      ),
  });
  const filtered = (conversations.data?.conversations ?? []).filter((c) =>
    `${c.title} ${c.last_message}`.toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <div className="grid h-full grid-cols-1 overflow-hidden lg:grid-cols-[300px_minmax(0,1fr)_240px]">
      <aside className="border-r border-border bg-surface">
        <div className="border-b border-border p-3">
          <div className="flex items-center justify-between">
            <h1 className="text-lg font-semibold">Messages</h1>
            <Button className="!h-8" onClick={() => setNewChat(true)}>
              New
            </Button>
          </div>
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search conversations"
            className="mt-3 h-9 w-full rounded-lg border border-border bg-background px-3 text-sm outline-none"
          />
        </div>
        {conversations.isLoading && (
          <PageLoader label="Loading conversations" />
        )}
        <div className="p-2">
          {filtered.map((c) => (
            <button
              key={c.id}
              onClick={() => setActive(c.id)}
              className={`mb-1 flex w-full items-center gap-3 rounded-lg p-3 text-left ${resolvedActive === c.id ? "bg-primary/10" : "hover:bg-muted"}`}
            >
              <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-primary/10 text-xs font-bold text-primary">
                {c.title.slice(0, 2).toUpperCase()}
              </span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-semibold">
                  {c.title}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {c.last_message || "No messages yet"}
                </span>
              </span>
              {c.unread > 0 && (
                <span className="rounded-full bg-primary px-2 py-0.5 text-[10px] font-bold text-white">
                  {c.unread}
                </span>
              )}
            </button>
          ))}
        </div>
      </aside>
      <main className="flex min-h-0 flex-col bg-background">
        {!current ? (
          <div className="grid flex-1 place-items-center text-sm text-muted-foreground">
            Select a conversation
          </div>
        ) : (
          <>
            <header className="glass-bar border-b border-border px-5 py-3">
              <h2 className="font-semibold">{current.title}</h2>
              <p className="text-xs text-muted-foreground">
                {current.kind === "group"
                  ? "Group conversation"
                  : "Direct conversation"}
              </p>
            </header>
            <div className="flex-1 space-y-3 overflow-y-auto p-4 lg:p-6">
              {messages.data?.messages.map((message) => {
                const mine = message.sender_user_id === me.data?.id;
                return (
                  <div
                    key={message.id}
                    className={`group flex ${mine ? "justify-end" : "justify-start"}`}
                  >
                    <div
                      className={`max-w-[75%] rounded-xl border px-3.5 py-2.5 ${mine ? "border-primary/20 bg-primary/10" : "border-border bg-surface-elevated"}`}
                    >
                      {!mine && (
                        <p className="mb-1 text-xs font-semibold text-primary">
                          {message.sender_name}
                        </p>
                      )}
                      {message.reply_to_id && (
                        <p className="mb-2 border-l-2 border-primary pl-2 text-xs text-muted-foreground">
                          Reply
                        </p>
                      )}
                      <p
                        className={`whitespace-pre-wrap text-sm ${message.deleted_at ? "italic text-muted-foreground" : ""}`}
                      >
                        {message.deleted_at ? "Message deleted" : message.body}
                      </p>
                      {message.attachments?.map((attachment) => (
                        <a key={attachment.id} href={`${API_URL}/api/v1/mail/attachments/${attachment.id}`} className="mt-2 block rounded-lg border border-border bg-background/60 px-2.5 py-2 text-xs font-medium text-primary">Attachment: {attachment.filename}</a>
                      ))}
                      <div className="mt-1 flex items-center justify-end gap-2 text-[10px] text-muted-foreground">
                        <button onClick={() => setReplyTo(message)}>
                          Reply
                        </button>
                        {mine && !message.deleted_at && (
                          <>
                            <button
                              onClick={() => {
                                const next = prompt(
                                  "Edit message",
                                  message.body,
                                );
                                if (next)
                                  void api.patch(
                                    `/api/v1/chat/messages/${message.id}`,
                                    { body: next },
                                  );
                              }}
                            >
                              Edit
                            </button>
                            <button
                              onClick={() =>
                                void api.delete(
                                  `/api/v1/chat/messages/${message.id}`,
                                )
                              }
                            >
                              Delete
                            </button>
                          </>
                        )}
                        <button
                          onClick={() =>
                            void api.post(
                              `/api/v1/chat/messages/${message.id}/reactions`,
                              { emoji: "👍" },
                            )
                          }
                        >
                          👍
                        </button>
                        <button onClick={() => void api.post("/api/v1/tasks", { title: message.body.slice(0, 80), source_type: "chat", source_id: message.id }).then(() => toast("success", "Added to My List"))}>+ List</button>
                        <span>
                          {new Date(message.created_at).toLocaleTimeString([], {
                            hour: "2-digit",
                            minute: "2-digit",
                          })}
                          {message.edited_at ? " · edited" : ""}
                        </span>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
            {replyTo && (
              <div className="mx-4 flex items-center justify-between rounded-t-lg border border-b-0 border-border bg-muted px-3 py-2 text-xs">
                <span>
                  Reply to {replyTo.sender_name}: {replyTo.body.slice(0, 80)}
                </span>
                <button onClick={() => setReplyTo(null)}>×</button>
              </div>
            )}
            <form
              className="flex flex-wrap gap-2 border-t border-border bg-surface p-3"
              onSubmit={(e) => {
                e.preventDefault();
                if (body.trim()) send.mutate();
              }}
            >
              {attachments.length > 0 && <div className="flex w-full flex-wrap gap-2">{attachments.map(item => <span key={item.id} className="rounded-full border border-border bg-muted px-2.5 py-1 text-xs">{item.filename} <button type="button" onClick={() => setAttachments(current => current.filter(file => file.id !== item.id))}>×</button></span>)}</div>}
              <input
                value={body}
                onChange={(e) => setBody(e.target.value)}
                placeholder="Write a message... Use @name to mention"
                className="h-10 flex-1 rounded-lg border border-border-strong bg-background px-3 text-sm outline-none focus:border-primary"
              />
              <label className="grid h-10 cursor-pointer place-items-center rounded-lg border border-border px-3 text-sm hover:bg-muted">{uploading ? "…" : "Attach"}<input type="file" className="hidden" disabled={uploading} onChange={async event => { const file=event.target.files?.[0]; if(!file)return; setUploading(true); try { const uploaded=await uploadAttachment(file,()=>{}); setAttachments(current=>[...current,uploaded]); } catch { toast("error","Could not upload file"); } finally { setUploading(false); event.target.value=""; } }}/></label>
              <Button type="submit" disabled={!body.trim() || send.isPending || uploading}>
                Send
              </Button>
            </form>
          </>
        )}
      </main>
      <aside className="hidden border-l border-border bg-surface p-4 lg:block">
        <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Chat details
        </p>
        {current && (
          <>
            <div className="mt-4 grid h-14 w-14 place-items-center rounded-xl bg-primary/10 text-lg font-bold text-primary">
              {current.title.slice(0, 2).toUpperCase()}
            </div>
            <h2 className="mt-3 font-semibold">{current.title}</h2>
            <p className="mt-1 text-xs text-muted-foreground">
              Realtime organization conversation
            </p>
          </>
        )}
      </aside>
      {newChat && (
        <div className="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4 backdrop-blur-sm">
          <div className="glass-panel w-full max-w-lg rounded-xl p-5">
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold">New conversation</h2>
              <button onClick={() => setNewChat(false)}>×</button>
            </div>
            {selectedUsers.length > 1 && (
              <Input
                label="Group name"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                className="mt-4"
              />
            )}
            <Input
              label="Find employees"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="mt-4"
            />
            <div className="mt-3 max-h-72 overflow-y-auto">
              {directory.data?.users
                .filter((u) => u.id !== me.data?.id)
                .map((user) => (
                  <label
                    key={user.id}
                    className="flex cursor-pointer items-center gap-3 rounded-lg p-2.5 hover:bg-muted"
                  >
                    <input
                      type="checkbox"
                      checked={selectedUsers.includes(user.id)}
                      onChange={() =>
                        setSelectedUsers((ids) =>
                          ids.includes(user.id)
                            ? ids.filter((id) => id !== user.id)
                            : [...ids, user.id],
                        )
                      }
                    />
                    <span>
                      <span className="block text-sm font-medium">
                        {user.display_name || user.email}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {user.email}
                        {user.department_name
                          ? ` · ${user.department_name}`
                          : ""}
                      </span>
                    </span>
                  </label>
                ))}
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setNewChat(false)}>
                Cancel
              </Button>
              <Button
                onClick={() => create.mutate()}
                disabled={
                  selectedUsers.length === 0 ||
                  (selectedUsers.length > 1 && !title.trim())
                }
              >
                Create
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
