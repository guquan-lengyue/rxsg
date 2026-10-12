import axios from 'axios'

import { legacyText } from '@/lang/zh-CN'
import type { APIError } from '@/types'

export const TOKEN_KEY = 'rxsg_token'

const http = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
})

// 请求拦截：注入 Bearer token
http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截：401 统一清 token 并跳登录
http.interceptors.response.use(
  (resp) => resp,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem(TOKEN_KEY)
      localStorage.removeItem('rxsg_user')
      if (!window.location.pathname.startsWith('/login')) {
        window.location.href = '/login'
      }
    }
    return Promise.reject(error)
  },
)

// 从统一错误体 {"code","message"} 提取可展示文案。
// 后端遗留消息可能是未翻译的 i18n key（如 not_enough_goods120）→ 经 legacyText 解析。
export function errorMessage(error: unknown): string {
  const data = (error as { response?: { data?: APIError } })?.response?.data
  if (data?.message) {
    return legacyText(data.message)
  }
  if (error instanceof Error) {
    return error.message
  }
  return '网络异常，请稍后重试'
}

export default http