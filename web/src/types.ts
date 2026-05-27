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
};

export type AppConfig = {
  gmailAuthenticated: boolean;
  googleClientSecret: { path: string; fileName: string; exists: boolean };
  openAIKey: { path: string; fileName: string; exists: boolean };
  openAIModel: string;
  openAIEnabled: boolean;
};

export type ActionResult = {
  emailId: string;
  status: string;
  message?: string;
  safeLink?: string;
};

