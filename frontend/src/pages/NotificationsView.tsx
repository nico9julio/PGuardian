import React, { useEffect, useState } from 'react';
import { Bell, Send, Users, Trash2, Plus, RefreshCw } from 'lucide-react';
import { apiCall, NotificationRule, ServerUserSession, TelegramRecipient } from '../lib/bridge';
import { useApp } from '../context/AppContext';

export const NotificationsView: React.FC = () => {
  const { t, notifyBanner } = useApp();
  const [rules, setRules] = useState<NotificationRule[]>([]);
  const [winUsers, setWinUsers] = useState<ServerUserSession[]>([]);
  const [recipients, setRecipients] = useState<TelegramRecipient[]>([]);
  const [hasToken, setHasToken] = useState(false);
  const [botToken, setBotToken] = useState('');
  const [newChatId, setNewChatId] = useState('');
  const [newChatLabel, setNewChatLabel] = useState('');

  const load = async () => {
    try {
      const cfg = await apiCall<{
        rules: NotificationRule[];
        hasTelegramToken: boolean;
        telegramRecipients: TelegramRecipient[];
        windowsUsers: ServerUserSession[];
      }>('GET', '/api/v1/notifications/config');
      setRules(cfg.rules || []);
      setHasToken(cfg.hasTelegramToken);
      setRecipients(cfg.telegramRecipients || []);
      setWinUsers(cfg.windowsUsers || []);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const updateRule = async (updated: NotificationRule) => {
    const next = rules.map((r) => (r.channel === updated.channel ? updated : r));
    setRules(next);
    try {
      await apiCall('POST', '/api/v1/notifications/rules', next);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  const toggleWindowsUser = async (username: string, enabled: boolean) => {
    setWinUsers((prev) => prev.map((u) => (u.username === username ? { ...u, notifyEnabled: enabled } : u)));
    try {
      await apiCall('POST', '/api/v1/notifications/windows-user', { username, enabled });
      notifyBanner(t('savedOk'), 'success');
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  const saveToken = async () => {
    if (!botToken.trim()) return;
    try {
      await apiCall('POST', '/api/v1/notifications/telegram-token', { botToken });
      setBotToken('');
      notifyBanner(t('savedOk'), 'success');
      await load();
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  const detectChats = async () => {
    try {
      const chats = await apiCall<{ chatId: string; title: string }[]>('POST', '/api/v1/notifications/telegram-detect', {
        botToken,
      });
      if (chats.length > 0) {
        setNewChatId(chats[0].chatId);
        setNewChatLabel(chats[0].title || 'Telegram Chat');
      } else {
        notifyBanner('Send /start to your bot first, then click detect again.', 'warning');
      }
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  const addRecipient = async () => {
    if (!newChatId.trim()) return;
    try {
      await apiCall('POST', '/api/v1/notifications/telegram-recipient', {
        chatId: newChatId.trim(),
        label: newChatLabel.trim() || 'Admin',
        enabled: true,
      });
      setNewChatId('');
      setNewChatLabel('');
      await load();
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  const testChannel = async (channel: 'windows' | 'telegram') => {
    try {
      await apiCall('POST', '/api/v1/notifications/test', { channel });
      notifyBanner(`Test sent (${channel})`, 'success');
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold">{t('notifTitle')}</h2>
        <div className="flex gap-2">
          <button
            onClick={() => testChannel('windows')}
            className="px-3.5 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs font-semibold flex items-center gap-1.5"
          >
            <Bell className="w-3.5 h-3.5 text-cyan-500" /> {t('notifTestWin')}
          </button>
          <button
            onClick={() => testChannel('telegram')}
            className="px-3.5 py-2 rounded-xl bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold flex items-center gap-1.5"
          >
            <Send className="w-3.5 h-3.5" /> {t('notifTestTg')}
          </button>
        </div>
      </div>

      <div className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4">
        <h3 className="font-bold text-sm">{t('notifRulesCard')}</h3>
        <div className="overflow-x-auto">
          <table className="w-full text-xs">
            <thead className="text-slate-500 border-b border-slate-200 dark:border-slate-800 text-left">
              <tr>
                <th className="py-2">Channel</th>
                <th className="py-2">{t('notifEvSuccess')}</th>
                <th className="py-2">{t('notifEvError')}</th>
                <th className="py-2">{t('notifEvRetry')}</th>
                <th className="py-2">{t('notifEvLowDisk')}</th>
                <th className="py-2">{t('notifEvSummary')}</th>
                <th className="py-2">Time</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {rules.map((r) => (
                <tr key={r.channel}>
                  <td className="py-3 font-bold uppercase">{r.channel}</td>
                  <td className="py-3">
                    <input
                      type="checkbox"
                      checked={r.onSuccess}
                      onChange={(e) => updateRule({ ...r, onSuccess: e.target.checked })}
                      className="accent-cyan-600"
                    />
                  </td>
                  <td className="py-3">
                    <input
                      type="checkbox"
                      checked={r.onError}
                      onChange={(e) => updateRule({ ...r, onError: e.target.checked })}
                      className="accent-cyan-600"
                    />
                  </td>
                  <td className="py-3">
                    <input
                      type="checkbox"
                      checked={r.onRetry}
                      onChange={(e) => updateRule({ ...r, onRetry: e.target.checked })}
                      className="accent-cyan-600"
                    />
                  </td>
                  <td className="py-3">
                    <input
                      type="checkbox"
                      checked={r.onLowDisk}
                      onChange={(e) => updateRule({ ...r, onLowDisk: e.target.checked })}
                      className="accent-cyan-600"
                    />
                  </td>
                  <td className="py-3">
                    <input
                      type="checkbox"
                      checked={r.dailySummary}
                      onChange={(e) => updateRule({ ...r, dailySummary: e.target.checked })}
                      className="accent-cyan-600"
                    />
                  </td>
                  <td className="py-3">
                    <input
                      type="time"
                      value={r.dailySummaryTime}
                      onChange={(e) => updateRule({ ...r, dailySummaryTime: e.target.value })}
                      className="px-2 py-1 rounded border border-slate-300 dark:border-slate-700 bg-transparent text-xs"
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="font-bold text-sm flex items-center gap-2">
              <Users className="w-4 h-4 text-cyan-500" /> {t('notifRdpUsersCard')}
            </h3>
            <button onClick={load} className="p-1.5 rounded hover:bg-slate-100 dark:hover:bg-slate-800">
              <RefreshCw className="w-3.5 h-3.5" />
            </button>
          </div>
          <p className="text-xs text-slate-500">{t('notifRdpDesc')}</p>
          <div className="divide-y divide-slate-100 dark:divide-slate-800 rounded-xl border border-slate-200 dark:border-slate-800">
            {winUsers.map((u) => (
              <label key={u.username} className="px-4 py-3 flex items-center justify-between text-xs cursor-pointer">
                <div>
                  <span className="font-bold">{u.username}</span>
                  <span
                    className={`ml-2 px-2 py-0.5 rounded-full text-[10px] font-semibold ${
                      u.connectionStat === 'Active'
                        ? 'bg-emerald-500/15 text-emerald-500'
                        : 'bg-slate-500/10 text-slate-400'
                    }`}
                  >
                    {u.sessionType} • {u.connectionStat}
                  </span>
                </div>
                <input
                  type="checkbox"
                  checked={u.notifyEnabled}
                  onChange={(e) => toggleWindowsUser(u.username, e.target.checked)}
                  className="w-4 h-4 accent-cyan-600"
                />
              </label>
            ))}
          </div>
        </div>

        <div className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4">
          <h3 className="font-bold text-sm">{t('notifTelegramCard')}</h3>
          <div className="flex gap-2">
            <input
              type="password"
              value={botToken}
              onChange={(e) => setBotToken(e.target.value)}
              placeholder={hasToken ? 'Bot Token guardado en DPAPI (ingresa nuevo para reemplazar)' : '123456:ABC-DEF...'}
              className="flex-1 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
            />
            <button
              onClick={saveToken}
              className="px-4 py-2 rounded-xl bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 text-xs font-semibold"
            >
              {t('save')}
            </button>
          </div>

          <div className="flex gap-2">
            <input
              value={newChatId}
              onChange={(e) => setNewChatId(e.target.value)}
              placeholder="Chat ID"
              className="w-36 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
            />
            <input
              value={newChatLabel}
              onChange={(e) => setNewChatLabel(e.target.value)}
              placeholder="Label (ej. Grupo Soporte)"
              className="flex-1 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
            />
            <button
              onClick={detectChats}
              className="px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs font-medium"
            >
              Detect
            </button>
            <button
              onClick={addRecipient}
              className="p-2 rounded-xl bg-cyan-600 text-white"
              title="Add Chat ID"
            >
              <Plus className="w-4 h-4" />
            </button>
          </div>

          <div className="divide-y divide-slate-100 dark:divide-slate-800 rounded-xl border border-slate-200 dark:border-slate-800">
            {recipients.map((r) => (
              <div key={r.id || r.chatId} className="px-4 py-2.5 flex items-center justify-between text-xs">
                <div>
                  <b>{r.label}</b> <code className="text-slate-400 ml-1">({r.chatId})</code>
                </div>
                <button
                  onClick={async () => {
                    await apiCall('DELETE', `/api/v1/notifications/telegram-recipient?id=${r.id}`);
                    await load();
                  }}
                  className="p-1 rounded hover:bg-red-500/10 text-red-500"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
