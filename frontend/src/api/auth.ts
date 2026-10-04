import http from './http'

import type { LoginAnnouncement, LoginResponse, User } from '@/types'

export interface LoginParams {
  passport: string
  password: string
  passtype?: string
}

export async function login(params: LoginParams): Promise<LoginResponse> {
  const { data } = await http.post<LoginResponse>('/auth/login', params)
  return data
}

export async function logout(): Promise<void> {
  await http.post('/auth/logout')
}

export async function me(): Promise<User> {
  const { data } = await http.get<User>('/auth/me')
  return data
}

export async function loginAnnouncement(): Promise<LoginAnnouncement> {
  const { data } = await http.get<LoginAnnouncement>('/announcements/login')
  return data
}