<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { KeyRound, ShieldCheck, Zap } from 'lucide-vue-next'
import { loginWithPassword } from '@/services/auth'
import { toAppError } from '@/services/api'

const route = useRoute()
const router = useRouter()
const password = ref('')
const error = ref('')
const loading = ref(false)
const serviceUnavailable = computed(() => route.query.service === 'unavailable')

function redirectTarget(): string {
  const value = route.query.redirect
  return typeof value === 'string' && value.startsWith('/') && !value.startsWith('//') ? value : '/'
}

async function login() {
  if (!password.value || loading.value) return
  loading.value = true
  error.value = ''
  try {
    await loginWithPassword(password.value)
    password.value = ''
    await router.replace(redirectTarget())
  } catch (value) {
    error.value = toAppError(value).message
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="grid min-h-screen place-items-center bg-[#f1f5f2] p-5 dark:bg-[#090e0b]">
    <section class="surface w-full max-w-md rounded-2xl p-7 shadow-xl">
      <div class="grid size-11 place-items-center rounded-xl bg-brand-600 text-white"><Zap :size="21" /></div>
      <div class="eyebrow mt-6">Secure Access</div>
      <h1 class="mt-2 text-2xl font-bold tracking-tight">登录 ADBControl</h1>
	  <p class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">仅可使用 Rust Core 的内置管理员密码登录。初始密码为 <span class="font-mono font-semibold text-slate-700 dark:text-slate-200">admin</span>，首次登录后必须修改。</p>
      <div v-if="serviceUnavailable" class="mt-4 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">暂时无法读取服务状态，请确认服务正在运行后重试。</div>
      <label class="mt-6 block text-xs font-medium" for="login-password">管理员密码</label>
      <div class="relative mt-2"><KeyRound :size="16" class="absolute top-3 left-3 text-slate-400" /><input id="login-password" v-model="password" type="password" class="field pl-9" autocomplete="current-password" placeholder="输入管理密码" autofocus @keyup.enter="login" /></div>
      <p v-if="error" class="mt-3 text-xs text-red-600" role="alert">{{ error }}</p>
      <button class="btn-primary mt-5 w-full" :disabled="!password || loading" @click="login">{{ loading ? '正在验证…' : '登录管理后台' }}</button>
	  <div class="mt-6 flex items-center gap-2 border-t border-slate-100 pt-5 text-xs text-slate-400 dark:border-white/7"><ShieldCheck :size="15" class="text-brand-600" />远程用户不能登录此后台；浏览器仅保存 HttpOnly 会话</div>
    </section>
  </main>
</template>
