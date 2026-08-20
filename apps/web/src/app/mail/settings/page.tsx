"use client";

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { useMe } from "@/components/providers";
import { Button, Input, useToast } from "@/components/ui";

export default function SettingsPage() {
  const me = useMe();
  const toast = useToast();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");

  const change = useMutation({
    mutationFn: () =>
      api.post("/api/v1/me/password", { current_password: current, new_password: next }),
    onSuccess: () => {
      setCurrent("");
      setNext("");
      setConfirm("");
      toast("success", "Password changed. Other sessions were signed out.");
    },
    onError: (e) =>
      toast("error", e instanceof ApiError ? e.message : "Could not change password"),
  });

  return (
    <div className="mx-auto max-w-xl p-4">
      <h1 className="mb-4 text-lg font-semibold">Settings</h1>

      <section className="mb-4 rounded-2xl border border-neutral-200 bg-white p-5 shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
        <h2 className="mb-3 text-sm font-semibold">Account</h2>
        <dl className="space-y-1 text-sm">
          <div className="flex gap-2">
            <dt className="w-24 text-neutral-400">Name</dt>
            <dd>{me.data?.display_name || "—"}</dd>
          </div>
          <div className="flex gap-2">
            <dt className="w-24 text-neutral-400">Login</dt>
            <dd>{me.data?.email}</dd>
          </div>
          <div className="flex gap-2">
            <dt className="w-24 text-neutral-400">Roles</dt>
            <dd className="text-neutral-500">{me.data?.roles.map((r) => r.role).join(", ")}</dd>
          </div>
        </dl>
      </section>

      <section className="rounded-2xl border border-neutral-200 bg-white p-5 shadow-sm dark:border-neutral-800 dark:bg-neutral-900">
        <h2 className="mb-3 text-sm font-semibold">Change password</h2>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (next !== confirm) {
              toast("error", "New passwords do not match");
              return;
            }
            change.mutate();
          }}
        >
          <Input
            label="Current password"
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            required
          />
          <Input
            label="New password (min 10 characters)"
            type="password"
            autoComplete="new-password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            required
            minLength={10}
          />
          <Input
            label="Repeat new password"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            required
          />
          <Button type="submit" disabled={change.isPending || !current || next.length < 10}>
            {change.isPending ? "Saving…" : "Change password"}
          </Button>
        </form>
      </section>
    </div>
  );
}
