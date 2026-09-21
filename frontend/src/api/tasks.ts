import type {
  TaskDetail,
  TaskFilterOptions,
  TaskListResult,
  TaskPayload
} from '../types/domain';
import { BASE_PATH, buildQuery, downloadFile, http } from './client';

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

/** 可以保存为常用筛选的条件（不含分页）。 */
export interface TaskFilterPreset {
  keyword?: string;
  status?: string;
  district?: string;
  roadName?: string;
  priority?: string;
  source?: string;
  teamName?: string;
  planFrom?: string;
  planTo?: string;
}

export const taskApi = {
  list: (query: TaskQuery) =>
    http.get<TaskListResult>(`/cleaning-tasks${buildQuery({ ...query })}`),
  filterOptions: (district?: string) =>
    http.get<TaskFilterOptions>(`/cleaning-tasks/options${buildQuery({ district })}`),
  detail: (id: number) => http.get<TaskDetail>(`/cleaning-tasks/${id}`),
  create: (payload: TaskPayload) => http.post<{ id: number }>('/cleaning-tasks', payload),
  update: (id: number, payload: TaskPayload) => http.put<{ id: number }>(`/cleaning-tasks/${id}`, payload),
  remove: (id: number) => http.del<{ id: number }>(`/cleaning-tasks/${id}`),
  start: (id: number) => http.post<{ id: number }>(`/cleaning-tasks/${id}/start`),
  complete: (id: number) => http.post<{ id: number }>(`/cleaning-tasks/${id}/complete`),
  cancel: (id: number, reason: string) => http.post<{ id: number }>(`/cleaning-tasks/${id}/cancel`, { reason }),
  /**
   * 按当前筛选条件导出 CSV。
   * 导出请求与列表使用完全相同的查询参数（不含分页），保证同口径。
   */
  exportList: (query: TaskQuery) =>
    downloadFile(
      `${BASE_PATH}/cleaning-tasks/export${buildQuery({ ...query, page: undefined, pageSize: undefined })}`,
      '清淤任务.csv'
    )
};
