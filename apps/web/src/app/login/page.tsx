"use client";

import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { z } from "zod";
import { api, ApiError } from "@/lib/api";
import { LocaleSwitcher, useI18n } from "@/components/providers";
import { Spinner, cx } from "@/components/ui";
import { Icon } from "@/components/icons";

const schema = z.object({
  email: z.string().email("Enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

const ADMIN_ACCOUNT = {
  email: process.env.NEXT_PUBLIC_DEMO_ADMIN_EMAIL ?? "admin@company.test",
  password: process.env.NEXT_PUBLIC_DEMO_ADMIN_PASSWORD ?? "admin-dev-password-1",
};

function QazEraMark({ inverted = false }: { inverted?: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cx(
        "relative grid h-12 w-12 shrink-0 place-items-center rounded-full border-2",
        inverted ? "border-white" : "border-[#181a1b]",
      )}
    >
      <span className={cx("mt-1 h-3 w-7 rounded-b-full", inverted ? "bg-white" : "bg-[#181a1b]")} />
    </span>
  );
}

function Field({ label, error, children }: { label: string; error?: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-2 block text-[15px] font-medium text-[#181a1b]">{label}</span>
      {children}
      {error && <span className="mt-1.5 block text-xs font-medium text-danger">{error}</span>}
    </label>
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
  const [quickBusy, setQuickBusy] = useState(false);

  async function authenticate(credentials: { email: string; password: string }, quick = false) {
    setFormError("");
    setBusy(!quick);
    setQuickBusy(quick);
    try {
      await api.post("/api/v1/auth/login", credentials);
      await qc.invalidateQueries();
      router.replace("/mail/inbox");
    } catch (err) {
      setFormError(
        err instanceof ApiError && err.status === 401
          ? t("invalidCredentials")
          : err instanceof ApiError && err.status === 429
            ? t("tooManyAttempts")
            : err instanceof ApiError && err.code === "USER_DISABLED"
              ? "This account is disabled. Contact your administrator."
              : err instanceof ApiError
                ? `${err.message} (${err.code})`
                : t("serverUnavailable"),
      );
    } finally {
      setBusy(false);
      setQuickBusy(false);
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const parsed = schema.safeParse({ email, password });
    if (!parsed.success) {
      const next: typeof errors = {};
      for (const issue of parsed.error.issues) next[issue.path[0] as "email" | "password"] = issue.message;
      setErrors(next);
      return;
    }
    setErrors({});
    await authenticate(parsed.data);
  }

  const inputClass = "h-[52px] w-full rounded-xl border border-[#d6d6d6] bg-white px-5 text-base text-[#181a1b] outline-none transition placeholder:text-[#a2a2a2] focus:border-[#181a1b] focus:ring-2 focus:ring-black/5";

  return (
    <main className="flex h-dvh w-screen flex-col overflow-hidden bg-white text-[#181a1b]">
      <header className="flex items-center justify-between px-7 pt-7 sm:px-12 sm:pt-12">
        <div className="flex items-center gap-3">
          <QazEraMark />
          <span className="text-[34px] font-medium leading-none tracking-[-0.045em] sm:text-[40px]">QazEra.</span>
        </div>
        <LocaleSwitcher className="border-[#dedede] bg-white" />
      </header>

      <section className="mx-auto flex min-h-0 w-full max-w-[640px] flex-1 flex-col justify-center px-6 py-8 sm:px-0">
        <div className="mb-16 text-center sm:mb-20">
          <h1 className="text-[36px] font-semibold leading-tight tracking-[-0.04em] sm:text-[48px]">{t("signInTitle")}</h1>
          <p className="mt-4 text-base text-[#747474] sm:text-lg">{t("signInHint")}</p>
        </div>

        <button
          type="button"
          disabled={busy || quickBusy}
          onClick={() => authenticate(ADMIN_ACCOUNT, true)}
          className="flex h-[60px] w-full items-center justify-center gap-3 rounded-xl border border-[#d4d4d4] bg-white px-5 text-base font-semibold transition-colors hover:bg-[#f7f7f7] disabled:cursor-not-allowed disabled:opacity-60"
        >
          {quickBusy ? <Spinner className="h-5 w-5" /> : <Icon name="shield" className="h-5 w-5" />}
          {t("quickAccess")} — {t("administrator")}
        </button>

        <div className="my-10 h-px bg-[#e5e5e5]" />

        <form className="space-y-6" onSubmit={submit} noValidate>
          <Field label={t("email")} error={errors.email}>
            <input
              type="email"
              autoComplete="email"
              autoFocus
              placeholder="name@company.kz"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className={cx(inputClass, errors.email && "border-danger focus:border-danger focus:ring-danger/10")}
            />
          </Field>

          <Field label={t("password")} error={errors.password}>
            <div className="relative">
              <input
                type={showPassword ? "text" : "password"}
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                className={cx(inputClass, "pr-14", errors.password && "border-danger focus:border-danger focus:ring-danger/10")}
              />
              <button
                type="button"
                className="absolute right-3 top-1/2 grid h-9 w-9 -translate-y-1/2 place-items-center rounded-lg text-[#777] hover:bg-[#f2f2f2] hover:text-[#181a1b]"
                onClick={() => setShowPassword((value) => !value)}
                aria-label={showPassword ? "Hide password" : "Show password"}
              >
                <Icon name={showPassword ? "eye-off" : "eye"} className="h-4 w-4" />
              </button>
            </div>
          </Field>

          {formError && (
            <div className="flex items-start gap-2 rounded-xl border border-danger/25 bg-danger/5 px-4 py-3 text-sm leading-5 text-danger" role="alert">
              <Icon name="alert-triangle" className="mt-0.5 h-4 w-4 shrink-0" />
              {formError}
            </div>
          )}

          <button
            type="submit"
            disabled={busy || quickBusy}
            className="flex h-[60px] w-full items-center justify-center gap-2 rounded-xl bg-[#181a1b] px-5 text-base font-semibold text-white transition-colors hover:bg-black disabled:cursor-not-allowed disabled:bg-[#aaa]"
          >
            {busy && <Spinner className="h-5 w-5" />}
            {busy ? `${t("signingIn")}…` : t("signIn")}
          </button>
        </form>
      </section>

      <p className="px-6 pb-8 text-center text-sm leading-6 text-[#777]">{t("protected")}</p>

    </main>
  );
}
