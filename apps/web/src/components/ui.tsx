"use client";

// QazEra UI kit: one control system for the whole product.
// Sizes: controls h-8 (32px), large controls h-9. Radius: 7px controls,
// 10px panels, 12px dialogs. Primary action = graphite; accent blue is
// reserved for links, focus and live indicators.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { Icon } from "@/components/icons";

export function cx(...parts: Array<string | false | null | undefined>) {
  return parts.filter(Boolean).join(" ");
}

/* ---------------------------------- Buttons ---------------------------------- */

export function Button({
  variant = "primary",
  size = "md",
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary" | "ghost" | "danger" | "accent";
  size?: "sm" | "md";
}) {
  const styles = {
    primary:
      "border border-transparent bg-graphite text-graphite-foreground hover:bg-graphite/90 disabled:border-border disabled:bg-muted disabled:text-muted-foreground",
    accent:
      "border border-transparent bg-primary text-primary-foreground hover:bg-primary-strong disabled:border-border disabled:bg-muted disabled:text-muted-foreground",
    secondary:
      "border border-border-strong bg-surface-elevated text-foreground hover:bg-background disabled:text-muted-foreground disabled:hover:bg-surface-elevated",
    ghost:
      "border border-transparent bg-transparent text-muted-foreground hover:bg-muted hover:text-foreground disabled:text-muted-foreground/50 disabled:hover:bg-transparent",
    danger:
      "border border-transparent bg-danger text-white hover:bg-danger/90 disabled:border-border disabled:bg-muted disabled:text-muted-foreground",
  }[variant];

  return (
    <button
      className={cx(
        "inline-flex items-center justify-center gap-1.5 rounded-[7px] font-medium transition-colors duration-100 disabled:cursor-not-allowed",
        size === "sm" ? "h-7 px-2.5 text-xs" : "h-8 px-3 text-[13px]",
        styles,
        className,
      )}
      {...props}
    />
  );
}

export function IconButton({
  label,
  icon,
  active,
  size = "md",
  className,
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & {
  label: string;
  icon: string;
  active?: boolean;
  size?: "sm" | "md";
}) {
  return (
    <button
      type="button"
      title={label}
      aria-label={label}
      className={cx(
        "grid place-items-center rounded-[7px] transition-colors duration-100 disabled:cursor-not-allowed disabled:opacity-40",
        size === "sm" ? "h-7 w-7" : "h-8 w-8",
        active
          ? "bg-muted text-foreground"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
        className,
      )}
      {...props}
    >
      <Icon name={icon} className={size === "sm" ? "h-3.5 w-3.5" : "h-4 w-4"} />
    </button>
  );
}

/* ---------------------------------- Inputs ---------------------------------- */

export function Input({
  label,
  error,
  className,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement> & { label?: string; error?: string }) {
  return (
    <label className="block">
      {label && <span className="mb-1 block text-xs font-medium text-foreground">{label}</span>}
      <input
        className={cx(
          "h-8 w-full rounded-[7px] border border-border-strong bg-surface-elevated px-2.5 text-[13px] text-foreground outline-none transition-[border-color,box-shadow] duration-100 placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary/15",
          error && "border-danger focus:border-danger focus:ring-danger/15",
          className,
        )}
        {...props}
      />
      {error && <span className="mt-1 block text-xs font-medium text-danger">{error}</span>}
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
      {label && <span className="mb-1 block text-xs font-medium text-foreground">{label}</span>}
      <textarea
        className={cx(
          "w-full rounded-[7px] border border-border-strong bg-surface-elevated px-2.5 py-2 text-[13px] leading-relaxed text-foreground outline-none transition-[border-color,box-shadow] duration-100 placeholder:text-faint focus:border-primary focus:ring-2 focus:ring-primary/15",
          className,
        )}
        {...props}
      />
    </label>
  );
}

/* ---------------------------------- Badges ---------------------------------- */

export function Badge({
  tone = "neutral",
  children,
  className,
}: {
  tone?: "neutral" | "success" | "warning" | "danger" | "accent" | "graphite";
  children: React.ReactNode;
  className?: string;
}) {
  const tones = {
    neutral: "border-border bg-background text-muted-foreground",
    success: "border-success/25 bg-success/10 text-success",
    warning: "border-warning/25 bg-warning/10 text-warning",
    danger: "border-danger/25 bg-danger/10 text-danger",
    accent: "border-primary/25 bg-primary/10 text-primary",
    graphite: "border-transparent bg-graphite text-graphite-foreground",
  }[tone];
  return (
    <span className={cx("inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium leading-4", tones, className)}>
      {children}
    </span>
  );
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded border border-border bg-background px-1.5 py-0.5 font-mono text-[10px] leading-none text-muted-foreground">
      {children}
    </kbd>
  );
}

/* --------------------------------- Feedback --------------------------------- */

export function Spinner({ className }: { className?: string }) {
  return (
    <span
      className={cx("inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent", className)}
      role="status"
      aria-label="Loading"
    />
  );
}

export function PageLoader({ label = "Loading…" }: { label?: string }) {
  return (
    <div className="flex min-h-40 items-center justify-center gap-3 px-6 py-10 text-[13px] text-muted-foreground">
      <Spinner /> {label}
    </div>
  );
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cx("skeleton", className)} aria-hidden="true" />;
}

