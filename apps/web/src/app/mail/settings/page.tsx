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
    onError: (e) => toast("error", e instanceof ApiError ? e.message : "Could not change password"),
  });

  return (
    <div className="mx-auto max-w-4xl p-4 lg:p-6">
      <div className="mb-4">
        <p className="qazera-label text-accent">Settings</p>
        <h1 className="mt-2 text-3xl font-black uppercase tracking-tight">Account</h1>
      </div>

      <section className="qazera-panel mb-6 p-5 lg:p-6">
        <p className="qazera-label text-accent">Identity</p>
        <dl className="mt-4 grid gap-4 text-sm lg:grid-cols-3">
          <div className="qazera-panel-muted p-4">
            <dt className="qazera-label">Name</dt>
            <dd className="mt-2 text-sm">{me.data?.display_name || "—"}</dd>
          </div>
          <div className="qazera-panel-muted p-4">
            <dt className="qazera-label">Login</dt>
            <dd className="mt-2 text-sm break-all">{me.data?.email}</dd>
          </div>
          <div className="qazera-panel-muted p-4">
            <dt className="qazera-label">Roles</dt>
            <dd className="mt-2 text-sm">{me.data?.roles.map((r) => r.role).join(", ")}</dd>
          </div>
        </dl>
      </section>

      <section className="qazera-panel p-5 lg:p-6">
        <p className="qazera-label text-accent">Change password</p>
        <form
          className="mt-4 space-y-4"
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
            {change.isPending ? "Saving..." : "Change password"}
          </Button>
        </form>
      </section>
    </div>
  );
}
