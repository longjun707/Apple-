import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { CheckCircle2, XCircle, Layers, Loader2, ChevronDown } from 'lucide-react'
import { api, getErrorMessage, type BatchCreateResult } from '@/api/client'
import { toast } from '@/stores/toastStore'
import Modal from '@/components/ui/Modal'
import Button from '@/components/ui/Button'
import { useForwardEmailOptions } from '@/hooks/useForwardEmailOptions'

interface BatchCreateModalProps {
  open: boolean
  onClose: () => void
  accountId: number
}

export default function BatchCreateModal({ open, onClose, accountId }: BatchCreateModalProps) {
  const [count, setCount] = useState(5)
  const [prefix, setPrefix] = useState('Auto')
  const [forwardTo, setForwardTo] = useState('')
  const [result, setResult] = useState<BatchCreateResult | null>(null)
  const queryClient = useQueryClient()

  // Fetch forward email options from Apple's actual forward email API
  const {
    data: forwardEmailData,
    isLoading: loadingForward,
    isError: forwardEmailQueryError,
    error: forwardEmailError,
  } = useForwardEmailOptions(accountId, open)

  // Reset form when modal opens, then auto-select first forward email
  useEffect(() => {
    if (open) {
      setCount(5)
      setPrefix('Auto')
      setResult(null)
      // 延迟设置 forwardTo，等待 forwardEmails 加载
      setForwardTo('')
    }
  }, [open])

  // Auto-select first forward email after data loads (only when forwardTo is empty)
  useEffect(() => {
    if (open && forwardEmailData?.availableEmails.length && forwardTo === '') {
      setForwardTo(forwardEmailData.availableEmails[0].address)
    }
  }, [open, forwardEmailData, forwardTo])

  const mutation = useMutation({
    mutationFn: () => api.batchCreateAccountHME(accountId, count, prefix, 1000, forwardTo || undefined),
    onSuccess: (res) => {
      if (res.success && res.data) {
        setResult(res.data)
        queryClient.invalidateQueries({ queryKey: ['account-hme', accountId] })
        toast.success(`成功创建 ${res.data.success} 个邮箱`)
      } else {
        toast.error(res.error || '批量创建失败')
      }
    },
    onError: (mutationError) => toast.error(getErrorMessage(mutationError)),
  })

  const handleClose = () => {
    setResult(null)
    mutation.reset()
    onClose()
  }

  return (
    <Modal open={open} onClose={handleClose} disableClose={mutation.isPending}>
      <div className="p-6">
        <div className="flex items-center gap-3 mb-5">
          <div className="w-10 h-10 rounded-full bg-apple-blue/10 flex items-center justify-center">
            <Layers className="w-5 h-5 text-apple-blue" />
          </div>
          <h2 className="text-lg font-semibold text-gray-900">批量创建</h2>
        </div>

        {!result ? (
          <>
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">数量</label>
                <input
                  type="number"
                  min={1}
                  max={100}
                  value={count}
                  onChange={(e) => setCount(Math.min(100, Math.max(1, parseInt(e.target.value) || 1)))}
                  className="input"
                />
                <p className="text-xs text-gray-400 mt-1">最多 100 个</p>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">标签前缀</label>
                <input
                  type="text"
                  value={prefix}
                  onChange={(e) => setPrefix(e.target.value)}
                  className="input"
                  placeholder="Auto"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">转发到</label>
                {loadingForward ? (
                  <div className="flex items-center gap-2 px-3 py-2.5 text-sm text-gray-400 border border-gray-200 rounded-xl bg-gray-50">
                    <Loader2 className="w-4 h-4 animate-spin" />
                    加载转发邮箱...
                  </div>
                ) : forwardEmailData?.availableEmails.length ? (
                  <div className="relative">
                    <select
                      value={forwardTo}
                      onChange={(e) => setForwardTo(e.target.value)}
                      className="input appearance-none pr-9"
                    >
                      {forwardEmailData.availableEmails.map((emailOption) => (
                        <option key={emailOption.id} value={emailOption.address}>
                          {emailOption.address}
                        </option>
                      ))}
                    </select>
                    <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400 pointer-events-none" />
                  </div>
                ) : forwardEmailQueryError ? (
                  <p className="text-sm text-red-500 px-3 py-2.5 border border-red-100 rounded-xl bg-red-50">
                    {getErrorMessage(forwardEmailError, '获取转发邮箱失败')}
                  </p>
                ) : (
                  <p className="text-sm text-gray-400 px-3 py-2.5 border border-gray-200 rounded-xl bg-gray-50">
                    {forwardEmailData?.needsLogin ? '请先登录账户后再批量创建' : '未获取到转发邮箱（将使用默认）'}
                  </p>
                )}
              </div>
            </div>
            <div className="flex gap-3 mt-6">
              <Button variant="secondary" onClick={handleClose} disabled={mutation.isPending} className="flex-1">
                取消
              </Button>
              <Button
                onClick={() => mutation.mutate()}
                loading={mutation.isPending}
                disabled={forwardEmailData?.needsLogin}
                className="flex-1"
              >
                创建 {count} 个
              </Button>
            </div>
          </>
        ) : (
          <>
            <div className="space-y-3">
              <div className="flex items-center gap-3 p-3 bg-green-50 rounded-xl">
                <CheckCircle2 className="w-5 h-5 text-green-600 flex-shrink-0" />
                <span className="text-sm text-green-800 font-medium">
                  成功: {result.success} 个
                </span>
              </div>
              {result.failed > 0 && (
                <div className="flex items-center gap-3 p-3 bg-red-50 rounded-xl">
                  <XCircle className="w-5 h-5 text-red-600 flex-shrink-0" />
                  <div>
                    <span className="text-sm text-red-800 font-medium">
                      失败: {result.failed} 个
                    </span>
                    {result.errors.length > 0 && (
                      <ul className="mt-1 text-xs text-red-600 space-y-0.5">
                        {result.errors.slice(0, 5).map((err, i) => (
                          <li key={i}>· {err}</li>
                        ))}
                      </ul>
                    )}
                  </div>
                </div>
              )}
            </div>
            <Button variant="secondary" onClick={handleClose} className="w-full mt-5">
              关闭
            </Button>
          </>
        )}
      </div>
    </Modal>
  )
}
