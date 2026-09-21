// 常用筛选条件的本地保存。
//
// 常用筛选属于个人使用习惯，不跨设备同步，存 localStorage 即可，
// 避免为此引入后端表与登录态。
import type { TaskFilterPreset } from '../../api/tasks';

export interface SavedFilter {
  id: string;
  name: string;
  filter: TaskFilterPreset;
  createdAt: number;
}

const STORAGE_KEY = 'desilting:task-filter-presets:v1';

export function loadSavedFilters(): SavedFilter[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw) as SavedFilter[];
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter(
      (item) => item && typeof item.id === 'string' && typeof item.name === 'string' && item.filter
    );
  } catch {
    return [];
  }
}

export function saveFilter(name: string, filter: TaskFilterPreset): SavedFilter[] {
  const filters = loadSavedFilters();
  const next: SavedFilter = {
    id: `f_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
    name,
    filter,
    createdAt: Date.now()
  };
  filters.unshift(next);
  persist(filters);
  return filters;
}

export function removeSavedFilter(id: string): SavedFilter[] {
  const filters = loadSavedFilters().filter((item) => item.id !== id);
  persist(filters);
  return filters;
}

function persist(filters: SavedFilter[]) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(filters.slice(0, 20)));
  } catch {
    // 隐私模式或存储写满时静默失败，不影响筛选主流程。
  }
}
