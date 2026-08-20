"use client";

import { createContext, useCallback, useContext, useRef, useState } from "react";

export function cx(...parts: Array<string | false | undefined>) {
  return parts.filter(Boolean).join(" ");
}

export function Button({
  variant = "primary",
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger";
}) {
  const styles = {
    primary:
      "bg-[#111111] text-[#f9f9f7] hover:bg-white hover:text-[#111111] hover:border-[#111111] disabled:bg-neutral-300 disabled:text-neutral-500",
    secondary:
      "border border-[#111111] bg-transparent text-[#111111] hover:bg-[#111111] hover:text-[#f9f9f7] disabled:border-neutral-300 disabled:text-neutral-400",
    ghost:
      "bg-transparent text-[#111111] hover:bg-[#e5e5e0] disabled:text-neutral-400",
    danger:
      "border border-[#111111] bg-[#cc0000] text-white hover:bg-[#111111] disabled:bg-neutral-300 disabled:text-neutral-500",
  }[variant];

  return (
    <button
      className={cx(
        "inline-flex min-h-11 items-center justify-center gap-2 rounded-none px-4 text-xs font-bold uppercase tracking-[0.18em] transition-all duration-200 ease-out disabled:cursor-not-allowed font-sans",
        styles,
        className,
      )}
      {...props}
    />
  );
}

export function Input({
  label,
  error,
  className,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement> & { label?: string; error?: string }) {
  return (
    <label className="block">
      {label && <span className="mb-2 block qazera-label">{label}</span>}
      <input
        className={cx(
          "w-full rounded-none border-b-2 border-[#111111] bg-transparent px-3 py-2 text-sm text-[#111111] outline-none transition-colors placeholder:text-neutral-500 focus-visible:bg-[#f0f0f0] focus-visible:ring-0 font-mono",
          error && "border-[#cc0000]",
          className,
        )}
        {...props}
      />
      {error && <span className="mt-2 block text-xs font-medium text-[#cc0000]">{error}</span>}
    </label>
  );
}

export function Textarea({
  label,
  className,
  ...props
}: React.TextareaHTMLAttributes<HTMLTextAreaElement> & { label?: string }) {
  return (
    <label className="block">
      {label && <span className="mb-2 block qazera-label">{label}</span>}
      <textarea
        className={cx(
          "w-full rounded-none border-b-2 border-[#111111] bg-transparent px-3 py-2 text-sm text-[#111111] outline-none transition-colors placeholder:text-neutral-500 focus-visible:bg-[#f0f0f0] focus-visible:ring-0 font-body",
          className,
        )}
        {...props}
      />
    </label>
  );
}

export function Spinner({ className }: { className?: string }) {
  return (
    <span
      className={cx(
        "inline-block h-4 w-4 animate-spin rounded-full border-2 border-[#111111] border-t-transparent",
        className,
      )}
      role="status"
      aria-label="Loading"
    />
  );
}

export function PageLoader({ label = "Loading…" }: { label?: string }) {
  return (
    <div className="flex min-h-48 items-center justify-center gap-3 px-6 py-10 text-xs font-medium uppercase tracking-[0.2em] text-[#111111] font-sans">
      <Spinner /> {label}
    </div>
  );
}

export function EmptyState({ title, hint }: { title: string; hint?: string }) {
  return (
    <div className="newsprint-texture qazera-panel flex min-h-48 flex-col items-start justify-center gap-2 p-8">
      <p className="qazera-label text-[#cc0000]">{title}</p>
      {hint && <p className="max-w-md text-sm leading-6 text-[#111111] font-body">{hint}</p>}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="newsprint-texture qazera-panel flex min-h-48 flex-col items-start justify-center gap-4 p-8">
      <p className="qazera-label text-[#cc0000]">{message}</p>
      {onRetry && <Button variant="secondary" onClick={onRetry}>Retry</Button>}
    </div>
  );
}

type Toast = { id: number; kind: "success" | "error"; text: string };
const ToastCtx = createContext<(kind: Toast["kind"], text: string) => void>(() => {});

export function useToast() {
  return useContext(ToastCtx);
}

export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const push = useCallback((kind: Toast["kind"], text: string) => {
    const id = nextId.current++;
    setToasts((items) => [...items, { id, kind, text }]);
    setTimeout(() => setToasts((items) => items.filter((item) => item.id !== id)), 3600);
  }, []);

  return (
    <ToastCtx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-[min(100vw-2rem,24rem)] flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={cx(
              "pointer-events-auto qazera-panel px-4 py-3 text-xs font-bold uppercase tracking-[0.16em] font-sans",
              t.kind === "success" ? "bg-[#111111] text-[#f9f9f7]" : "bg-[#cc0000] text-white",
            )}
          >
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

export function ConfirmDialog({
  open,
  title,
  body,
  confirmLabel = "Confirm",
  danger,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  body?: string;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-[1px]"
      onClick={onCancel}
    >
      <div className="qazera-panel w-full max-w-md bg-[#f9f9f7] p-6" onClick={(e) => e.stopPropagation()}>
        <h3 className="qazera-label">{title}</h3>
        {body && <p className="mt-3 text-sm leading-6 text-[#111111] font-body">{body}</p>}
        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <Button variant="secondary" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant={danger ? "danger" : "primary"} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}
