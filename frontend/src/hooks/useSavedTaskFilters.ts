// 常用筛选条件的本地保存：任务列表支持把当前一组筛选条件命名后保存，
// 之后一键套用。数据只存浏览器本地，不与其他设备同步。
import { useCallback, useEffect, useState } from 'react';

export interface SavedFilter {
  id: string;
  name: string;
  params: Record<string, string>;
  createdAt: number;
}

const STORAGE_KEY = 'desilting:task-saved-filters:v1';
const MAX_FILTERS = 20;

function readAll(): SavedFilter[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.filter(
      (item): item is SavedFilter =>
        typeof item === 'object' &&
        item !== null &&
        typeof (item as SavedFilter).id === 'string' &&
        typeof (item as SavedFilter).name === 'string' &&
        typeof (item as SavedFilter).params === 'object'
    );
  } catch {
    return [];
  }
}

export function useSavedTaskFilters() {
  const [filters, setFilters] = useState<SavedFilter[]>(() => readAll());

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, JSON.stringify(filters));
    } catch {
      // 隐私模式或存储已满时静默失败，不影响列表本身的使用。
    }
  }, [filters]);

  const saveFilter = useCallback((name: string, params: Record<string, string>): boolean => {
    const trimmed = name.trim();
    if (!trimmed || Object.keys(params).length === 0) {
      return false;
    }
    const saved: SavedFilter = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      name: trimmed,
      params,
      createdAt: Date.now()
    };
    setFilters((current) => [saved, ...current].slice(0, MAX_FILTERS));
    return true;
  }, []);

  const removeFilter = useCallback((id: string) => {
    setFilters((current) => current.filter((item) => item.id !== id));
  }, []);

  return { filters, saveFilter, removeFilter };
}
