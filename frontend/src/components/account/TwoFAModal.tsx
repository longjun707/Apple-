import { useState, useRef, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api } from '@/api/client'
import { toast } from '@/stores/toastStore'
import Modal from '@/components/ui/Modal'
import Button from '@/components/ui/Button'
import { Shield } from 'lucide-react'

interface TwoFAModalProps {
  open: boolean
  accountId: number | null
  onClose: () => void
  onSuccess: () => void
}

export default function TwoFAModal({ open, accountId, onClose, onSuccess }: TwoFAModalProps) {
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setCode('')
      setError('')
      setTimeout(() => inputRef.current?.focus(), 100)
    }
  }, [open])

  const mutation = useMutation({
    mutationFn: () => api.verify2FAForAccount(accountId!, code),
    onSuccess: (res) => {
      if (res.success) {
        toast.success('2FA 验证成功')
        onSuccess()
      } else {
        setError(res.error || '验证失败')
      }
    },
    onError: () => setError('网络错误'),
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!accountId || code.length !== 6) return
    setError('')
    mutation.mutate()
  }

  return (
    <Modal open={open} onClose={onClose}>
      <div className="p-6">
        <div className="flex justify-center mb-5">
          <div className="w-16 h-16 bg-apple-blue/10 rounded-full flex items-center justify-center">
            <Shield className="w-8 h-8 text-apple-blue" />
          </div>
        </div>

        <h2 className="text-lg font-semibold text-center text-gray-900 mb-1">双重认证</h2>
        <p className="text-sm text-center text-gray-500 mb-5">请在 Apple 设备上查看验证码</p>

        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            ref={inputRef}
            type="text"
            inputMode="numeric"
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            className="input text-center text-2xl tracking-[0.5em] font-mono"
            placeholder="000000"
            maxLength={6}
            autoComplete="one-time-code"
            required
          />

          {error && (
            <div className="p-3 bg-red-50 border border-red-200 rounded-xl text-red-600 text-sm animate-fade-in">
              {error}
            </div>
          )}

          <div className="flex gap-3">
            <Button type="button" variant="secondary" onClick={onClose} className="flex-1">
              取消
            </Button>
            <Button
              type="submit"
              disabled={code.length !== 6}
              loading={mutation.isPending}
              className="flex-1"
            >
              验证
            </Button>
          </div>
        </form>
      </div>
    </Modal>
  )
}
