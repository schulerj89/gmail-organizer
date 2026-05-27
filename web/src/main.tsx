import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { Archive, Bot, Check, CircleHelp, Eye, Inbox, LoaderCircle, MailCheck, Moon, MoveRight, RefreshCcw, Search, Shield, Sun, Trash2, Unlink, Wifi, X } from "lucide-react";
import { classifyEmails, fetchConfig, fetchEmails, fetchMonitorStatus, fetchReviewEmails, fetchReviewStats, fetchScanStatus, getGoogleAuthURL, runAction, startMonitor, startScan, stopMonitor, stopScan, updateCategories } from "./api";
import type { ActionResult, AppConfig, Category, EmailSummary, MonitorStatus, ReviewEmailPage, ReviewStats, ScanStatus } from "./types";
import "./styles.css";

const categories: { id: Category; label: string }[] = [
  { id: "needs_review", label: "Review needed" },
  { id: "promotions", label: "Promotions" },
  { id: "newsletters", label: "Newsletters" },
  { id: "receipts", label: "Receipts" },
  { id: "security", label: "Security" },
  { id: "finance", label: "Finance" },
  { id: "work", label: "Work" },
  { id: "travel", label: "Travel" },
  { id: "social", label: "Social" },
  { id: "personal", label: "Personal" },
  { id: "unwanted", label: "Likely junk" }
];

type PendingAction = {
  action: "trash" | "unsubscribe";
  ids: string[];
  confirmationToken: string;
  confirmationExpiresAt?: string;
};

type QueueMode = "category" | "unsubscribe" | "cleanup" | "senders" | "ai";
type ThemeMode = "light" | "dark";

