"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { z } from "zod";
import { api, ApiError } from "@/lib/api";
import { Button, Input, Spinner } from "@/components/ui";

const schema = z.object({
  email: z.string().email("Enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

export default function LoginPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [errors, setErrors] = useState<{ email?: string; password?: string }>({});
  const [formError, setFormError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setFormError("");

    const parsed = schema.safeParse({ email, password });
    if (!parsed.success) {
      const next: typeof errors = {};
      for (const issue of parsed.error.issues) {
        next[issue.path[0] as "email" | "password"] = issue.message;
      }
      setErrors(next);
      return;
    }

    setErrors({});
    setBusy(true);

    try {
      await api.post("/api/v1/auth/login", parsed.data);
      await qc.invalidateQueries();
      router.replace("/mail/inbox");
    } catch (err) {
      setFormError(
        err instanceof ApiError && err.status === 401
          ? "Invalid email or password."
          : err instanceof ApiError && err.status === 429
            ? "Too many attempts. Try again in a few minutes."
            : "Could not sign in. Check that the server is running.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="newsprint-texture min-h-screen bg-[#f9f9f7] px-4 py-6 lg:px-6">
      <div className="mx-auto grid min-h-[calc(100vh-3rem)] max-w-screen-xl gap-0 border-2 border-[#111111] lg:grid-cols-12">
        <section className="border-b border-[#111111] p-6 lg:col-span-7 lg:border-b-0 lg:border-r">
          <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Issue 01</p>
          <h1 className="mt-4 font-serif text-[clamp(3.5rem,10vw,8rem)] leading-[0.9] tracking-tighter text-[#111111]">
            Secure entry for the QazEra edition.
          </h1>
          <p className="mt-6 max-w-2xl text-lg leading-8 text-[#111111] font-body text-justify">
            Sign in to reach mail, security, and administration from a single paper-like control
            surface. The page is intentionally direct: form on the right, editorial context on the
            left.
          </p>

          <div className="mt-8 grid gap-0 border border-[#111111] md:grid-cols-3">
            {[
              "Encrypted sessions",
              "Sharp audit trails",
              "Role-based access",
            ].map((item, index) => (
              <div
                key={item}
                className={`border-b border-[#111111] p-4 ${index < 2 ? "md:border-r" : ""} md:border-b-0`}
              >
                <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">{item}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="bg-[#e5e5e0] p-6 lg:col-span-5">
          <div className="border border-[#111111] bg-[#f9f9f7] p-5 lg:p-6">
            <div className="flex items-end justify-between gap-4">
              <div>
                <p className="font-mono text-xs uppercase tracking-[0.3em] text-[#cc0000]">Login</p>
                <h2 className="mt-2 font-serif text-4xl tracking-tight text-[#111111]">
                  Open your session
                </h2>
              </div>
              <Link href="/" className="font-mono text-xs uppercase tracking-[0.3em] text-[#111111]">
                Home
              </Link>
            </div>

            <form className="mt-6 space-y-4" onSubmit={submit}>
              <Input
                label="Email"
                type="email"
                autoComplete="email"
                placeholder="you@company.test"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                error={errors.email}
                autoFocus
              />
              <Input
                label="Password"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                error={errors.password}
              />
              {formError && <p className="text-sm font-medium text-[#cc0000] font-sans">{formError}</p>}
              <div className="flex flex-wrap items-center gap-3 pt-2">
                <Button type="submit" disabled={busy}>
                  {busy ? <Spinner className="border-white border-t-transparent" /> : "Sign in"}
                </Button>
                <Link
                  href="/"
                  className="font-mono text-xs uppercase tracking-[0.3em] text-[#111111] underline decoration-[#cc0000] decoration-2 underline-offset-4"
                >
                  Return home
                </Link>
              </div>
            </form>
          </div>
        </section>
      </div>
    </main>
  );
}
