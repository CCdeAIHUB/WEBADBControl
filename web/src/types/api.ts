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
  hardwareId?: string
  aliases?: string[]
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

export interface PackageInfo {
  package: string
  displayName: string
  hasResolvedDisplayName?: boolean
  apkPath?: string
  versionCode?: number
  system?: boolean
  enabled?: boolean
  iconPngBase64?: string
  metadataSource?: 'package-name' | 'adb-label' | 'companion' | string
  iconText: string
  iconColor: string
}

export interface CompanionStatus {
  installed: boolean
  adbResponsive: boolean
  state: 'missing' | 'outdated' | 'adb-responsive' | 'adb-unreachable' | string
  message: string
  errorCode?: string
  installedVersionCode?: number
  installedVersionName?: string
  requiredVersionCode?: number
  requiredVersionName?: string
  updateRequired: boolean
}

export interface CompanionUpgradeResult {
  state: 'ready' | 'installed' | 'updated' | string
  updated: boolean
  previousVersionCode?: number
  previousVersionName?: string
  installedVersionCode: number
  installedVersionName: string
  requiredVersionCode: number
  requiredVersionName: string
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
	remoteEnabled: boolean
	remoteAddress: string
	remotePort: number
}

export interface RemoteAccount {
	username: string
	role: 'user'
	devices: string[]
	builtIn: false
	passwordChangeRequired: boolean
}

export interface LogEvent {
  id: string
  type: 'request' | 'audit' | 'client_error' | 'system' | string
  level: 'info' | 'warn' | 'error' | string
  module: string
  action: string
  message: string
  traceId: string
  errorCode?: string
  deviceId?: string
  method?: string
  path?: string
  status?: number
  durationMs?: number
  actor?: string
  ipAddress?: string
  userAgent?: string
  details?: Record<string, unknown>
  createdAt: string
}

export interface LogStats {
  total: number
  errorCount: number
  warnCount: number
  byType: Record<string, number>
  byLevel: Record<string, number>
  lastErrorAt?: string
}
