import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type AppleAccount, type PhoneNumber } from '@/api/client'
import { toast } from '@/stores/toastStore'
import {
  Plus, Trash2, LogIn, Mail, RefreshCw, Search,
  Edit, Loader2, Users, Shield, Smartphone, Eye, Phone, Upload,
} from 'lucide-react'
import Button from '@/components/ui/Button'
import ConfirmDialog from '@/components/ui/ConfirmDialog'
import AccountFormModal from '@/components/account/AccountFormModal'
import TwoFAModal from '@/components/account/TwoFAModal'
import AccountHMEPanel from '@/components/account/AccountHMEPanel'
import BatchImportModal from '@/components/account/BatchImportModal'

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

  // ---- Account list view ----
  return (
    <div className="animate-fade-in">
      {/* Header */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-8">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 tracking-tight">Apple 账户管理</h2>
          <p className="text-sm text-gray-500 mt-1">共 <span className="font-medium text-gray-700 tabular-nums">{total}</span> 个账户</p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => refetch()} variant="outline" size="sm" icon={<RefreshCw className="w-4 h-4" />}>
            刷新
          </Button>
          <Button onClick={() => setBatchImportOpen(true)} variant="outline" size="sm" icon={<Upload className="w-4 h-4" />}>
            批量导入
          </Button>
          <Button onClick={() => setFormAccount(null)} size="sm" icon={<Plus className="w-4 h-4" />}>
            添加账户
          </Button>
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
        <div className="relative max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="搜索 Apple ID、姓名或备注..."
            className="w-full pl-9 pr-4 py-2.5 text-sm border border-gray-200 rounded-xl bg-white focus:outline-none focus:ring-2 focus:ring-apple-blue/20 focus:border-apple-blue transition-all placeholder:text-gray-400"
          />
        </div>
      </form>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="w-8 h-8 animate-spin text-apple-blue" />
        </div>
      )}

      {/* Empty */}
      {!isLoading && accounts.length === 0 && (
        <div className="text-center py-20">
          <Mail className="w-12 h-12 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 font-medium">暂无账户</p>
          <p className="text-sm text-gray-400 mt-1">点击「添加账户」开始</p>
        </div>
      )}

      {/* Account table */}
      {!isLoading && accounts.length > 0 && (
        <>
          <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 overflow-x-auto">
            <table className="min-w-full table-fixed">
              <thead>
                <tr className="border-b border-gray-100 bg-gray-50/50">
                  <th className="w-[220px] px-4 py-3 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider">Apple ID / 姓名</th>
                  <th className="w-[120px] px-4 py-3 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden lg:table-cell">个人信息</th>
                  <th className="w-[80px] px-4 py-3 text-center text-[11px] font-semibold text-gray-500 uppercase tracking-wider">状态</th>
                  <th className="w-[100px] px-4 py-3 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden sm:table-cell">安全</th>
                  <th className="w-[60px] px-4 py-3 text-center text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden sm:table-cell">HME</th>
                  <th className="w-[110px] px-4 py-3 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden md:table-cell">家人共享</th>
                  <th className="w-[180px] px-4 py-3 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden xl:table-cell">备注 / 错误</th>
                  <th className="w-[140px] px-4 py-3 text-center text-[11px] font-semibold text-gray-500 uppercase tracking-wider">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-50">
                {accounts.map((account) => (
                  <tr key={account.id} className="hover:bg-blue-50/30 transition-colors">
                    <td className="px-4 py-3">
                      <button
                        onClick={() => setSelectedAccount(account)}
                        className="flex items-center gap-2.5 text-left hover:bg-gray-50 rounded-lg -m-1 p-1 transition-colors w-full"
                      >
                        <div className="w-8 h-8 bg-gradient-to-br from-gray-100 to-gray-50 rounded-lg flex items-center justify-center flex-shrink-0 border border-gray-100">
                          <Mail className="w-3.5 h-3.5 text-gray-400" />
                        </div>
                        <div className="min-w-0 flex-1">
                          <div className="text-[13px] font-medium text-gray-900 truncate">{account.appleId}</div>
                          {account.fullName && <div className="text-[11px] text-gray-400 truncate">{account.fullName}</div>}
                          {/* Phone Numbers */}
                          {(() => {
                            try {
                              const phones = account.phoneNumbers ? JSON.parse(account.phoneNumbers) : [];
                              if (phones.length === 0) return null;
                              const getPhone = (p: { fullNumberWithCountryPrefix?: string; numberWithDialCode?: string }) => 
                                p.fullNumberWithCountryPrefix || p.numberWithDialCode || '';
                              return (
                                <div className="flex items-center gap-1 mt-0.5" title={phones.map(getPhone).join('\n')}>
                                  <Phone className="w-2.5 h-2.5 text-green-500" />
                                  <span className="text-[10px] text-green-600 truncate">
                                    {getPhone(phones[0])}
                                  </span>
                                </div>
                              );
                            } catch {
                              return null;
                            }
                          })()}
                        </div>
                      </button>
                    </td>
                    <td className="px-4 py-3 hidden lg:table-cell">
                      <div className="text-[11px] space-y-0.5">
                        {account.birthday && <div className="text-gray-500">🎂 {account.birthday}</div>}
                        {account.country && <div className="text-gray-400">🌏 {account.country === 'CHN' ? '中国' : account.country}</div>}
                        {(() => {
                          try {
                            const emails: string[] = typeof account.alternateEmails === 'string' 
                              ? (account.alternateEmails ? JSON.parse(account.alternateEmails) : []) 
                              : (account.alternateEmails || []);
                            if (emails.length === 0) return null;
                            const totalEmails = 1 + emails.length;
                            return (
                              <div 
                                className="text-blue-500 cursor-help" 
                                title={`主要: ${account.appleId}\n备用: ${emails.join('\n')}`}
                              >
                                📧 {totalEmails} 个邮箱
                              </div>
                            );
                          } catch {
                            return null;
                          }
                        })()}
                        {!account.birthday && !account.country && !account.alternateEmails && <span className="text-gray-300">—</span>}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-center">
                      {account.status === 1 ? (
                        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[11px] font-medium bg-emerald-50 text-emerald-700">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" /> 正常
                        </span>
                      ) : (
                        <span
                          className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[11px] font-medium bg-red-50 text-red-600 cursor-help"
                          title={account.lastError}
                        >
                          <span className="w-1.5 h-1.5 rounded-full bg-red-500" /> 异常
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 hidden sm:table-cell">
                      <div className="flex flex-col gap-0.5">
                        {account.sessionSavedAt ? (
                          <span className="text-[10px] text-emerald-600 font-medium" title={`会话保存于 ${new Date(account.sessionSavedAt).toLocaleString()}`}>
                            ✓ 已登录
                          </span>
                        ) : (
                          <span className="text-[10px] text-orange-500 font-medium">
                            ⚠ 未登录
                          </span>
                        )}
                        <div className="flex items-center gap-0.5">
                          <Shield className={`w-3 h-3 ${account.twoFactorEnabled ? 'text-emerald-500' : 'text-gray-300'}`} />
                          <span className={`text-[10px] ${account.twoFactorEnabled ? 'text-emerald-600' : 'text-gray-400'}`}>
                            {account.twoFactorEnabled ? '2FA' : '无2FA'}
                          </span>
                        </div>
                        {account.trustedDeviceCount !== undefined && account.trustedDeviceCount > 0 && (
                          <div className="flex items-center gap-0.5">
                            <Smartphone className="w-3 h-3 text-blue-400" />
                            <span className="text-[10px] text-gray-500">{account.trustedDeviceCount}设备</span>
                          </div>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3 text-center text-[13px] text-gray-600 tabular-nums font-medium hidden sm:table-cell">
                      {account.hmeCount}
                    </td>
                    <td className="px-4 py-3 hidden md:table-cell">
                      {account.familyMemberCount && account.familyMemberCount > 0 ? (
                        <div className="flex items-center gap-1">
                          <span className={`inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[10px] font-medium ${
                            account.isFamilyOrganizer
                              ? 'bg-purple-50 text-purple-700'
                              : account.familyRole === 'parent'
                              ? 'bg-orange-50 text-orange-700'
                              : 'bg-gray-50 text-gray-600'
                          }`}>
                            <Users className="w-2.5 h-2.5" />
                            {account.familyMemberCount}
                          </span>
                          <span className="text-[10px] text-gray-400">
                            {account.isFamilyOrganizer ? '组织者' : account.familyRole === 'parent' ? '家长' : account.familyRole === 'child' ? '儿童' : '成员'}
                          </span>
                        </div>
                      ) : (
                        <span className="text-gray-300 text-[11px]">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3 hidden xl:table-cell">
                      <div className="text-[11px] space-y-0.5">
                        {account.remark && <div className="text-gray-500 truncate" title={account.remark}>{account.remark}</div>}
                        {account.lastError && <div className="text-red-500 truncate" title={account.lastError}>⚠ {account.lastError}</div>}
                        {!account.remark && !account.lastError && <span className="text-gray-300">—</span>}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex justify-center gap-0.5">
                        <Button
                          size="sm" variant="ghost" title="查看详情"
                          onClick={() => setSelectedAccount(account)}
                          icon={<Eye className="w-3.5 h-3.5" />}
                        />
                        <Button
                          size="sm" variant="ghost" title="登录 Apple 账户"
                          onClick={() => loginMutation.mutate(account.id)}
                          loading={loginMutation.isPending && loginMutation.variables === account.id}
                          icon={<LogIn className="w-3.5 h-3.5" />}
                        />
                        <Button
                          size="sm" variant="ghost" title="编辑"
                          onClick={() => setFormAccount(account)}
                          icon={<Edit className="w-3.5 h-3.5" />}
                        />
                        <Button
                          size="sm" variant="ghost" title="删除"
                          className="hover:!text-red-500 hover:!bg-red-50"
                          onClick={() => setDeleteTarget(account)}
                          icon={<Trash2 className="w-3.5 h-3.5" />}
                        />
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex justify-center items-center gap-4 mt-6">
              <Button size="sm" variant="outline" disabled={page === 1} onClick={() => setPage((p) => p - 1)}>
                上一页
              </Button>
              <span className="text-[13px] text-gray-500 tabular-nums font-medium">
                {page} <span className="text-gray-300 mx-0.5">/</span> {totalPages}
              </span>
              <Button size="sm" variant="outline" disabled={page === totalPages} onClick={() => setPage((p) => p + 1)}>
                下一页
              </Button>
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
