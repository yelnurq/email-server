"use client";

// Tasks & Reminders: one prioritized list grouped by urgency — Overdue,
// Today, Upcoming, No date — with completed items collapsed at the end.
// Tasks can be linked to emails ("Remind me" in the reading pane).

import { useMemo, useState } from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type DirectoryUser } from "@/lib/api";
import { Badge, Button, Drawer, EmptyState, ErrorState, IconButton, Input, ListSkeleton, Menu, Textarea, cx, useToast } from "@/components/ui";
import { Icon } from "@/components/icons";
import { useMe } from "@/components/providers";

type Task = {
  id: string;
  owner_user_id: string;
  assigned_by_user_id?: string;
  assigned_by_name?: string;
  title: string;
  description: string;
  due_at?: string;
  priority: string;
  status: string;
  source_type: string;
  source_id?: string;
  reminder_at?: string;
  created_at: string;
};

type GroupKey = "overdue" | "today" | "upcoming" | "nodate" | "done";

function groupOf(task: Task): GroupKey {
  if (task.status === "done") return "done";
  const when = task.due_at || task.reminder_at;
  if (!when) return "nodate";
  const d = new Date(when);
  const now = new Date();
  const endOfDay = new Date(now); endOfDay.setHours(23, 59, 59, 999);
  if (d.getTime() < now.getTime()) return "overdue";
  if (d.getTime() <= endOfDay.getTime()) return "today";
  return "upcoming";
}

const GROUPS: Array<{ key: GroupKey; label: string; tone?: "danger" | "accent" }> = [
  { key: "overdue", label: "Overdue", tone: "danger" },
  { key: "today", label: "Today", tone: "accent" },
  { key: "upcoming", label: "Upcoming" },
  { key: "nodate", label: "No date" },
  { key: "done", label: "Completed" },
];

const PRIORITY_TONE: Record<string, "danger" | "warning" | "neutral"> = {
  urgent: "danger",
  high: "warning",
  normal: "neutral",
  low: "neutral",
};

