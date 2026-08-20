"use client";

import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { z } from "zod";
import { API_URL, api, ApiError } from "@/lib/api";
import { LocaleSwitcher, useI18n } from "@/components/providers";
import { Button, Spinner, cx } from "@/components/ui";
import { Icon } from "@/components/icons";

const schema = z.object({ email: z.string().email("Enter a valid email address"), password: z.string().min(1, "Password is required") });
const QUICK_ACCOUNTS = [
  { kind: "administrator", email: process.env.NEXT_PUBLIC_DEMO_ADMIN_EMAIL ?? "admin@company.test", password: process.env.NEXT_PUBLIC_DEMO_ADMIN_PASSWORD ?? "admin-dev-password-1" },
  { kind: "mailUser", email: process.env.NEXT_PUBLIC_DEMO_USER_EMAIL ?? "user1@company.test", password: process.env.NEXT_PUBLIC_DEMO_USER_PASSWORD ?? "e2e-tenantb-pass1" },
] as const;

function Field({
  label, error, children,
}: { label: string; error?: string; children: React.ReactNode }) {
  return (
    <div>
      <span className="mb-1 block text-xs font-medium text-foreground">{label}</span>
      {children}
      {error && <span className="mt-1 block text-xs font-medium text-danger">{error}</span>}
    </div>
  );
}

