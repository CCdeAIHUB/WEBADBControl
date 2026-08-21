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
    // The network boundary still returns a stable local error when a reverse proxy emits HTML.
  }
  return new ApiError({
    errorCode: response.status === 401 ? 'AUTH_REQUIRED' : 'HTTP_ERROR',
    message: response.status === 401 ? '需要登录后继续' : `服务请求失败（${response.status}）`,
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
  if (!response.ok) throw await parseError(response)
  if (response.status === 204) return undefined as T
  return ((await response.json()) as Envelope<T>).data
}

export async function authenticate(token: string): Promise<void> {
  await api('/session', { method: 'POST', body: JSON.stringify({ token }) })
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
