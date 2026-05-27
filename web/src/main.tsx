import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Archive, Bot, Check, CircleHelp, Database, LoaderCircle, MailCheck, MoveRight, RefreshCcw, Search, Shield, Trash2, Unlink, Wifi } from "lucide-react";
import { classifyEmails, fetchConfig, fetchEmails, fetchMonitorStatus, fetchReviewEmails, fetchReviewStats, fetchScanStatus, getGoogleAuthURL, runAction, startMonitor, startScan, stopMonitor, stopScan, updateCategories } from "./api";
import type { ActionResult, AppConfig, Category, EmailSummary, MonitorStatus, ReviewEmailPage, ReviewStats, ScanStatus } from "./types";
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

type PendingAction = {
  action: "trash" | "unsubscribe";
  ids: string[];
  confirmationToken: string;
  confirmationExpiresAt?: string;
};

type DateFilterId =
  | "last_7d"
  | "last_30d"
  | "last_90d"
  | "last_180d"
  | "last_365d"
  | "last_2y"
  | "older_1y"
  | "older_2y"
  | "after_date"
  | "before_date";

const dateFilters: { id: DateFilterId; label: string; query?: string; operator?: "after" | "before" }[] = [
  { id: "last_7d", label: "Last 7 days", query: "newer_than:7d" },
  { id: "last_30d", label: "Last 30 days", query: "newer_than:30d" },
  { id: "last_90d", label: "Last 90 days", query: "newer_than:90d" },
  { id: "last_180d", label: "Last 180 days", query: "newer_than:180d" },
  { id: "last_365d", label: "Last 365 days", query: "newer_than:365d" },
  { id: "last_2y", label: "Last 2 years", query: "newer_than:2y" },
  { id: "older_1y", label: "Older than 1 year", query: "older_than:1y" },
  { id: "older_2y", label: "Older than 2 years", query: "older_than:2y" },
  { id: "after_date", label: "After date...", operator: "after" },
  { id: "before_date", label: "Before date...", operator: "before" }
];

const tutorialStorageKey = "gmail-organizer:tutorial";

const tutorialSteps = [
  {
    target: "status",
    title: "Connection status",
    body: "These badges show whether Gmail and OpenAI are connected. The app stays local, but live mailbox cleanup needs Gmail authorized and AI features need the OpenAI key file available."
  },
  {
    target: "query",
    title: "Mailbox scope",
    body: "The date dropdown and optional extra Gmail terms create the search query. Max controls the visible page size, and scan limit controls larger background coverage without keeping everything in browser memory."
  },
  {
    target: "scan-monitor",
    title: "Scan and monitor",
    body: "Scan pages through Gmail metadata and persists classifications into SQLite. Monitor polls for incoming mail and classifies it into the same review state."
  },
  {
    target: "categorize",
    title: "Categorization",
    body: "Categorize uses fast local rules. AI Categorize sends bounded metadata chunks through OpenAI for harder messages while keeping prompts, output tokens, and retries controlled."
  },
  {
    target: "stored",
    title: "Stored review pages",
    body: "Stored reloads a category page from SQLite. Lane Load buttons do the same thing directly, which lets you work through cleanup pages without rescanning Gmail."
  },
  {
    target: "coverage",
    title: "Coverage totals",
    body: "These counts come from SQLite, not just the visible page. They show total categorized mail, remaining needs-review items, manual moves, sender rules, and reviewed count."
  },
  {
    target: "lane",
    title: "Category lanes",
    body: "Each lane shows visible emails and the stored total for that category. Click a card to select it, All selects the visible lane, and Load pulls that stored category page."
  },
  {
    target: "move",
    title: "Manual corrections",
    body: "Move selected messages into the target category. Leave Sender enabled when future messages from that sender should automatically land in the same category."
  },
  {
    target: "cleanup",
    title: "Cleanup actions",
    body: "Read marks selected messages read. Unsubscribe prepares safe review links or one-click requests. Trash moves selected messages to Gmail trash, not permanent delete."
  },
  {
    target: "confirmation",
    title: "Destructive confirmation",
    body: "Trash and one-click unsubscribe preview first. If a cleanup could change Gmail or contact an unsubscribe endpoint, the app requires confirmation before it executes."
  }
];

