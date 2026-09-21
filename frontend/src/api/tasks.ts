import type { TaskDetail, TaskFilterOptions, TaskListResult, TaskPayload } from '../types/domain';
import { buildQuery, http } from './client';

export interface TaskQuery {
  keyword?: string;
  status?: string;
  district?: string;
  roadName?: string;
  priority?: string;
  source?: string;
  teamName?: string;
  pipeSegmentId?: number;
  planFrom?: string;
  planTo?: string;
  page?: number;
  pageSize?: number;
}

export const taskApi = {
  list: (query: TaskQuery) => http.get<TaskListResult>(`/cleaning-tasks${buildQuery({ ...query })}`),
  filterOptions: (district?: string) =>
    http.get<TaskFilterOptions>(`/cleaning-tasks/filter-options${buildQuery({ district })}`),
  exportUrl: (filters: Record<string, string>) => `/api/v1/cleaning-tasks/export${buildQuery({ ...filters })}`,
  detail: (id: number) => http.get<TaskDetail>(`/cleaning-tasks/${id}`),
  create: (payload: TaskPayload) => http.post<{ id: number }>('/cleaning-tasks', payload),
  update: (id: number, payload: TaskPayload) => http.put<{ id: number }>(`/cleaning-tasks/${id}`, payload),
  remove: (id: number) => http.del<{ id: number }>(`/cleaning-tasks/${id}`),
  start: (id: number) => http.post<{ id: number }>(`/cleaning-tasks/${id}/start`),
  complete: (id: number) => http.post<{ id: number }>(`/cleaning-tasks/${id}/complete`),
  cancel: (id: number, reason: string) => http.post<{ id: number }>(`/cleaning-tasks/${id}/cancel`, { reason })
};