export default function LoginPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { t } = useI18n();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<{ email?: string; password?: string }>({});
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);
  const [quickBusy, setQuickBusy] = useState<string | null>(null);

  // Real readiness signal from the platform, not a decorative badge.
  const health = useQuery({
    queryKey: ["health"],
    queryFn: async () => {
      const res = await fetch(`${API_URL}/health/ready`, { cache: "no-store" });
      return (await res.json()) as { status: string; checks: Record<string, string> };
    },
    retry: false,
    refetchInterval: 30_000,
  });
  const healthy = health.data?.status === "ok";

  async function authenticate(credentials: { email: string; password: string }, quick?: string) {
    setFormError(""); setBusy(!quick); setQuickBusy(quick ?? null);
    try {
      await api.post("/api/v1/auth/login", { email: credentials.email, password: credentials.password });
      await qc.invalidateQueries();
      router.replace("/mail/inbox");
    } catch (err) {
      setFormError(
        err instanceof ApiError && err.status === 401 ? t("invalidCredentials")
          : err instanceof ApiError && err.status === 429 ? t("tooManyAttempts")
          : err instanceof ApiError && err.code === "USER_DISABLED" ? "This account is disabled. Contact your administrator."
          : err instanceof ApiError ? `${err.message} (${err.code})`
          : t("serverUnavailable"),
      );
    } finally { setBusy(false); setQuickBusy(null); }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const parsed = schema.safeParse({ email, password });
    if (!parsed.success) { const next: typeof errors = {}; for (const issue of parsed.error.issues) next[issue.path[0] as "email" | "password"] = issue.message; setErrors(next); return; }
    setErrors({}); await authenticate(parsed.data);
  }

  const inputCls =
    "h-9 w-full rounded-[7px] border border-border-strong bg-surface-elevated px-3 text-[13px] text-foreground outline-none transition-[border-color,box-shadow] duration-100 placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary/15";

  return (
    <main className="grid min-h-screen bg-background lg:grid-cols-[minmax(0,1fr)_460px]">
      {/* Context panel: identity + security posture. Hidden below lg. */}
      <section className="relative hidden flex-col justify-between border-r border-border bg-surface p-10 lg:flex">
        <div className="flex items-center gap-2.5">
          <span className="grid h-8 w-8 place-items-center rounded-[8px] bg-graphite text-sm font-semibold text-graphite-foreground">Q</span>
          <div>
            <p className="text-sm font-semibold leading-4 tracking-tight">QazEra</p>
            <p className="mt-0.5 text-[11px] leading-3 text-muted-foreground">Corporate Communication Platform</p>
          </div>
        </div>

        <div className="max-w-md">
          <p className="qazera-label">Internal system</p>
          <h1 className="mt-3 text-[22px] font-semibold leading-8 tracking-[-.01em]">
            Mail, internal messaging and administration for your organization.
          </h1>
          <div className="mt-8 space-y-4 border-t border-border pt-6">
            {[
              { icon: "shield-check", title: t("featureAccess"), text: "Sessions are cookie-bound, rate-limited and revocable." },
              { icon: "scroll-text", title: t("featureAudit"), text: "Administrative and authentication events are recorded in the audit log." },
              { icon: "mail", title: t("featureUnified"), text: "Mail, contacts, departments and announcements in one workspace." },
            ].map((f) => (
              <div key={f.icon} className="flex gap-3">
                <span className="mt-0.5 grid h-7 w-7 shrink-0 place-items-center rounded-[7px] border border-border bg-surface-elevated text-muted-foreground">
                  <Icon name={f.icon} className="h-3.5 w-3.5" />
                </span>
                <div>
                  <p className="text-[13px] font-medium">{f.title}</p>
                  <p className="mt-0.5 text-xs leading-4.5 text-muted-foreground">{f.text}</p>
                </div>
              </div>
            ))}
          </div>
        </div>

        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span className="inline-flex items-center gap-2">
            <span className={cx("h-1.5 w-1.5 rounded-full", health.isLoading ? "bg-border-strong" : healthy ? "bg-success" : "bg-warning")} />
            {health.isLoading ? "Checking system status…" : healthy ? "All systems operational" : "Degraded — some services unavailable"}
          </span>
          <span>© {new Date().getFullYear()} QazEra</span>
        </div>
      </section>

      {/* Sign-in panel */}
      <section className="flex flex-col px-6 py-8 sm:px-12">
        <div className="flex items-center justify-between lg:justify-end">
          <div className="flex items-center gap-2.5 lg:hidden">
            <span className="grid h-8 w-8 place-items-center rounded-[8px] bg-graphite text-sm font-semibold text-graphite-foreground">Q</span>
            <p className="text-sm font-semibold tracking-tight">QazEra</p>
          </div>
          <LocaleSwitcher />
        </div>

        <div className="mx-auto flex w-full max-w-sm flex-1 flex-col justify-center py-10">
          <h2 className="text-lg font-semibold tracking-tight">{t("signInTitle")}</h2>
          <p className="mt-1 text-[13px] text-muted-foreground">{t("signInHint")}</p>

          <form className="mt-7 space-y-4" onSubmit={submit} noValidate>
            <Field label={t("email")} error={errors.email}>
              <input
                type="email"
                autoComplete="email"
                autoFocus
                placeholder="name@company.kz"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className={cx(inputCls, errors.email && "border-danger focus:border-danger focus:ring-danger/15")}
              />
            </Field>
            <Field label={t("password")} error={errors.password}>
              <div className="relative">
                <input
                  type={showPassword ? "text" : "password"}
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className={cx(inputCls, "pr-10", errors.password && "border-danger focus:border-danger focus:ring-danger/15")}
                />
                <button
                  type="button"
                  className="absolute right-1 top-1/2 grid h-7 w-7 -translate-y-1/2 place-items-center rounded-[6px] text-muted-foreground hover:bg-muted hover:text-foreground"
                  onClick={() => setShowPassword((v) => !v)}
                  aria-label={showPassword ? "Hide password" : "Show password"}
                >
                  <Icon name={showPassword ? "eye-off" : "eye"} className="h-3.5 w-3.5" />
                </button>
              </div>
            </Field>

            {formError && (
              <div className="flex items-start gap-2 rounded-[7px] border border-danger/25 bg-danger/5 px-3 py-2.5 text-[13px] leading-5 text-danger" role="alert">
                <Icon name="alert-triangle" className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                {formError}
              </div>
            )}

            <Button type="submit" disabled={busy || !!quickBusy} className="h-9 w-full">
              {busy ? <><Spinner className="h-3.5 w-3.5 border-white" /> {t("signingIn")}…</> : t("signIn")}
            </Button>
          </form>

          <div className="mt-8">
            <div className="flex items-center gap-3">
              <span className="text-[11px] font-medium uppercase tracking-[.05em] text-faint">{t("quickAccess")}</span>
              <span className="h-px flex-1 bg-border" />
            </div>
            <div className="mt-3 grid gap-2">
              {QUICK_ACCOUNTS.map((account) => (
                <button
                  key={account.email}
                  type="button"
                  disabled={!!quickBusy || busy}
                  onClick={() => authenticate(account, account.email)}
                  className="flex items-center gap-3 rounded-[8px] border border-border bg-surface-elevated px-3 py-2 text-left transition-colors hover:border-border-strong hover:bg-background disabled:opacity-60"
                >
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-[6px] bg-background text-muted-foreground">
                    <Icon name={account.kind === "administrator" ? "shield" : "user"} className="h-3.5 w-3.5" />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-[13px] font-medium leading-4">{t(account.kind)}</span>
                    <span className="block truncate text-xs text-muted-foreground">{account.email}</span>
                  </span>
                  {quickBusy === account.email ? <Spinner className="h-3.5 w-3.5" /> : <Icon name="chevron-right" className="h-3.5 w-3.5 text-faint" />}
                </button>
              ))}
            </div>
            <p className="mt-2 text-xs text-faint">{t("quickHint")}</p>
          </div>
        </div>

        <p className="text-center text-xs text-faint">{t("protected")}</p>
      </section>
    </main>
  );
}
