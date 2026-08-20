"use client";

import { useEffect, useState } from "react";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

type HealthResponse = {
  status: string;
  checks: Record<string, string>;
};

type ApiState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ok"; live: boolean; ready: HealthResponse };

export default function Home() {
  const [state, setState] = useState<ApiState>({ kind: "loading" });
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;

    async function check() {
      try {
        const liveRes = await fetch(`${API_URL}/health/live`, { cache: "no-store" });
        const readyRes = await fetch(`${API_URL}/health/ready`, { cache: "no-store" });
        const ready = (await readyRes.json()) as HealthResponse;
        if (!cancelled) {
          setState({ kind: "ok", live: liveRes.ok, ready });
        }
      } catch (err) {
        if (!cancelled) {
          setState({
            kind: "error",
            message: err instanceof Error ? err.message : "Unknown error",
          });
        }
      }
    }

    void check();
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const refresh = () => {
    setState({ kind: "loading" });
    setReloadKey((k) => k + 1);
  };

  return (
    <main className="mx-auto flex min-h-screen max-w-2xl flex-col justify-center gap-8 p-8 font-sans">
      <header>
        <h1 className="text-3xl font-semibold tracking-tight">Mail Platform</h1>
        <p className="mt-1 text-sm text-neutral-500">
          Phase 1 · Milestone 1 — foundation status
        </p>
      </header>

      <section className="rounded-xl border border-neutral-200 p-6 dark:border-neutral-800">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-medium">Backend API</h2>
          <button
            onClick={refresh}
            className="rounded-md border border-neutral-300 px-3 py-1 text-sm hover:bg-neutral-100 dark:border-neutral-700 dark:hover:bg-neutral-900"
          >
            Refresh
          </button>
        </div>
        <p className="mt-1 break-all text-xs text-neutral-500">{API_URL}</p>

        <div className="mt-4">
          {state.kind === "loading" && (
            <p className="text-sm text-neutral-500">Checking…</p>
          )}

          {state.kind === "error" && (
            <p className="text-sm text-red-600 dark:text-red-400">
              Cannot reach backend: {state.message}
            </p>
          )}

          {state.kind === "ok" && (
            <ul className="space-y-2 text-sm">
              <StatusRow name="api (live)" ok={state.live} detail="" />
              {Object.entries(state.ready.checks).map(([name, value]) => (
                <StatusRow
                  key={name}
                  name={name}
                  ok={value === "ok"}
                  detail={value === "ok" ? "" : value}
                />
              ))}
            </ul>
          )}
        </div>
      </section>
    </main>
  );
}

function StatusRow({ name, ok, detail }: { name: string; ok: boolean; detail: string }) {
  return (
    <li className="flex items-start gap-2">
      <span
        className={`mt-1 inline-block h-2.5 w-2.5 shrink-0 rounded-full ${
          ok ? "bg-emerald-500" : "bg-red-500"
        }`}
      />
      <span className="font-mono">{name}</span>
      {detail && <span className="text-xs text-neutral-500">{detail}</span>}
    </li>
  );
}
