import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type HMEWithAccount } from '@/api/client'
import { toast } from '@/stores/toastStore'
import {
  Mail, Search, Copy, Check, Download,
  ClipboardCopy, Loader2,
} from 'lucide-react'
import Button from '@/components/ui/Button'
import { cn } from '@/lib/cn'

export default function EmailsPage() {
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [copiedId, setCopiedId] = useState<number | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['all-hme', page, search],
    queryFn: () => api.listAllHME(page, 20, search),
  })

  const list = data?.data?.list || []
  const total = data?.data?.total || 0
  const pageSize = data?.data?.pageSize || 20
  const totalPages = Math.ceil(total / pageSize)

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    setSearch(searchInput)
    setPage(1)
  }

  const handleCopy = useCallback(async (email: HMEWithAccount) => {
    try {
      await navigator.clipboard.writeText(email.emailAddress)
      setCopiedId(email.id)
      toast.success('已复制到剪贴板')
      setTimeout(() => setCopiedId(null), 2000)
    } catch {
      toast.error('复制失败')
    }
  }, [])

  const handleCopyAll = useCallback(async () => {
    if (!list.length) return
    try {
      const emails = list.map((e) => e.emailAddress).join('\n')
      await navigator.clipboard.writeText(emails)
      toast.success(`已复制 ${list.length} 个邮箱`)
    } catch {
      toast.error('复制失败')
    }
  }, [list])

  const handleExport = useCallback(() => {
    if (!list.length) return
    const esc = (v: string) =>
      v.includes(',') || v.includes('"') ? `"${v.replace(/"/g, '""')}"` : v
    const csv = [
      'Email,Label,Account,ForwardTo,Active,CreatedAt',
      ...list.map(
        (e) =>
          `${esc(e.emailAddress)},${esc(e.label)},${esc(e.appleId)},${esc(e.forwardToEmail)},${e.active},${e.createdAt}`,
      ),
    ].join('\n')
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `hme-all-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
    toast.success('已导出 CSV 文件')
  }, [list])

  return (
    <div className="animate-fade-in">
      {/* Header */}
      <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-8">
        <div>
          <h2 className="text-2xl font-bold text-gray-900 tracking-tight">HME 邮箱</h2>
          <p className="text-sm text-gray-500 mt-1">
            共 <span className="font-medium text-gray-700 tabular-nums">{total}</span> 个邮箱
          </p>
        </div>
        <div className="flex gap-2">
          {list.length > 0 && (
            <>
              <Button variant="ghost" size="sm" onClick={handleCopyAll} icon={<ClipboardCopy className="w-4 h-4" />}>
                复制当前页
              </Button>
              <Button variant="ghost" size="sm" onClick={handleExport} icon={<Download className="w-4 h-4" />}>
                导出当前页
              </Button>
            </>
          )}
        </div>
      </div>

      {/* Search */}
      <form onSubmit={handleSearch} className="mb-5">
        <div className="relative max-w-md">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
          <input
            type="text"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            placeholder="搜索邮箱、标签或 Apple ID..."
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
      {!isLoading && list.length === 0 && (
        <div className="text-center py-20">
          <Mail className="w-12 h-12 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 font-medium">
            {search ? '没有匹配的邮箱' : '暂无 HME 邮箱记录'}
          </p>
          <p className="text-sm text-gray-400 mt-1">
            {search ? '试试修改搜索条件' : '请先在 Apple 账户页面登录并创建 HME'}
          </p>
        </div>
      )}

      {/* Table */}
      {!isLoading && list.length > 0 && (
        <>
          <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 overflow-hidden">
            <table className="min-w-full">
              <thead>
                <tr className="border-b border-gray-100">
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider">邮箱地址</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden md:table-cell">标签</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden lg:table-cell">所属账户</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider">状态</th>
                  <th className="px-5 py-3.5 text-left text-[11px] font-semibold text-gray-500 uppercase tracking-wider hidden lg:table-cell">创建时间</th>
                  <th className="px-5 py-3.5 text-right text-[11px] font-semibold text-gray-500 uppercase tracking-wider">操作</th>
                </tr>
              </thead>
              <tbody>
                {list.map((email, idx) => (
                  <tr
                    key={email.id}
                    className={cn(
                      'hover:bg-blue-50/40 transition-colors',
                      idx !== list.length - 1 && 'border-b border-gray-50',
                    )}
                  >
                    <td className="px-5 py-4">
                      <div className="flex items-center gap-2.5">
                        <div
                          className={cn(
                            'w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0',
                            email.active ? 'bg-green-50 text-green-600' : 'bg-gray-100 text-gray-400',
                          )}
                        >
                          <Mail className="w-4 h-4" />
                        </div>
                        <span className="text-sm font-medium text-gray-900 truncate max-w-[240px]">
                          {email.emailAddress}
                        </span>
                      </div>
                    </td>
                    <td className="px-5 py-4 text-sm text-gray-500 hidden md:table-cell">
                      {email.label || <span className="text-gray-300">—</span>}
                    </td>
                    <td className="px-5 py-4 text-sm text-gray-500 hidden lg:table-cell">
                      <span className="truncate max-w-[180px] inline-block">{email.appleId}</span>
                    </td>
                    <td className="px-5 py-4">
                      {email.active ? (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[12px] font-medium bg-emerald-50 text-emerald-700">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500" /> 启用
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-[12px] font-medium bg-gray-100 text-gray-500">
                          <span className="w-1.5 h-1.5 rounded-full bg-gray-400" /> 停用
                        </span>
                      )}
                    </td>
                    <td className="px-5 py-4 text-[13px] text-gray-400 hidden lg:table-cell">
                      {new Date(email.createdAt).toLocaleString()}
                    </td>
                    <td className="px-5 py-4">
                      <div className="flex justify-end">
                        <button
                          onClick={() => handleCopy(email)}
                          className={cn(
                            'p-2 rounded-lg transition-colors',
                            copiedId === email.id
                              ? 'text-green-500 bg-green-50'
                              : 'text-gray-400 hover:text-apple-blue hover:bg-apple-blue/10',
                          )}
                          title="复制邮箱"
                        >
                          {copiedId === email.id ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                        </button>
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
    </div>
  )
}
