import type { DeviceFileEntry } from '@/types/api'

const STORAGE_ROOTS = ['/sdcard', '/storage/emulated/0'] as const

export function normalizeRemotePath(path: string): string {
  const normalized = `/${path.split('/').filter(Boolean).join('/')}`
  return normalized === '/' ? '/sdcard' : normalized
}

export function joinRemotePath(basePath: string, name: string): string {
  return normalizeRemotePath(`${basePath.replace(/\/$/, '')}/${name}`)
}

export function parentRemotePath(path: string): string {
  const normalized = normalizeRemotePath(path)
  if (STORAGE_ROOTS.includes(normalized as (typeof STORAGE_ROOTS)[number])) return normalized
  return normalizeRemotePath(normalized.split('/').slice(0, -1).join('/'))
}

export function pathSegments(path: string): Array<{ label: string; path: string }> {
  const normalized = normalizeRemotePath(path)
  const parts = normalized.split('/').filter(Boolean)
  return parts.map((part, index) => ({ label: index === 0 ? `/${part}` : part, path: `/${parts.slice(0, index + 1).join('/')}` }))
}

export function formatFileSize(size: number): string {
  if (!Number.isFinite(size) || size <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = size
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}

export function sortFileEntries(entries: DeviceFileEntry[]): DeviceFileEntry[] {
  return [...entries].sort((left, right) => {
    const leftRank = left.type === 'directory' ? 0 : left.type === 'link' ? 1 : 2
    const rightRank = right.type === 'directory' ? 0 : right.type === 'link' ? 1 : 2
    if (leftRank !== rightRank) return leftRank - rightRank
    return left.name.localeCompare(right.name, 'zh-Hans-CN')
  })
}
