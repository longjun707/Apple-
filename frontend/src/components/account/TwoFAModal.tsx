import { useState, useRef, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api } from '@/api/client'
import { toast } from '@/stores/toastStore'
import Modal from '@/components/ui/Modal'
import Button from '@/components/ui/Button'
import { Shield, MessageSquare } from 'lucide-react'
import type { PhoneNumber } from '@/api/client'

interface TwoFAModalProps {
  open: boolean
  accountId: number | null
  phoneNumbers?: PhoneNumber[]
  onClose: () => void
  onSuccess: () => void
}

export default function TwoFAModal({ open, accountId, phoneNumbers, onClose, onSuccess }: TwoFAModalProps) {
  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [method, setMethod] = useState<'device' | 'sms'>('device')
  const [smsSent, setSmsSent] = useState(false)
  const [selectedPhoneId, setSelectedPhoneId] = useState(1)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setCode('')
      setError('')
      setMethod('device')
      setSmsSent(false)
      setSelectedPhoneId(phoneNumbers?.[0]?.id ?? 1)
      setTimeout(() => inputRef.current?.focus(), 100)
    }
  }, [open, phoneNumbers])

  const verifyMutation = useMutation({
    mutationFn: () => api.verify2FAForAccount(accountId!, code, method, selectedPhoneId),
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

  const smsMutation = useMutation({
    mutationFn: (phoneId: number) => api.requestSMSForAccount(accountId!, phoneId),
    onSuccess: (res) => {
      if (res.success) {
        setSmsSent(true)
        setMethod('sms')
        setCode('')
        setError('')
        toast.success('短信验证码已发送')
        setTimeout(() => inputRef.current?.focus(), 100)
      } else {
        setError(res.error || '发送失败')
      }
    },
    onError: () => setError('发送短信失败'),
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!accountId || code.length !== 6) return
    setError('')
    verifyMutation.mutate()
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
        <p className="text-sm text-center text-gray-500 mb-5">
          {method === 'sms' ? '请输入短信验证码' : '请在 Apple 设备上查看验证码'}
        </p>

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
              loading={verifyMutation.isPending}
              className="flex-1"
            >
              验证
            </Button>
          </div>

          <div className="pt-2 border-t border-gray-100 space-y-2">
            {phoneNumbers && phoneNumbers.length > 0 ? (
              phoneNumbers.map((phone) => (
                <button
                  key={phone.id}
                  type="button"
                  onClick={() => {
                    setSelectedPhoneId(phone.id)
                    smsMutation.mutate(phone.id)
                  }}
                  disabled={smsMutation.isPending}
                  className="w-full flex items-center justify-center gap-2 py-2.5 text-sm text-apple-blue hover:text-blue-700 transition-colors disabled:opacity-50"
                >
                  <MessageSquare className="w-4 h-4" />
                  {smsMutation.isPending ? '发送中...' : smsSent && selectedPhoneId === phone.id ? `重新发送到 ${phone.numberWithDialCode}` : `发送短信到 ${phone.numberWithDialCode}`}
                </button>
              ))
            ) : (
              <button
                type="button"
                onClick={() => smsMutation.mutate(1)}
                disabled={smsMutation.isPending}
                className="w-full flex items-center justify-center gap-2 py-2.5 text-sm text-apple-blue hover:text-blue-700 transition-colors disabled:opacity-50"
              >
                <MessageSquare className="w-4 h-4" />
                {smsMutation.isPending ? '发送中...' : smsSent ? '重新发送短信验证码' : '使用短信验证码'}
              </button>
            )}
          </div>
        </form>
      </div>
    </Modal>
  )
}
