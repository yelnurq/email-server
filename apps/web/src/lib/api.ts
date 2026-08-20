// API client: thin fetch wrapper over the Go backend with cookie sessions
// and the unified error envelope.

export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  code: string;
  status: number;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    credentials: "include",
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  let body: unknown;
  try {
    body = await res.json();
  } catch {
    body = undefined;
  }
  if (!res.ok) {
    const err = (body as { error?: { code?: string; message?: string } })?.error;
    throw new ApiError(res.status, err?.code ?? "UNKNOWN", err?.message ?? `Request failed (${res.status})`);
  }
  return body as T;
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "POST", body: body === undefined ? undefined : JSON.stringify(body) }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "PUT", body: JSON.stringify(body) }),
  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: "PATCH", body: JSON.stringify(body) }),
  delete: <T>(path: string) => request<T>(path, { method: "DELETE" }),
};

// ---- Types mirrored from the backend ----

export type RoleGrant = { role: string; scope_type: string; scope_id?: string };

export type Me = {
  id: string;
  email: string;
  display_name: string;
  tenant_id: string;
  organization_id?: string;
  roles: RoleGrant[];
  permissions: string[];
};

export type Folder = {
  id: string;
  name: string;
  type: string;
  unread: number;
  total: number;
};

export type MailSummary = {
  mailbox: { id: string; address: string };
  folders: Folder[];
};

export type MessageListItem = {
  id: string;
  message_id: string;
  from: string;
  from_display: string;
  subject: string;
  snippet: string;
  date: string;
  is_read: boolean;
  is_starred: boolean;
  has_attachments: boolean;
};

export type MessageList = {
  messages: MessageListItem[];
  total: number;
  limit: number;
  offset: number;
};

export type Recipient = { kind: "to" | "cc" | "bcc"; address: string };

export type ThreadItem = { id: string; subject: string; from: string; date: string };

export type Attachment = {
  id: string;
  filename: string;
  content_type: string;
  size_bytes: number;
};

export type MessageDetail = {
  id: string;
  message_id: string;
  folder: string;
  from: string;
  from_display: string;
  recipients: Recipient[];
  subject: string;
  body_text: string;
  body_html: string;
  date: string;
  is_read: boolean;
  is_starred: boolean;
  has_attachments: boolean;
  attachments: Attachment[];
  thread: ThreadItem[];
};

// uploadAttachment posts a file with progress callbacks (XHR: fetch has no
// upload progress). Resolves with the staged attachment metadata.
export function uploadAttachment(
  file: File,
  onProgress: (fraction: number) => void,
): Promise<Attachment> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API_URL}/api/v1/mail/attachments`);
    xhr.withCredentials = true;
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded / e.total);
    };
    xhr.onload = () => {
      try {
        const body = JSON.parse(xhr.responseText);
        if (xhr.status >= 200 && xhr.status < 300) resolve(body as Attachment);
        else reject(new ApiError(xhr.status, body?.error?.code ?? "UNKNOWN", body?.error?.message ?? "Upload failed"));
      } catch {
        reject(new ApiError(xhr.status, "UNKNOWN", "Upload failed"));
      }
    };
    xhr.onerror = () => reject(new ApiError(0, "NETWORK", "Upload failed"));
    const form = new FormData();
    form.append("file", file);
    xhr.send(form);
  });
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export type Organization = { id: string; name: string; slug: string; status: string; created_at: string };
export type Domain = {
  id: string; organization_id: string; name: string; status: string;
  verification_mode: string; created_at: string;
};
export type AdminUser = {
  id: string; organization_id?: string; email: string; display_name: string;
  status: string; mailbox_address?: string; created_at: string;
};
export type Mailbox = {
  id: string; organization_id: string; domain_id: string; user_id?: string; user_email?: string;
  address: string; status: string; quota_bytes: number; used_bytes: number; created_at: string;
};

export function formatDate(iso: string): string {
  // Backend returns "2026-08-20 08:49:50.801968+00".
  const d = new Date(iso.replace(" ", "T").replace("+00", "Z"));
  if (isNaN(d.getTime())) return iso;
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay) return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const sameYear = d.getFullYear() === now.getFullYear();
  return d.toLocaleDateString([], sameYear
    ? { month: "short", day: "numeric" }
    : { year: "numeric", month: "short", day: "numeric" });
}
