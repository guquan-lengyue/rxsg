import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import * as authApi from '@/api/auth'
import type { LoginParams } from '@/api/auth'
import { TOKEN_KEY } from '@/api/http'
import type { User } from '@/types'

const USER_KEY = 'rxsg_user'

function restoreUser(): User | null {
  const raw = localStorage.getItem(USER_KEY)
  if (!raw) {
    return null
  }
  try {
    return JSON.parse(raw) as User
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem(TOKEN_KEY) ?? '')
  const user = ref<User | null>(restoreUser())

  const isLoggedIn = computed(() => token.value !== '')

  async function login(params: LoginParams): Promise<User> {
    const resp = await authApi.login(params)
    token.value = resp.token
    user.value = resp.user
    localStorage.setItem(TOKEN_KEY, resp.token)
    localStorage.setItem(USER_KEY, JSON.stringify(resp.user))
    return resp.user
  }

  async function logout(): Promise<void> {
    try {
      await authApi.logout()
    } catch {
      // 忽略登出请求失败，本地照常清理
    }
    clear()
  }

  async function refresh(): Promise<User | null> {
    if (!token.value) {
      return null
    }
    user.value = await authApi.me()
    localStorage.setItem(USER_KEY, JSON.stringify(user.value))
    return user.value
  }

  function clear(): void {
    token.value = ''
    user.value = null
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(USER_KEY)
  }

  return { token, user, isLoggedIn, login, logout, refresh, clear }
})