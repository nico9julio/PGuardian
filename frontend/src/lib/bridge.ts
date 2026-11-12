export interface APIResponse {
  status: number;
  body: string;
  error?: string;
}

export interface LiveEvent {
  type: string;
  title: string;
  message: string;
  severity: 'info' | 'success' | 'warning' | 'error';
  showToast?: boolean;
  executionId?: number;
  dbName?: string;
  percent?: number;
  timestamp: string;
}

export interface PGServer {
  id: number;
  name: string;
  host: string;
  port: number;
  username: string;
  password?: string;
  hasPassword?: boolean;
  pgDumpPath: string;
  isDefault: boolean;
}

export interface BackupProfile {
  id: number;
  name: string;
  pgServerId: number;
  serverName?: string;
  enabled: boolean;
  scheduleCron: string;
  scheduleLabel: string;
  format: 'custom' | 'plain_gz';
  selectionMode: 'manual' | 'all_auto';
  includedDatabases: string[];
  excludedDatabases: string[];
  localDirOverride: string;
  localRetentionMode: 'count' | 'days';
  localRetentionValue: number;
  driveEnabled: boolean;
  driveFolderId: string;
  driveFolderPath: string;
  driveRetentionMode: 'count' | 'days';
  driveRetentionValue: number;
}

export interface BackupExecution {
  id: number;
  profileId?: number;
  profileName: string;
  pgServerId: number;
  serverName: string;
  dbName: string;
  triggerType: string;
  format: string;
  startedAt: string;
  finishedAt: string;
  durationMs: number;
  localStatus: string;
  localFilePath: string;
  fileSizeBytes: number;
  sha256Checksum: string;
  cloudStatus: string;
  driveFileId: string;
  retryCount: number;
  nextRetryAt: string;
  errorMessage: string;
  deletedLocalAt: string;
  deletedCloudAt: string;
}

export interface DatabaseBackupSummary {
  serverId: number;
  serverName: string;
  dbName: string;
  liveSizeBytes: number;
  assignedProfiles: string[];
  totalCopies: number;
  localCopies: number;
  cloudCopies: number;
  lastBackupAt: string;
  lastLocalStatus: string;
  lastCloudStatus: string;
}

export interface DriveStatus {
  configured: boolean;
  connected: boolean;
  clientId: string;
  hasSecret: boolean;
  accountEmail: string;
  rootFolderId: string;
  rootFolderName: string;
}

export interface DriveFolder {
  id: string;
  name: string;
}

export interface ServerUserSession {
  username: string;
  shortName: string;
  sid: string;
  sessionId: number;
  sessionType: string;
  connectionStat: string;
  notifyEnabled: boolean;
}

export interface NotificationRule {
  channel: 'windows' | 'telegram';
  onSuccess: boolean;
  onError: boolean;
  onRetry: boolean;
  onLowDisk: boolean;
  dailySummary: boolean;
  dailySummaryTime: string;
}

export interface TelegramRecipient {
  id?: number;
  chatId: string;
  label: string;
  enabled: boolean;
}

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          CallService(method: string, path: string, token: string, bodyJSON: string): Promise<APIResponse>;
          StartEventStream(token: string): Promise<void>;
          StopEventStream(): Promise<void>;
          PickDirectory(title: string, defaultDir: string): Promise<string>;
          PickExeFile(title: string): Promise<string>;
          PickUpdateZip(title: string): Promise<string>;
          OpenExternalURL(url: string): Promise<void>;
          RevealInExplorer(path: string): Promise<void>;
        };
      };
    };
    runtime?: {
      EventsOn(eventName: string, callback: (data: LiveEvent) => void): () => void;
    };
  }
}

let currentAuthToken = '';

export function setAuthToken(tok: string) {
  currentAuthToken = tok;
  if (tok && window.go?.main?.App?.StartEventStream) {
    void window.go.main.App.StartEventStream(tok);
  } else if (!tok && window.go?.main?.App?.StopEventStream) {
    void window.go.main.App.StopEventStream();
  }
}

export function getAuthToken(): string {
  return currentAuthToken;
}

export async function apiCall<T>(method: string, path: string, body?: unknown): Promise<T> {
  const app = window.go?.main?.App;
  if (!app) {
    throw new Error('Wails runtime bridge unavailable');
  }
  const payload = body !== undefined ? JSON.stringify(body) : '';
  const res = await app.CallService(method, path, currentAuthToken, payload);
  if (res.error) {
    throw new Error(res.error);
  }
  let parsed: unknown = null;
  if (res.body) {
    try {
      parsed = JSON.parse(res.body);
    } catch {
      parsed = res.body;
    }
  }
  if (res.status < 200 || res.status >= 300) {
    const errMsg =
      typeof parsed === 'object' && parsed !== null && 'error' in parsed
        ? String((parsed as { error: unknown }).error)
        : `HTTP ${res.status}`;
    throw new Error(errMsg);
  }
  return parsed as T;
}

export const nativeBridge = {
  pickDirectory: async (title: string, defaultDir = '') => window.go?.main?.App?.PickDirectory(title, defaultDir) ?? '',
  pickExeFile: async (title: string) => window.go?.main?.App?.PickExeFile(title) ?? '',
  pickUpdateZip: async (title: string) => window.go?.main?.App?.PickUpdateZip(title) ?? '',
  openURL: async (url: string) => window.go?.main?.App?.OpenExternalURL(url),
  revealInExplorer: async (path: string) => window.go?.main?.App?.RevealInExplorer(path),
  onEvent: (cb: (ev: LiveEvent) => void) => {
    if (window.runtime?.EventsOn) {
      return window.runtime.EventsOn('pguardian:event', cb);
    }
    return () => {};
  },
};

export function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const idx = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const val = bytes / Math.pow(1024, idx);
  return `${val.toFixed(idx === 0 ? 0 : 2)} ${units[idx]}`;
}

export function formatDateTime(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}
