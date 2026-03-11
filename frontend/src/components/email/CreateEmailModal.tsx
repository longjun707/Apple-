import { useState, useEffect } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Mail, Loader2, ChevronDown } from 'lucide-react'
import { api, getErrorMessage, type HMEEmail } from '@/api/client'
import { toast } from '@/stores/toastStore'
import Modal from '@/components/ui/Modal'
import Button from '@/components/ui/Button'
import { useForwardEmailOptions } from '@/hooks/useForwardEmailOptions'

interface CreateEmailModalProps {
  open: boolean
  onClose: () => void
  accountId: number
}

export default function CreateEmailModal({ open, onClose, accountId }: CreateEmailModalProps) {
  const [label, setLabel] = useState('')
  const [note, setNote] = useState('')
  const [forwardTo, setForwardTo] = useState('')
  const [createdEmail, setCreatedEmail] = useState<HMEEmail | null>(null)
  const queryClient = useQueryClient()

  // Fetch forward email options from Apple's actual forward email API
  const {
    data: forwardEmailData,
    isLoading: loadingForward,
    isError: forwardEmailQueryError,
    error: forwardEmailError,
  } = useForwardEmailOptions(accountId, open)

  // Reset form when modal opens
  useEffect(() => {
    if (open) {
      setLabel('')
      setNote('')
      setCreatedEmail(null)
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
    mutationFn: () =>
      api.createAccountHME(accountId, label || undefined, note || undefined, forwardTo || undefined),
    onSuccess: (res) => {
      if (res.success && res.data) {
        setCreatedEmail(res.data)
        queryClient.invalidateQueries({ queryKey: ['account-hme', accountId] })
        queryClient.invalidateQueries({ queryKey: ['accounts'] })
        toast.success('邮箱创建成功')
      } else {
        toast.error(res.error || '创建失败')
      }
    },
    onError: (mutationError) => toast.error(getErrorMessage(mutationError)),
  })

  const handleClose = () => {
    mutation.reset()
    onClose()
  }

  const handleCopy = async () => {
    if (!createdEmail) return
    try {
      await navigator.clipboard.writeText(createdEmail.emailAddress)
      toast.success('已复制到剪贴板')
    } catch {
      toast.error('复制失败')
    }
  }

  return (
    <Modal open={open} onClose={handleClose} disableClose={mutation.isPending}>
      <div className="p-6">
        {/* Header */}
        <div className="flex items-center gap-3 mb-5">
          <div className="w-10 h-10 rounded-full bg-apple-blue/10 flex items-center justify-center">
            <Mail className="w-5 h-5 text-apple-blue" />
          </div>
          <h2 className="text-lg font-semibold text-gray-900">创建隐藏邮箱</h2>
        </div>

        {!createdEmail ? (
          <>
            <div className="space-y-4">
              {/* Label */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">标签</label>
                <input
                  type="text"
                  value={label}
                  onChange={(e) => setLabel(e.target.value)}
                  className="input"
                  placeholder="留空则自动生成"
                />
              </div>

              {/* Note */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">备注</label>
                <input
                  type="text"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  className="input"
                  placeholder="留空则自动生成"
                />
              </div>

              {/* Forward To */}
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
                    {forwardEmailData?.needsLogin ? '请先登录账户后再创建' : '未获取到转发邮箱（将使用默认）'}
                  </p>
                )}
              </div>
            </div>

            {/* Actions */}
            <div className="flex gap-3 mt-6">
              <Button variant="secondary" onClick={handleClose} disabled={mutation.isPending} className="flex-1">
                取消
              </Button>
              <Button
                onClick={() => mutation.mutate()}
                loading={mutation.isPending}
                disabled={forwardEmailData?.needsLogin}
                className="flex-1"
                icon={<Mail className="w-4 h-4" />}
              >
                创建
              </Button>
            </div>
          </>
        ) : (
          <>
            {/* Success state */}
            <div className="bg-green-50 rounded-xl p-4 mb-4">
              <p className="text-sm text-green-800 font-medium mb-2">创建成功!</p>
              <div className="flex items-center gap-2">
                <code className="text-sm font-mono text-green-900 bg-green-100 px-2 py-1 rounded-lg flex-1 truncate">
                  {createdEmail.emailAddress}
                </code>
              </div>
              {createdEmail.forwardToEmail && (
                <p className="text-xs text-green-600 mt-2">
                  转发到: {createdEmail.forwardToEmail}
                </p>
              )}
              {createdEmail.label && (
                <p className="text-xs text-green-600 mt-1">
                  标签: {createdEmail.label}
                </p>
              )}
            </div>
            <div className="flex gap-3">
              <Button variant="secondary" onClick={handleCopy} className="flex-1">
                复制邮箱
              </Button>
              <Button onClick={handleClose} className="flex-1">
                完成
              </Button>
            </div>
          </>
        )}
      </div>
    </Modal>
  )
}