function App() {
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [emails, setEmails] = useState<EmailSummary[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [dateFilter, setDateFilter] = useState<DateFilterId>("last_365d");
  const [customDate, setCustomDate] = useState(localDateInputValue());
  const [query, setQuery] = useState("");
  const [max, setMax] = useState(50);
  const [scanLimit, setScanLimit] = useState(1000);
  const [targetCategory, setTargetCategory] = useState<Category>("needs_review");
  const [storedCategory, setStoredCategory] = useState<Category>("unwanted");
  const [applySenderRule, setApplySenderRule] = useState(true);
  const [useAIForJobs, setUseAIForJobs] = useState(false);
  const [source, setSource] = useState("demo");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [monitor, setMonitor] = useState<MonitorStatus | null>(null);
  const [scan, setScan] = useState<ScanStatus | null>(null);
  const [reviewStats, setReviewStats] = useState<ReviewStats | null>(null);
  const [storedPage, setStoredPage] = useState<ReviewEmailPage | null>(null);
  const [actionResults, setActionResults] = useState<ActionResult[]>([]);
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null);
  const [tutorialStep, setTutorialStep] = useState<number | null>(null);
  const [loadingScreen, setLoadingScreen] = useState<{ title: string; body: string } | null>(null);

  useEffect(() => {
    void loadConfig();
    void loadEmails();
    void refreshMonitor();
    void refreshScan();
    void refreshReviewStats();
    if (window.localStorage.getItem(tutorialStorageKey) !== "completed" && window.localStorage.getItem(tutorialStorageKey) !== "skipped") {
      setTutorialStep(0);
    }
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
  const activeQuery = useMemo(() => buildGmailQuery(dateFilter, customDate, query), [dateFilter, customDate, query]);
  const selectedDateFilter = dateFilters.find((item) => item.id === dateFilter);
  const showCustomDate = Boolean(selectedDateFilter?.operator);
  const activeTutorialStep = tutorialStep === null ? null : tutorialSteps[tutorialStep];

  useEffect(() => {
    document.querySelectorAll(".tour-highlight").forEach((element) => element.classList.remove("tour-highlight"));
    if (!activeTutorialStep) {
      return;
    }
    const target = document.querySelector(`[data-tour="${activeTutorialStep.target}"]`);
    if (!target) {
      return;
    }
    target.classList.add("tour-highlight");
    target.scrollIntoView({ block: "center", inline: "nearest", behavior: "smooth" });
    return () => target.classList.remove("tour-highlight");
  }, [activeTutorialStep]);

  async function loadConfig() {
    setConfig(await fetchConfig());
  }

  async function loadEmails() {
    setBusy(true);
    try {
      const result = await fetchEmails(activeQuery, max);
      setEmails(result.emails);
      setSource(result.source);
      setSelected(new Set());
      setPendingAction(null);
      setStoredPage(null);
      setNotice(result.source === "demo" ? "Showing demo data until Gmail is authorized." : "Loaded Gmail metadata.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to load emails.");
    } finally {
      setBusy(false);
    }
  }

  async function loadStoredEmails(offset = 0, category = storedCategory) {
    setBusy(true);
    try {
      const result = await fetchReviewEmails(category, max, offset);
      setEmails(result.emails);
      setSource(result.source);
      setStoredPage(result);
      setSelected(new Set());
      setPendingAction(null);
      setNotice(`Loaded ${result.emails.length} of ${result.total} stored ${categoryLabel(category)} email(s).`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Failed to load stored review emails.");
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
    if (useAI) {
      setLoadingScreen({
        title: "AI categorization running",
        body: `Reviewing ${emails.length} visible email(s) with bounded OpenAI chunks. This can take a moment while the backend respects token and retry limits.`
      });
    }
    try {
      const result = await classifyEmails(emails, useAI);
      setEmails(result.emails);
      await refreshReviewStats();
      setNotice(useAI ? "AI categorization finished." : "Local categorization finished.");
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Classification failed.");
    } finally {
      setLoadingScreen(null);
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
      const status = monitor?.running ? await stopMonitor() : await startMonitor(activeQuery, max, useAIForJobs);
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
      const status = scan?.running ? await stopScan() : await startScan(activeQuery, scanLimit, Math.min(max, 200), useAIForJobs);
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
    setPendingAction(null);
    setBusy(true);
    try {
      const result = await runAction(action, ids);
      setActionResults(result.results);
      if (result.requiresConfirmation && (action === "trash" || action === "unsubscribe")) {
        if (!result.confirmationToken) {
          throw new Error("Server did not return a confirmation token.");
        }
        setPendingAction({ action, ids, confirmationToken: result.confirmationToken, confirmationExpiresAt: result.confirmationExpiresAt });
        setNotice(`${result.results.length} action preview(s). Confirm to execute ${actionLabel(action)}.`);
      } else {
        finishAction(action, result.results);
      }
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Bulk action failed.");
    } finally {
      setBusy(false);
    }
  }

  async function confirmPendingAction() {
    if (!pendingAction) {
      return;
    }
    setBusy(true);
    try {
      const result = await runAction(pendingAction.action, pendingAction.ids, pendingAction.confirmationToken);
      setActionResults(result.results);
      finishAction(pendingAction.action, result.results);
      setPendingAction(null);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Bulk action failed.");
    } finally {
      setBusy(false);
    }
  }

  function cancelPendingAction() {
    setPendingAction(null);
    setActionResults([]);
    setNotice("Pending cleanup action cancelled.");
  }

  function finishAction(action: "trash" | "mark_read" | "unsubscribe", results: ActionResult[]) {
    const preparedLinks = results.filter((item) => item.safeLink);
    const executed = results.filter((item) => item.status === "unsubscribed");
    setNotice(`${results.length} action result(s). ${executed.length ? `${executed.length} one-click unsubscribe request(s) accepted. ` : ""}${preparedLinks.length ? "Review links are ready below." : ""}`);
    if (action === "trash") {
      const trashed = new Set(results.filter((item) => item.status === "trashed").map((item) => item.emailId));
      setEmails((current) => current.filter((email) => !trashed.has(email.id)));
      setSelected((current) => new Set(Array.from(current).filter((id) => !trashed.has(id))));
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

  function restartTutorial() {
    window.localStorage.removeItem(tutorialStorageKey);
    setTutorialStep(0);
  }

  function skipTutorial() {
    window.localStorage.setItem(tutorialStorageKey, "skipped");
    setTutorialStep(null);
  }

  function nextTutorialStep() {
    if (tutorialStep === null) {
      return;
    }
    if (tutorialStep >= tutorialSteps.length - 1) {
      window.localStorage.setItem(tutorialStorageKey, "completed");
      setTutorialStep(null);
      return;
    }
    setTutorialStep(tutorialStep + 1);
  }

  return (
    <main className="app-shell">
      <section className="topbar">
        <div>
          <h1>Gmail Organizer</h1>
          <p>{sourceLabel(source)} - {emails.length} emails - {selected.size} selected</p>
        </div>
        <div className="status-row">
          <button className="tutorial-button" onClick={restartTutorial} title="Restart the guided dashboard tutorial"><CircleHelp size={16} />Tutorial</button>
          <span className="status-cluster" data-tour="status">
            <Status label="Gmail" active={Boolean(config?.gmailAuthenticated)} />
            <Status label="OpenAI" active={Boolean(config?.openAIKey.exists && config.openAIEnabled)} />
            <Status label="Local" active />
          </span>
        </div>
      </section>

      <section className="toolbar">
        <div className="query-group" data-tour="query">
          <select className="date-filter" value={dateFilter} onChange={(event) => setDateFilter(event.target.value as DateFilterId)} aria-label="Date filter" title="Choose a Gmail date search window">
            {dateFilters.map((filter) => <option key={filter.id} value={filter.id}>{filter.label}</option>)}
          </select>
          {showCustomDate && (
            <input className="date-input" type="date" value={customDate} onChange={(event) => setCustomDate(event.target.value)} aria-label="Custom date" title={`Gmail ${selectedDateFilter?.operator}: date`} />
          )}
          <input className="query-input" value={query} onChange={(event) => setQuery(event.target.value)} aria-label="Extra Gmail search terms" placeholder="Extra terms, optional" title="Optional Gmail operators, such as category:promotions, from:name@example.com, is:unread, or has:attachment" />
          <span className="query-preview" title="Generated Gmail query">{activeQuery}</span>
          <input className="max-input" type="number" min={1} max={200} value={max} onChange={(event) => setMax(Number(event.target.value))} aria-label="Max emails" title="Number of emails to load into the visible page" />
          <input className="scan-input" type="number" min={100} max={10000} value={scanLimit} onChange={(event) => setScanLimit(Number(event.target.value))} aria-label="Scan limit" title="Maximum emails for the background scan to classify and store" />
          <button onClick={loadEmails} disabled={busy} title="Reload the visible page from Gmail or stored review state"><RefreshCcw size={16} />Refresh</button>
          <span className="inline-toolset" data-tour="scan-monitor">
            <button className={scan?.running ? "monitoring" : ""} onClick={toggleScan} disabled={busy} title="Start or stop a larger Gmail scan that stores classifications in SQLite"><Search size={16} />Scan</button>
            <button className={monitor?.running ? "monitoring" : ""} onClick={toggleMonitor} disabled={busy} title="Start or stop polling for new Gmail messages"><Wifi size={16} />Monitor</button>
            <label className="inline-toggle" title="Use OpenAI for scan and monitor jobs when enabled">
              <input type="checkbox" checked={useAIForJobs} onChange={(event) => setUseAIForJobs(event.target.checked)} />
              AI jobs
            </label>
          </span>
          <button onClick={authorizeGmail} title="Open the Gmail OAuth authorization flow"><Shield size={16} />Authorize</button>
        </div>
        <div className="action-group">
          <span className="inline-toolset" data-tour="categorize">
            <button onClick={() => classify(false)} disabled={busy || emails.length === 0} title="Categorize the visible emails with local rules"><Archive size={16} />Categorize</button>
            <button onClick={() => classify(true)} disabled={busy || emails.length === 0} title="Categorize the visible emails with OpenAI using bounded chunks"><Bot size={16} />AI Categorize</button>
          </span>
          <select value={targetCategory} onChange={(event) => setTargetCategory(event.target.value as Category)} aria-label="Target category" title="Category to use when moving selected emails">
            {categories.map((category) => <option key={category.id} value={category.id}>{category.label}</option>)}
          </select>
          <label className="inline-toggle" title="Save a sender rule with the manual move">
            <input type="checkbox" checked={applySenderRule} onChange={(event) => setApplySenderRule(event.target.checked)} />
            Sender
          </label>
          <button data-tour="move" onClick={moveSelected} disabled={busy || selected.size === 0} title="Move selected emails into the chosen local category"><MoveRight size={16} />Move</button>
          <span className="inline-toolset" data-tour="stored">
            <select value={storedCategory} onChange={(event) => setStoredCategory(event.target.value as Category)} aria-label="Stored category" title="Stored category page to load from SQLite">
              {categories.map((category) => <option key={category.id} value={category.id}>{category.label}</option>)}
            </select>
            <button onClick={() => loadStoredEmails()} disabled={busy} title="Load the selected category from SQLite review state"><Database size={16} />Stored</button>
          </span>
          <span className="inline-toolset" data-tour="cleanup">
            <button onClick={() => bulk("mark_read")} disabled={busy} title="Mark selected Gmail messages as read"><MailCheck size={16} />Read</button>
            <button onClick={() => bulk("unsubscribe")} disabled={busy} title="Preview unsubscribe options for selected messages"><Unlink size={16} />Unsubscribe</button>
            <button className="danger" onClick={() => bulk("trash")} disabled={busy} title="Preview moving selected messages to Gmail trash"><Trash2 size={16} />Trash</button>
          </span>
        </div>
      </section>

      {notice && <div className="notice">{notice}</div>}

      {monitor && (
        <section className="monitor-panel">
          <span>{monitor.running ? "Monitoring on" : "Monitoring off"}</span>
          <span>{monitor.useAI ? "AI" : "Local"} classify</span>
          <span>{monitor.cacheSize}/{monitor.cacheLimit} cached</span>
          <span>{monitor.intervalSeconds}s interval</span>
          {monitor.lastSuccessAt && <span>Last success {new Date(monitor.lastSuccessAt).toLocaleTimeString()}</span>}
          {monitor.lastError && <span className="monitor-error">{monitor.lastError}</span>}
        </section>
      )}

      {scan && (
        <section className="monitor-panel">
          <span>{scan.running ? "Scan running" : scan.completed ? "Scan complete" : "Scan idle"}</span>
          <span>{scan.useAI ? "AI" : "Local"} classify</span>
          <span>{scan.processed}/{scan.limit} processed</span>
          <span>{scan.batchSize} batch</span>
          <span>{scan.cacheSize}/{scan.cacheLimit} cached</span>
          {scan.hasMore && <span>More available</span>}
          {scan.lastError && <span className="monitor-error">{scan.lastError}</span>}
        </section>
      )}

      {storedPage && source === "review_store" && (
        <section className="monitor-panel">
          <span>Stored {categoryLabel(storedCategory)}</span>
          <span>{Math.min(storedPage.offset + storedPage.emails.length, storedPage.total)}/{storedPage.total} loaded</span>
          <button onClick={() => loadStoredEmails(Math.max(0, storedPage.offset - storedPage.limit))} disabled={busy || storedPage.offset === 0} title="Load the previous stored page">Previous</button>
          <button onClick={() => loadStoredEmails(storedPage.offset + storedPage.limit)} disabled={busy || storedPage.offset + storedPage.limit >= storedPage.total} title="Load the next stored page">Next</button>
        </section>
      )}

      {reviewStats && (
        <section className="coverage-panel" data-tour="coverage">
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
        <section className="action-results" data-tour="confirmation">
          {pendingAction && (
            <div className="confirm-row">
              <strong>{actionLabel(pendingAction.action)} pending</strong>
              <span>{pendingAction.ids.length} email(s){pendingAction.confirmationExpiresAt ? ` until ${new Date(pendingAction.confirmationExpiresAt).toLocaleTimeString()}` : ""}</span>
              <button className="danger" onClick={confirmPendingAction} disabled={busy} title="Execute the previewed cleanup action">Confirm</button>
              <button onClick={cancelPendingAction} disabled={busy} title="Cancel the pending cleanup action">Cancel</button>
            </div>
          )}
          {actionResults.map((result, index) => (
            <div key={`${result.emailId}-${result.status}-${index}`} className="action-result">
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
            storedTotal={reviewStats?.byCategory[category.id] ?? 0}
            tourTarget={category.id === "needs_review" ? "lane" : undefined}
            selected={selected}
            onToggle={toggle}
            onSelectAll={() => selectLane(category.id)}
            onLoadStored={() => {
              setStoredCategory(category.id);
              void loadStoredEmails(0, category.id);
            }}
          />
        ))}
      </section>
      {activeTutorialStep && (
        <TutorialOverlay
          step={activeTutorialStep}
          index={tutorialStep ?? 0}
          total={tutorialSteps.length}
          onNext={nextTutorialStep}
          onSkip={skipTutorial}
        />
      )}
      {loadingScreen && <LoadingOverlay title={loadingScreen.title} body={loadingScreen.body} />}
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

function localDateInputValue() {
  const now = new Date();
  const offsetMs = now.getTimezoneOffset() * 60 * 1000;
  return new Date(now.getTime() - offsetMs).toISOString().slice(0, 10);
}

function buildGmailQuery(dateFilter: DateFilterId, customDate: string, extraTerms: string) {
  const filter = dateFilters.find((item) => item.id === dateFilter);
  const dateQuery = filter?.operator && customDate
    ? `${filter.operator}:${customDate.replace(/-/g, "/")}`
    : filter?.query ?? "newer_than:365d";
  return [dateQuery, extraTerms.trim()].filter(Boolean).join(" ");
}

function sourceLabel(source: string) {
  if (source === "gmail") {
    return "Live Gmail metadata";
  }
  if (source === "review_store") {
    return "Stored review state";
  }
  return "Demo mailbox";
}

function actionLabel(action: "trash" | "mark_read" | "unsubscribe") {
  if (action === "trash") {
    return "trash";
  }
  if (action === "unsubscribe") {
    return "unsubscribe";
  }
  return "mark read";
}

function TutorialOverlay({ step, index, total, onNext, onSkip }: { step: { title: string; body: string }; index: number; total: number; onNext: () => void; onSkip: () => void }) {
  return (
    <>
      <div className="tour-backdrop" />
      <aside className="tour-card" role="dialog" aria-live="polite" aria-label="Dashboard tutorial">
        <span>{index + 1} of {total}</span>
        <h2>{step.title}</h2>
        <p>{step.body}</p>
        <div>
          <button onClick={onSkip}>Skip</button>
          <button onClick={onNext}>{index === total - 1 ? "Finish" : "Next"}</button>
        </div>
      </aside>
    </>
  );
}

function LoadingOverlay({ title, body }: { title: string; body: string }) {
  return (
    <div className="loading-backdrop" role="status" aria-live="polite" aria-label={title}>
      <div className="loading-card">
        <LoaderCircle className="loading-spinner" size={28} />
        <div>
          <h2>{title}</h2>
          <p>{body}</p>
        </div>
      </div>
    </div>
  );
}

function Lane({ label, emails, storedTotal, tourTarget, selected, onToggle, onSelectAll, onLoadStored }: { label: string; emails: EmailSummary[]; storedTotal: number; tourTarget?: string; selected: Set<string>; onToggle: (id: string) => void; onSelectAll: () => void; onLoadStored: () => void }) {
  return (
    <div className="lane" data-tour={tourTarget}>
      <header>
        <h2>{label}</h2>
        <div className="lane-actions">
          <button onClick={onSelectAll} disabled={emails.length === 0} title="Select visible emails">All</button>
          <button onClick={onLoadStored} disabled={storedTotal === 0} title="Load stored emails">Load</button>
          <span title={`${storedTotal} stored`}>{emails.length}/{storedTotal}</span>
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
