import type { ActionResult, AppConfig, EmailSummary, MonitorStatus } from "./types";

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: response.statusText }));
    throw new Error(body.error ?? response.statusText);
  }
  return response.json() as Promise<T>;
}

export async function fetchConfig() {
  return request<AppConfig>("/api/config");
}

export async function fetchEmails(query: string, max: number) {
  return request<{ source: string; emails: EmailSummary[] }>(
    `/api/emails?query=${encodeURIComponent(query)}&max=${max}`
  );
}

export async function classifyEmails(emails: EmailSummary[], useAI: boolean) {
  return request<{ emails: EmailSummary[] }>("/api/classify", {
    method: "POST",
    body: JSON.stringify({ emails, useAI })
  });
}

export async function runAction(action: "trash" | "mark_read" | "unsubscribe", ids: string[]) {
  return request<{ results: ActionResult[] }>("/api/actions", {
    method: "POST",
    body: JSON.stringify({ action, ids })
  });
}

export async function getGoogleAuthURL() {
  return request<{ url: string }>("/api/auth/google/url");
}

export async function fetchMonitorStatus() {
  return request<MonitorStatus>("/api/monitor");
}

export async function startMonitor(query: string, max: number, useAI: boolean) {
  return request<MonitorStatus>("/api/monitor/start", {
    method: "POST",
    body: JSON.stringify({ query, max, useAI })
  });
}

export async function stopMonitor() {
  return request<MonitorStatus>("/api/monitor/stop", {
    method: "POST",
    body: "{}"
  });
}
