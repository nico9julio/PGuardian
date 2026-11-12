import React, { useEffect, useState } from 'react';
import { CheckCircle2, Cloud, AlertTriangle, HardDrive, Activity, FolderOpen, RefreshCw } from 'lucide-react';
import { apiCall, BackupExecution, DriveStatus, formatBytes, formatDateTime, nativeBridge } from '../lib/bridge';
import { useApp } from '../context/AppContext';

interface DashboardData {
  metrics: {
    totalExecutions24h: number;
    success24h: number;
    failed24h: number;
    pendingCloudCount: number;
    totalLocalBytes: number;
    activeProfiles: number;
    configuredServers: number;
  };
  disk: {
    path: string;
    freeGB: number;
    totalGB: number;
    usedPct: number;
  };
  recentBackups: BackupExecution[];
  driveStatus: DriveStatus;
  activeJobs: number;
  serviceVersion: string;
}

export const DashboardView: React.FC = () => {
  const { lang, t, liveJob, eventTick } = useApp();
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);

  const load = async () => {
    try {
      const res = await apiCall<DashboardData>('GET', '/api/v1/dashboard');
      setData(res);
    } catch {}
    setLoading(false);
  };

  useEffect(() => {
    void load();
  }, [eventTick]);

  if (loading && !data) {
    return <div className="p-8 text-sm text-slate-400">Loading...</div>;
  }

  const m = data?.metrics;
  const d = data?.disk;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-bold">{t('navDashboard')}</h2>
          <p className="text-xs text-slate-500 dark:text-slate-400">
            {data?.driveStatus?.connected
              ? `Google Drive: ${data.driveStatus.accountEmail} (${data.driveStatus.rootFolderName || '/'})`
              : lang === 'es'
              ? 'Google Drive no vinculado aún'
              : 'Google Drive not linked yet'}
          </p>
        </div>
        <button
          onClick={load}
          className="px-3.5 py-2 rounded-xl border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium flex items-center gap-1.5"
        >
          <RefreshCw className="w-3.5 h-3.5" /> {t('refresh')}
        </button>
      </div>

      {liveJob && (
        <div className="p-4 rounded-2xl bg-cyan-500/10 border border-cyan-500/30 flex items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <Activity className="w-5 h-5 text-cyan-500 animate-pulse" />
            <div>
              <div className="text-xs font-bold text-cyan-600 dark:text-cyan-400">
                {t('dashActiveNow')}: {liveJob.title}
              </div>
              <div className="text-xs text-slate-500">{liveJob.message}</div>
            </div>
          </div>
          {typeof liveJob.percent === 'number' && liveJob.percent > 0 && (
            <div className="w-48 bg-slate-200 dark:bg-slate-800 h-2 rounded-full overflow-hidden">
              <div className="bg-cyan-500 h-full transition-all duration-300" style={{ width: `${liveJob.percent}%` }} />
            </div>
          )}
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-2">
            <span>{t('dash24hBackups')}</span>
            <CheckCircle2 className="w-4 h-4 text-emerald-500" />
          </div>
          <div className="text-2xl font-bold">{m?.totalExecutions24h ?? 0}</div>
          <div className="text-xs mt-1 text-slate-500">
            <span className="text-emerald-500 font-semibold">{m?.success24h ?? 0} OK</span> •{' '}
            <span className={m?.failed24h ? 'text-red-500 font-semibold' : ''}>{m?.failed24h ?? 0} Err</span>
          </div>
        </div>

        <div className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-2">
            <span>{t('dashCloudQueue')}</span>
            <Cloud className="w-4 h-4 text-blue-500" />
          </div>
          <div className="text-2xl font-bold">{m?.pendingCloudCount ?? 0}</div>
          <div className="text-xs mt-1 text-slate-500">
            {data?.driveStatus?.connected ? 'Resumable Sync Ready' : 'Offline / Unlinked'}
          </div>
        </div>

        <div className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-2">
            <span>{t('dashLocalUsed')}</span>
            <HardDrive className="w-4 h-4 text-cyan-500" />
          </div>
          <div className="text-2xl font-bold">{formatBytes(m?.totalLocalBytes ?? 0)}</div>
          <div className="text-xs mt-1 text-slate-500">
            {m?.activeProfiles ?? 0} {lang === 'es' ? 'perfiles activos' : 'active profiles'}
          </div>
        </div>

        <div className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900">
          <div className="flex items-center justify-between text-xs text-slate-500 mb-2">
            <span>{t('dashFreeDisk')}</span>
            <AlertTriangle className={`w-4 h-4 ${(d?.freeGB ?? 99) < 10 ? 'text-amber-500' : 'text-slate-400'}`} />
          </div>
          <div className="text-2xl font-bold">{d ? `${d.freeGB.toFixed(1)} GB` : '—'}</div>
          <div className="w-full bg-slate-200 dark:bg-slate-800 h-1.5 rounded-full mt-2 overflow-hidden">
            <div
              className={`h-full ${(d?.usedPct ?? 0) > 88 ? 'bg-red-500' : 'bg-cyan-500'}`}
              style={{ width: `${Math.min(100, d?.usedPct ?? 0)}%` }}
            />
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 overflow-hidden">
        <div className="px-5 py-4 border-b border-slate-200 dark:border-slate-800 font-semibold text-sm">
          {t('dashRecentActivity')}
        </div>
        {!data?.recentBackups?.length ? (
          <div className="p-8 text-center text-xs text-slate-400">{t('dashNoBackupsYet')}</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="bg-slate-50 dark:bg-slate-950/60 text-slate-500 text-left">
                <tr>
                  <th className="px-4 py-3">Database</th>
                  <th className="px-4 py-3">Profile</th>
                  <th className="px-4 py-3">Started</th>
                  <th className="px-4 py-3">Size</th>
                  <th className="px-4 py-3">Local</th>
                  <th className="px-4 py-3">Google Drive</th>
                  <th className="px-4 py-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800/70">
                {data.recentBackups.map((ex) => (
                  <tr key={ex.id} className="hover:bg-slate-50/70 dark:hover:bg-slate-800/40">
                    <td className="px-4 py-3 font-semibold">{ex.dbName}</td>
                    <td className="px-4 py-3 text-slate-500">{ex.profileName || ex.triggerType}</td>
                    <td className="px-4 py-3 text-slate-500">{formatDateTime(ex.startedAt)}</td>
                    <td className="px-4 py-3 font-mono">{formatBytes(ex.fileSizeBytes)}</td>
                    <td className="px-4 py-3">
                      <StatusBadge status={ex.localStatus} />
                    </td>
                    <td className="px-4 py-3">
                      <StatusBadge status={ex.cloudStatus} />
                    </td>
                    <td className="px-4 py-3 text-right">
                      {ex.localFilePath && !ex.deletedLocalAt && (
                        <button
                          onClick={() => nativeBridge.revealInExplorer(ex.localFilePath)}
                          className="p-1.5 rounded-lg hover:bg-slate-200 dark:hover:bg-slate-700 text-slate-500"
                          title={t('dbOpenExplorer')}
                        >
                          <FolderOpen className="w-3.5 h-3.5" />
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};

export const StatusBadge: React.FC<{ status: string }> = ({ status }) => {
  const styles: Record<string, string> = {
    completed: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400',
    running: 'bg-cyan-500/15 text-cyan-600 dark:text-cyan-400',
    uploading: 'bg-blue-500/15 text-blue-600 dark:text-blue-400',
    pending: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    retrying: 'bg-amber-500/15 text-amber-600 dark:text-amber-400',
    failed: 'bg-red-500/15 text-red-600 dark:text-red-400',
    not_applicable: 'bg-slate-500/10 text-slate-400',
  };
  return (
    <span className={`px-2 py-0.5 rounded-full text-[11px] font-semibold ${styles[status] || styles.not_applicable}`}>
      {status === 'not_applicable' ? '—' : status}
    </span>
  );
};
