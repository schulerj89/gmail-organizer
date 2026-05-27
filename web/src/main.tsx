import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Archive, Bot, Check, MailCheck, MoveRight, RefreshCcw, Search, Shield, Trash2, Unlink, Wifi } from "lucide-react";
import { classifyEmails, fetchConfig, fetchEmails, fetchMonitorStatus, fetchReviewStats, fetchScanStatus, getGoogleAuthURL, runAction, startMonitor, startScan, stopMonitor, stopScan, updateCategories } from "./api";
import type { ActionResult, AppConfig, Category, EmailSummary, MonitorStatus, ReviewStats, ScanStatus } from "./types";
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
  const [scanLimit, setScanLimit] = useState(1000);
  const [targetCategory, setTargetCategory] = useState<Category>("needs_review");
  const [applySenderRule, setApplySenderRule] = useState(true);
  const [source, setSource] = useState("demo");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [monitor, setMonitor] = useState<MonitorStatus | null>(null);
  const [scan, setScan] = useState<ScanStatus | null>(null);
  const [reviewStats, setReviewStats] = useState<ReviewStats | null>(null);
  const [actionResults, setActionResults] = useState<ActionResult[]>([]);

  useEffect(() => {
    void loadConfig();
    void loadEmails();
    void refreshMonitor();
    void refreshScan();
    void refreshReviewStats();
  }, []);

  useEffect(() => {
    const interval = window.setInterval(() => {
      void refreshMonitor();
      void refreshScan();
      void refreshReviewStats();
    }, 5000);
    return () => window.clearInterval(interval);
  }, []);

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

  async function refreshMonitor() {
    try {
      const status = await fetchMonitorStatus();
      setMonitor(status);
      if (status.running && status.emails.length > 0) {
        setEmails(status.emails);
        setSource(status.source || "monitor");
      }
    } catch {
      // Monitoring status should not interrupt inbox review.
    }
  }

  async function refreshScan() {
    try {
      const status = await fetchScanStatus();
      setScan(status);
      if ((status.running || status.completed) && status.emails.length > 0) {
        setEmails(status.emails);
        setSource(status.source || "scan");
      }
    } catch {
      // Scan status should not interrupt inbox review.
    }
  }

  async function refreshReviewStats() {
    try {
      setReviewStats(await fetchReviewStats());
    } catch {
      // Review stats should not interrupt inbox review.
    }
  }

  async function classify(useAI: boolean) {
    setBusy(true);
    try {
      const result = await classifyEmails(emails, useAI);
      setEmails(result.emails);
      await refreshReviewStats();
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

  async function toggleMonitor() {
    setBusy(true);
    try {
      const status = monitor?.running ? await stopMonitor() : await startMonitor(query, max, false);
      setMonitor(status);
      if (status.emails.length > 0) {
        setEmails(status.emails);
        setSource(status.source || "monitor");
      }
      setNotice(status.running ? "Backend monitoring is running." : "Backend monitoring stopped.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Monitoring update failed.");
    } finally {
      setBusy(false);
    }
  }

  async function toggleScan() {
    setBusy(true);
    try {
      const status = scan?.running ? await stopScan() : await startScan(query, scanLimit, Math.min(max, 200), false);
      setScan(status);
      if (status.emails.length > 0) {
        setEmails(status.emails);
        setSource(status.source || "scan");
      }
      setNotice(status.running ? "Mailbox scan started." : "Mailbox scan stopped.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Scan update failed.");
    } finally {
      setBusy(false);
    }
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
      setActionResults(result.results);
      const preparedLinks = result.results.filter((item) => item.safeLink);
      const executed = result.results.filter((item) => item.status === "unsubscribed");
      setNotice(`${result.results.length} action result(s). ${executed.length ? `${executed.length} one-click unsubscribe request(s) accepted. ` : ""}${preparedLinks.length ? "Review links are ready below." : ""}`);
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

  async function moveSelected() {
    const ids = Array.from(selected);
    if (ids.length === 0) {
      setNotice("Select at least one email first.");
      return;
    }
    setBusy(true);
    try {
      const result = await updateCategories(ids, targetCategory, applySenderRule);
      setEmails(result.emails);
      setSelected(new Set());
      await refreshReviewStats();
      setNotice(`${ids.length} email(s) moved to ${categoryLabel(targetCategory)}.${applySenderRule ? " Sender rule saved." : ""}`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Category update failed.");
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

  function selectLane(category: Category) {
    const laneIds = emails.filter((email) => email.category === category).map((email) => email.id);
    setSelected((current) => {
      const next = new Set(current);
      laneIds.forEach((id) => next.add(id));
      return next;
    });
  }

  return (
    <main className="app-shell">
      <section className="topbar">
        <div>
          <h1>Gmail Organizer</h1>
          <p>{source === "gmail" ? "Live Gmail metadata" : "Demo mailbox"} - {emails.length} emails - {selected.size} selected</p>
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
          <input className="scan-input" type="number" min={100} max={10000} value={scanLimit} onChange={(event) => setScanLimit(Number(event.target.value))} aria-label="Scan limit" />
          <button onClick={loadEmails} disabled={busy}><RefreshCcw size={16} />Refresh</button>
          <button className={scan?.running ? "monitoring" : ""} onClick={toggleScan} disabled={busy}><Search size={16} />Scan</button>
          <button className={monitor?.running ? "monitoring" : ""} onClick={toggleMonitor} disabled={busy}><Wifi size={16} />Monitor</button>
          <button onClick={authorizeGmail}><Shield size={16} />Authorize</button>
        </div>
        <div className="action-group">
          <button onClick={() => classify(false)} disabled={busy || emails.length === 0}><Archive size={16} />Categorize</button>
          <button onClick={() => classify(true)} disabled={busy || emails.length === 0}><Bot size={16} />AI Categorize</button>
          <select value={targetCategory} onChange={(event) => setTargetCategory(event.target.value as Category)} aria-label="Target category">
            {categories.map((category) => <option key={category.id} value={category.id}>{category.label}</option>)}
          </select>
          <label className="inline-toggle">
            <input type="checkbox" checked={applySenderRule} onChange={(event) => setApplySenderRule(event.target.checked)} />
            Sender
          </label>
          <button onClick={moveSelected} disabled={busy || selected.size === 0}><MoveRight size={16} />Move</button>
          <button onClick={() => bulk("mark_read")} disabled={busy}><MailCheck size={16} />Read</button>
          <button onClick={() => bulk("unsubscribe")} disabled={busy}><Unlink size={16} />Unsubscribe</button>
          <button className="danger" onClick={() => bulk("trash")} disabled={busy}><Trash2 size={16} />Trash</button>
        </div>
      </section>

      {notice && <div className="notice">{notice}</div>}

      {monitor && (
        <section className="monitor-panel">
          <span>{monitor.running ? "Monitoring on" : "Monitoring off"}</span>
          <span>{monitor.cacheSize}/{monitor.cacheLimit} cached</span>
          <span>{monitor.intervalSeconds}s interval</span>
          {monitor.lastSuccessAt && <span>Last success {new Date(monitor.lastSuccessAt).toLocaleTimeString()}</span>}
          {monitor.lastError && <span className="monitor-error">{monitor.lastError}</span>}
        </section>
      )}

      {scan && (
        <section className="monitor-panel">
          <span>{scan.running ? "Scan running" : scan.completed ? "Scan complete" : "Scan idle"}</span>
          <span>{scan.processed}/{scan.limit} processed</span>
          <span>{scan.batchSize} batch</span>
          <span>{scan.cacheSize}/{scan.cacheLimit} cached</span>
          {scan.hasMore && <span>More available</span>}
          {scan.lastError && <span className="monitor-error">{scan.lastError}</span>}
        </section>
      )}

      {reviewStats && (
        <section className="coverage-panel">
          <div>
            <span>Persisted review state</span>
            <strong>{reviewStats.total}</strong>
          </div>
          <div>
            <span>Needs review</span>
            <strong>{reviewStats.needsReview}</strong>
          </div>
          <div>
            <span>Manual moves</span>
            <strong>{reviewStats.manual}</strong>
          </div>
          <div>
            <span>Sender rules</span>
            <strong>{reviewStats.senderRules}</strong>
          </div>
          <div>
            <span>Reviewed</span>
            <strong>{Math.max(0, reviewStats.total - reviewStats.needsReview)}</strong>
          </div>
        </section>
      )}

      {actionResults.length > 0 && (
        <section className="action-results">
          {actionResults.map((result) => (
            <div key={`${result.emailId}-${result.status}`} className="action-result">
              <strong>{result.status}</strong>
              <span>{result.message || result.emailId}</span>
              {result.safeLink && <a href={result.safeLink} target="_blank" rel="noreferrer">Open review link</a>}
            </div>
          ))}
        </section>
      )}

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
            onSelectAll={() => selectLane(category.id)}
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

function categoryLabel(category: Category) {
  return categories.find((item) => item.id === category)?.label ?? category;
}

function Lane({ label, emails, selected, onToggle, onSelectAll }: { label: string; emails: EmailSummary[]; selected: Set<string>; onToggle: (id: string) => void; onSelectAll: () => void }) {
  return (
    <div className="lane">
      <header>
        <h2>{label}</h2>
        <div className="lane-actions">
          <button onClick={onSelectAll} disabled={emails.length === 0}>All</button>
          <span>{emails.length}</span>
        </div>
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
            {email.hasUnsubscribe && <span className="tag">{email.canAutoUnsubscribe ? "one-click" : "unsubscribe"}</span>}
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