function fmt(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export default function TasksPage() {
  const me = useMe();
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [showDone, setShowDone] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [dueAt, setDueAt] = useState("");
  const [reminderAt, setReminderAt] = useState("");
  const [priority, setPriority] = useState("normal");
  const [owner, setOwner] = useState("");

  const tasks = useQuery({ queryKey: ["tasks"], queryFn: () => api.get<{ tasks: Task[] }>("/api/v1/tasks") });
  const canAssign = me.data?.permissions.includes("tasks.assign.department");
  const directory = useQuery({
    queryKey: ["directory", "tasks"],
    queryFn: () => api.get<{ users: DirectoryUser[] }>("/api/v1/directory/users?limit=1000"),
    enabled: !!canAssign && open,
  });

  const create = useMutation({
    mutationFn: () =>
      api.post("/api/v1/tasks", {
        title,
        description,
        due_at: dueAt ? new Date(dueAt).toISOString() : "",
        reminder_at: reminderAt ? new Date(reminderAt).toISOString() : "",
        priority,
        owner_user_id: owner,
      }),
    onSuccess: () => {
      setOpen(false); setTitle(""); setDescription(""); setDueAt(""); setReminderAt(""); setPriority("normal"); setOwner("");
      qc.invalidateQueries({ queryKey: ["tasks"] });
      toast("success", "Reminder created");
    },
    onError: () => toast("error", "Could not create the task"),
  });

  async function patch(id: string, data: unknown, message?: string) {
    try {
      await api.patch(`/api/v1/tasks/${id}`, data);
      qc.invalidateQueries({ queryKey: ["tasks"] });
      if (message) toast("success", message);
    } catch {
      toast("error", "Action failed");
    }
  }

  async function remove(id: string) {
    try {
      await api.delete(`/api/v1/tasks/${id}`);
      qc.invalidateQueries({ queryKey: ["tasks"] });
      toast("success", "Deleted");
    } catch {
      toast("error", "Could not delete");
    }
  }

  const grouped = useMemo(() => {
    const all = tasks.data?.tasks ?? [];
    const map = new Map<GroupKey, Task[]>();
    for (const g of GROUPS) map.set(g.key, []);
    for (const t of all) map.get(groupOf(t))!.push(t);
    const order = (t: Task) => new Date(t.due_at || t.reminder_at || t.created_at).getTime();
    for (const [, list] of map) list.sort((a, b) => order(a) - order(b));
    return map;
  }, [tasks.data]);

  const openCount = (tasks.data?.tasks ?? []).filter((t) => t.status !== "done").length;
  const doneCount = grouped.get("done")!.length;

  function TaskRow({ task }: { task: Task }) {
    const done = task.status === "done";
    return (
      <li className="group flex items-start gap-3 px-4 py-2.5 transition-colors hover:bg-background">
        <button
          type="button"
          className={cx(
            "mt-0.5 grid h-4 w-4 shrink-0 place-items-center rounded-full border transition-colors",
            done ? "border-success bg-success text-white" : "border-border-strong hover:border-graphite",
          )}
          title={done ? "Mark as not done" : "Mark as done"}
          onClick={() => patch(task.id, { status: done ? "todo" : "done" }, done ? undefined : "Completed")}
        >
          {done && <Icon name="check" className="h-2.5 w-2.5" strokeWidth={3} />}
        </button>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
            <span className={cx("text-[13px] font-medium leading-5", done && "text-muted-foreground line-through")}>{task.title}</span>
            {task.priority !== "normal" && <Badge tone={PRIORITY_TONE[task.priority] ?? "neutral"}>{task.priority}</Badge>}
            {task.status === "in_progress" && !done && <Badge tone="accent">in progress</Badge>}
            {task.assigned_by_name && <Badge tone="neutral">from {task.assigned_by_name}</Badge>}
          </div>
          {task.description && <p className="mt-0.5 line-clamp-2 text-xs leading-4.5 text-muted-foreground">{task.description}</p>}
          <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-faint">
            {task.due_at && <span className="inline-flex items-center gap-1"><Icon name="calendar" className="h-3 w-3" /> Due {fmt(task.due_at)}</span>}
            {task.reminder_at && <span className="inline-flex items-center gap-1"><Icon name="bell" className="h-3 w-3" /> {fmt(task.reminder_at)}</span>}
            {task.source_id && (
              <Link className="inline-flex items-center gap-1 text-primary hover:underline" href={task.source_type === "email" ? `/mail/inbox?m=${task.source_id}` : "/mail/messages"}>
                <Icon name={task.source_type === "email" ? "mail" : "message-circle"} className="h-3 w-3" /> Open source
              </Link>
            )}
          </div>
        </div>
        <div className="opacity-0 transition-opacity group-hover:opacity-100">
          <Menu
            trigger={<IconButton size="sm" label="Task actions" icon="more-horizontal" />}
            items={[
              ...(!done && task.status !== "in_progress" ? [{ label: "Mark in progress", icon: "clock", onSelect: () => void patch(task.id, { status: "in_progress" }) }] : []),
              ...(task.status === "in_progress" ? [{ label: "Back to To do", icon: "corner-up-left", onSelect: () => void patch(task.id, { status: "todo" }) }] : []),
              { label: done ? "Mark as not done" : "Mark as done", icon: "check-circle", onSelect: () => void patch(task.id, { status: done ? "todo" : "done" }) },
              ...(task.owner_user_id === me.data?.id
                ? [{ type: "separator" as const }, { label: "Delete", icon: "trash", danger: true, onSelect: () => void remove(task.id) }]
                : []),
            ]}
          />
        </div>
      </li>
    );
  }

  return (
    <div className="h-full w-full overflow-y-auto">
      <div className="mx-auto max-w-3xl p-4 lg:p-6">
        <header className="page-header">
          <div>
            <h1 className="page-title">Tasks & Reminders</h1>
            <p className="page-description">
              {tasks.isSuccess ? `${openCount} open · ${doneCount} completed` : "Follow-ups, notes and reminders connected to your mail."}
            </p>
          </div>
          <Button onClick={() => setOpen(true)}><Icon name="plus" className="h-3.5 w-3.5" /> New reminder</Button>
        </header>

        {tasks.isLoading && <div className="rounded-[10px] border border-border bg-surface-elevated"><ListSkeleton rows={6} /></div>}
        {tasks.isError && <ErrorState message="Could not load tasks" onRetry={() => tasks.refetch()} />}
        {tasks.isSuccess && (tasks.data.tasks ?? []).length === 0 && (
          <div className="rounded-[10px] border border-border bg-surface-elevated">
            <EmptyState
              icon="check-circle"
              title="Nothing on your list"
              hint="Create a reminder here, or use “Remind me” on any email to bring it back at the right time."
              action={<Button size="sm" variant="secondary" onClick={() => setOpen(true)}>New reminder</Button>}
            />
          </div>
        )}

        {tasks.isSuccess && (tasks.data.tasks ?? []).length > 0 && (
          <div className="space-y-4">
            {GROUPS.filter((g) => g.key !== "done").map((g) => {
              const list = grouped.get(g.key)!;
              if (list.length === 0) return null;
              return (
                <section key={g.key} className="overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
                  <header className="flex items-center gap-2 border-b border-border bg-background/60 px-4 py-2">
                    <h2 className={cx("text-xs font-semibold uppercase tracking-[.05em]", g.tone === "danger" ? "text-danger" : g.tone === "accent" ? "text-primary" : "text-muted-foreground")}>
                      {g.label}
                    </h2>
                    <span className="text-[11px] text-faint">{list.length}</span>
                  </header>
                  <ul className="divide-y divide-border/70">
                    {list.map((task) => <TaskRow key={task.id} task={task} />)}
                  </ul>
                </section>
              );
            })}

            {doneCount > 0 && (
              <section className="overflow-hidden rounded-[10px] border border-border bg-surface-elevated">
                <button
                  type="button"
                  className="flex w-full items-center gap-2 px-4 py-2 text-left"
                  onClick={() => setShowDone((v) => !v)}
                >
                  <Icon name="chevron-down" className={cx("h-3 w-3 text-faint transition-transform", !showDone && "-rotate-90")} />
                  <h2 className="text-xs font-semibold uppercase tracking-[.05em] text-muted-foreground">Completed</h2>
                  <span className="text-[11px] text-faint">{doneCount}</span>
                </button>
                {showDone && (
                  <ul className="divide-y divide-border/70 border-t border-border">
                    {grouped.get("done")!.map((task) => <TaskRow key={task.id} task={task} />)}
                  </ul>
                )}
              </section>
            )}
          </div>
        )}
      </div>

      <Drawer open={open} onClose={() => setOpen(false)} title="New task or reminder" width="max-w-md">
        <form className="space-y-4 p-4" onSubmit={(e) => { e.preventDefault(); create.mutate(); }}>
          <Input label="Title" value={title} onChange={(e) => setTitle(e.target.value)} required autoFocus />
          <Textarea label="Description / note" rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />
          <div className="grid gap-3 sm:grid-cols-2">
            <Input label="Due date" type="datetime-local" value={dueAt} onChange={(e) => setDueAt(e.target.value)} />
            <Input label="Remind at" type="datetime-local" value={reminderAt} onChange={(e) => setReminderAt(e.target.value)} />
          </div>
          <label className="block">
            <span className="mb-1 block text-xs font-medium">Priority</span>
            <select value={priority} onChange={(e) => setPriority(e.target.value)} className="w-full">
              <option value="low">Low</option>
              <option value="normal">Normal</option>
              <option value="high">High</option>
              <option value="urgent">Urgent</option>
            </select>
          </label>
          {canAssign && (
            <label className="block">
              <span className="mb-1 block text-xs font-medium">Assign to</span>
              <select value={owner} onChange={(e) => setOwner(e.target.value)} className="w-full">
                <option value="">Myself</option>
                {directory.data?.users.filter((u) => u.id !== me.data?.id).map((u) => (
                  <option key={u.id} value={u.id}>{u.display_name || u.email}{u.department_name ? ` · ${u.department_name}` : ""}</option>
                ))}
              </select>
            </label>
          )}
          <div className="flex justify-end gap-2 border-t border-border pt-4">
            <Button type="button" variant="secondary" onClick={() => setOpen(false)}>Cancel</Button>
            <Button type="submit" disabled={!title.trim() || create.isPending}>{create.isPending ? "Creating…" : "Create"}</Button>
          </div>
        </form>
      </Drawer>
    </div>
  );
}
