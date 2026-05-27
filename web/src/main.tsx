import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Archive, Bot, Check, MailCheck, RefreshCcw, Shield, Trash2, Unlink, Wifi } from "lucide-react";
import { classifyEmails, fetchConfig, fetchEmails, getGoogleAuthURL, runAction } from "./api";
import type { AppConfig, Category, EmailSummary } from "./types";
import "./styles.css";

const categories: { id: Category; label: string }[] = [
  { id: "needs_review", label: "Needs review" },
  { id: "promotions", label: "Promotions" },
  { id: "newsletters", label: "Newsletters" },
  { id: "receipts", label: "Receipts" },
  { id: "security", label: "Security" },
  { id: "finance", label: "Finance" },
  { id: "work", label: "Work" },
  { id: "travel", label: "Travel" },
  { id: "social", label: "Social" },
  { id: "unwanted", label: "Unwanted" }
];

function App() {
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [emails, setEmails] = useState<EmailSummary[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [query, setQuery] = useState("newer_than:365d");
  const [max, setMax] = useState(50);
  const [source, setSource] = useState("demo");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [monitoring, setMonitoring] = useState(false);

  useEffect(() => {
    void loadConfig();
    void loadEmails();
  }, []);

  useEffect(() => {
    if (!monitoring) {
      return;
    }
    const interval = window.setInterval(() => {
      void loadEmails();
    }, 60000);
    return () => window.clearInterval(interval);
  }, [monitoring, query, max]);

  const grouped = useMemo(() => {
    const map = new Map<Category, EmailSummary[]>();
    categories.forEach((category) => map.set(category.id, []));
    emails.forEach((email) => {
      map.get(email.category)?.push(email);
    });
    return map;
  }, [emails]);

  const selectedEmails = useMemo(() => emails.filter((email) => selected.has(email.id)), [emails, selected]);

  async function loadConfig() {
    setConfig(await fetchConfig());
  }

  async function loadEmails() {
    setBusy(true);
    try {
      const result = await fetchEmails(query, max);
      setEmails(result.emails);
      setSource(result.source);
      setSelected(new Set());
      setNotice(result.source === "demo" ? "Showing demo data until Gmail is authorized." : "Loaded Gmail metadata.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to load emails.");
    } finally {
      setBusy(false);
    }
  }

  async function classify(useAI: boolean) {
    setBusy(true);
    try {
      const result = await classifyEmails(emails, useAI);
      setEmails(result.emails);
      setNotice(useAI ? "AI categorization finished." : "Local categorization finished.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Classification failed.");
    } finally {
      setBusy(false);
    }
  }

  async function authorizeGmail() {
    const { url } = await getGoogleAuthURL();
    window.open(url, "_blank", "noopener,noreferrer");
  }

  async function bulk(action: "trash" | "mark_read" | "unsubscribe") {
    const ids = Array.from(selected);
    if (ids.length === 0) {
      setNotice("Select at least one email first.");
      return;
    }
    setBusy(true);
    try {
      const result = await runAction(action, ids);
      const preparedLinks = result.results.filter((item) => item.safeLink);
      setNotice(`${result.results.length} action result(s). ${preparedLinks.length ? "Unsubscribe links are ready for review." : ""}`);
      if (action === "trash") {
        setEmails((current) => current.filter((email) => !selected.has(email.id)));
        setSelected(new Set());
      }
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Bulk action failed.");
    } finally {
      setBusy(false);
    }
  }

  function toggle(id: string) {
    setSelected((current) => {
      const next = new Set(current);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  return (
    <main className="app-shell">
      <section className="topbar">
        <div>
          <h1>Gmail Organizer</h1>
          <p>{source === "gmail" ? "Live Gmail metadata" : "Demo mailbox"} · {emails.length} emails · {selected.size} selected</p>
        </div>
        <div className="status-row">
          <Status label="Gmail" active={Boolean(config?.gmailAuthenticated)} />
          <Status label="OpenAI" active={Boolean(config?.openAIKey.exists && config.openAIEnabled)} />
          <Status label="Local" active />
        </div>
      </section>

      <section className="toolbar">
        <div className="query-group">
          <input value={query} onChange={(event) => setQuery(event.target.value)} aria-label="Gmail query" />
          <input className="max-input" type="number" min={1} max={200} value={max} onChange={(event) => setMax(Number(event.target.value))} aria-label="Max emails" />
          <button onClick={loadEmails} disabled={busy}><RefreshCcw size={16} />Refresh</button>
          <button className={monitoring ? "monitoring" : ""} onClick={() => setMonitoring((current) => !current)}><Wifi size={16} />Monitor</button>
          <button onClick={authorizeGmail}><Shield size={16} />Authorize</button>
        </div>
        <div className="action-group">
          <button onClick={() => classify(false)} disabled={busy || emails.length === 0}><Archive size={16} />Categorize</button>
          <button onClick={() => classify(true)} disabled={busy || emails.length === 0}><Bot size={16} />AI Categorize</button>
          <button onClick={() => bulk("mark_read")} disabled={busy}><MailCheck size={16} />Read</button>
          <button onClick={() => bulk("unsubscribe")} disabled={busy}><Unlink size={16} />Unsubscribe</button>
          <button className="danger" onClick={() => bulk("trash")} disabled={busy}><Trash2 size={16} />Trash</button>
        </div>
      </section>

      {notice && <div className="notice">{notice}</div>}

      <section className="metrics">
        <Metric label="Selected" value={selectedEmails.length} />
        <Metric label="Unsubscribe" value={emails.filter((email) => email.hasUnsubscribe).length} />
        <Metric label="Needs review" value={grouped.get("needs_review")?.length ?? 0} />
        <Metric label="High confidence" value={emails.filter((email) => email.confidence >= 0.75).length} />
      </section>

      <section className="lanes">
        {categories.map((category) => (
          <Lane
            key={category.id}
            label={category.label}
            emails={grouped.get(category.id) ?? []}
            selected={selected}
            onToggle={toggle}
          />
        ))}
      </section>
    </main>
  );
}

function Status({ label, active }: { label: string; active: boolean }) {
  return <span className={active ? "status active" : "status"}><Check size={14} />{label}</span>;
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="metric">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function Lane({ label, emails, selected, onToggle }: { label: string; emails: EmailSummary[]; selected: Set<string>; onToggle: (id: string) => void }) {
  return (
    <div className="lane">
      <header>
        <h2>{label}</h2>
        <span>{emails.length}</span>
      </header>
      <div className="cards">
        {emails.map((email) => (
          <button key={email.id} className={selected.has(email.id) ? "email-card selected" : "email-card"} onClick={() => onToggle(email.id)}>
            <span className="card-top">
              <strong>{email.subject || "(No subject)"}</strong>
              <span>{Math.round(email.confidence * 100)}%</span>
            </span>
            <span className="from">{email.from}</span>
            <span className="snippet">{email.snippet}</span>
            <span className="reason">{email.reason}</span>
            {email.hasUnsubscribe && <span className="tag">unsubscribe</span>}
          </button>
        ))}
      </div>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
