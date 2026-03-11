import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type AppleAccount, type PhoneNumber } from '@/api/client'
import { toast } from '@/stores/toastStore'
import {
  Plus, Trash2, LogIn, Mail, RefreshCw, Search,
  Edit, Loader2, Users, Shield, Smartphone, Eye, Phone, Upload,
  CheckCircle2, AlertCircle, Clock, X,
} from 'lucide-react'
import Button from '@/components/ui/Button'
import ConfirmDialog from '@/components/ui/ConfirmDialog'
import AccountFormModal from '@/components/account/AccountFormModal'
import TwoFAModal from '@/components/account/TwoFAModal'
import AccountHMEPanel from '@/components/account/AccountHMEPanel'
import BatchImportModal from '@/components/account/BatchImportModal'
import Pagination from '@/components/ui/Pagination'
import { cn } from '@/lib/cn'

export default function AccountsPage() {
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  // undefined = closed, null = add mode, AppleAccount = edit mode
  const [formAccount, setFormAccount] = useState<AppleAccount | null | undefined>(undefined)
  const [deleteTarget, setDeleteTarget] = useState<AppleAccount | null>(null)
  const [selectedAccount, setSelectedAccount] = useState<AppleAccount | null>(null)
  const [twoFAAccountId, setTwoFAAccountId] = useState<number | null>(null)
  const [twoFAPhones, setTwoFAPhones] = useState<PhoneNumber[]>([])
  const [batchImportOpen, setBatchImportOpen] = useState(false)

  // ---- Data ----
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['accounts', page, search],
    queryFn: () => api.listAccounts(page, 20, search),
  })

  const accounts = data?.data?.list || []
  const total = data?.data?.total || 0
  const pageSize = data?.data?.pageSize || 20
  const totalPages = Math.ceil(total / pageSize)

  // 全局统计数据（不受分页/搜索影响）
  const { data: statsData } = useQuery({
    queryKey: ['admin-stats'],
    queryFn: () => api.getStats(),
  })
  const stats = {
    totalAccounts: statsData?.data?.totalAccounts ?? 0,
    activeAccounts: statsData?.data?.activeAccounts ?? 0,
    errorAccounts: statsData?.data?.errorAccounts ?? 0,
    totalHME: statsData?.data?.totalHME ?? 0,
  }

  // ---- Mutations ----
  const loginMutation = useMutation({
    mutationFn: (id: number) => api.loginAppleAccount(id),
    onSuccess: async (res, id) => {
      console.log('[Login] Response:', JSON.stringify(res, null, 2))
      if (!res.success) {
        toast.error(res.error || '登录失败')
        return
      }
      if (res.data?.requires2fa) {
        console.log('[Login] phoneNumbers:', res.data?.phoneNumbers)
        setTwoFAAccountId(id)
        setTwoFAPhones(res.data?.phoneNumbers || [])
        queryClient.invalidateQueries({ queryKey: ['accounts'] })
      } else {
        toast.success('Apple 账户登录成功')
        // 先刷新数据，再获取最新账户信息进入详情页
        await queryClient.invalidateQueries({ queryKey: ['accounts'] })
        // 从最新缓存中获取账户数据
        const freshData = queryClient.getQueryData<{ data?: { list: AppleAccount[] } }>(['accounts', page, search])
        const freshAccount = freshData?.data?.list?.find((a) => a.id === id)
        if (freshAccount) {
          setSelectedAccount(freshAccount)
        }
      }
    },
    onError: () => toast.error('网络错误'),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteAccount(id),
    onSuccess: (res) => {
      if (res.success) {
        toast.success('账户已删除')
        queryClient.invalidateQueries({ queryKey: ['accounts'] })
      } else {
        toast.error(res.error || '删除失败')
      }
      setDeleteTarget(null)
    },
    onError: () => {
      toast.error('网络错误')
      setDeleteTarget(null)
    },
  })

  // ---- HME detail view ----
  if (selectedAccount) {
    return <AccountHMEPanel account={selectedAccount} onBack={() => setSelectedAccount(null)} />
  }

  // 解析账户的电话号码
  const parsePhones = (phoneNumbers: string | undefined) => {
    try {
      const phones = phoneNumbers ? JSON.parse(phoneNumbers) : []
      if (!Array.isArray(phones)) return []
      return phones.map((p: { fullNumberWithCountryPrefix?: string; numberWithDialCode?: string }) =>
        p.fullNumberWithCountryPrefix || p.numberWithDialCode || ''
      ).filter(Boolean)
    } catch {
      return []
    }
  }

  // 解析备用邮箱
  const parseAlternateEmails = (alternateEmails: string | string[] | undefined | null): string[] => {
    try {
      if (typeof alternateEmails === 'string') {
        const parsed = alternateEmails ? JSON.parse(alternateEmails) : []
        return Array.isArray(parsed) ? parsed : []
      }
      return Array.isArray(alternateEmails) ? alternateEmails : []
    } catch {
      return []
    }
  }

  // ---- Account list view ----
  return (
    <div className="animate-fade-in">
      {/* Header */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6">
        <div>
          <h2 className="text-xl sm:text-2xl font-bold text-gray-900 tracking-tight">Apple 账户管理</h2>
          <p className="text-sm text-gray-500 mt-1">管理你的 Apple ID 账户和隐藏邮箱</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button onClick={() => refetch()} variant="outline" size="sm" icon={<RefreshCw className="w-4 h-4" />}>
            <span className="hidden sm:inline">刷新</span>
          </Button>
          <Button onClick={() => setBatchImportOpen(true)} variant="outline" size="sm" icon={<Upload className="w-4 h-4" />}>
            <span className="hidden sm:inline">批量导入</span>
          </Button>
          <Button onClick={() => setFormAccount(null)} size="sm" icon={<Plus className="w-4 h-4" />}>
            <span className="hidden xs:inline">添加</span>账户
          </Button>
        </div>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-2 sm:gap-3 mb-6">
        <div className="bg-white rounded-xl border border-gray-100 p-3 sm:p-4 flex items-center gap-2 sm:gap-3">
          <div className="w-8 h-8 sm:w-10 sm:h-10 rounded-lg bg-blue-50 flex items-center justify-center flex-shrink-0">
            <Users className="w-4 h-4 sm:w-5 sm:h-5 text-blue-600" />
          </div>
          <div className="min-w-0">
            <p className="text-lg sm:text-2xl font-bold text-gray-900 tabular-nums">{stats.totalAccounts}</p>
            <p className="text-[10px] sm:text-xs text-gray-500 truncate">账户总数</p>
          </div>
        </div>
        <div className="bg-white rounded-xl border border-gray-100 p-3 sm:p-4 flex items-center gap-2 sm:gap-3">
          <div className="w-8 h-8 sm:w-10 sm:h-10 rounded-lg bg-green-50 flex items-center justify-center flex-shrink-0">
            <CheckCircle2 className="w-4 h-4 sm:w-5 sm:h-5 text-green-600" />
          </div>
          <div className="min-w-0">
            <p className="text-lg sm:text-2xl font-bold text-gray-900 tabular-nums">{stats.activeAccounts}</p>
            <p className="text-[10px] sm:text-xs text-gray-500 truncate">正常</p>
          </div>
        </div>
        <div className="bg-white rounded-xl border border-gray-100 p-3 sm:p-4 flex items-center gap-2 sm:gap-3">
          <div className="w-8 h-8 sm:w-10 sm:h-10 rounded-lg bg-red-50 flex items-center justify-center flex-shrink-0">
            <AlertCircle className="w-4 h-4 sm:w-5 sm:h-5 text-red-500" />
          </div>
          <div className="min-w-0">
            <p className="text-lg sm:text-2xl font-bold text-gray-900 tabular-nums">{stats.errorAccounts}</p>
            <p className="text-[10px] sm:text-xs text-gray-500 truncate">异常</p>
          </div>
        </div>
        <div className="bg-white rounded-xl border border-gray-100 p-3 sm:p-4 flex items-center gap-2 sm:gap-3">
          <div className="w-8 h-8 sm:w-10 sm:h-10 rounded-lg bg-purple-50 flex items-center justify-center flex-shrink-0">
            <Mail className="w-4 h-4 sm:w-5 sm:h-5 text-purple-600" />
          </div>
          <div className="min-w-0">
            <p className="text-lg sm:text-2xl font-bold text-gray-900 tabular-nums">{stats.totalHME}</p>
            <p className="text-[10px] sm:text-xs text-gray-500 truncate">HME 邮箱</p>
          </div>
        </div>
      </div>

      {/* Search */}
      <form
        onSubmit={(e) => {
          e.preventDefault()
          setSearch(searchInput)
          setPage(1)
        }}
        className="mb-5"
      >
        <div className="flex gap-2">
          <div className="relative flex-1 max-w-md">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
            <input
              type="text"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="搜索 Apple ID、姓名或备注..."
              className="w-full pl-9 pr-9 py-2 sm:py-2.5 text-sm border border-gray-200 rounded-xl bg-white focus:outline-none focus:ring-2 focus:ring-apple-blue/20 focus:border-apple-blue transition-all placeholder:text-gray-400"
            />
            {/* 清除输入按钮 */}
            {searchInput && (
              <button
                type="button"
                onClick={() => {
                  setSearchInput('')
                  setSearch('')
                  setPage(1)
                }}
                className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 hover:text-gray-600"
              >
                <X className="w-4 h-4" />
              </button>
            )}
          </div>
          <Button type="submit" size="sm" icon={<Search className="w-4 h-4" />}>
            <span className="hidden sm:inline">搜索</span>
          </Button>
        </div>
        {/* 显示当前搜索条件 */}
        {search && (
          <div className="flex flex-wrap items-center gap-2 mt-3">
            <span className="text-xs sm:text-sm text-gray-500">搜索：</span>
            <span className="inline-flex items-center gap-1.5 px-2 py-0.5 sm:px-2.5 sm:py-1 bg-blue-50 text-blue-700 text-xs sm:text-sm rounded-lg">
              "{search}"
              <button
                type="button"
                onClick={() => {
                  setSearchInput('')
                  setSearch('')
                  setPage(1)
                }}
                className="hover:text-blue-900"
              >
                <X className="w-3 h-3 sm:w-3.5 sm:h-3.5" />
              </button>
            </span>
            <span className="text-xs sm:text-sm text-gray-400">{total} 个结果</span>
          </div>
        )}
      </form>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="w-8 h-8 animate-spin text-apple-blue" />
        </div>
      )}

      {/* Empty */}
      {!isLoading && accounts.length === 0 && (
        <div className="text-center py-20 bg-white rounded-2xl border border-gray-100">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-4">
            <Mail className="w-8 h-8 text-gray-300" />
          </div>
          <p className="text-gray-600 font-medium">暂无账户</p>
          <p className="text-sm text-gray-400 mt-1 mb-4">点击上方按钮添加你的第一个 Apple ID</p>
          <Button onClick={() => setFormAccount(null)} size="sm" icon={<Plus className="w-4 h-4" />}>
            添加账户
          </Button>
        </div>
      )}

      {/* Account Cards */}
      {!isLoading && accounts.length > 0 && (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4">
            {accounts.map((account) => {
              const phones = parsePhones(account.phoneNumbers)
              const altEmails = parseAlternateEmails(account.alternateEmails)
              const isLoggedIn = !!account.sessionSavedAt
              const isNormal = account.status === 1

              return (
                <div
                  key={account.id}
                  className={cn(
                    'bg-white rounded-2xl border p-4 transition-all hover:shadow-lg hover:border-gray-200 group',
                    !isNormal ? 'border-red-100 bg-red-50/30' : 'border-gray-100'
                  )}
                >
                  {/* Card Header */}
                  <div className="flex items-start justify-between mb-3">
                    <button
                      onClick={() => setSelectedAccount(account)}
                      className="flex items-center gap-3 text-left flex-1 min-w-0 hover:opacity-80 transition-opacity"
                    >
                      <div className={cn(
                        'w-11 h-11 rounded-xl flex items-center justify-center flex-shrink-0',
                        isLoggedIn ? 'bg-gradient-to-br from-green-400 to-emerald-500' : 'bg-gradient-to-br from-gray-200 to-gray-300'
                      )}>
                        <Mail className="w-5 h-5 text-white" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="text-sm font-semibold text-gray-900 truncate">{account.appleId}</div>
                        {account.fullName && (
                          <div className="text-xs text-gray-500 truncate">{account.fullName}</div>
                        )}
                      </div>
                    </button>
                    {/* Status Badge */}
                    <div className={cn(
                      'px-2 py-1 rounded-lg text-[10px] font-semibold flex-shrink-0',
                      isNormal ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-600'
                    )}>
                      {isNormal ? '正常' : '异常'}
                    </div>
                  </div>

                  {/* Info Grid */}
                  <div className="grid grid-cols-3 gap-2 mb-3">
                    {/* Login Status */}
                    <div className="bg-gray-50 rounded-lg p-2 text-center">
                      <div className={cn(
                        'text-xs font-medium',
                        isLoggedIn ? 'text-green-600' : 'text-orange-500'
                      )}>
                        {isLoggedIn ? '✓ 已登录' : '⚠ 未登录'}
                      </div>
                      {isLoggedIn && account.sessionSavedAt && (
                        <div className="text-[10px] text-gray-400 mt-0.5" title={new Date(account.sessionSavedAt).toLocaleString()}>
                          <Clock className="w-2.5 h-2.5 inline mr-0.5" />
                          {new Date(account.sessionSavedAt).toLocaleDateString()}
                        </div>
                      )}
                    </div>
                    {/* HME Count */}
                    <div className="bg-gray-50 rounded-lg p-2 text-center">
                      <div className="text-sm font-bold text-gray-900 tabular-nums">{account.hmeCount}</div>
                      <div className="text-[10px] text-gray-400">HME</div>
                    </div>
                    {/* Security */}
                    <div className="bg-gray-50 rounded-lg p-2 text-center">
                      <div className="flex items-center justify-center gap-1">
                        <Shield className={cn('w-3.5 h-3.5', account.twoFactorEnabled ? 'text-green-500' : 'text-gray-300')} />
                        <span className={cn('text-xs font-medium', account.twoFactorEnabled ? 'text-green-600' : 'text-gray-400')}>
                          {account.twoFactorEnabled ? '2FA' : '无'}
                        </span>
                      </div>
                      {account.trustedDeviceCount !== undefined && account.trustedDeviceCount > 0 && (
                        <div className="text-[10px] text-gray-400 mt-0.5">
                          <Smartphone className="w-2.5 h-2.5 inline mr-0.5" />
                          {account.trustedDeviceCount} 设备
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Extra Info */}
                  <div className="space-y-1.5 mb-3 text-xs">
                    {/* Phone */}
                    {phones.length > 0 && (
                      <div className="flex items-center gap-1.5 text-gray-500">
                        <Phone className="w-3.5 h-3.5 text-green-500 flex-shrink-0" />
                        <span className="truncate" title={phones.join(', ')}>{phones[0]}</span>
                        {phones.length > 1 && <span className="text-gray-400">+{phones.length - 1}</span>}
                      </div>
                    )}
                    {/* Family */}
                    {account.familyMemberCount != null && account.familyMemberCount > 0 && (
                      <div className="flex items-center gap-1.5 text-gray-500">
                        <Users className={cn(
                          'w-3.5 h-3.5 flex-shrink-0',
                          account.isFamilyOrganizer ? 'text-purple-500' : 'text-gray-400'
                        )} />
                        <span>
                          家人共享 {account.familyMemberCount} 人
                          {account.isFamilyOrganizer && <span className="text-purple-600 ml-1">(组织者)</span>}
                        </span>
                      </div>
                    )}
                    {/* Alternate Emails */}
                    {altEmails.length > 0 && (
                      <div className="flex items-center gap-1.5 text-gray-500" title={altEmails.join(', ')}>
                        <Mail className="w-3.5 h-3.5 text-blue-400 flex-shrink-0" />
                        <span>{altEmails.length + 1} 个邮箱地址</span>
                      </div>
                    )}
                    {/* Remark */}
                    {account.remark && (
                      <div className="text-gray-400 truncate" title={account.remark}>
                        📝 {account.remark}
                      </div>
                    )}
                    {/* Error */}
                    {account.lastError && (
                      <div className="text-red-500 truncate" title={account.lastError}>
                        ⚠️ {account.lastError}
                      </div>
                    )}
                  </div>

                  {/* Actions */}
                  <div className="flex items-center justify-between pt-3 border-t border-gray-100 gap-2">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setSelectedAccount(account)}
                      icon={<Eye className="w-3.5 h-3.5" />}
                      className="flex-shrink-0"
                    >
                      <span className="hidden xs:inline">详情</span>
                    </Button>
                    <div className="flex items-center gap-0.5 sm:gap-1 flex-shrink-0">
                      <Button
                        size="sm"
                        variant={isLoggedIn ? 'ghost' : 'primary'}
                        title="登录 Apple 账户"
                        onClick={() => loginMutation.mutate(account.id)}
                        loading={loginMutation.isPending && loginMutation.variables === account.id}
                        icon={<LogIn className="w-3.5 h-3.5" />}
                      >
                        <span className="hidden sm:inline">{isLoggedIn ? '重登' : '登录'}</span>
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        title="编辑"
                        onClick={() => setFormAccount(account)}
                        icon={<Edit className="w-3.5 h-3.5" />}
                      />
                      <Button
                        size="sm"
                        variant="ghost"
                        title="删除"
                        className="hover:!text-red-500 hover:!bg-red-50"
                        onClick={() => setDeleteTarget(account)}
                        icon={<Trash2 className="w-3.5 h-3.5" />}
                      />
                    </div>
                  </div>
                </div>
              )
            })}
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="mt-6">
              <Pagination
                currentPage={page}
                totalPages={totalPages}
                totalItems={total}
                pageSize={pageSize}
                onPageChange={setPage}
              />
            </div>
          )}
        </>
      )}

      {/* Modals */}
      <AccountFormModal
        open={formAccount !== undefined}
        account={formAccount ?? null}
        onClose={() => setFormAccount(undefined)}
        onSuccess={() => {
          setFormAccount(undefined)
          queryClient.invalidateQueries({ queryKey: ['accounts'] })
        }}
      />

      <TwoFAModal
        open={twoFAAccountId !== null}
        accountId={twoFAAccountId}
        phoneNumbers={twoFAPhones}
        onClose={() => setTwoFAAccountId(null)}
        onSuccess={async () => {
          setTwoFAAccountId(null)
          toast.success('Apple 账户登录成功')
          // Refresh accounts list to get updated data
          await queryClient.invalidateQueries({ queryKey: ['accounts'] })
        }}
      />

      <BatchImportModal
        open={batchImportOpen}
        onClose={() => setBatchImportOpen(false)}
        onSuccess={() => {
          setBatchImportOpen(false)
          queryClient.invalidateQueries({ queryKey: ['accounts'] })
        }}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        title="删除账户"
        message={`确定要删除 ${deleteTarget?.appleId ?? ''} 吗？关联的 HME 数据也将被清除。`}
        confirmText="删除"
        loading={deleteMutation.isPending}
      />
    </div>
  )
}
