const API_BASE = '/api'

let sessionId: string | null = localStorage.getItem('sessionId')
let onUnauthorized: (() => void) | null = null

/** Register a callback that fires when a 401 response is received */
export function setOnUnauthorized(cb: () => void) {
  onUnauthorized = cb
}

export interface APIResponse<T = unknown> {
  success: boolean
  data?: T
  error?: string
}

export interface AdminInfo {
  id: number
  username: string
  nickname?: string
  role?: string
}

export interface AppleAccount {
  id: number
  appleId: string
  remark: string
  status: number // 1=normal, 2=locked
  hmeCount: number
  lastLogin?: string
  lastError?: string
  createdAt: string
  sessionSavedAt?: string
  isFamilyOrganizer?: boolean
  familyMemberCount?: number
  familyRole?: string // organizer, parent, adult, child
  // Profile info
  fullName?: string
  birthday?: string
  country?: string
  alternateEmails?: string | string[] // JSON string or array of alternate emails
  trustedDeviceCount?: number
  twoFactorEnabled?: boolean
}

export interface AccountListResult {
  list: AppleAccount[]
  total: number
  page: number
  pageSize: number
}

export interface PhoneNumber {
  id: number
  numberWithDialCode: string
}

export interface AppleLoginResult {
  requires2fa: boolean
  message?: string
  phoneNumbers?: PhoneNumber[]
}

export interface HMEEmail {
  id: string
  emailAddress: string
  label: string
  note: string
  forwardToEmail: string
  active: boolean
  createTime: number
}

export interface BatchCreateResult {
  created: HMEEmail[]
  errors: string[]
  total: number
  success: number
  failed: number
}

export interface StatsResult {
  totalAccounts: number
  activeAccounts: number
  errorAccounts: number
  totalHME: number
}

export interface HMEWithAccount {
  id: number
  accountId: number
  hmeId: string
  emailAddress: string
  label: string
  note: string
  forwardToEmail: string
  active: boolean
  createdAt: string
  appleId: string
}

export interface HMEListResult {
  list: HMEWithAccount[]
  total: number
  page: number
  pageSize: number
}

export interface FamilyMember {
  dsid: string
  firstName: string
  lastName: string
  fullName: string
  ageClassification: string // ADULT, CHILD
  appleId: string
  ageInYears: number
  isParent: boolean
}

export interface FamilyInfo {
  familyId: string
  organizerDsid: string
}

export interface FamilyResponse {
  currentDsid: string
  currentUserAppleId: string
  family?: FamilyInfo
  familyMembers: FamilyMember[]
  isLinkedToFamily: boolean
  isMemberOfFamily: boolean
}

export interface ForwardEmailOption {
  id: number
  type: string // official, profile
  address: string
  vetted: boolean
}

export interface ForwardEmailResponse {
  forwardToOptions: {
    availableEmails: ForwardEmailOption[]
    forwardToEmail?: {
      address: string
    }
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown
): Promise<APIResponse<T>> {
  const headers: Record<string, string> = {}

  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  if (sessionId) {
    headers['X-Session-ID'] = sessionId
  }

  let response: Response
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    return { success: false, error: '网络连接失败' }
  }

  // Handle 401 — session expired
  if (response.status === 401) {
    clearSession()
    onUnauthorized?.()
    return { success: false, error: '会话已过期，请重新登录' }
  }

  // Update session ID from response
  const newSessionId = response.headers.get('X-Session-ID')
  if (newSessionId) {
    sessionId = newSessionId
    localStorage.setItem('sessionId', newSessionId)
  }

  try {
    return await response.json()
  } catch {
    return { success: false, error: `服务器错误 (${response.status})` }
  }
}

