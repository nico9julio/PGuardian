import React, { useEffect, useState } from 'react';
import {
  FolderArchive,
  FolderOpen,
  Plus,
  Play,
  History,
  Edit3,
  Trash2,
  RefreshCw,
  RotateCw,
  AlertTriangle,
  X,
} from 'lucide-react';
import {
  apiCall,
  BackupDirectory,
  BackupExecution,
  DirectoryBackupSummary,
  formatBytes,
  formatDateTime,
  nativeBridge,
} from '../lib/bridge';
import { useApp } from '../context/AppContext';
import { StatusBadge } from './DashboardView';

const PRESET_EXCLUSIONS = ['node_modules', '.git', '*.tmp', '*.log', '~$*'];

function parseExclusionsText(text: string): string[] {
  return text
    .split(/[\n,]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}

export const DirectoriesView: React.FC = () => {
  const { lang, t, notifyBanner, eventTick } = useApp();
  const [dirs, setDirs] = useState<DirectoryBackupSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<{
    id: number;
    name: string;
    path: string;
    exclusionsText: string;
  } | null>(null);
  const [historyTarget, setHistoryTarget] = useState<{ directoryId: number; name: string } | null>(null);
  const [historyItems, setHistoryItems] = useState<BackupExecution[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  const loadCatalog = async () => {
    setLoading(true);
    try {
      const list = await apiCall<DirectoryBackupSummary[]>('GET', '/api/v1/directories');
      setDirs(list);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadCatalog();
  }, [eventTick]);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editing) return;
    try {
      const payload: BackupDirectory = {
        id: editing.id,
        name: editing.name.trim(),
        path: editing.path.trim(),
        excludePatterns: parseExclusionsText(editing.exclusionsText),
      };
      await apiCall('POST', '/api/v1/directories', payload);
      notifyBanner(t('savedOk'), 'success');
      setEditing(null);
      await loadCatalog();
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await apiCall('DELETE', `/api/v1/directories?id=${id}`);
      await loadCatalog();
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const triggerBackupNow = async (directoryId: number, name: string) => {
    try {
      await apiCall('POST', '/api/v1/backups/run-dir', { directoryId, profileId: 0 });
      notifyBanner(
        lang === 'es' ? `Iniciando copia de ${name}...` : `Starting backup for ${name}...`,
        'info',
      );
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  const openHistory = async (directoryId: number, name: string) => {
    setHistoryTarget({ directoryId, name });
    setHistoryLoading(true);
    try {
      const items = await apiCall<BackupExecution[]>(
        'GET',
        `/api/v1/executions?directoryId=${directoryId}&limit=100`,
      );
      setHistoryItems(items);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    } finally {
      setHistoryLoading(false);
    }
  };

  const retryCloud = async (executionId: number) => {
    try {
      await apiCall('POST', '/api/v1/backups/retry-cloud', { executionId });
      notifyBanner(lang === 'es' ? 'Reintentando subida a Google Drive...' : 'Retrying Google Drive upload...', 'info');
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  const togglePreset = (pattern: string) => {
    if (!editing) return;
    const current = parseExclusionsText(editing.exclusionsText);
    const exists = current.some((p) => p.toLowerCase() === pattern.toLowerCase());
    const next = exists
      ? current.filter((p) => p.toLowerCase() !== pattern.toLowerCase())
      : [...current, pattern];
    setEditing({ ...editing, exclusionsText: next.join(', ') });
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-bold">{t('dirTitle')}</h2>
          <p className="text-xs text-slate-500">{t('dirSubtitle')}</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={loadCatalog}
            className="px-3.5 py-2 rounded-xl border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium flex items-center gap-1.5"
          >
            <RefreshCw className="w-3.5 h-3.5" /> {t('refresh')}
          </button>
          <button
            onClick={() =>
              setEditing({
                id: 0,
                name: '',
                path: '',
                exclusionsText: '',
              })
            }
            className="px-4 py-2 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center gap-1.5"
          >
            <Plus className="w-4 h-4" /> {t('dirNew')}
          </button>
        </div>
      </div>

      {editing ? (
        <form
          onSubmit={handleSave}
          className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4 max-w-2xl"
        >
          <h3 className="font-bold text-base border-b border-slate-200 dark:border-slate-800 pb-3">
            {editing.id ? t('dirEdit') : t('dirNew')}
          </h3>

          <div>
            <label className="block text-xs font-semibold mb-1">{t('dirPath')}</label>
            <div className="flex gap-2">
              <input
                required
                value={editing.path}
                onChange={(e) => setEditing({ ...editing, path: e.target.value })}
                placeholder="C:\inetpub\wwwroot"
                className="flex-1 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
              />
              <button
                type="button"
                onClick={async () => {
                  const picked = await nativeBridge.pickDirectory(t('dirPath'), editing.path);
                  if (picked) {
                    const baseName = picked.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || '';
                    setEditing({
                      ...editing,
                      path: picked,
                      name: editing.name.trim() ? editing.name : baseName,
                    });
                  }
                }}
                className="px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs flex items-center gap-1"
              >
                <FolderOpen className="w-3.5 h-3.5" /> {t('browse')}
              </button>
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold mb-1">{t('dirName')}</label>
            <input
              required
              value={editing.name}
              onChange={(e) => setEditing({ ...editing, name: e.target.value })}
              placeholder="WebUploads"
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
            />
          </div>

          <div className="space-y-2">
            <label className="block text-xs font-semibold">{t('dirExclusions')}</label>
            <input
              value={editing.exclusionsText}
              onChange={(e) => setEditing({ ...editing, exclusionsText: e.target.value })}
              placeholder={t('dirExclusionsHint')}
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
            />
            <div className="flex flex-wrap gap-1.5 pt-1">
              {PRESET_EXCLUSIONS.map((pat) => {
                const active = parseExclusionsText(editing.exclusionsText).some(
                  (p) => p.toLowerCase() === pat.toLowerCase(),
                );
                return (
                  <button
                    key={pat}
                    type="button"
                    onClick={() => togglePreset(pat)}
                    className={`px-2.5 py-1 rounded-lg text-[11px] font-mono border transition ${
                      active
                        ? 'bg-cyan-600 text-white border-cyan-600'
                        : 'border-slate-300 dark:border-slate-700 text-slate-500 hover:bg-slate-100 dark:hover:bg-slate-800'
                    }`}
                  >
                    {active ? '✓ ' : '+ '}
                    {pat}
                  </button>
                );
              })}
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-2 border-t border-slate-200 dark:border-slate-800">
            <button
              type="button"
              onClick={() => setEditing(null)}
              className="px-4 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs font-medium"
            >
              {t('cancel')}
            </button>
            <button type="submit" className="px-5 py-2 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold">
              {t('save')}
            </button>
          </div>
        </form>
      ) : loading && dirs.length === 0 ? (
        <div className="p-8 text-xs text-slate-400">Loading directories...</div>
      ) : dirs.length === 0 ? (
        <div className="p-10 rounded-2xl border border-dashed border-slate-300 dark:border-slate-800 text-center text-xs text-slate-400">
          {t('dirEmptyCatalog')}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {dirs.map((item) => (
            <div
              key={item.directoryId}
              className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 flex flex-col justify-between gap-4 hover:border-cyan-500/40 transition shadow-sm"
            >
              <div>
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-start gap-2.5 min-w-0">
                    <div className="p-2 rounded-xl bg-indigo-500/10 text-indigo-500 shrink-0">
                      <FolderArchive className="w-5 h-5" />
                    </div>
                    <div className="min-w-0">
                      <h3 className="font-bold text-sm truncate">{item.name}</h3>
                      <p className="text-[11px] text-slate-400 font-mono truncate" title={item.path}>
                        {item.path}
                      </p>
                      <div className="mt-1 text-[11px] text-slate-400">
                        {item.exists ? (
                          <span>
                            {formatBytes(item.liveSizeBytes)} • {item.liveFileCount} {t('dirFilesCount').toLowerCase()}
                          </span>
                        ) : (
                          <span className="text-amber-500 font-semibold flex items-center gap-1">
                            <AlertTriangle className="w-3 h-3" /> {t('dirMissing')}
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-1 shrink-0">
                    <button
                      onClick={() =>
                        setEditing({
                          id: item.directoryId,
                          name: item.name,
                          path: item.path,
                          exclusionsText: (item.excludePatterns || []).join(', '),
                        })
                      }
                      className="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800"
                    >
                      <Edit3 className="w-3.5 h-3.5" />
                    </button>
                    <button
                      onClick={() => handleDelete(item.directoryId)}
                      className="p-1.5 rounded-lg hover:bg-red-500/10 text-red-500"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>

                <div className="mt-3 flex flex-wrap gap-1.5">
                  {item.assignedProfiles.length > 0 ? (
                    item.assignedProfiles.map((p) => (
                      <span
                        key={p}
                        className="px-2 py-0.5 rounded-md bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-[11px] font-medium"
                      >
                        {p}
                      </span>
                    ))
                  ) : (
                    <span className="px-2 py-0.5 rounded-md bg-slate-200 dark:bg-slate-800 text-slate-500 text-[11px]">
                      {t('dbUnprotected')}
                    </span>
                  )}
                  {item.excludePatterns.length > 0 && (
                    <span
                      className="px-2 py-0.5 rounded-md bg-slate-100 dark:bg-slate-800 text-slate-500 text-[11px] font-mono"
                      title={item.excludePatterns.join(', ')}
                    >
                      -{item.excludePatterns.length} excl.
                    </span>
                  )}
                </div>

                <div className="mt-4 grid grid-cols-2 gap-2 p-3 rounded-xl bg-slate-50 dark:bg-slate-950/60 text-xs">
                  <div>
                    <span className="text-slate-400 block text-[11px]">{t('dbCopiesLocal')}</span>
                    <span className="font-bold text-sm">{item.localCopies}</span>{' '}
                    {item.lastLocalStatus && <StatusBadge status={item.lastLocalStatus} />}
                  </div>
                  <div>
                    <span className="text-slate-400 block text-[11px]">{t('dbCopiesCloud')}</span>
                    <span className="font-bold text-sm">{item.cloudCopies}</span>{' '}
                    {item.lastCloudStatus && <StatusBadge status={item.lastCloudStatus} />}
                  </div>
                </div>
                {item.lastBackupAt && (
                  <div className="mt-2 text-[11px] text-slate-400">
                    Última copia / Last: {formatDateTime(item.lastBackupAt)}
                  </div>
                )}
              </div>

              <div className="flex items-center gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
                <button
                  onClick={() => triggerBackupNow(item.directoryId, item.name)}
                  className="flex-1 py-2 px-3 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center justify-center gap-1.5"
                >
                  <Play className="w-3.5 h-3.5" /> {t('dbBackupNow')}
                </button>
                <button
                  onClick={() => openHistory(item.directoryId, item.name)}
                  className="py-2 px-3 rounded-xl border border-slate-200 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium flex items-center gap-1.5"
                >
                  <History className="w-3.5 h-3.5" /> {t('dbViewHistory')} ({item.totalCopies})
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {historyTarget && (
        <div className="fixed inset-0 z-40 bg-slate-950/70 backdrop-blur-sm flex items-center justify-center p-6">
          <div className="w-full max-w-4xl max-h-[85vh] flex flex-col rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-2xl overflow-hidden">
            <div className="px-6 py-4 border-b border-slate-200 dark:border-slate-800 flex items-center justify-between">
              <h3 className="font-bold text-base">
                {t('dbHistoryTitle')}: <span className="text-indigo-500">{historyTarget.name}</span>
              </h3>
              <button
                onClick={() => setHistoryTarget(null)}
                className="p-1.5 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto">
              {historyLoading ? (
                <div className="p-8 text-center text-xs text-slate-400">Loading...</div>
              ) : historyItems.length === 0 ? (
                <div className="p-8 text-center text-xs text-slate-400">{t('dashNoBackupsYet')}</div>
              ) : (
                <table className="w-full text-xs">
                  <thead className="bg-slate-50 dark:bg-slate-950/70 text-slate-500 text-left sticky top-0">
                    <tr>
                      <th className="px-4 py-3">Date</th>
                      <th className="px-4 py-3">Profile / Format</th>
                      <th className="px-4 py-3">Size & Duration</th>
                      <th className="px-4 py-3">Local</th>
                      <th className="px-4 py-3">Google Drive</th>
                      <th className="px-4 py-3">SHA-256</th>
                      <th className="px-4 py-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                    {historyItems.map((ex) => (
                      <tr key={ex.id} className="hover:bg-slate-50 dark:hover:bg-slate-800/40">
                        <td className="px-4 py-3 whitespace-nowrap">{formatDateTime(ex.startedAt)}</td>
                        <td className="px-4 py-3">
                          <div className="font-medium">{ex.profileName || ex.triggerType}</div>
                          <div className="text-[10px] text-slate-400 uppercase">{ex.format}</div>
                        </td>
                        <td className="px-4 py-3 font-mono">
                          <div>{formatBytes(ex.fileSizeBytes)}</div>
                          <div className="text-[10px] text-slate-400">{(ex.durationMs / 1000).toFixed(1)}s</div>
                        </td>
                        <td className="px-4 py-3">
                          {ex.deletedLocalAt ? (
                            <span className="text-[11px] text-slate-400 line-through">Pruned</span>
                          ) : (
                            <StatusBadge status={ex.localStatus} />
                          )}
                          {ex.warningMessage && (
                            <div className="text-[10px] text-amber-500 max-w-[200px] truncate mt-0.5" title={ex.warningMessage}>
                              ⚠ {ex.warningMessage}
                            </div>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          {ex.deletedCloudAt ? (
                            <span className="text-[11px] text-slate-400 line-through">Pruned</span>
                          ) : (
                            <StatusBadge status={ex.cloudStatus} />
                          )}
                          {ex.errorMessage && (
                            <div className="text-[10px] text-red-400 max-w-[180px] truncate" title={ex.errorMessage}>
                              {ex.errorMessage}
                            </div>
                          )}
                        </td>
                        <td className="px-4 py-3 font-mono text-[10px] text-slate-400">
                          {ex.sha256Checksum ? `${ex.sha256Checksum.slice(0, 12)}…` : '—'}
                        </td>
                        <td className="px-4 py-3 text-right space-x-1">
                          {ex.localFilePath && !ex.deletedLocalAt && (
                            <button
                              onClick={() => nativeBridge.revealInExplorer(ex.localFilePath)}
                              className="p-1.5 rounded hover:bg-slate-200 dark:hover:bg-slate-700"
                              title={t('dbOpenExplorer')}
                            >
                              <FolderOpen className="w-3.5 h-3.5" />
                            </button>
                          )}
                          {(ex.cloudStatus === 'retrying' || ex.cloudStatus === 'pending' || ex.cloudStatus === 'failed') &&
                            ex.localStatus === 'completed' &&
                            !ex.deletedLocalAt && (
                              <button
                                onClick={() => retryCloud(ex.id)}
                                className="p-1.5 rounded hover:bg-amber-500/20 text-amber-500"
                                title={t('dbRetryCloud')}
                              >
                                <RotateCw className="w-3.5 h-3.5" />
                              </button>
                            )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
