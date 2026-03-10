import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type AppleAccount, type PhoneNumber } from '@/api/client'
import { toast } from '@/stores/toastStore'
import {
  Plus, Trash2, LogIn, Mail, RefreshCw,
  Edit, Loader2,
} from 'lucide-react'
import Button from '@/components/ui/Button'
import ConfirmDialog from '@/components/ui/ConfirmDialog'
import AccountFormModal from '@/components/account/AccountFormModal'
import TwoFAModal from '@/components/account/TwoFAModal'
import AccountHMEPanel from '@/components/account/AccountHMEPanel'

export default function AccountsPage() {
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  // undefined = closed, null = add mode, AppleAccount = edit mode
  const [formAccount, setFormAccount] = useState<AppleAccount | null | undefined>(undefined)
  const [deleteTarget, setDeleteTarget] = useState<AppleAccount | null>(null)
  const [selectedAccount, setSelectedAccount] = useState<AppleAccount | null>(null)
  const [twoFAAccountId, setTwoFAAccountId] = useState<number | null>(null)
  const [twoFAPhones, setTwoFAPhones] = useState<PhoneNumber[]>([])

  // ---- Data ----
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['accounts', page],
    queryFn: () => api.listAccounts(page),
  })

  const accounts = data?.data?.list || []
  const total = data?.data?.total || 0
  const pageSize = data?.data?.pageSize || 20
  const totalPages = Math.ceil(total / pageSize)

  // ---- Mutations ----
  const loginMutation = useMutation({
    mutationFn: (id: number) => api.loginAppleAccount(id),
    onSuccess: (res, id) => {
      if (!res.success) {
        toast.error(res.error || '登录失败')
        return
      }
      if (res.data?.requires2fa) {
        setTwoFAAccountId(id)
        setTwoFAPhones(res.data?.phoneNumbers || [])
      } else {
        toast.success('Apple 账户登录成功')
        const account = accounts.find((a) => a.id === id)
        if (account) setSelectedAccount(account)
      }
      queryClient.invalidateQueries({ queryKey: ['accounts'] })
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
          <Button onClick={() => setFormAccount(null)} size="sm" icon={<Plus className="w-4 h-4" />}>
            添加账户
          </Button>
        </div>
      </div>

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
          <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 overflow-hidden">
            <table className="min-w-full">
              <thead>
                <tr className="border-b border-gray-100">
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider">Apple ID</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden md:table-cell">备注</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider">状态</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden sm:table-cell">HME</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden lg:table-cell">最后登录</th>
                  <th className="px-5 py-3.5 text-right text-[11px] font-semibold text-gray-500 uppercase tracking-wider">操作</th>
                </tr>
              </thead>
              <tbody>
                {accounts.map((account, idx) => (
                  <tr key={account.id} className={`hover:bg-blue-50/40 transition-colors ${idx !== accounts.length - 1 ? 'border-b border-gray-50' : ''}`}>
                    <td className="px-5 py-4">
                      <button
                        onClick={() => setSelectedAccount(account)}
                        className="flex items-center gap-2.5 text-sm font-medium text-gray-900 hover:text-apple-blue transition-colors"
                      >
                        <div className="w-8 h-8 bg-gray-50 rounded-lg flex items-center justify-center flex-shrink-0">
                          <Mail className="w-4 h-4 text-gray-400" />
                        </div>
                        <span className="truncate max-w-[200px]">{account.appleId}</span>
                      </button>
                    </td>
                    <td className="px-5 py-4 text-sm text-gray-500 hidden md:table-cell">
                      {account.remark || <span className="text-gray-300">—</span>}
                    </td>
                    <td className="px-5 py-4">
                      {account.status === 1 ? (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[12px] font-medium bg-emerald-50 text-emerald-700">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" /> 正常
                        </span>
                      ) : (
                        <span
                          className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[12px] font-medium bg-red-50 text-red-600 cursor-help"
                          title={account.lastError}
                        >
                          <span className="w-1.5 h-1.5 rounded-full bg-red-500" /> 异常
                        </span>
                      )}
                    </td>
                    <td className="px-5 py-4 text-sm text-gray-600 tabular-nums font-medium hidden sm:table-cell">
                      {account.hmeCount}
                    </td>
                    <td className="px-5 py-4 text-[13px] text-gray-400 hidden lg:table-cell">
                      {account.lastLogin ? new Date(account.lastLogin).toLocaleString() : '—'}
                    </td>
                    <td className="px-5 py-4">
                      <div className="flex justify-end gap-1">
                        <Button
                          size="sm" variant="ghost" title="登录 Apple 账户"
                          onClick={() => loginMutation.mutate(account.id)}
                          loading={loginMutation.isPending && loginMutation.variables === account.id}
                          icon={<LogIn className="w-4 h-4" />}
                        />
                        <Button
                          size="sm" variant="ghost" title="编辑"
                          onClick={() => setFormAccount(account)}
                          icon={<Edit className="w-4 h-4" />}
                        />
                        <Button
                          size="sm" variant="ghost" title="删除"
                          className="hover:!text-red-500 hover:!bg-red-50"
                          onClick={() => setDeleteTarget(account)}
                          icon={<Trash2 className="w-4 h-4" />}
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
        onSuccess={() => {
          const account = accounts.find((a) => a.id === twoFAAccountId)
          setTwoFAAccountId(null)
          queryClient.invalidateQueries({ queryKey: ['accounts'] })
          if (account) {
            toast.success('Apple 账户登录成功')
            setSelectedAccount(account)
          }
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
