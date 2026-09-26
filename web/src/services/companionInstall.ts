import type { AppError } from '@/types/api'

const reinstallRequiredCodes = new Set([
  'COMPANION_SIGNATURE_MISMATCH',
  'COMPANION_REINSTALL_REQUIRED',
])

export function companionReinstallRequired(error: AppError) {
  return reinstallRequiredCodes.has(error.errorCode)
}

export function companionReinstallDescription(error?: AppError | null) {
  const reason = error?.errorCode === 'COMPANION_SIGNATURE_MISMATCH'
    ? '检测到设备中的伴侣应用与服务端内置 APK 签名不一致，Android 无法覆盖安装。'
    : 'Android 拒绝覆盖当前伴侣应用，需要先卸载旧版本才能继续。'
  return `${reason}卸载会清除伴侣应用数据，并需要重新授予无障碍、悬浮窗等权限。是否卸载旧版并安装当前版本？`
}
