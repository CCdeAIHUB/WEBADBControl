import { defineStore } from 'pinia'
import { api, toAppError } from '@/services/api'
import type { AppError, Device } from '@/types/api'

type DeviceRequest = () => Promise<Device[]>

export const useDevicesStore = defineStore('devices', {
  state: () => ({
    devices: [] as Device[],
    status: 'idle' as 'idle' | 'loading' | 'ready' | 'error',
    error: null as AppError | null,
    lastUpdated: null as Date | null,
  }),
  getters: {
    online: (state) => state.devices.filter((device) => ['device', 'online'].includes(device.state)),
  },
  actions: {
    async refresh(request: DeviceRequest = () => api<Device[]>('/devices')) {
      this.status = 'loading'
      this.error = null
      try {
        this.devices = await request()
        this.status = 'ready'
        this.lastUpdated = new Date()
      } catch (error) {
        this.devices = []
        this.error = toAppError(error)
        this.status = 'error'
      }
    },
    async connect(endpoint: string) {
      await api('/devices/connect', { method: 'POST', body: JSON.stringify({ endpoint }) })
      await this.refresh()
    },
    async remove(device: Device) {
      await api(`/devices/${encodeURIComponent(device.id)}/remove`, { method: 'POST', body: '{}' })
      await this.refresh()
    },
  },
})
