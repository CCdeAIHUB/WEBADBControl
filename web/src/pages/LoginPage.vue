<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { KeyRound, ShieldCheck, Zap } from 'lucide-vue-next'
import { authenticate, toAppError } from '@/services/api'

const router = useRouter()
const token = ref('')
const error = ref('')
const loading = ref(false)

async function login() {
  loading.value = true; error.value = ''
  try { await authenticate(token.value); await router.replace('/') }
  catch (value) { error.value = toAppError(value).message }
  finally { loading.value = false }
}
</script>

<template>
  <main class="grid min-h-screen place-items-center bg-[#f1f5f2] p-5 dark:bg-[#090e0b]"><section class="surface w-full max-w-md rounded-2xl p-7 shadow-xl"><div class="grid size-11 place-items-center rounded-xl bg-brand-600 text-white"><Zap :size="21" /></div><div class="eyebrow mt-6">Secure Access</div><h1 class="mt-2 text-2xl font-bold tracking-tight">登录 ADBControl</h1><p class="mt-3 text-sm leading-6 text-slate-500 dark:text-slate-400">输入服务器管理员提供的访问令牌。令牌只用于建立 HttpOnly 会话，不会存入浏览器脚本存储。</p><label class="mt-6 block text-xs font-medium">访问令牌</label><div class="relative mt-2"><KeyRound :size="16" class="absolute top-3 left-3 text-slate-400" /><input v-model="token" type="password" class="field pl-9" autocomplete="current-password" placeholder="输入访问令牌" @keyup.enter="login" /></div><p v-if="error" class="mt-3 text-xs text-red-600">{{ error }}</p><button class="btn-primary mt-5 w-full" :disabled="!token || loading" @click="login">{{ loading ? '正在验证…' : '安全登录' }}</button><div class="mt-6 flex items-center gap-2 border-t border-slate-100 pt-5 text-xs text-slate-400 dark:border-white/7"><ShieldCheck :size="15" class="text-brand-600" />端到端使用同源安全策略</div></section></main>
</template>
