import React, { useEffect, useState } from 'react';
import { Plus, Play, Trash2, Edit3, Folder, Check, Clock } from 'lucide-react';
import { apiCall, BackupProfile, formatBytes, PGServer } from '../lib/bridge';
import { useApp } from '../context/AppContext';
import { DriveFolderModal } from '../components/DriveFolderModal';
import { ScheduleBuilder } from '../components/ScheduleBuilder';
import { describeCron } from '../lib/scheduleUtils';

const emptyProfile = (defaultServerId: number): BackupProfile => ({
  id: 0,
  name: '',
  pgServerId: defaultServerId,
  enabled: true,
  scheduleCron: '0 2 * * *',
  scheduleLabel: 'Daily 02:00',
  format: 'custom',
  selectionMode: 'manual',
  includedDatabases: [],
  excludedDatabases: [],
  localDirOverride: '',
  localRetentionMode: 'count',
  localRetentionValue: 7,
  driveEnabled: true,
  driveFolderId: '',
  driveFolderPath: '',
  driveRetentionMode: 'count',
  driveRetentionValue: 14,
});

export const ProfilesView: React.FC = () => {
  const { lang, t, notifyBanner } = useApp();
  const [profiles, setProfiles] = useState<BackupProfile[]>([]);
  const [servers, setServers] = useState<PGServer[]>([]);
  const [editing, setEditing] = useState<BackupProfile | null>(null);
  const [serverDBs, setServerDBs] = useState<{ name: string; sizeBytes: number }[]>([]);
  const [folderModalOpen, setFolderModalOpen] = useState(false);

  const loadAll = async () => {
    try {
      const [profs, srvs] = await Promise.all([
        apiCall<BackupProfile[]>('GET', '/api/v1/profiles'),
        apiCall<PGServer[]>('GET', '/api/v1/servers'),
      ]);
      setProfiles(profs);
      setServers(srvs);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  useEffect(() => {
    void loadAll();
  }, []);

  const loadDBsForServer = async (srvId: number) => {
    const srv = servers.find((s) => s.id === srvId);
    if (!srv) return;
    try {
      const list = await apiCall<{ name: string; sizeBytes: number }[]>('POST', '/api/v1/postgres/databases-preview', srv);
      setServerDBs(list);
    } catch {
      setServerDBs([]);
    }
  };

  const startEdit = (p: BackupProfile) => {
    setEditing({ ...p });
    void loadDBsForServer(p.pgServerId);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editing) return;
    try {
      await apiCall('POST', '/api/v1/profiles', editing);
      notifyBanner(t('savedOk'), 'success');
      setEditing(null);
      await loadAll();
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const handleDelete = async (id: number) => {
    try {
      await apiCall('DELETE', `/api/v1/profiles?id=${id}`);
      await loadAll();
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const runProfileNow = async (profileId: number) => {
    try {
      await apiCall('POST', '/api/v1/backups/run-profile', { profileId });
      notifyBanner(lang === 'es' ? 'Perfil ejecutándose en segundo plano' : 'Profile started in background', 'info');
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-bold">{t('profTitle')}</h2>
        <button
          onClick={() => {
            const defId = servers[0]?.id || 0;
            const p = emptyProfile(defId);
            startEdit(p);
          }}
          className="px-4 py-2 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center gap-1.5"
        >
          <Plus className="w-4 h-4" /> {t('profNew')}
        </button>
      </div>

      {!editing ? (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          {profiles.map((p) => (
            <div
              key={p.id}
              className="p-5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4 shadow-sm"
            >
              <div className="flex items-start justify-between">
                <div>
                  <div className="flex items-center gap-2">
                    <h3 className="font-bold text-base">{p.name}</h3>
                    <span
                      className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${
                        p.enabled ? 'bg-emerald-500/15 text-emerald-500' : 'bg-slate-500/15 text-slate-400'
                      }`}
                    >
                      {p.enabled ? 'ON' : 'OFF'}
                    </span>
                  </div>
                  <p className="text-xs text-slate-500 mt-0.5">
                    {p.serverName} • {p.format === 'plain_gz' ? '.sql.gz' : '-Fc .backup'}
                  </p>
                  <p className="text-xs text-cyan-700 dark:text-cyan-400 font-medium mt-1 flex items-center gap-1.5">
                    <Clock className="w-3.5 h-3.5 shrink-0" />
                    <span>{describeCron(p.scheduleCron, lang)}</span>
                    <code className="text-[10px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-500 font-mono">
                      {p.scheduleCron}
                    </code>
                  </p>
                </div>
                <div className="flex gap-1">
                  <button
                    onClick={() => runProfileNow(p.id)}
                    className="p-2 rounded-lg bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 hover:bg-cyan-500/20"
                    title={t('profRunNow')}
                  >
                    <Play className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => startEdit(p)}
                    className="p-2 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800"
                  >
                    <Edit3 className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => handleDelete(p.id)}
                    className="p-2 rounded-lg hover:bg-red-500/10 text-red-500"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3 text-xs p-3 rounded-xl bg-slate-50 dark:bg-slate-950/60">
                <div>
                  <span className="text-slate-400 block text-[11px]">{t('profLocalRet')}</span>
                  <b>
                    {p.localRetentionValue} {p.localRetentionMode === 'days' ? 'days' : 'copies'}
                  </b>
                </div>
                <div>
                  <span className="text-slate-400 block text-[11px]">Google Drive</span>
                  <b>
                    {p.driveEnabled
                      ? `${p.driveRetentionValue} ${p.driveRetentionMode === 'days' ? 'days' : 'copies'}`
                      : 'Disabled'}
                  </b>
                </div>
              </div>

              <div className="text-xs text-slate-500">
                {p.selectionMode === 'all_auto' ? (
                  <span className="text-emerald-600 dark:text-emerald-400 font-medium">
                    ★ {t('profModeAutoAll')} ({p.excludedDatabases.length} excl.)
                  </span>
                ) : (
                  <span>
                    DBs ({p.includedDatabases.length}): {p.includedDatabases.join(', ') || '—'}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <form
          onSubmit={handleSave}
          className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-5"
        >
          <div className="flex items-center justify-between border-b border-slate-200 dark:border-slate-800 pb-3">
            <h3 className="font-bold text-base">{editing.id ? t('profEdit') : t('profNew')}</h3>
            <label className="flex items-center gap-2 text-xs font-semibold cursor-pointer">
              <input
                type="checkbox"
                checked={editing.enabled}
                onChange={(e) => setEditing({ ...editing, enabled: e.target.checked })}
                className="accent-cyan-600"
              />
              {lang === 'es' ? 'Perfil Habilitado' : 'Profile Enabled'}
            </label>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div>
              <label className="block text-xs font-semibold mb-1">{t('profName')}</label>
              <input
                required
                value={editing.name}
                onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
              />
            </div>
            <div>
              <label className="block text-xs font-semibold mb-1">PostgreSQL Server</label>
              <select
                value={editing.pgServerId}
                onChange={(e) => {
                  const id = Number(e.target.value);
                  setEditing({ ...editing, pgServerId: id, includedDatabases: [], excludedDatabases: [] });
                  void loadDBsForServer(id);
                }}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
              >
                {servers.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name} ({s.host}:{s.port})
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs font-semibold mb-1">{t('profFormat')}</label>
              <select
                value={editing.format}
                onChange={(e) => setEditing({ ...editing, format: e.target.value as 'custom' | 'plain_gz' })}
                className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
              >
                <option value="custom">{t('profFormatCustom')}</option>
                <option value="plain_gz">{t('profFormatPlainGz')}</option>
              </select>
            </div>
          </div>

          <ScheduleBuilder
            value={editing.scheduleCron}
            onChange={(cronExpr, humanLabel) =>
              setEditing({ ...editing, scheduleCron: cronExpr, scheduleLabel: humanLabel })
            }
          />

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6 pt-2">
            <div className="space-y-3 p-4 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800">
              <h4 className="font-semibold text-xs">{t('profLocalRet')}</h4>
              <div className="flex gap-2">
                <select
                  value={editing.localRetentionMode}
                  onChange={(e) => setEditing({ ...editing, localRetentionMode: e.target.value as 'count' | 'days' })}
                  className="flex-1 px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs"
                >
                  <option value="count">{t('profKeepCount')}</option>
                  <option value="days">{t('profKeepDays')}</option>
                </select>
                <input
                  type="number"
                  min={1}
                  value={editing.localRetentionValue}
                  onChange={(e) => setEditing({ ...editing, localRetentionValue: Number(e.target.value) })}
                  className="w-24 px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs"
                />
              </div>
            </div>

            <div className="space-y-3 p-4 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800">
              <div className="flex items-center justify-between">
                <h4 className="font-semibold text-xs">{t('profDriveSync')}</h4>
                <input
                  type="checkbox"
                  checked={editing.driveEnabled}
                  onChange={(e) => setEditing({ ...editing, driveEnabled: e.target.checked })}
                  className="accent-cyan-600"
                />
              </div>
              {editing.driveEnabled && (
                <>
                  <div className="flex gap-2">
                    <select
                      value={editing.driveRetentionMode}
                      onChange={(e) =>
                        setEditing({ ...editing, driveRetentionMode: e.target.value as 'count' | 'days' })
                      }
                      className="flex-1 px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs"
                    >
                      <option value="count">{t('profKeepCount')}</option>
                      <option value="days">{t('profKeepDays')}</option>
                    </select>
                    <input
                      type="number"
                      min={1}
                      value={editing.driveRetentionValue}
                      onChange={(e) => setEditing({ ...editing, driveRetentionValue: Number(e.target.value) })}
                      className="w-24 px-3 py-2 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs"
                    />
                  </div>
                  <div className="flex items-center justify-between text-xs pt-1">
                    <span className="text-slate-500 truncate">
                      {editing.driveFolderPath || (lang === 'es' ? 'Usar carpeta global de Drive' : 'Use global Drive folder')}
                    </span>
                    <button
                      type="button"
                      onClick={() => setFolderModalOpen(true)}
                      className="px-2.5 py-1 rounded bg-slate-200 dark:bg-slate-800 text-[11px] font-medium flex items-center gap-1"
                    >
                      <Folder className="w-3 h-3" /> {t('cloudChooseFolder')}
                    </button>
                  </div>
                </>
              )}
            </div>
          </div>

          <div className="space-y-2">
            <label className="block text-xs font-semibold">{t('profSelectionMode')}</label>
            <select
              value={editing.selectionMode}
              onChange={(e) => setEditing({ ...editing, selectionMode: e.target.value as 'manual' | 'all_auto' })}
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
            >
              <option value="manual">{t('profModeManual')}</option>
              <option value="all_auto">{t('profModeAutoAll')}</option>
            </select>

            <div className="max-h-48 overflow-y-auto rounded-xl border border-slate-200 dark:border-slate-800 grid grid-cols-2 md:grid-cols-3 gap-1 p-2">
              {serverDBs.map((db) => {
                const isManual = editing.selectionMode === 'manual';
                const list = isManual ? editing.includedDatabases : editing.excludedDatabases;
                const checked = list.includes(db.name);
                return (
                  <label
                    key={db.name}
                    className="px-3 py-2 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800/60 flex items-center justify-between text-xs cursor-pointer"
                  >
                    <span className="truncate">
                      <b>{db.name}</b> <span className="text-[10px] text-slate-400">({formatBytes(db.sizeBytes)})</span>
                    </span>
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={(e) => {
                        const updated = e.target.checked ? [...list, db.name] : list.filter((x) => x !== db.name);
                        setEditing(
                          isManual
                            ? { ...editing, includedDatabases: updated }
                            : { ...editing, excludedDatabases: updated },
                        );
                      }}
                      className="accent-cyan-600 ml-2"
                    />
                  </label>
                );
              })}
            </div>
            {editing.selectionMode === 'all_auto' && (
              <p className="text-[11px] text-amber-500">
                {lang === 'es'
                  ? 'En modo automático, marca únicamente las bases que quieras EXCLUIR.'
                  : 'In auto-include mode, check only the databases you want to EXCLUDE.'}
              </p>
            )}
          </div>

          <div className="flex justify-end gap-2 pt-3 border-t border-slate-200 dark:border-slate-800">
            <button
              type="button"
              onClick={() => setEditing(null)}
              className="px-4 py-2 rounded-xl border border-slate-300 dark:border-slate-700 text-xs font-medium"
            >
              {t('cancel')}
            </button>
            <button
              type="submit"
              className="px-5 py-2 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center gap-1.5"
            >
              <Check className="w-4 h-4" /> {t('save')}
            </button>
          </div>
        </form>
      )}

      <DriveFolderModal
        open={folderModalOpen}
        onClose={() => setFolderModalOpen(false)}
        onSelect={(id, path) => {
          if (editing) setEditing({ ...editing, driveFolderId: id, driveFolderPath: path });
        }}
      />
    </div>
  );
};