export const api = {
  // Admin Auth
  adminLogin: (username: string, password: string, rememberMe = false) =>
    request<AdminInfo>('POST', '/admin/login', { username, password, rememberMe }),

  adminLogout: () => request('POST', '/admin/logout'),

  adminInfo: () => request<AdminInfo>('GET', '/admin/info'),

  // Apple Account Management
  listAccounts: (page = 1, pageSize = 20) =>
    request<AccountListResult>('GET', `/accounts?page=${page}&pageSize=${pageSize}`),

  createAccount: (appleId: string, password: string, remark?: string) =>
    request<AppleAccount>('POST', '/accounts', { appleId, password, remark }),

  updateAccount: (id: number, appleId: string, password?: string, remark?: string) =>
    request('PUT', `/accounts/${id}`, { appleId, password, remark }),

  deleteAccount: (id: number) => request('DELETE', `/accounts/${id}`),

  loginAppleAccount: (id: number) =>
    request<AppleLoginResult>('POST', `/accounts/${id}/login`),

  verify2FAForAccount: (id: number, code: string, method: 'device' | 'sms' = 'device', phoneId?: number) =>
    request('POST', `/accounts/${id}/2fa`, { code, method, phoneId }),

  requestSMSForAccount: (id: number, phoneId = 1) =>
    request('POST', `/accounts/${id}/request-sms`, { phoneId }),

  getAccountHME: (id: number) =>
    request<HMEEmail[]>('GET', `/accounts/${id}/hme`),

  createAccountHME: (id: number, label?: string, note?: string, forwardToEmail?: string) =>
    request<HMEEmail>('POST', `/accounts/${id}/hme`, { label, note, forwardToEmail }),

  batchCreateAccountHME: (id: number, count: number, labelPrefix?: string, delayMs?: number, forwardToEmail?: string) =>
    request<BatchCreateResult>('POST', `/accounts/${id}/hme/batch`, { count, labelPrefix, delayMs, forwardToEmail }),

  deleteAccountHME: (accountId: number, hmeId: string) =>
    request('DELETE', `/accounts/${accountId}/hme/${hmeId}`),

  getAccountForwardEmails: (id: number) =>
    request<string[]>('GET', `/accounts/${id}/forward-emails`),

  // Admin extended
  getStats: () => request<StatsResult>('GET', '/admin/stats'),

  listAllHME: (page = 1, pageSize = 20, search = '') =>
    request<HMEListResult>('GET', `/admin/hme?page=${page}&pageSize=${pageSize}&search=${encodeURIComponent(search)}`),

  changePassword: (oldPassword: string, newPassword: string) =>
    request('PUT', '/admin/password', { oldPassword, newPassword }),

  // Account info refresh
  refreshAccountInfo: (id: number) =>
    request<AppleAccount>('POST', `/accounts/${id}/refresh`),

  // Alternate email management
  sendAlternateEmailVerification: (accountId: number, email: string) =>
    request<{ verificationId: string; address: string; length: number }>(
      'POST',
      `/accounts/${accountId}/alternate-email/send-verification`,
      { email }
    ),

  verifyAlternateEmail: (accountId: number, email: string, verificationId: string, code: string) =>
    request<{ address: string; vetted: boolean }>(
      'POST',
      `/accounts/${accountId}/alternate-email/verify`,
      { email, verificationId, code }
    ),

  removeAlternateEmail: (accountId: number, email: string) =>
    request('DELETE', `/accounts/${accountId}/alternate-email`, { email }),

  // Family members
  getFamilyMembers: (accountId: number) =>
    request<FamilyResponse>('GET', `/accounts/${accountId}/family`),

  // Forward email settings
  getForwardEmailOptions: (accountId: number) =>
    request<ForwardEmailResponse>('GET', `/accounts/${accountId}/forward-email`),

  setForwardEmail: (accountId: number, email: string) =>
    request('PUT', `/accounts/${accountId}/forward-email`, { email }),

  // Health
  health: () => request('GET', '/health'),
}

export function clearSession() {
  sessionId = null
  localStorage.removeItem('sessionId')
}
