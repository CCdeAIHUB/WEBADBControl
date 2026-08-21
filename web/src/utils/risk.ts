export interface OperationRisk {
  confirmationRequired: boolean
  level: 'routine' | 'sensitive' | 'destructive'
  title?: string
  description?: string
}

const risks: Record<string, OperationRisk> = {
  'package.uninstall': { confirmationRequired: true, level: 'destructive', title: '卸载应用', description: '应用及其本地数据将从设备移除。' },
  'package.clear': { confirmationRequired: true, level: 'destructive', title: '清除应用数据', description: '登录状态、设置与缓存将被永久清除。' },
  'sms.send': { confirmationRequired: true, level: 'sensitive', title: '发送短信', description: '此操作可能产生运营商费用。' },
  'phone.call': { confirmationRequired: true, level: 'sensitive', title: '发起电话', description: '设备将立即尝试拨打指定号码。' },
  'device.power': { confirmationRequired: true, level: 'sensitive', title: '电源操作', description: '可能导致当前远程会话中断。' },
}

export function riskForOperation(operation: string): OperationRisk {
  return risks[operation] ?? { confirmationRequired: false, level: 'routine' }
}
