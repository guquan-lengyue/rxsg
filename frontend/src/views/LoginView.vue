<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { loginAnnouncement } from '@/api/auth'
import { errorMessage } from '@/api/http'
import { img } from '@/assets/img'
import { zhCN } from '@/lang/zh-CN'
import { useAuthStore } from '@/stores/auth'

const REMEMBER_KEY = 'rxsg_remember'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const remembered = localStorage.getItem(REMEMBER_KEY) ?? ''
const form = reactive({
  passport: remembered,
  password: '',
  remember: remembered !== '',
})

const loading = ref(false)
const error = ref('')
const announcement = ref('')

onMounted(async () => {
  try {
    const { content } = await loginAnnouncement()
    announcement.value = content
  } catch {
    // 公告获取失败不阻塞登录
  }
})

async function onSubmit(): Promise<void> {
  error.value = ''
  const passport = form.passport.trim()
  if (!passport || !form.password) {
    error.value = '请输入账号和密码'
    return
  }

  loading.value = true
  try {
    await auth.login({ passport, password: form.password })
    if (form.remember) {
      localStorage.setItem(REMEMBER_KEY, passport)
    } else {
      localStorage.removeItem(REMEMBER_KEY)
    }
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/city'
    await router.replace(redirect)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-page">
    <form class="login-card" @submit.prevent="onSubmit">
      <img class="logo" :src="img('title.png')" :alt="zhCN.appTitle" />
      <p class="subtitle">{{ zhCN.login.title }}</p>

      <label class="field">
        <span>{{ zhCN.login.passport }}</span>
        <input v-model="form.passport" type="text" autocomplete="username" />
      </label>

      <label class="field">
        <span>{{ zhCN.login.password }}</span>
        <input v-model="form.password" type="password" autocomplete="current-password" />
      </label>

      <label class="remember">
        <input v-model="form.remember" type="checkbox" />
        <span>{{ zhCN.login.remember }}</span>
      </label>

      <p v-if="error" class="error">{{ error }}</p>

      <button type="submit" :disabled="loading">
        {{ loading ? zhCN.login.submitting : zhCN.login.submit }}
      </button>

      <div v-if="announcement" class="announcement">
        <strong>{{ zhCN.login.announcement }}</strong>
        <p>{{ announcement }}</p>
      </div>
    </form>
  </div>
</template>

<style scoped>
.login-page {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: url('/images/bj.jpg') center / cover no-repeat;
}

.login-card {
  display: flex;
  flex-direction: column;
  width: 360px;
  padding: 28px 30px;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.5);
}

.logo {
  align-self: center;
  width: auto;
  height: 46px;
  margin-bottom: 6px;
  object-fit: contain;
}

.subtitle {
  margin: 4px 0 20px;
  color: var(--text-dim);
  text-align: center;
}

.field {
  display: block;
  margin-bottom: 14px;
}

.field span {
  display: block;
  margin-bottom: 6px;
  color: var(--text-dim);
  font-size: 13px;
}

.field input {
  width: 100%;
  padding: 9px 10px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.field input:focus {
  border-color: var(--accent);
  outline: none;
}

.remember {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 14px;
  color: var(--text-dim);
  font-size: 13px;
}

.error {
  margin: 0 0 12px;
  color: var(--danger);
  font-size: 13px;
}

button[type='submit'] {
  width: 100%;
  padding: 10px;
  color: #1b1b1b;
  font-weight: 600;
  background: var(--accent);
  border: none;
  border-radius: 4px;
}

.announcement {
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--panel-border);
  color: var(--text-dim);
  font-size: 13px;
}

.announcement p {
  margin: 6px 0 0;
  white-space: pre-wrap;
}
</style>