import React, { useEffect, useState } from 'react';
import { Folder, FolderPlus, ChevronRight, X, Check, Loader2 } from 'lucide-react';
import { apiCall, DriveFolder } from '../lib/bridge';
import { useApp } from '../context/AppContext';

interface Props {
  open: boolean;
  onClose: () => void;
  onSelect: (folderId: string, folderPath: string) => void;
}

export const DriveFolderModal: React.FC<Props> = ({ open, onClose, onSelect }) => {
  const { lang, t } = useApp();
  const [crumbs, setCrumbs] = useState<{ id: string; name: string }[]>([
    { id: 'root', name: lang === 'es' ? 'Mi unidad' : 'My Drive' },
  ]);
  const [folders, setFolders] = useState<DriveFolder[]>([]);
  const [loading, setLoading] = useState(false);
  const [newFolderName, setNewFolderName] = useState('');
  const [creating, setCreating] = useState(false);
  const [err, setErr] = useState('');

  const currentParent = crumbs[crumbs.length - 1];

  const loadFolders = async (parentId: string) => {
    setLoading(true);
    setErr('');
    try {
      const list = await apiCall<DriveFolder[]>('GET', `/api/v1/cloud/folders?parentId=${encodeURIComponent(parentId)}`);
      setFolders(list);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (open) {
      void loadFolders(currentParent.id);
    }
  }, [open, currentParent.id]);

  if (!open) return null;

  const handleCreate = async () => {
    if (!newFolderName.trim()) return;
    setCreating(true);
    setErr('');
    try {
      const created = await apiCall<DriveFolder>('POST', '/api/v1/cloud/folders', {
        parentId: currentParent.id,
        name: newFolderName.trim(),
      });
      setNewFolderName('');
      await loadFolders(currentParent.id);
      setCrumbs((prev) => [...prev, { id: created.id, name: created.name }]);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  const fullPath = '/' + crumbs.map((c) => c.name).join('/');

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/70 backdrop-blur-sm p-4">
      <div className="w-full max-w-lg rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-2xl overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-slate-200 dark:border-slate-800">
          <h3 className="font-semibold text-base flex items-center gap-2">
            <Folder className="w-5 h-5 text-cyan-500" />
            {t('cloudChooseFolder')}
          </h3>
          <button onClick={onClose} className="p-1 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800">
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="px-5 py-2.5 bg-slate-50 dark:bg-slate-950/60 border-b border-slate-200 dark:border-slate-800 flex items-center flex-wrap gap-1 text-xs">
          {crumbs.map((c, idx) => (
            <React.Fragment key={c.id + idx}>
              {idx > 0 && <ChevronRight className="w-3.5 h-3.5 text-slate-400" />}
              <button
                onClick={() => setCrumbs(crumbs.slice(0, idx + 1))}
                className="px-2 py-1 rounded hover:bg-slate-200/70 dark:hover:bg-slate-800 font-medium text-cyan-600 dark:text-cyan-400"
              >
                {c.name}
              </button>
            </React.Fragment>
          ))}
        </div>

        <div className="p-5 space-y-4">
          {err && (
            <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/30 text-red-500 text-xs">{err}</div>
          )}

          <div className="h-60 overflow-y-auto rounded-xl border border-slate-200 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800/60">
            {loading ? (
              <div className="h-full flex items-center justify-center text-slate-400 gap-2 text-sm">
                <Loader2 className="w-4 h-4 animate-spin" />
                {lang === 'es' ? 'Cargando carpetas...' : 'Loading folders...'}
              </div>
            ) : folders.length === 0 ? (
              <div className="h-full flex items-center justify-center text-slate-400 text-sm">
                {lang === 'es' ? 'Carpeta vacía (se crearán subcarpetas por BD aquí)' : 'Empty folder (DB subfolders will be created here)'}
              </div>
            ) : (
              folders.map((f) => (
                <button
                  key={f.id}
                  onClick={() => setCrumbs((prev) => [...prev, { id: f.id, name: f.name }])}
                  className="w-full px-4 py-2.5 flex items-center justify-between hover:bg-slate-50 dark:hover:bg-slate-800/60 text-sm text-left transition"
                >
                  <span className="flex items-center gap-2.5 font-medium">
                    <Folder className="w-4 h-4 text-amber-500 fill-amber-500/20" />
                    {f.name}
                  </span>
                  <ChevronRight className="w-4 h-4 text-slate-400" />
                </button>
              ))
            )}
          </div>

          <div className="flex gap-2">
            <input
              type="text"
              value={newFolderName}
              onChange={(e) => setNewFolderName(e.target.value)}
              placeholder={lang === 'es' ? 'Nombre de nueva carpeta (ej. PGuardianBackups)' : 'New folder name (e.g. PGuardianBackups)'}
              className="flex-1 px-3 py-2 text-sm rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 focus:outline-none focus:ring-2 focus:ring-cyan-500"
            />
            <button
              onClick={handleCreate}
              disabled={creating || !newFolderName.trim()}
              className="px-3 py-2 rounded-lg bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-xs font-medium flex items-center gap-1.5 disabled:opacity-50"
            >
              <FolderPlus className="w-4 h-4" />
              {t('cloudNewFolder')}
            </button>
          </div>
        </div>

        <div className="px-5 py-3.5 bg-slate-50 dark:bg-slate-950/60 border-t border-slate-200 dark:border-slate-800 flex items-center justify-between">
          <span className="text-xs text-slate-500 truncate max-w-[240px]">{fullPath}</span>
          <div className="flex gap-2">
            <button
              onClick={onClose}
              className="px-3.5 py-2 rounded-lg text-xs font-medium hover:bg-slate-200 dark:hover:bg-slate-800"
            >
              {t('cancel')}
            </button>
            <button
              onClick={() => {
                onSelect(currentParent.id, fullPath);
                onClose();
              }}
              className="px-4 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center gap-1.5 shadow-sm"
            >
              <Check className="w-4 h-4" />
              {t('cloudSelectCurrent')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
};