export function ListSkeleton({ rows = 6 }: { rows?: number }) {
  return (
    <div className="space-y-2.5 p-4" aria-label="Loading" role="status">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="flex items-center gap-3">
          <Skeleton className="h-7 w-7 rounded-full" />
          <div className="flex-1 space-y-1.5">
            <Skeleton className="h-3 w-1/3" />
            <Skeleton className="h-3 w-2/3" />
          </div>
        </div>
      ))}
    </div>
  );
}

export function EmptyState({
  icon = "inbox",
  title,
  hint,
  action,
}: {
  icon?: string;
  title: string;
  hint?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex min-h-44 flex-col items-center justify-center gap-1.5 p-8 text-center">
      <span className="mb-1 grid h-10 w-10 place-items-center rounded-full border border-border bg-background text-muted-foreground">
        <Icon name={icon} className="h-4.5 w-4.5" />
      </span>
      <p className="text-[13px] font-semibold text-foreground">{title}</p>
      {hint && <p className="max-w-sm text-[13px] leading-5 text-muted-foreground">{hint}</p>}
      {action && <div className="mt-3">{action}</div>}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex min-h-44 flex-col items-center justify-center gap-3 p-8 text-center">
      <span className="grid h-10 w-10 place-items-center rounded-full border border-danger/25 bg-danger/10 text-danger">
        <Icon name="alert-triangle" className="h-4.5 w-4.5" />
      </span>
      <p className="text-[13px] font-medium text-danger">{message}</p>
      {onRetry && <Button variant="secondary" size="sm" onClick={onRetry}>Retry</Button>}
    </div>
  );
}

/* ---------------------------------- Toasts ---------------------------------- */

type Toast = { id: number; kind: "success" | "error" | "info"; text: string };
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
      <div className="pointer-events-none fixed bottom-4 right-4 z-[70] flex w-[min(100vw-2rem,22rem)] flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            role="status"
            className="anim-rise pointer-events-auto flex items-center gap-2.5 rounded-[9px] bg-graphite px-3.5 py-2.5 text-[13px] text-graphite-foreground shadow-[var(--shadow-popover)]"
          >
            <Icon
              name={t.kind === "success" ? "check-circle" : t.kind === "error" ? "alert-triangle" : "info"}
              className={cx("h-4 w-4 shrink-0", t.kind === "error" ? "text-red-300" : t.kind === "success" ? "text-emerald-300" : "text-slate-300")}
            />
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  );
}

/* --------------------------------- Overlays --------------------------------- */

