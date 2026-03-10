import { useState, useMemo, useCallback, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type AppleAccount, type HMEEmail } from '@/api/client'
import { toast } from '@/stores/toastStore'
import { Loader2, ArrowLeft, RefreshCw } from 'lucide-react'
import Button from '@/components/ui/Button'
import ConfirmDialog from '@/components/ui/ConfirmDialog'
import Pagination from '@/components/ui/Pagination'
import StatsBar from '@/components/email/StatsBar'
import SearchBar, { type FilterStatus } from '@/components/email/SearchBar'
import EmailItem from '@/components/email/EmailItem'
import EmptyState from '@/components/email/EmptyState'
import BatchCreateModal from '@/components/email/BatchCreateModal'
import CreateEmailModal from '@/components/email/CreateEmailModal'

interface AccountHMEPanelProps {
  account: AppleAccount
  onBack: () => void
}

export default function AccountHMEPanel({ account, onBack }: AccountHMEPanelProps) {
  const [showBatchModal, setShowBatchModal] = useState(false)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [filterStatus, setFilterStatus] = useState<FilterStatus>('all')
  const [deleteTarget, setDeleteTarget] = useState<HMEEmail | null>(null)
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const queryClient = useQueryClient()

  // ---- Data ----
  const { data, isLoading, refetch } = useQuery({
    queryKey: ['account-hme', account.id],
    queryFn: async () => {
      const res = await api.getAccountHME(account.id)
      if (!res.success) throw new Error(res.error)
      return res.data || []
    },
  })

  const hmeList = data || []

  // ---- Mutations ----
  const deleteMutation = useMutation({
    mutationFn: (hmeId: string) => api.deleteAccountHME(account.id, hmeId),
    onSuccess: (res) => {
      if (res.success) {
        queryClient.invalidateQueries({ queryKey: ['account-hme', account.id] })
        queryClient.invalidateQueries({ queryKey: ['accounts'] })
        toast.success('邮箱已删除')
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

  // ---- Filtering ----
  const filteredList = useMemo(() => {
    return hmeList.filter((e: HMEEmail) => {
      if (filterStatus === 'active' && !e.active) return false
      if (filterStatus === 'inactive' && e.active) return false
      if (searchQuery) {
        const q = searchQuery.toLowerCase()
        return e.emailAddress.toLowerCase().includes(q) || e.label.toLowerCase().includes(q)
      }
      return true
    })
  }, [hmeList, searchQuery, filterStatus])

  // ---- Pagination ----
  const totalPages = Math.ceil(filteredList.length / pageSize)
  const paginatedList = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return filteredList.slice(start, start + pageSize)
  }, [filteredList, currentPage, pageSize])

  // Reset to page 1 when filter/search changes
  useEffect(() => {
    setCurrentPage(1)
  }, [searchQuery, filterStatus])

  const isFiltered = searchQuery !== '' || filterStatus !== 'all'

  // ---- Handlers ----
  const handleCopy = useCallback(async (email: string) => {
    try {
      await navigator.clipboard.writeText(email)
      toast.success('已复制到剪贴板')
    } catch {
      toast.error('复制失败，请手动复制')
    }
  }, [])

  const handleCopyAll = useCallback(async () => {
    if (!hmeList.length) return
    try {
      const emails = hmeList.map((e: HMEEmail) => e.emailAddress).join('\n')
      await navigator.clipboard.writeText(emails)
      toast.success(`已复制 ${hmeList.length} 个邮箱`)
    } catch {
      toast.error('复制失败，请手动复制')
    }
  }, [hmeList])

  const handleExport = useCallback(() => {
    if (!hmeList.length) return
    const esc = (v: string) => v.includes(',') || v.includes('"') ? `"${v.replace(/"/g, '""')}"` : v
    const csv = [
      'Email,Label,ForwardTo,Active,CreateTime',
      ...hmeList.map(
        (e: HMEEmail) =>
          `${esc(e.emailAddress)},${esc(e.label)},${esc(e.forwardToEmail)},${e.active},${new Date(e.createTime).toISOString()}`,
      ),
    ].join('\n')
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `hme-${account.appleId}-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
    toast.success('已导出 CSV 文件')
  }, [hmeList, account.appleId])

  return (
    <div className="animate-fade-in">
      {/* Back + Title */}
      <div className="flex items-center gap-4 mb-8">
        <button
          onClick={onBack}
          className="w-9 h-9 flex items-center justify-center rounded-xl border border-gray-200 bg-white hover:bg-gray-50 text-gray-500 hover:text-gray-700 transition-all shadow-sm"
        >
          <ArrowLeft className="w-4 h-4" />
        </button>
        <div className="min-w-0 flex-1">
          <h2 className="text-xl font-bold text-gray-900 truncate tracking-tight">HME 管理</h2>
          <p className="text-sm text-gray-500 truncate mt-0.5">{account.appleId}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => refetch()} icon={<RefreshCw className="w-3.5 h-3.5" />}>
          刷新
        </Button>
      </div>

      {/* Stats + Actions */}
      <div className="mb-5">
        <StatsBar
          total={hmeList.length}
          isCreating={false}
          onCreate={() => setShowCreateModal(true)}
          onBatchCreate={() => setShowBatchModal(true)}
          onCopyAll={handleCopyAll}
          onExport={handleExport}
        />
      </div>

      {/* Search & Filter */}
      {hmeList.length > 0 && (
        <div className="mb-5">
          <SearchBar
            query={searchQuery}
            onQueryChange={setSearchQuery}
            filter={filterStatus}
            onFilterChange={setFilterStatus}
            total={hmeList.length}
            filtered={filteredList.length}
          />
        </div>
      )}

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="w-8 h-8 animate-spin text-apple-blue" />
        </div>
      )}

      {/* Email list */}
      {!isLoading && (
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 overflow-hidden">
          {filteredList.length === 0 ? (
            <EmptyState isFiltered={isFiltered} />
          ) : (
            <>
              <div className="divide-y divide-gray-50">
                {paginatedList.map((email: HMEEmail) => (
                  <EmailItem
                    key={email.id}
                    email={email}
                    onCopy={handleCopy}
                    onDelete={setDeleteTarget}
                  />
                ))}
              </div>
              {/* Pagination */}
              {totalPages > 1 && (
                <div className="border-t border-gray-100">
                  <Pagination
                    currentPage={currentPage}
                    totalPages={totalPages}
                    totalItems={filteredList.length}
                    pageSize={pageSize}
                    onPageChange={setCurrentPage}
                    onPageSizeChange={(size) => {
                      setPageSize(size)
                      setCurrentPage(1)
                    }}
                  />
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* Modals */}
      <CreateEmailModal
        open={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        accountId={account.id}
      />

      <BatchCreateModal
        open={showBatchModal}
        onClose={() => setShowBatchModal(false)}
        accountId={account.id}
      />

      <ConfirmDialog
        open={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        title="删除邮箱"
        message={`确定要删除 ${deleteTarget?.emailAddress ?? ''} 吗？此操作不可撤销。`}
        confirmText="删除"
        loading={deleteMutation.isPending}
      />
    </div>
  )
}
