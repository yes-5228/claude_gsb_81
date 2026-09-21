// 后端接口的统一请求封装。
//
// 后端所有响应都是 { code, message, data, timestamp } 信封结构，
// 这里统一拆包并抛出带中文提示的 ApiError，页面只需要处理成功数据。

const BASE_PATH = '/api/v1';

export { BASE_PATH };

export interface Envelope<T> {
  code: number;
  message: string;
  data: T;
  timestamp: number;
}

export class ApiError extends Error {
  readonly code: number;
  readonly status: number;

  constructor(message: string, code: number, status: number) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

async function unwrap<T>(response: Response): Promise<T> {
  const text = await response.text();
  if (!text) {
    throw new ApiError('后端返回了空响应，请稍后重试', -1, response.status);
  }

  let envelope: Envelope<T>;
  try {
    envelope = JSON.parse(text) as Envelope<T>;
  } catch {
    throw new ApiError(`后端返回的内容无法解析（HTTP ${response.status}）`, -1, response.status);
  }

  if (!response.ok || envelope.code !== 0) {
    const message = envelope.message || `请求失败（HTTP ${response.status}）`;
    throw new ApiError(message, envelope.code, response.status);
  }
  return envelope.data;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${BASE_PATH}${path}`, {
      headers: { 'Content-Type': 'application/json' },
      ...init
    });
  } catch {
    throw new ApiError('无法连接后端服务，请确认后端已启动', -1, 0);
  }
  return unwrap<T>(response);
}

export const http = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) }),
  put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body: JSON.stringify(body) }),
  del: <T>(path: string) => request<T>(path, { method: 'DELETE' })
};

type QueryValue = string | number | boolean | undefined | null;

/** 把查询条件拼成查询串，自动跳过空值。 */
export function buildQuery(params: Record<string, QueryValue>): string {
  const search = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === '') {
      return;
    }
    search.set(key, String(value));
  });
  const query = search.toString();
  return query ? `?${query}` : '';
}

/** 把任意异常转换成可以直接展示的中文提示。 */
export function toErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return '发生未知错误，请稍后重试';
}

/**
 * 下载文件（导出等场景）。
 *
 * 后端出错时仍返回统一信封 JSON，这里先按内容类型判断：JSON 走错误解析，
 * 其余内容按文件落盘；文件名优先取 Content-Disposition。
 */
export async function downloadFile(url: string, fallbackFilename: string): Promise<void> {
  let response: Response;
  try {
    response = await fetch(url);
  } catch {
    throw new ApiError('无法连接后端服务，请确认后端已启动', -1, 0);
  }

  const contentType = response.headers.get('Content-Type') ?? '';
  if (!response.ok || contentType.includes('application/json')) {
    const text = await response.text();
    let message = `导出失败（HTTP ${response.status}）`;
    try {
      const envelope = JSON.parse(text) as Envelope<unknown>;
      if (envelope.message) {
        message = envelope.message;
      }
    } catch {
      // 非 JSON 错误体，保留默认提示。
    }
    throw new ApiError(message, -1, response.status);
  }

  const blob = await response.blob();
  const disposition = response.headers.get('Content-Disposition') ?? '';
  const filename = parseFilename(disposition) ?? fallbackFilename;

  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = objectUrl;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(objectUrl);
}

function parseFilename(disposition: string): string | null {
  const utf8Match = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  if (utf8Match?.[1]) {
    try {
      return decodeURIComponent(utf8Match[1]);
    } catch {
      return utf8Match[1];
    }
  }
  const plainMatch = /filename="?([^";]+)"?/i.exec(disposition);
  return plainMatch?.[1] ? plainMatch[1] : null;
}
