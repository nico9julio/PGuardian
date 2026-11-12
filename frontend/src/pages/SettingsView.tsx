import React, { useEffect, useState } from 'react';
import { FolderOpen, KeyRound, ScrollText, RefreshCw } from 'lucide-react';
import { apiCall, nativeBridge } from '../lib/bridge';
import { useApp } from '../context/AppContext';
import { Lang, ThemeMode } from '../i18n/translations';

interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
}

export const SettingsView: React.FC = () => {
  const { lang, setLang, theme, setTheme, t, notifyBanner } = useApp();
  const [localRoot, setLocalRoot] = useState('');
  const [minFreeGB, setMinFreeGB] = useState(5);
  const [curPwd, setCurPwd] = useState('');
  const [newPwd, setNewPwd] = useState('');
  const [logs, setLogs] = useState<LogEntry[]>([]);

  const load = async () => {
    try {
      const [st, lg] = await Promise.all([
        apiCall<{ language: Lang; theme: ThemeMode; localBackupRoot: string; minFreeDiskGb: number }>(
          'GET',
          '/api/v1/settings',
        ),
        apiCall<LogEntry[]>('GET', '/api/v1/logs'),
      ]);
      setLocalRoot(st.localBackupRoot);
      setMinFreeGB(st.minFreeDiskGb);
      setLogs(lg || []);
    } catch {}
  };

  useEffect(() => {
    void load();
  }, []);

  const handleSaveGeneral = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await apiCall('POST', '/api/v1/settings', {
        language: lang,
        theme,
        localBackupRoot: localRoot,
        minFreeDiskGb: minFreeGB,
      });
      notifyBanner(t('savedOk'), 'success');
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const handleChangePwd = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await apiCall('POST', '/api/v1/settings/password', {
        currentPassword: curPwd,
        newPassword: newPwd,
      });
      setCurPwd('');
      setNewPwd('');
      notifyBanner(t('savedOk'), 'success');
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-bold">{t('navSettings')}</h2>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <form
          onSubmit={handleSaveGeneral}
          className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4"
        >
          <h3 className="font-bold text-sm">General & Storage</h3>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-semibold mb-1">Language / Idioma</label>
              <select
                value={lang}
                onChange={(e) => setLang(e.target.value as Lang)}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
              >
                <option value="es">Español</option>
                <option value="en">English</option>
              </select>
            </div>
            <div>
              <label className="block text-xs font-semibold mb-1">Theme / Tema</label>
              <select
                value={theme}
                onChange={(e) => setTheme(e.target.value as ThemeMode)}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
              >
                <option value="system">{t('themeSystem')}</option>
                <option value="dark">{t('themeDark')}</option>
                <option value="light">{t('themeLight')}</option>
              </select>
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold mb-1">Local Backups Root Folder</label>
            <div className="flex gap-2">
              <input
                value={localRoot}
                onChange={(e) => setLocalRoot(e.target.value)}
                className="flex-1 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
              />
              <button
                type="button"
                onClick={async () => {
                  const picked = await nativeBridge.pickDirectory('Local Backup Root', localRoot);
                  if (picked) setLocalRoot(picked);
                }}
                className="px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs flex items-center gap-1"
              >
                <FolderOpen className="w-3.5 h-3.5" /> {t('browse')}
              </button>
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold mb-1">Low Disk Warning Threshold (GB)</label>
            <input
              type="number"
              min={1}
              value={minFreeGB}
              onChange={(e) => setMinFreeGB(Number(e.target.value))}
              className="w-32 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
            />
          </div>

          <button type="submit" className="px-5 py-2 rounded-xl bg-cyan-600 text-white text-xs font-semibold">
            {t('save')}
          </button>
        </form>

        <form
          onSubmit={handleChangePwd}
          className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4"
        >
          <h3 className="font-bold text-sm flex items-center gap-2">
            <KeyRound className="w-4 h-4 text-cyan-500" /> PIN / Master Password
          </h3>
          <div>
            <label className="block text-xs font-semibold mb-1">Current PIN / Password</label>
            <input
              type="password"
              required
              value={curPwd}
              onChange={(e) => setCurPwd(e.target.value)}
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
            />
          </div>
          <div>
            <label className="block text-xs font-semibold mb-1">New PIN / Password (min 4 chars)</label>
            <input
              type="password"
              required
              value={newPwd}
              onChange={(e) => setNewPwd(e.target.value)}
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
            />
          </div>
          <button
            type="submit"
            className="px-5 py-2 rounded-xl bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 text-xs font-semibold"
          >
            Update PIN / Password
          </button>
        </form>
      </div>

      <div className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="font-bold text-sm flex items-center gap-2">
            <ScrollText className="w-4 h-4 text-cyan-500" /> Service Rotating Ring Logs
          </h3>
          <button onClick={load} className="p-1.5 rounded hover:bg-slate-100 dark:hover:bg-slate-800">
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
        </div>
        <div className="h-64 overflow-y-auto rounded-xl bg-slate-950 text-slate-200 p-4 font-mono text-[11px] space-y-1">
          {logs.length === 0 ? (
            <div className="text-slate-500">No log entries recorded in current buffer yet.</div>
          ) : (
            logs.map((l, idx) => (
              <div key={idx} className="leading-relaxed">
                <span className="text-slate-500">{l.timestamp}</span>{' '}
                <span
                  className={
                    l.level === 'ERROR'
                      ? 'text-red-400 font-bold'
                      : l.level === 'WARN'
                      ? 'text-amber-400'
                      : 'text-cyan-400'
                  }
                >
                  [{l.level}]
                </span>{' '}
                <span>{l.message}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
};
