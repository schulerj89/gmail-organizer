export type Category =
  | "needs_review"
  | "promotions"
  | "newsletters"
  | "social"
  | "finance"
  | "travel"
  | "work"
  | "receipts"
  | "security"
  | "personal"
  | "unwanted";

export type EmailSummary = {
  id: string;
  threadId: string;
  from: string;
  subject: string;
  snippet: string;
  receivedAt: string;
  labelIds: string[];
  category: Category;
  confidence: number;
  reason: string;
  hasUnsubscribe: boolean;
  unsubscribeTarget?: string;
  unsubscribeMethod?: string;
  canAutoUnsubscribe: boolean;
};

export type AppConfig = {
  gmailAuthenticated: boolean;
  googleClientSecret: { path: string; fileName: string; exists: boolean };
  openAIKey: { path: string; fileName: string; exists: boolean };
  openAIModel: string;
  openAIEnabled: boolean;
};

export type MonitorStatus = {
  running: boolean;
  query: string;
  max: number;
  useAI: boolean;
  source: string;
  cacheSize: number;
  cacheLimit: number;
  intervalSeconds: number;
  lastPollAt?: string;
  lastSuccessAt?: string;
  lastError?: string;
  emails: EmailSummary[];
};

export type ScanStatus = {
  running: boolean;
  completed: boolean;
  query: string;
  useAI: boolean;
  processed: number;
  limit: number;
  batchSize: number;
  source: string;
  hasMore: boolean;
  cacheSize: number;
  cacheLimit: number;
  lastError?: string;
  startedAt?: string;
  endedAt?: string;
  emails: EmailSummary[];
};

export type ReviewStats = {
  total: number;
  manual: number;
  needsReview: number;
  senderRules: number;
  byCategory: Partial<Record<Category, number>>;
  updatedAt?: string;
};

export type ActionResult = {
  emailId: string;
  status: string;
  message?: string;
  safeLink?: string;
};
