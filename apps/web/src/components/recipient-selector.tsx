"use client";

// Recipient picker: type an address, search people, or pick a whole
// department. Selected recipients render as compact chips.

import { useDeferredValue, useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type Department, type DirectoryUser } from "@/lib/api";
import { useI18n } from "@/components/providers";
import { Avatar, cx } from "@/components/ui";
import { Icon } from "@/components/icons";

function parse(value: string) {
  return [...new Set(value.split(/[,;\s]+/).map((item) => item.trim().toLowerCase()).filter(Boolean))];
}

export function RecipientSelector({
  value,
  onChange,
  autoFocus,
  placeholder,
  onPendingAddressChange,
}: {
  value: string;
  onChange: (value: string) => void;
  autoFocus?: boolean;
  placeholder?: string;
  onPendingAddressChange?: (value: string) => void;
}) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [departmentId, setDepartmentId] = useState("");
  const [search, setSearch] = useState("");
  const deferredSearch = useDeferredValue(search);
  const rootRef = useRef<HTMLDivElement>(null);
  const selected = parse(value);

  const departments = useQuery({
    queryKey: ["departments"],
    queryFn: () => api.get<{ departments: Department[] }>("/api/v1/departments"),
  });
  const directory = useQuery({
    queryKey: ["directory", departmentId, deferredSearch],
    queryFn: () => {
      const params = new URLSearchParams();
      params.set("limit", "1000");
      if (departmentId) params.set("department_id", departmentId);
      if (deferredSearch.trim()) params.set("q", deferredSearch.trim());
      return api.get<{ users: DirectoryUser[] }>(`/api/v1/directory/users?${params}`);
    },
    enabled: open,
  });

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  const people = directory.data?.users ?? [];
  const selectedDepartment = departments.data?.departments.find((d) => d.id === departmentId);

  function commit(addresses: string[]) {
    onChange([...new Set(addresses.map((address) => address.toLowerCase()))].join(", "));
  }

  function toggle(person: DirectoryUser) {
    const address = person.mailbox_address.toLowerCase();
    commit(selected.includes(address) ? selected.filter((item) => item !== address) : [...selected, address]);
  }

  function addTyped() {
    const typed = parse(search).filter((item) => item.includes("@"));
    if (typed.length === 0) return;
    commit([...selected, ...typed]);
    setSearch("");
    onPendingAddressChange?.("");
  }

  const allVisibleSelected =
    people.length > 0 && people.every((person) => selected.includes(person.mailbox_address.toLowerCase()));

  return (
    <div className="relative" ref={rootRef}>
      <div className="flex min-h-9 flex-wrap items-center gap-1.5 py-1">
        {selected.map((address) => (
          <span
            key={address}
            className="inline-flex max-w-full items-center gap-1 rounded-full border border-border bg-background py-0.5 pl-2 pr-0.5 text-xs font-medium text-foreground"
          >
            <span className="truncate">{address}</span>
            <button
              type="button"
              className="grid h-4.5 w-4.5 shrink-0 place-items-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground"
              onClick={() => commit(selected.filter((item) => item !== address))}
              aria-label={`Remove ${address}`}
            >
              <Icon name="x" className="h-2.5 w-2.5" />
            </button>
          </span>
        ))}
        <input
          autoFocus={autoFocus}
          value={search}
          onChange={(event) => {
            const next = event.target.value;
            setSearch(next);
            onPendingAddressChange?.(next.includes("@") ? next : "");
            setOpen(true);
          }}
          onBlur={() => {
            // A typed address is also valid without a trailing Enter/comma.
            // Commit it before actions such as Send read the controlled value.
            addTyped();
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter" || event.key === "," || event.key === ";") {
              if (search.includes("@")) { event.preventDefault(); addTyped(); }
            }
            if (event.key === "Backspace" && !search && selected.length > 0) {
              commit(selected.slice(0, -1));
            }
            if (event.key === "Escape" && open) {
              event.stopPropagation();
              setOpen(false);
            }
          }}
          placeholder={selected.length === 0 ? (placeholder ?? t("searchRecipients")) : ""}
          className="h-7 min-w-36 flex-1 bg-transparent text-[13px] outline-none placeholder:text-faint"
        />
        <button
          type="button"
          className="inline-flex shrink-0 items-center gap-1 rounded-[6px] px-1.5 py-1 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
          onClick={() => setOpen((current) => !current)}
        >
          <Icon name="users" className="h-3.5 w-3.5" /> {t("chooseDepartment")}
        </button>
      </div>

      {open && (
        <div className="anim-rise absolute left-0 right-0 top-full z-40 mt-1 overflow-hidden rounded-[9px] border border-border bg-surface-elevated shadow-[var(--shadow-popover)]">
          <div className="flex flex-wrap items-center gap-2 border-b border-border bg-background/60 p-2">
            <select
              value={departmentId}
              onChange={(event) => setDepartmentId(event.target.value)}
              className="min-w-48 flex-1"
            >
              <option value="">{t("allDepartments")}</option>
              {(departments.data?.departments ?? []).map((department) => (
                <option key={department.id} value={department.id}>
                  {department.name} · {department.employee_count} {t("people")}
                </option>
              ))}
            </select>
            <button
              type="button"
              disabled={people.length === 0}
              className="h-8 rounded-[7px] px-2.5 text-xs font-medium text-primary hover:bg-primary/10 disabled:opacity-40"
              onClick={() =>
                commit(
                  allVisibleSelected
                    ? selected.filter((address) => !people.some((p) => p.mailbox_address.toLowerCase() === address))
                    : [...selected, ...people.map((p) => p.mailbox_address)],
                )
              }
            >
              {allVisibleSelected ? t("clearAll") : selectedDepartment ? `${t("selectAll")} (${people.length})` : t("selectAll")}
            </button>
          </div>
          <div className="max-h-64 overflow-y-auto p-1.5">
            {directory.isLoading && <p className="p-4 text-center text-[13px] text-muted-foreground">{t("loadingContacts")}</p>}
            {!directory.isLoading && people.length === 0 && (
              <p className="p-4 text-center text-[13px] text-muted-foreground">{t("noPeopleFound")}</p>
            )}
            {people.map((person) => {
              const checked = selected.includes(person.mailbox_address.toLowerCase());
              return (
                <button
                  key={person.id}
                  type="button"
                  className={cx(
                    "flex w-full items-center gap-2.5 rounded-[7px] p-1.5 text-left transition-colors hover:bg-muted",
                    checked && "bg-primary/5",
                  )}
                  onClick={() => toggle(person)}
                >
                  <Avatar name={person.display_name || person.email} size="md" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13px] font-medium leading-4">
                      {person.display_name || person.email}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {person.mailbox_address}
                      {person.department_name ? ` · ${person.department_name}` : ""}
                    </span>
                  </span>
                  <span
                    className={cx(
                      "grid h-4 w-4 shrink-0 place-items-center rounded border",
                      checked ? "border-primary bg-primary text-white" : "border-border-strong bg-surface-elevated",
                    )}
                  >
                    {checked && <Icon name="check" className="h-3 w-3" strokeWidth={2.5} />}
                  </span>
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
