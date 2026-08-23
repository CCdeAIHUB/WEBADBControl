export interface AppError {
  errorCode: string
  message: string
  module: string
  recoverable: boolean
  cause?: string
  suggestion?: string
  traceId: string
}

export interface Device {
  id: string
  name: string
  model?: string
  product?: string
  state: 'device' | 'online' | 'offline' | 'unauthorized' | string
  transport: 'usb' | 'wireless' | 'companion' | string
}

export interface DeviceOverview {
  deviceId: string
  properties: Record<string, string>
  battery: Record<string, string | number>
  collectedAt: string
}

export interface DeviceFileEntry {
  name: string
  path: string
  type: 'file' | 'directory' | 'link'
  permissions: string
  owner?: string
  group?: string
  size: number
  modified?: string
  target?: string
}

export interface AutomationPermissions {
  allowAdb: boolean
  allowShell: boolean
  allowCompanion: boolean
  allowAi: boolean
}

export interface AutomationTask {
  schemaVersion: number
  id: string
  name: string
  description?: string
  deviceId?: string
  enabled: boolean
  concurrencyPolicy: string
  permissions: AutomationPermissions
  triggers: Array<Record<string, unknown> & { type: string }>
  actions: Array<Record<string, unknown> & { type: string }>
  createdAt?: string
  updatedAt?: string
}

export interface AutomationRun {
  id: string
  taskId: string
  status: 'queued' | 'running' | 'paused' | 'succeeded' | 'failed' | 'stopped' | 'skipped'
  progress: number
  completedSteps: number
  totalSteps: number
  errorCode?: string
  startedAt?: string
  finishedAt?: string
}

export interface AIModel {
  id: string
  name: string
  baseUrl: string
  model: string
  apiKey?: string
  vision: boolean
}

export interface AppSettings {
  theme: 'system' | 'light' | 'dark'
  language: string
  refreshSeconds: number
  screenFps: number
  aiModels: AIModel[]
  defaultModelId?: string
}
