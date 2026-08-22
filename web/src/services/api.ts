import type { AppError } from '@/types/api'

type Envelope<T> = { data: T }

export class ApiError extends Error implements AppError {
  errorCode: string
  module: string
  recoverable: boolean
  cause?: string
  suggestion?: string
  traceId: string

  constructor(error: AppError) {
    super(error.message)
    this.name = 'ApiError'
    this.errorCode = error.errorCode
    this.module = error.module
    this.recoverable = error.recoverable
    this.cause = error.cause
    this.suggestion = error.suggestion
    this.traceId = error.traceId
  }
}

async function parseError(response: Response): Promise<ApiError> {
  try {
    const payload = (await response.json()) as { error: AppError }
    if (payload.error?.errorCode) return new ApiError(payload.error)
  } catch {
    // Reverse proxies may return HTML; keep a stable local error contract.
  }
  return new ApiError({
    errorCode: response.status === 401 ? 'AUTH_REQUIRED' : 'HTTP_ERROR',
    message: response.status === 401 ? '登录状态已失效，请重新登录' : `服务请求失败（${response.status}）`,
    module: 'api.client',
    recoverable: true,
    traceId: response.headers.get('x-request-id') ?? 'client-http',
  })
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...(init.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
      ...init.headers,
    },
  })
  if (!response.ok) {
    const error = await parseError(response)
    if (response.status === 401 && path !== '/session' && typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('webadb:auth-required'))
    }
    throw error
  }
  if (response.status === 204) return undefined as T
  return ((await response.json()) as Envelope<T>).data
}

export function toAppError(value: unknown): AppError {
  if (value instanceof ApiError) return value
  const candidate = value as Partial<AppError>
  if (candidate?.errorCode && candidate.message) {
    return {
      errorCode: candidate.errorCode,
      message: candidate.message,
      module: candidate.module ?? 'client',
      recoverable: candidate.recoverable ?? true,
      traceId: candidate.traceId ?? 'client-unknown',
      suggestion: candidate.suggestion,
      cause: candidate.cause,
    }
  }
  return {
    errorCode: 'NETWORK_ERROR',
    message: value instanceof Error ? value.message : '无法连接到服务',
    module: 'api.client',
    recoverable: true,
    traceId: 'client-network',
  }
}
