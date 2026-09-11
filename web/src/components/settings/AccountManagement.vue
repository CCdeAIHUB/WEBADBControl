<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { KeyRound, Plus, Trash2, UserRound } from 'lucide-vue-next'
import ConfirmDialog from '@/components/feedback/ConfirmDialog.vue'
import { api, toAppError } from '@/services/api'
import {
  assignRemoteDevice,
  createRemoteUser,
  deleteRemoteUser,
  listRemoteUsers,
  resetRemoteUserPassword,
} from '@/services/remoteUsers'
import { useUiStore } from '@/stores/ui'
import type { Device, RemoteAccount } from '@/types/api'

const ui = useUiStore()
const accounts = ref<RemoteAccount[]>([])
const devices = ref<Device[]>([])
const username = ref('')
const password = ref('')
const resetPasswords = ref<Record<string, string>>({})
const deleting = ref<RemoteAccount>()
const busyAction = ref('')

async function load() {
  try {
	const [remoteUsers, connectedDevices] = await Promise.all([
      listRemoteUsers(),
      api<Device[]>('/devices'),
    ])
	accounts.value = remoteUsers
	devices.value = connectedDevices
  } catch (error) {
    ui.failure(toAppError(error))
  }
}

async function create() {
  if (busyAction.value || !username.value || !password.value) return
  busyAction.value = 'create'
  try {
    await createRemoteUser(username.value, password.value)
    username.value = ''
    password.value = ''
    await load()
    ui.notify('远程用户已创建', '现在可以为该用户分配设备。', 'success')
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    busyAction.value = ''
  }
}

async function reset(account: RemoteAccount) {
  const value = resetPasswords.value[account.username] ?? ''
  if (!value || busyAction.value) return
  busyAction.value = `reset:${account.username}`
  try {
    await resetRemoteUserPassword(account.username, value)
    resetPasswords.value[account.username] = ''
    await load()
    ui.notify('临时密码已设置', '旧远程会话已撤销，用户下次登录必须修改密码。', 'success')
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    busyAction.value = ''
  }
}

async function assign(account: RemoteAccount, deviceId: string) {
  if (busyAction.value) return
  busyAction.value = `device:${account.username}:${deviceId}`
  try {
    await assignRemoteDevice(account.username, deviceId, !account.devices.includes(deviceId))
    await load()
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    busyAction.value = ''
  }
}

async function remove() {
  if (!deleting.value || busyAction.value) return
  const account = deleting.value
  busyAction.value = `delete:${account.username}`
  try {
    await deleteRemoteUser(account.username)
    deleting.value = undefined
    await load()
    ui.notify('远程用户已删除', '该用户的所有远程会话已撤销。', 'success')
  } catch (error) {
    ui.failure(toAppError(error))
  } finally {
    busyAction.value = ''
  }
}

onMounted(load)
</script>

<template>
  <section class="card overflow-hidden">
    <div class="flex items-start gap-3 border-b border-slate-100 p-5 dark:border-white/7">
      <div class="grid size-9 place-items-center rounded-lg bg-brand-50 text-brand-700 dark:bg-brand-500/10 dark:text-brand-300">
        <UserRound :size="18" />
      </div>
      <div>
        <h2 class="section-title m-0">远程用户与设备权限</h2>
        <p class="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">
          这些账户仅供 QUIC 远程控制客户端使用，不能登录 Web 管理后台。
        </p>
      </div>
    </div>

    <div class="grid gap-3 border-b border-slate-100 p-5 dark:border-white/7 sm:grid-cols-[1fr_1fr_auto]">
      <label class="text-xs font-medium">
        用户名
        <input v-model.trim="username" class="field mt-2" autocomplete="off" placeholder="3–64 位英文、数字或 ._-" />
      </label>
      <label class="text-xs font-medium">
        初始密码
        <input v-model="password" type="password" class="field mt-2" autocomplete="new-password" placeholder="8–1024 字节" @keyup.enter="create" />
      </label>
      <button class="btn-primary self-end" :disabled="!!busyAction || !username || !password" @click="create">
        <Plus :size="15" />{{ busyAction === 'create' ? '正在创建…' : '添加远程用户' }}
      </button>
    </div>

    <div v-if="accounts.length" class="divide-y divide-slate-100 dark:divide-white/7">
      <article v-for="account in accounts" :key="account.username" class="p-5">
        <div class="flex flex-wrap items-center gap-2">
          <strong class="text-sm text-slate-900 dark:text-slate-100">{{ account.username }}</strong>
          <span class="rounded-full bg-sky-50 px-2 py-0.5 text-[10px] font-medium text-sky-700 dark:bg-sky-500/10 dark:text-sky-300">远程用户</span>
          <span v-if="account.passwordChangeRequired" class="rounded-full bg-amber-50 px-2 py-0.5 text-[10px] font-medium text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">等待修改临时密码</span>
          <button class="icon-button ml-auto text-red-600" title="删除远程用户" :disabled="!!busyAction" @click="deleting = account">
            <Trash2 :size="15" />
          </button>
        </div>

        <div class="mt-4">
          <div class="mb-2 text-[11px] font-medium text-slate-500 dark:text-slate-400">允许访问的在线设备</div>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="device in devices"
              :key="device.id"
              class="btn-secondary !h-8 !text-xs"
              :class="account.devices.includes(device.id) ? '!border-brand-500 !bg-brand-50 !text-brand-700 dark:!bg-brand-500/10 dark:!text-brand-300' : ''"
              :disabled="!!busyAction"
              @click="assign(account, device.id)"
            >
              {{ account.devices.includes(device.id) ? '✓ ' : '' }}{{ device.name || device.id }}
            </button>
            <span v-if="!devices.length" class="text-xs text-slate-400">暂无在线设备</span>
          </div>
        </div>

        <div class="mt-4 flex max-w-xl gap-2">
          <input v-model="resetPasswords[account.username]" type="password" class="field" autocomplete="new-password" placeholder="设置临时密码（8–1024 字节）" />
          <button class="btn-secondary shrink-0" :disabled="!!busyAction || !resetPasswords[account.username]" @click="reset(account)">
            <KeyRound :size="14" />重置密码
          </button>
        </div>
      </article>
    </div>
    <div v-else class="px-5 py-10 text-center text-xs text-slate-500 dark:text-slate-400">
      暂无远程用户。管理员账户不会显示在此列表中。
    </div>
  </section>

  <ConfirmDialog
    :open="!!deleting"
    title="删除远程用户"
    :description="`将删除 ${deleting?.username} 并立即撤销其所有远程会话。`"
    confirm-text="删除用户"
    destructive
    @confirm="remove"
    @cancel="deleting = undefined"
  />
</template>
