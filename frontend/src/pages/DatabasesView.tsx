import React, { useEffect, useState } from 'react';
import { Database, Play, History, FolderOpen, RotateCw, RefreshCw, X } from 'lucide-react';
import {
  apiCall,
  BackupExecution,
  DatabaseBackupSummary,
  formatBytes,
  formatDateTime,
  nativeBridge,
} from '../lib/bridge';
import { useApp } from '../context/AppContext';
import { StatusBadge } from './DashboardView';

export const DatabasesView: React.FC = () => {
  const { lang, t, notifyBanner, eventTick } = useApp();
  const [dbs, setDbs] = useState<DatabaseBackupSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [historyTarget, setHistoryTarget] = useState<{ serverId: number; dbName: string } | null>(null);
  const [historyItems, setHistoryItems] = useState<BackupExecution[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  const loadCatalog = async () => {
    setLoading(true);
    try {
      const list = await apiCall<DatabaseBackupSummary[]>('GET', '/api/v1/databases');
      setDbs(list);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadCatalog();
  }, [eventTick]);

  const openHistory = async (serverId: number, dbName: string) => {
    setHistoryTarget({ serverId, dbName });
    setHistoryLoading(true);
    try {
      const items = await apiCall<BackupExecution[]>(
        'GET',
        `/api/v1/executions?serverId=${serverId}&dbName=${encodeURIComponent(dbName)}&limit=100`,
      );
      setHistoryItems(items);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    } finally {
      setHistoryLoading(false);
    }
  };

  const triggerBackupNow = async (serverId: number, dbName: string) => {
    try {
      await apiCall('POST', '/api/v1/backups/run-db', { serverId, dbName, profileId: 0 });
      notifyBanner(
        lang === 'es' ? `Iniciando copia de ${dbName}...` : `Starting backup for ${dbName}...`,
        'info',
      );
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-bold">{t('dbTitle')}</h2>
          <p className="text-xs text-slate-500">{t('dbSubtitle')}</p>
        </div>
        <button
          onClick={loadCatalog}
          className="px-3.5 py-2 rounded-xl border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium flex items-center gap-1.5"
        >
          <RefreshCw className="w-3.5 h-3.5" /> {t('refresh')}
        </button>
      </div>

      {loading && dbs.length === 0 ? (
        <div className="p-8 text-xs text-slate-400">Loading databases...</div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {dbs.map((item) => (
            <div
              key={`${item.serverId}-${item.dbName}`}
              className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 flex flex-col justify-between gap-4 hover:border-cyan-500/40 transition shadow-sm"
            >
              <div>
                <div className="flex items-start justify-between gap-2">
                  <div className="flex items-center gap-2.5">
                    <div className="p-2 rounded-xl bg-cyan-500/10 text-cyan-500">
                      <Database className="w-5 h-5" />
                    </div>
                    <div>
                      <h3 className="font-bold text-sm">{item.dbName}</h3>
                      <span className="text-[11px] text-slate-400">
                        {item.serverName} • {t('dbSizeLive')}: <b>{formatBytes(item.liveSizeBytes)}</b>
                      </span>
                    </div>
                  </div>
                </div>

                <div className="mt-4 flex flex-wrap gap-1.5">
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
                  onClick={() => triggerBackupNow(item.serverId, item.dbName)}
                  className="flex-1 py-2 px-3 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center justify-center gap-1.5"
                >
                  <Play className="w-3.5 h-3.5" /> {t('dbBackupNow')}
                </button>
                <button
                  onClick={() => openHistory(item.serverId, item.dbName)}
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
                {t('dbHistoryTitle')}: <span className="text-cyan-500">{historyTarget.dbName}</span>
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
