import { useState, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, getErrorMessage, type AppleAccount } from '@/api/client'
import { toast } from '@/stores/toastStore'
import Modal from '@/components/ui/Modal'
import Button from '@/components/ui/Button'
import { User } from 'lucide-react'

interface AccountFormModalProps {
  open: boolean
  account: AppleAccount | null  // null = add, non-null = edit
  onClose: () => void
  onSuccess: () => void
}

export default function AccountFormModal({ open, account, onClose, onSuccess }: AccountFormModalProps) {
  const [appleId, setAppleId] = useState('')
  const [password, setPassword] = useState('')
  const [remark, setRemark] = useState('')
  const [error, setError] = useState('')
  const normalizedAppleId = appleId.trim().toLowerCase()
  const normalizedRemark = remark.trim()

  // Sync form state when modal opens or account changes
  useEffect(() => {
    if (open) {
      setAppleId(account?.appleId || '')
      setPassword('')
      setRemark(account?.remark || '')
      setError('')
    }
  }, [open, account])

  const submitMutation = useMutation({
    mutationFn: () =>
      account
        ? api.updateAccount(
            account.id,
            normalizedAppleId,
            password.trim() || undefined,
            normalizedRemark || undefined,
          )
        : api.createAccount(
            normalizedAppleId,
            password,
            normalizedRemark || undefined,
          ),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(account ? '账户更新成功' : '账户添加成功')
        onSuccess()
      } else {
        setError(res.error || (account ? '更新失败' : '创建失败'))
      }
    },
    onError: (mutationError) => setError(getErrorMessage(mutationError)),
  })

  const handleClose = () => {
    if (submitMutation.isPending) return
    submitMutation.reset()
    setError('')
    onClose()
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    submitMutation.mutate()
  }

  return (
    <Modal open={open} onClose={handleClose} disableClose={submitMutation.isPending}>
      <div className="p-6">
        <div className="flex items-center gap-3 mb-5">
          <div className="w-10 h-10 rounded-full bg-apple-blue/10 flex items-center justify-center">
            <User className="w-5 h-5 text-apple-blue" />
          </div>
          <h2 className="text-lg font-semibold text-gray-900">
            {account ? '编辑账户' : '添加账户'}
          </h2>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Apple ID</label>
            <input
              type="email"
              value={appleId}
              onChange={(e) => {
                setAppleId(e.target.value)
                setError('')
              }}
              className="input"
              placeholder="your@email.com"
              required
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              密码 {account && <span className="text-gray-400 font-normal">(留空不修改)</span>}
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => {
                setPassword(e.target.value)
                setError('')
              }}
              className="input"
              placeholder="••••••••"
              required={!account}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">备注</label>
            <input
              type="text"
              value={remark}
              onChange={(e) => {
                setRemark(e.target.value)
                setError('')
              }}
              className="input"
              placeholder="可选"
            />
          </div>

          {error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-red-600 text-sm animate-fade-in">
              {error}
            </div>
          )}

          <div className="flex gap-3 pt-2">
            <Button
              type="button"
              variant="secondary"
              onClick={handleClose}
              disabled={submitMutation.isPending}
              className="flex-1"
            >
              取消
            </Button>
            <Button
              type="submit"
              loading={submitMutation.isPending}
              disabled={!normalizedAppleId || (!account && !password)}
              className="flex-1"
            >
              {account ? '保存' : '添加'}
            </Button>
          </div>
        </form>
      </div>
    </Modal>
  )
}