type SenderGroup = {
  key: string;
  sender: string;
  domain: string;
  emails: EmailSummary[];
  autoCount: number;
  categories: Category[];
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

type TutorialStep = {
  target: string;
  title: string;
  body: string;
  queue?: QueueMode;
};

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
const themeStorageKey = "gmail-organizer:theme";
const aiSuggestionThreshold = 0.75;

const tutorialSteps: TutorialStep[] = [
  {
    target: "status",
    title: "Connection status",
    body: "These badges show whether Gmail and OpenAI are connected. The app stays local, but live mailbox cleanup needs Gmail authorized and AI features need the OpenAI key file available."
  },
  {
    target: "query",
    title: "Choose the mailbox scope",
    body: "Use the date dropdown, calendar options, and optional search terms to build the Gmail query. Refresh loads the visible batch, while Find Emails scans and saves more matches for cleanup."
  },
  {
    target: "scan-monitor",
    title: "Advanced controls",
    body: "Advanced keeps the raw Gmail query visible when you need it. It also controls result limits, scan size, monitoring, Gmail authorization, and whether background jobs can use AI."
  },
  {
    target: "coverage",
    title: "Progress summary",
    body: "These totals come from the local SQLite review store. They show how much mail has been saved, what still needs review, and how much is ready for unsubscribe or cleanup."
  },
  {
    target: "quick-queues",
    title: "Start with a cleanup goal",
    body: "The left side is organized around jobs: unsubscribe candidates, sender cleanup, high-confidence AI suggestions, suggested cleanup, and saved category pages."
  },
  {
    target: "lane",
    title: "Sender cleanup",
    body: "Sender cleanup groups messages by sender so you can review one sample, select an entire sender, or preview unsubscribe and trash actions before anything runs.",
    queue: "senders"
  },
  {
    target: "lane",
    title: "AI suggestions queue",
    body: "AI suggestions collects high-confidence categorized messages. You can accept them in bulk, inspect rows first, or select a subset before applying the recommendation.",
    queue: "ai"
  },
  {
    target: "categorize",
    title: "AI-assisted sorting",
    body: "Sort Emails uses fast local rules. Suggest Categories uses OpenAI with bounded batches, token limits, retry handling, and progress feedback while the buttons stay disabled.",
    queue: "ai"
  },
  {
    target: "lane",
    title: "Review a message",
    body: "Open a row to see the snippet, why it was categorized, and the review decision controls. You can correct the category, save a sender rule, mark read, unsubscribe, or trash from there.",
    queue: "category"
  },
  {
    target: "lane",
    title: "Preview before cleanup",
    body: "Selecting rows reveals the bulk action bar. Move is immediate for local review state, while unsubscribe and Move to Trash open a Step 3 preview before changing Gmail or contacting a sender endpoint.",
    queue: "cleanup"
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
  const [activeCategory, setActiveCategory] = useState<Category>("needs_review");
  const [queueMode, setQueueMode] = useState<QueueMode>("category");
  const [detailEmailId, setDetailEmailId] = useState<string | null>(null);
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
  const [theme, setTheme] = useState<ThemeMode>(() => initialTheme());

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
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem(themeStorageKey, theme);
  }, [theme]);

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
  const senderGroups = useMemo(() => buildSenderGroups(emails), [emails]);
  const aiSuggestedEmails = useMemo(() => emails.filter((email) => email.category !== "needs_review" && email.confidence >= aiSuggestionThreshold), [emails]);
  const visibleEmails = useMemo(() => {
    if (queueMode === "ai") {
      return aiSuggestedEmails;
    }
    if (queueMode === "unsubscribe") {
      return emails.filter((email) => email.hasUnsubscribe);
    }
    if (queueMode === "cleanup") {
      return emails.filter((email) => ["promotions", "newsletters", "unwanted"].includes(email.category));
    }
    if (source === "review_store") {
      return emails;
    }
    return emails.filter((email) => email.category === activeCategory);
  }, [activeCategory, aiSuggestedEmails, emails, queueMode, source]);
  const activeQueueTitle = queueMode === "ai" ? "AI suggestions" : queueMode === "senders" ? "Sender cleanup" : queueMode === "unsubscribe" ? "Ready to unsubscribe" : queueMode === "cleanup" ? "Suggested cleanup" : categoryLabel(activeCategory);
  const detailEmail = useMemo(() => emails.find((email) => email.id === detailEmailId) ?? null, [detailEmailId, emails]);
  const activeQuery = useMemo(() => buildGmailQuery(dateFilter, customDate, query), [dateFilter, customDate, query]);
  const selectedDateFilter = dateFilters.find((item) => item.id === dateFilter);
  const showCustomDate = Boolean(selectedDateFilter?.operator);
  const activeTutorialStep = tutorialStep === null ? null : tutorialSteps[tutorialStep];

  useEffect(() => {
    if (!activeTutorialStep) {
      return;
    }
    setActionResults([]);
    setPendingAction(null);

    if (activeTutorialStep.queue && queueMode !== activeTutorialStep.queue) {
      setQueueMode(activeTutorialStep.queue);
      setSelected(new Set());
      setDetailEmailId(null);
      return;
    }

    setDetailEmailId(null);
    setSelected(new Set());
  }, [activeTutorialStep, queueMode]);

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
  }, [activeTutorialStep, detailEmailId, queueMode, selected.size]);

  useEffect(() => {
    if (detailEmailId && !emails.some((email) => email.id === detailEmailId)) {
      setDetailEmailId(null);
    }
  }, [detailEmailId, emails]);

  async function loadConfig() {
    setConfig(await fetchConfig());
  }

  async function loadEmails() {
    setBusy(true);
    try {
      const result = await fetchEmails(activeQuery, max);
      setEmails(result.emails);
      setSource(result.source);
      setQueueMode("category");
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
      setActiveCategory(category);
      setQueueMode("category");
      setSelected(new Set());
      setPendingAction(null);
      setNotice(`Opened ${result.emails.length} of ${result.total} saved ${categoryLabel(category)} email(s).`);
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

  async function bulk(action: "trash" | "mark_read" | "unsubscribe", overrideIds?: string[]) {
    const ids = overrideIds ?? Array.from(selected);
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
        setNotice("");
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

  function closeActionResults() {
    setPendingAction(null);
    setActionResults([]);
  }

  function finishAction(action: "trash" | "mark_read" | "unsubscribe", results: ActionResult[]) {
    const preparedLinks = results.filter((item) => item.safeLink);
    const executed = results.filter((item) => item.status === "unsubscribed");
    setNotice(`${results.length} action result(s). ${executed.length ? `${executed.length} one-click unsubscribe request(s) accepted. ` : ""}${preparedLinks.length ? "Review links are ready below." : ""}`);
    if (action === "trash") {
      const trashed = new Set(results.filter((item) => item.status === "trashed").map((item) => item.emailId));
      setEmails((current) => current.filter((email) => !trashed.has(email.id)));
      setSelected((current) => new Set(Array.from(current).filter((id) => !trashed.has(id))));
      if (detailEmailId && trashed.has(detailEmailId)) {
        setDetailEmailId(null);
      }
    }
  }

  async function moveSelected() {
    const ids = Array.from(selected);
    if (ids.length === 0) {
      setNotice("Select at least one email first.");
      return;
    }
    await moveEmails(ids, targetCategory, applySenderRule);
  }

  async function moveEmails(ids: string[], category: Category, applyRule: boolean) {
    setBusy(true);
    try {
      const result = await updateCategories(ids, category, applyRule);
      setEmails(result.emails);
      setSelected((current) => new Set(Array.from(current).filter((id) => !ids.includes(id))));
      await refreshReviewStats();
      setNotice(`${ids.length} email(s) moved to ${categoryLabel(category)}.${applyRule ? " Future emails from this sender will follow that category." : ""}`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Category update failed.");
    } finally {
      setBusy(false);
    }
  }

  async function acceptSuggestions(ids = Array.from(selected)) {
    const selectedSuggestions = emails.filter((email) => ids.includes(email.id) && email.category !== "needs_review");
    if (selectedSuggestions.length === 0) {
      setNotice("Select at least one AI suggestion first.");
      return;
    }
    const byCategory = new Map<Category, string[]>();
    selectedSuggestions.forEach((email) => {
      byCategory.set(email.category, [...(byCategory.get(email.category) ?? []), email.id]);
    });
    setBusy(true);
    try {
      let latest = emails;
      for (const [category, categoryIds] of byCategory) {
        const result = await updateCategories(categoryIds, category, false);
        latest = result.emails;
      }
      setEmails(latest);
      setSelected((current) => new Set(Array.from(current).filter((id) => !ids.includes(id))));
      await refreshReviewStats();
      setNotice(`${selectedSuggestions.length} AI suggestion(s) accepted across ${byCategory.size} categor${byCategory.size === 1 ? "y" : "ies"}.`);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : "Accepting AI suggestions failed.");
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

  function selectVisibleEmails() {
    const visibleIds = visibleEmails.map((email) => email.id);
    setSelected((current) => {
      const next = new Set(current);
      visibleIds.forEach((id) => next.add(id));
      return next;
    });
  }

  function selectEmailIds(ids: string[]) {
    setSelected((current) => {
      const next = new Set(current);
      ids.forEach((id) => next.add(id));
      return next;
    });
  }

  function clearSelected() {
    setSelected(new Set());
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
        <div className="brand-lockup">
          <img src="/logo.svg" alt="" aria-hidden="true" />
          <div>
            <h1>Gmail Organizer</h1>
            <p>{sourceLabel(source)} - {emails.length} loaded - {selected.size} selected</p>
          </div>
        </div>
        <div className="status-row">
          <button className="theme-button" onClick={() => setTheme(theme === "dark" ? "light" : "dark")} title={`Switch to ${theme === "dark" ? "light" : "dark"} mode`} aria-label={`Switch to ${theme === "dark" ? "light" : "dark"} mode`}>
            {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
            {theme === "dark" ? "Light" : "Dark"}
          </button>
          <button className="tutorial-button" onClick={restartTutorial} title="Restart the guided dashboard tutorial"><CircleHelp size={16} />Tutorial</button>
          <span className="status-cluster" data-tour="status">
            <Status label="Gmail" active={Boolean(config?.gmailAuthenticated)} />
            <Status label="OpenAI" active={Boolean(config?.openAIKey.exists && config.openAIEnabled)} />
            <Status label="Local" active />
          </span>
        </div>
      </section>

      <section className="toolbar compact-toolbar" data-tour="query">
        <div className="query-group">
          <select className="date-filter" value={dateFilter} onChange={(event) => setDateFilter(event.target.value as DateFilterId)} aria-label="Date filter" title="Choose a Gmail date search window">
            {dateFilters.map((filter) => <option key={filter.id} value={filter.id}>{filter.label}</option>)}
          </select>
          {showCustomDate && (
            <input className="date-input" type="date" value={customDate} onChange={(event) => setCustomDate(event.target.value)} aria-label="Custom date" title={`Gmail ${selectedDateFilter?.operator}: date`} />
          )}
          <input className="query-input" value={query} onChange={(event) => setQuery(event.target.value)} aria-label="Extra Gmail search terms" placeholder="Search terms, optional" title="Optional Gmail search terms such as from:name@example.com, is:unread, or has:attachment" />
          <button onClick={loadEmails} disabled={busy} title="Load matching live Gmail metadata"><RefreshCcw size={16} />Refresh</button>
          <button onClick={toggleScan} className={scan?.running ? "monitoring" : ""} disabled={busy} title="Find and save more matching emails"><Search size={16} />Find Emails</button>
        </div>
        <details className="advanced-menu" data-tour="scan-monitor">
          <summary>Advanced</summary>
          <div className="toolbar-secondary">
            <span className="query-preview" title="Generated Gmail query">{activeQuery}</span>
            <label>
              <span>Visible results</span>
              <input className="max-input" type="number" min={1} max={200} value={max} onChange={(event) => setMax(Number(event.target.value))} aria-label="Visible results" title="Visible results per page" />
            </label>
            <label>
              <span>Emails to check</span>
              <input className="scan-input" type="number" min={100} max={10000} value={scanLimit} onChange={(event) => setScanLimit(Number(event.target.value))} aria-label="Emails to check" title="Emails to check during Find Emails" />
            </label>
            <button onClick={toggleMonitor} className={monitor?.running ? "monitoring" : ""} disabled={busy} title="Start or stop checking for new mail"><Wifi size={16} />Monitor</button>
            <label className="inline-toggle" title="Use OpenAI for Find Emails and Monitor">
              <input type="checkbox" checked={useAIForJobs} onChange={(event) => setUseAIForJobs(event.target.checked)} />
              Use AI
            </label>
            <button onClick={authorizeGmail} title="Open Gmail authorization"><Shield size={16} />Authorize</button>
          </div>
        </details>
      </section>

      {notice && <div className="notice">{notice}</div>}

        <section className="progress-strip" data-tour="coverage">
        {reviewStats && (
          <>
            <Metric label="Saved emails" value={reviewStats.total} />
            <Metric label="Review needed" value={reviewStats.needsReview} />
            <Metric label="Can unsubscribe" value={emails.filter((email) => email.hasUnsubscribe).length} />
            <Metric label="Cleaned up" value={Math.max(0, reviewStats.total - reviewStats.needsReview)} />
          </>
        )}
        {scan && <span>{scan.running ? "Finding emails" : scan.completed ? "Find complete" : "Find idle"} - {scan.processed}/{scan.limit}</span>}
        {monitor && <span>{monitor.running ? "Monitoring on" : "Monitoring off"} - {monitor.useAI ? "AI" : "Local"}</span>}
      </section>

      <section className="workspace">
        <aside className="left-nav" aria-label="Cleanup sections" data-tour="quick-queues">
          <div className="nav-section-title"><Inbox size={16} />Quick cleanup</div>
          <button
            className={queueMode === "unsubscribe" ? "nav-item active" : "nav-item"}
            onClick={() => {
              setQueueMode("unsubscribe");
              setSelected(new Set());
            }}
          >
            <span>Ready to unsubscribe</span>
            <span>{emails.filter((email) => email.hasUnsubscribe).length}</span>
          </button>
          <button
            className={queueMode === "senders" ? "nav-item active" : "nav-item"}
            onClick={() => {
              setQueueMode("senders");
              setSelected(new Set());
            }}
          >
            <span>Sender cleanup</span>
            <span>{senderGroups.length}</span>
          </button>
          <button
            className={queueMode === "ai" ? "nav-item active" : "nav-item"}
            onClick={() => {
              setQueueMode("ai");
              setSelected(new Set());
            }}
          >
            <span>AI suggestions</span>
            <span>{aiSuggestedEmails.length}</span>
          </button>
          <button
            className={queueMode === "cleanup" ? "nav-item active" : "nav-item"}
            onClick={() => {
              setQueueMode("cleanup");
              setSelected(new Set());
            }}
          >
            <span>Suggested cleanup</span>
            <span>{emails.filter((email) => ["promotions", "newsletters", "unwanted"].includes(email.category)).length}</span>
          </button>
          <div className="workflow-hint">
            <strong>3-step cleanup</strong>
            <span>Pick a queue or sender, select emails, then preview and confirm.</span>
          </div>
          <div className="nav-section-title">Saved categories</div>
          {categories.map((category) => {
            const visibleCount = grouped.get(category.id)?.length ?? 0;
            const storedCount = reviewStats?.byCategory[category.id] ?? 0;
            return (
              <button
                key={category.id}
                className={queueMode === "category" && activeCategory === category.id ? "nav-item active" : "nav-item"}
                onClick={() => {
                  setStoredCategory(category.id);
                  void loadStoredEmails(0, category.id);
                }}
              >
                <span>{category.label}</span>
                <span>{source === "review_store" && activeCategory === category.id ? visibleEmails.length : visibleCount}/{storedCount}</span>
              </button>
            );
          })}
        </aside>

        <section className="mail-workbench" data-tour="lane">
          <header className="workbench-header">
            <div>
              <h2>{activeQueueTitle}</h2>
              <p>{queueMode === "ai" ? `${aiSuggestedEmails.length} high-confidence suggestions ready to accept or inspect` : queueMode === "senders" ? `${senderGroups.length} senders - ${emails.filter((email) => email.hasUnsubscribe).length} unsubscribe-ready messages` : `${visibleEmails.length} visible - ${queueMode === "category" ? `${reviewStats?.byCategory[activeCategory] ?? 0} saved - ` : ""}${sourceLabel(source)}`}</p>
            </div>
            <div className="workbench-actions" data-tour="categorize">
              <button onClick={() => classify(false)} disabled={busy || emails.length === 0} title="Sort loaded emails with local rules"><Archive size={16} />Sort Emails</button>
              <button onClick={() => classify(true)} disabled={busy || emails.length === 0} title="Use OpenAI to suggest categories"><Bot size={16} />Suggest Categories</button>
              {queueMode === "ai" && <button onClick={() => void acceptSuggestions(visibleEmails.map((email) => email.id))} disabled={busy || visibleEmails.length === 0} title="Accept every visible high-confidence suggestion"><Check size={16} />Accept Visible</button>}
              <button
                onClick={() => queueMode === "senders" ? selectEmailIds(senderGroups.flatMap((group) => group.emails.map((email) => email.id))) : selectVisibleEmails()}
                disabled={queueMode === "senders" ? senderGroups.length === 0 : visibleEmails.length === 0}
                title={queueMode === "senders" ? "Select every unsubscribe-ready message grouped by sender" : "Select all visible emails"}
              >
                <Check size={16} />{queueMode === "senders" ? "Select All Senders" : "Select Visible"}
              </button>
            </div>
          </header>

          {storedPage && source === "review_store" && queueMode === "category" && (
            <div className="page-controls">
              <span>Saved Results {Math.min(storedPage.offset + storedPage.emails.length, storedPage.total)}/{storedPage.total}</span>
              <button onClick={() => loadStoredEmails(Math.max(0, storedPage.offset - storedPage.limit))} disabled={busy || storedPage.offset === 0}>Previous</button>
              <button onClick={() => loadStoredEmails(storedPage.offset + storedPage.limit)} disabled={busy || storedPage.offset + storedPage.limit >= storedPage.total}>Next</button>
            </div>
          )}

          {queueMode === "senders" ? (
            <SenderGroupList
              groups={senderGroups}
              selected={selected}
              onSelect={(ids) => selectEmailIds(ids)}
              onPreviewUnsubscribe={(ids) => bulk("unsubscribe", ids)}
              onPreviewTrash={(ids) => bulk("trash", ids)}
              onOpenSample={(id) => setDetailEmailId(id)}
            />
          ) : (
            <EmailList
              emails={visibleEmails}
              selected={selected}
              onToggle={toggle}
              onOpen={(id) => setDetailEmailId(id)}
            />
          )}
        </section>
      </section>

      {selected.size > 0 && actionResults.length === 0 && (
        <section className="bulk-bar" data-tour="cleanup">
          <strong>{selected.size} selected</strong>
          <button onClick={clearSelected}>Clear</button>
          <select value={targetCategory} onChange={(event) => setTargetCategory(event.target.value as Category)} aria-label="Move selected to category">
            {categories.map((category) => <option key={category.id} value={category.id}>{category.label}</option>)}
          </select>
          <label className="inline-toggle" title="Apply this category to future emails from the same sender">
            <input type="checkbox" checked={applySenderRule} onChange={(event) => setApplySenderRule(event.target.checked)} />
            Apply to future emails
          </label>
          <button data-tour="move" onClick={moveSelected} disabled={busy}><MoveRight size={16} />Move</button>
          {queueMode === "ai" && <button onClick={() => void acceptSuggestions()} disabled={busy}><Check size={16} />Accept Suggestions</button>}
          <button onClick={() => bulk("mark_read")} disabled={busy}><MailCheck size={16} />Mark Read</button>
          <button onClick={() => bulk("unsubscribe")} disabled={busy}><Unlink size={16} />Unsubscribe</button>
          <button className="danger" onClick={() => bulk("trash")} disabled={busy}><Trash2 size={16} />Move to Trash</button>
        </section>
      )}

      {detailEmail && (
        <EmailDetailModal
          email={detailEmail}
          selected={selected.has(detailEmail.id)}
          categories={categories}
          onClose={() => setDetailEmailId(null)}
          onToggle={() => toggle(detailEmail.id)}
          onMove={async (category, applyRule) => {
            await moveEmails([detailEmail.id], category, applyRule);
            setDetailEmailId(null);
          }}
          onMarkRead={() => bulk("mark_read", [detailEmail.id])}
          onUnsubscribe={() => bulk("unsubscribe", [detailEmail.id])}
          onTrash={() => bulk("trash", [detailEmail.id])}
        />
      )}

      {actionResults.length > 0 && (
        <ActionResultsModal
          pendingAction={pendingAction}
          results={actionResults}
          busy={busy}
          onConfirm={confirmPendingAction}
          onCancel={cancelPendingAction}
          onClose={closeActionResults}
        />
      )}

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

function initialTheme(): ThemeMode {
  const storedTheme = window.localStorage.getItem(themeStorageKey);
  if (storedTheme === "light" || storedTheme === "dark") {
    return storedTheme;
  }
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

function buildGmailQuery(dateFilter: DateFilterId, customDate: string, extraTerms: string) {
  const filter = dateFilters.find((item) => item.id === dateFilter);
  const dateQuery = filter?.operator && customDate
    ? `${filter.operator}:${customDate.replace(/-/g, "/")}`
    : filter?.query ?? "newer_than:365d";
  return [dateQuery, extraTerms.trim()].filter(Boolean).join(" ");
}

function buildSenderGroups(emails: EmailSummary[]): SenderGroup[] {
  const groups = new Map<string, SenderGroup>();
  emails.filter((email) => email.hasUnsubscribe).forEach((email) => {
    const key = senderAddress(email.from);
    const existing = groups.get(key);
    if (existing) {
      existing.emails.push(email);
      existing.autoCount += email.canAutoUnsubscribe ? 1 : 0;
      if (!existing.categories.includes(email.category)) {
        existing.categories.push(email.category);
      }
      return;
    }
    groups.set(key, {
      key,
      sender: senderDisplay(email.from),
      domain: senderDomain(key),
      emails: [email],
      autoCount: email.canAutoUnsubscribe ? 1 : 0,
      categories: [email.category]
    });
  });
  return Array.from(groups.values()).sort((left, right) => right.emails.length - left.emails.length || right.autoCount - left.autoCount || left.sender.localeCompare(right.sender));
}

function senderAddress(from: string) {
  const bracketed = from.match(/<([^>]+)>/);
  if (bracketed?.[1]) {
    return bracketed[1].trim().toLowerCase();
  }
  const email = from.match(/[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}/i);
  return (email?.[0] ?? from).trim().toLowerCase();
}

function senderDisplay(from: string) {
  const beforeAddress = from.split("<")[0]?.trim().replace(/^"|"$/g, "");
  return beforeAddress || senderAddress(from);
}

function senderDomain(address: string) {
  return address.includes("@") ? address.split("@").pop() ?? address : address;
}

function sourceLabel(source: string) {
  if (source === "gmail") {
    return "Live Gmail metadata";
  }
  if (source === "review_store") {
    return "Saved Results";
  }
  return "Demo mailbox";
}

function actionLabel(action: "trash" | "mark_read" | "unsubscribe") {
  if (action === "trash") {
    return "move to trash";
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

function EmailList({ emails, selected, onToggle, onOpen }: { emails: EmailSummary[]; selected: Set<string>; onToggle: (id: string) => void; onOpen: (id: string) => void }) {
  if (emails.length === 0) {
    return (
      <div className="empty-state">
        <Inbox size={28} />
        <h2>No emails in this queue</h2>
        <p>Try a wider date range, refresh live Gmail, or run Find Emails to add more saved results.</p>
      </div>
    );
  }

  return (
    <div className="email-list" role="list">
      {emails.map((email) => (
        <article key={email.id} className={selected.has(email.id) ? "email-row selected" : "email-row"} role="listitem">
          <input
            type="checkbox"
            checked={selected.has(email.id)}
            onChange={() => onToggle(email.id)}
            aria-label={`Select ${email.subject || "email"}`}
          />
          <button className="email-row-main" onClick={() => onOpen(email.id)}>
            <span className="card-top">
              <strong>{email.subject || "(No subject)"}</strong>
              <span>{Math.round(email.confidence * 100)}%</span>
            </span>
            <span className="from">{email.from}</span>
            <span className="snippet">{email.snippet}</span>
          </button>
          <span className="row-category">{categoryLabel(email.category)}</span>
          {email.hasUnsubscribe && <span className="tag">{email.canAutoUnsubscribe ? "Auto unsubscribe" : "Review unsubscribe"}</span>}
          <button className="icon-button" onClick={() => onOpen(email.id)} title="Open email details" aria-label="Open email details"><Eye size={16} /></button>
        </article>
      ))}
    </div>
  );
}

function SenderGroupList({ groups, selected, onSelect, onPreviewUnsubscribe, onPreviewTrash, onOpenSample }: { groups: SenderGroup[]; selected: Set<string>; onSelect: (ids: string[]) => void; onPreviewUnsubscribe: (ids: string[]) => void; onPreviewTrash: (ids: string[]) => void; onOpenSample: (id: string) => void }) {
  if (groups.length === 0) {
    return (
      <div className="empty-state">
        <Unlink size={28} />
        <h2>No unsubscribe senders found</h2>
        <p>Refresh live Gmail or widen the date range to find more senders with unsubscribe options.</p>
      </div>
    );
  }

  return (
    <div className="sender-list" role="list">
      {groups.map((group) => {
        const ids = group.emails.map((email) => email.id);
        const selectedCount = ids.filter((id) => selected.has(id)).length;
        const sample = group.emails[0];
        return (
          <article key={group.key} className="sender-row" role="listitem">
            <div className="sender-main">
              <span className="detail-eyebrow">{group.domain}</span>
              <h3>{group.sender}</h3>
              <p>{group.emails.length} message(s), {group.autoCount} automatic unsubscribe, {group.categories.map(categoryLabel).join(", ")}</p>
              {sample && <span className="snippet">Latest: {sample.subject || "(No subject)"}</span>}
            </div>
            <div className="sender-actions">
              <span>{selectedCount}/{group.emails.length} selected</span>
              <button onClick={() => onSelect(ids)}><Check size={16} />Select sender</button>
              {sample && <button onClick={() => onOpenSample(sample.id)}><Eye size={16} />Review sample</button>}
              <button onClick={() => onPreviewUnsubscribe(ids)}><Unlink size={16} />Preview unsubscribe</button>
              <button className="danger" onClick={() => onPreviewTrash(ids)}><Trash2 size={16} />Preview trash</button>
            </div>
          </article>
        );
      })}
    </div>
  );
}

function EmailDetailModal({ email, selected, categories, onClose, onToggle, onMove, onMarkRead, onUnsubscribe, onTrash }: { email: EmailSummary; selected: boolean; categories: { id: Category; label: string }[]; onClose: () => void; onToggle: () => void; onMove: (category: Category, applyRule: boolean) => Promise<void>; onMarkRead: () => void; onUnsubscribe: () => void; onTrash: () => void }) {
  const [decisionCategory, setDecisionCategory] = useState<Category>(email.category === "needs_review" ? "promotions" : email.category);
  const [applyRule, setApplyRule] = useState(true);

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  useEffect(() => {
    setDecisionCategory(email.category === "needs_review" ? "promotions" : email.category);
    setApplyRule(true);
  }, [email.id, email.category]);

  return (
    <div className="detail-backdrop" role="presentation" onMouseDown={onClose}>
      <aside className="detail-modal" role="dialog" aria-modal="true" aria-label="Email details" onMouseDown={(event) => event.stopPropagation()} data-tour="detail">
        <header>
          <div>
            <span className="detail-eyebrow">{categoryLabel(email.category)} - {Math.round(email.confidence * 100)}% confidence</span>
            <h2>{email.subject || "(No subject)"}</h2>
            <p>{email.from}</p>
          </div>
          <button className="icon-button" onClick={onClose} aria-label="Close email details"><X size={18} /></button>
        </header>

        <section className="detail-section">
          <h3>Summary</h3>
          <p>{email.snippet || "No snippet available."}</p>
        </section>

        <section className="detail-section">
          <h3>Why it is here</h3>
          <p>{email.reason || "No classification reason was recorded."}</p>
        </section>

        <section className="decision-panel">
          <div>
            <h3>Review decision</h3>
            <p>Choose the category this email should use. Saving a sender rule helps future emails from this sender land in the same place.</p>
          </div>
          <div className="decision-controls">
            <select value={decisionCategory} onChange={(event) => setDecisionCategory(event.target.value as Category)} aria-label="Decision category">
              {categories.filter((category) => category.id !== "needs_review").map((category) => <option key={category.id} value={category.id}>{category.label}</option>)}
            </select>
            <label className="inline-toggle">
              <input type="checkbox" checked={applyRule} onChange={(event) => setApplyRule(event.target.checked)} />
              Apply to future emails
            </label>
            <button onClick={() => void onMove(decisionCategory, applyRule)}><MoveRight size={16} />Move to category</button>
          </div>
        </section>

        <section className="detail-grid">
          <div>
            <span>Unsubscribe</span>
            <strong>{email.hasUnsubscribe ? (email.canAutoUnsubscribe ? "Can unsubscribe automatically" : "Review link available") : "Not found"}</strong>
          </div>
          <div>
            <span>Received</span>
            <strong>{email.receivedAt ? new Date(email.receivedAt).toLocaleString() : "Unknown"}</strong>
          </div>
        </section>

        <footer className="detail-actions">
          <button onClick={onToggle}>{selected ? "Deselect" : "Select"}</button>
          <button onClick={onMarkRead}><MailCheck size={16} />Mark Read</button>
          <button onClick={onUnsubscribe} disabled={!email.hasUnsubscribe}><Unlink size={16} />Unsubscribe</button>
          <button className="danger" onClick={onTrash}><Trash2 size={16} />Move to Trash</button>
        </footer>
      </aside>
    </div>
  );
}

function ActionResultsModal({ pendingAction, results, busy, onConfirm, onCancel, onClose }: { pendingAction: PendingAction | null; results: ActionResult[]; busy: boolean; onConfirm: () => void; onCancel: () => void; onClose: () => void }) {
  const preparedLinks = results.filter((result) => result.safeLink);
  const title = pendingAction ? `Preview ${actionLabel(pendingAction.action)}` : "Action results";

  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        pendingAction ? onCancel() : onClose();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onCancel, onClose, pendingAction]);

  return (
    <div className="detail-backdrop" role="presentation" onMouseDown={pendingAction ? undefined : onClose}>
      <aside className="action-modal" role="dialog" aria-modal="true" aria-label={title} onMouseDown={(event) => event.stopPropagation()} data-tour="confirmation">
        <header>
          <div>
            <span className="detail-eyebrow">{pendingAction ? "Step 3 of 3" : "Done"}</span>
            <h2>{title}</h2>
            <p>{pendingAction ? `${pendingAction.ids.length} email(s) will be affected. Review the preview before confirming.` : `${results.length} result(s) returned.`}</p>
          </div>
          <button className="icon-button" onClick={pendingAction ? onCancel : onClose} aria-label="Close action results"><X size={18} /></button>
        </header>

        {pendingAction && (
          <div className="risk-note">
            <strong>{pendingAction.action === "trash" ? "Move to Trash is recoverable in Gmail for a limited time." : "One-click unsubscribe can contact sender unsubscribe endpoints."}</strong>
            <span>{pendingAction.confirmationExpiresAt ? `Confirmation expires at ${new Date(pendingAction.confirmationExpiresAt).toLocaleTimeString()}.` : "Confirm only if this preview looks right."}</span>
          </div>
        )}

        <div className="action-result-list">
          {results.slice(0, 12).map((result, index) => (
            <div key={`${result.emailId}-${result.status}-${index}`} className="action-result">
              <strong>{result.status}</strong>
              <span>{result.message || result.emailId}</span>
              {result.safeLink && <a href={result.safeLink} target="_blank" rel="noreferrer">Open review link</a>}
            </div>
          ))}
          {results.length > 12 && <p className="result-overflow">Showing 12 of {results.length} results.</p>}
        </div>

        <footer className="detail-actions">
          {preparedLinks.length > 0 && <span className="prepared-links">{preparedLinks.length} review link(s) ready</span>}
          {pendingAction ? (
            <>
              <button onClick={onCancel} disabled={busy}>Cancel</button>
              <button className="danger" onClick={onConfirm} disabled={busy}>{pendingAction.action === "trash" ? "Confirm Move to Trash" : "Confirm Unsubscribe"}</button>
            </>
          ) : (
            <button onClick={onClose}>Done</button>
          )}
        </footer>
      </aside>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
