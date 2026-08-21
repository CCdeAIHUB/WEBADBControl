import { defineStore } from 'pinia'
import type { AppError } from '@/types/api'

export interface Toast {
  id: number
  title: string
  message: string
  tone: 'success' | 'warning' | 'danger' | 'info'
}

export const useUiStore = defineStore('ui', {
  state: () => ({ sidebarOpen: false, toasts: [] as Toast[], sequence: 0 }),
  actions: {
    notify(title: string, message: string, tone: Toast['tone'] = 'info') {
      const id = ++this.sequence
      this.toasts.push({ id, title, message, tone })
      window.setTimeout(() => this.dismiss(id), 4200)
    },
    failure(error: AppError) {
      this.notify(error.message, error.suggestion ?? `错误码：${error.errorCode}`, 'danger')
    },
    dismiss(id: number) {
      this.toasts = this.toasts.filter((toast) => toast.id !== id)
    },
  },
})