function useEscape(active: boolean, onClose: () => void) {
  useEffect(() => {
    if (!active) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [active, onClose]);
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
  useEscape(open, onCancel);
  if (!open) return null;

  return (
    <div
      className="anim-fade fixed inset-0 z-[60] flex items-center justify-center bg-graphite/40 p-4"
      onClick={onCancel}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div
        className="anim-rise w-full max-w-sm rounded-xl border border-border bg-surface-elevated p-5 shadow-[var(--shadow-window)]"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-sm font-semibold">{title}</h3>
        {body && <p className="mt-1.5 text-[13px] leading-5 text-muted-foreground">{body}</p>}
        <div className="mt-5 flex flex-wrap justify-end gap-2">
          <Button variant="secondary" onClick={onCancel}>Cancel</Button>
          <Button autoFocus variant={danger ? "danger" : "primary"} onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function Drawer({
  open,
  onClose,
  title,
  width = "max-w-xl",
  children,
}: {
  open: boolean;
  onClose: () => void;
  title: React.ReactNode;
  width?: string;
  children: React.ReactNode;
}) {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <div className="anim-fade fixed inset-0 z-[55] bg-graphite/35" onClick={onClose} role="dialog" aria-modal="true">
      <aside
        className={cx("anim-slide-right absolute inset-y-0 right-0 flex w-full flex-col border-l border-border bg-surface-elevated shadow-[var(--shadow-window)]", width)}
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex h-12 shrink-0 items-center justify-between gap-3 border-b border-border px-4">
          <div className="min-w-0 flex-1 text-sm font-semibold">{title}</div>
          <IconButton label="Close" icon="x" onClick={onClose} />
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
      </aside>
    </div>
  );
}

/* ----------------------------------- Menu ----------------------------------- */

export function Menu({
  trigger,
  align = "end",
  items,
  width = "w-52",
}: {
  trigger: React.ReactNode;
  align?: "start" | "end";
  width?: string;
  items: Array<
    | { type?: "item"; label: string; icon?: string; danger?: boolean; disabled?: boolean; onSelect: () => void }
    | { type: "separator" }
    | { type: "label"; label: string }
  >;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEscape(open, () => setOpen(false));
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  return (
    <div className="relative" ref={ref}>
      <span onClick={() => setOpen((v) => !v)}>{trigger}</span>
      {open && (
        <div
          className={cx(
            "anim-rise absolute z-50 mt-1 overflow-hidden rounded-[9px] border border-border bg-surface-elevated py-1 shadow-[var(--shadow-popover)]",
            width,
            align === "end" ? "right-0" : "left-0",
          )}
          role="menu"
        >
          {items.map((item, i) => {
            if (item.type === "separator") return <div key={i} className="my-1 h-px bg-border" />;
            if (item.type === "label")
              return <p key={i} className="px-3 pb-1 pt-1.5 text-[10.5px] font-medium uppercase tracking-[.06em] text-faint">{item.label}</p>;
            return (
              <button
                key={i}
                type="button"
                role="menuitem"
                disabled={item.disabled}
                className={cx(
                  "flex w-full items-center gap-2.5 px-3 py-1.5 text-left text-[13px] transition-colors disabled:opacity-40",
                  item.danger ? "text-danger hover:bg-danger/10" : "text-foreground hover:bg-muted",
                )}
                onClick={() => {
                  setOpen(false);
                  item.onSelect();
                }}
              >
                {item.icon && <Icon name={item.icon} className="h-3.5 w-3.5 shrink-0 opacity-70" />}
                {item.label}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

/* ----------------------------------- Tabs ----------------------------------- */

export function Tabs({
  value,
  onChange,
  tabs,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  tabs: Array<{ value: string; label: React.ReactNode }>;
  className?: string;
}) {
  return (
    <div className={cx("flex items-center gap-1 border-b border-border", className)} role="tablist">
      {tabs.map((tab) => (
        <button
          key={tab.value}
          role="tab"
          aria-selected={value === tab.value}
          className={cx(
            "-mb-px border-b-2 px-3 py-2 text-[13px] font-medium transition-colors",
            value === tab.value
              ? "border-graphite text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground",
          )}
          onClick={() => onChange(tab.value)}
        >
          {tab.label}
        </button>
      ))}
    </div>
  );
}

/* ------------------------------ Segmented control ---------------------------- */

export function Segmented({
  value,
  onChange,
  options,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  options: Array<{ value: string; label: string }>;
  className?: string;
}) {
  return (
    <div className={cx("inline-flex items-center gap-0.5 rounded-[8px] border border-border bg-background p-0.5", className)}>
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          className={cx(
            "rounded-[6px] px-2.5 py-1 text-xs font-medium transition-colors",
            value === opt.value
              ? "bg-surface-elevated text-foreground shadow-sm"
              : "text-muted-foreground hover:text-foreground",
          )}
          onClick={() => onChange(opt.value)}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

/* --------------------------------- Avatars ---------------------------------- */

const AVATAR_TONES = [
  "bg-slate-200 text-slate-700",
  "bg-zinc-200 text-zinc-700",
  "bg-stone-200 text-stone-700",
  "bg-blue-100 text-blue-800",
  "bg-indigo-100 text-indigo-800",
  "bg-emerald-100 text-emerald-800",
  "bg-amber-100 text-amber-800",
];

export function Avatar({ name, size = "md", className }: { name: string; size?: "sm" | "md" | "lg"; className?: string }) {
  const initials = name
    .split(/[\s@._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]!.toUpperCase())
    .join("") || "?";
  let hash = 0;
  for (let i = 0; i < name.length; i++) hash = (hash * 31 + name.charCodeAt(i)) | 0;
  const tone = AVATAR_TONES[Math.abs(hash) % AVATAR_TONES.length];
  const sizes = { sm: "h-6 w-6 text-[10px]", md: "h-7 w-7 text-[11px]", lg: "h-9 w-9 text-xs" }[size];
  return (
    <span className={cx("grid shrink-0 select-none place-items-center rounded-full font-semibold", sizes, tone, className)} aria-hidden="true">
      {initials}
    </span>
  );
}
