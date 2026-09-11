<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { KeyRound } from 'lucide-vue-next'
import { toAppError } from '@/services/api'
import { updatePassword } from '@/services/auth'

const router = useRouter()
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  const bytes = new TextEncoder().encode(newPassword.value).length
  if (bytes < 8 || bytes > 1024) {
    error.value = '新密码必须为 8–1024 个字节'
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = '两次输入的新密码不一致'
    return
  }
  busy.value = true
  try {
    await updatePassword(currentPassword.value, newPassword.value)
    await router.replace('/')
  } catch (value) {
    error.value = toAppError(value).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="grid min-h-[70vh] place-items-center p-4">
    <section class="card w-full max-w-lg p-6">
      <div class="grid size-10 place-items-center rounded-lg bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">
        <KeyRound :size="19" />
      </div>
      <h1 class="mt-5 text-xl font-bold text-slate-900 dark:text-slate-100">首次登录必须修改管理员密码</h1>
      <p class="mt-2 text-sm leading-6 text-slate-500 dark:text-slate-400">
        当前使用 Core 初始管理员密码。请设置 8–1024 字节的新密码后继续进入管理后台。
      </p>
      <div class="mt-5 grid gap-4">
        <label class="text-xs font-medium">当前密码<input v-model="currentPassword" type="password" autocomplete="current-password" class="field mt-2" /></label>
        <label class="text-xs font-medium">新密码<input v-model="newPassword" type="password" autocomplete="new-password" class="field mt-2" /></label>
        <label class="text-xs font-medium">确认新密码<input v-model="confirmPassword" type="password" autocomplete="new-password" class="field mt-2" @keyup.enter="submit" /></label>
      </div>
      <p v-if="error" class="mt-3 text-xs text-red-600" role="alert">{{ error }}</p>
      <button class="btn-primary mt-5 w-full" :disabled="busy || !currentPassword || !newPassword || !confirmPassword" @click="submit">
        {{ busy ? '正在修改…' : '修改密码并继续' }}
      </button>
    </section>
  </main>
</template>
