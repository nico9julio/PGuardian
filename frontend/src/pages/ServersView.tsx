import React, { useEffect, useState } from 'react';
import { Server, Plus, Trash2, Edit3, CheckCircle2, FolderOpen } from 'lucide-react';
import { apiCall, nativeBridge, PGServer } from '../lib/bridge';
import { useApp } from '../context/AppContext';

export const ServersView: React.FC = () => {
  const { t, notifyBanner } = useApp();
  const [servers, setServers] = useState<PGServer[]>([]);
  const [editing, setEditing] = useState<PGServer | null>(null);
  const [detectedBins, setDetectedBins] = useState<{ path: string; version: string }[]>([]);
  const [testResult, setTestResult] = useState('');

  const load = async () => {
    try {
      const [srvs, bins] = await Promise.all([
        apiCall<PGServer[]>('GET', '/api/v1/servers'),
        apiCall<{ path: string; version: string }[]>('GET', '/api/v1/postgres/discover'),
      ]);
      setServers(srvs);
      setDetectedBins(bins);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editing) return;
    try {
      await apiCall('POST', '/api/v1/servers', editing);
      notifyBanner(t('savedOk'), 'success');
      setEditing(null);
      setTestResult('');
      await load();
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const handleTest = async () => {
    if (!editing) return;
    setTestResult('');
    try {
      const res = await apiCall<{ version: string }>('POST', '/api/v1/postgres/test', editing);
      setTestResult(res.version);
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold">{t('srvTitle')}</h2>
        <button
          onClick={() => {
            setTestResult('');
            setEditing({
              id: 0,
              name: 'PostgreSQL Server',
              host: '127.0.0.1',
              port: 5432,
              username: 'postgres',
              password: '',
              pgDumpPath: detectedBins[0]?.path || '',
              isDefault: servers.length === 0,
            });
          }}
          className="px-4 py-2 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center gap-1.5"
        >
          <Plus className="w-4 h-4" /> {t('srvNew')}
        </button>
      </div>

      {!editing ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {servers.map((s) => (
            <div
              key={s.id}
              className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 flex items-start justify-between"
            >
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <Server className="w-4 h-4 text-cyan-500" />
                  <h3 className="font-bold text-sm">{s.name}</h3>
                  {s.isDefault && (
                    <span className="px-2 py-0.5 rounded-full bg-cyan-500/15 text-cyan-500 text-[10px] font-bold">
                      DEFAULT
                    </span>
                  )}
                </div>
                <p className="text-xs text-slate-500 font-mono">
                  {s.username}@{s.host}:{s.port}
                </p>
                <p className="text-[11px] text-slate-400 font-mono truncate max-w-sm">{s.pgDumpPath || 'pg_dump'}</p>
              </div>
              <div className="flex gap-1">
                <button
                  onClick={() => {
                    setTestResult('');
                    setEditing({ ...s, password: '' });
                  }}
                  className="p-2 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  <Edit3 className="w-4 h-4" />
                </button>
                <button
                  onClick={async () => {
                    await apiCall('DELETE', `/api/v1/servers?id=${s.id}`);
                    await load();
                  }}
                  className="p-2 rounded-lg hover:bg-red-500/10 text-red-500"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <form
          onSubmit={handleSave}
          className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4 max-w-2xl"
        >
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-semibold mb-1">Name</label>
              <input
                required
                value={editing.name}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
              />
            </div>
            <div className="grid grid-cols-3 gap-2">
              <div className="col-span-2">
                <label className="block text-xs font-semibold mb-1">Host</label>
                <input
                  required
                  value={editing.host}
                  onChange={(e) => setEditing({ ...editing, host: e.target.value })}
                  className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold mb-1">Port</label>
                <input
                  type="number"
                  value={editing.port}
                  onChange={(e) => setEditing({ ...editing, port: Number(e.target.value) })}
                  className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                />
              </div>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-semibold mb-1">Username</label>
              <input
                required
                value={editing.username}
                onChange={(e) => setEditing({ ...editing, username: e.target.value })}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
              />
            </div>
            <div>
              <label className="block text-xs font-semibold mb-1">
                Password {editing.hasPassword ? '(leave blank to keep stored DPAPI secret)' : ''}
              </label>
              <input
                type="password"
                value={editing.password || ''}
                onChange={(e) => setEditing({ ...editing, password: e.target.value })}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold mb-1">pg_dump.exe Path</label>
            <div className="flex gap-2">
              <input
                value={editing.pgDumpPath}
                onChange={(e) => setEditing({ ...editing, pgDumpPath: e.target.value })}
                className="flex-1 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
              />
              <button
                type="button"
                onClick={async () => {
                  const picked = await nativeBridge.pickExeFile('Select pg_dump.exe');
                  if (picked) setEditing({ ...editing, pgDumpPath: picked });
                }}
                className="px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs flex items-center gap-1"
              >
                <FolderOpen className="w-3.5 h-3.5" /> {t('browse')}
              </button>
            </div>
          </div>

          <div className="flex items-center justify-between pt-2">
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={handleTest}
                className="px-3.5 py-2 rounded-xl bg-slate-200 dark:bg-slate-800 text-xs font-semibold"
              >
                {t('srvTestConn')}
              </button>
              {testResult && (
                <span className="text-xs text-emerald-500 flex items-center gap-1">
                  <CheckCircle2 className="w-4 h-4" /> {testResult.slice(0, 45)}
                </span>
              )}
            </div>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setEditing(null)}
                className="px-4 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs font-medium"
              >
                {t('cancel')}
              </button>
              <button type="submit" className="px-5 py-2 rounded-xl bg-cyan-600 text-white text-xs font-semibold">
                {t('save')}
              </button>
            </div>
          </div>
        </form>
      )}
    </div>
  );
};
