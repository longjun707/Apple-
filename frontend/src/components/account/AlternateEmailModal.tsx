import { useState, useRef, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api } from '@/api/client'
import { toast } from '@/stores/toastStore'
import Modal from '@/components/ui/Modal'
import Button from '@/components/ui/Button'
import { Mail, ArrowRight, CheckCircle } from 'lucide-react'

interface AlternateEmailModalProps {
  open: boolean
  accountId: number | null
  onClose: () => void
  onSuccess: () => void
}

type Step = 'input' | 'verify' | 'success'

export default function AlternateEmailModal({ open, accountId, onClose, onSuccess }: AlternateEmailModalProps) {
  const [step, setStep] = useState<Step>('input')
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')
  const [verificationId, setVerificationId] = useState('')
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const codeRef = useRef<HTMLInputElement>(null)

  // Reset state when modal opens/closes
  useEffect(() => {
    if (open) {
      setStep('input')
      setEmail('')
      setCode('')
      setVerificationId('')
      setError('')
      setTimeout(() => inputRef.current?.focus(), 100)
    }
  }, [open])

  // Focus code input when moving to verify step
  useEffect(() => {
    if (step === 'verify') {
      setTimeout(() => codeRef.current?.focus(), 100)
    }
  }, [step])

  // Send verification code
  const sendMutation = useMutation({
    mutationFn: () => api.sendAlternateEmailVerification(accountId!, email),
    onSuccess: (res) => {
      if (res.success && res.data) {
        setVerificationId(res.data.verificationId)
        setStep('verify')
        setError('')
        toast.success(`验证码已发送到 ${email}`)
      } else {
        setError(res.error || '发送验证码失败')
      }
    },
    onError: () => setError('网络错误，请重试'),
  })

  // Verify code
  const verifyMutation = useMutation({
    mutationFn: () => api.verifyAlternateEmail(accountId!, email, verificationId, code),
    onSuccess: (res) => {
      if (res.success) {
        setStep('success')
        setError('')
        toast.success('备用邮箱添加成功')
        // Auto close after 1.5s
        setTimeout(() => {
          onSuccess()
          onClose()
        }, 1500)
      } else {
        setError(res.error || '验证码错误')
      }
    },
    onError: () => setError('验证失败，请重试'),
  })

  const handleSendCode = (e: React.FormEvent) => {
    e.preventDefault()
    if (!accountId || !email) return
    setError('')
    sendMutation.mutate()
  }

  const handleVerifyCode = (e: React.FormEvent) => {
    e.preventDefault()
    if (!accountId || code.length !== 6) return
    setError('')
    verifyMutation.mutate()
  }

  const handleResend = () => {
    setError('')
    sendMutation.mutate()
  }

  return (
    <Modal open={open} onClose={onClose}>
      <div className="p-6">
        {/* Header */}
        <div className="flex justify-center mb-5">
          <div className={`w-16 h-16 rounded-full flex items-center justify-center ${
            step === 'success' ? 'bg-emerald-100' : 'bg-blue-50'
          }`}>
            {step === 'success' ? (
              <CheckCircle className="w-8 h-8 text-emerald-500" />
            ) : (
              <Mail className="w-8 h-8 text-apple-blue" />
            )}
          </div>
        </div>

        {/* Step: Input Email */}
        {step === 'input' && (
          <>
            <h2 className="text-lg font-semibold text-center text-gray-900 mb-1">
              添加备用邮箱
            </h2>
            <p className="text-sm text-center text-gray-500 mb-5">
              验证码将发送至此电子邮件地址
            </p>

            <form onSubmit={handleSendCode} className="space-y-4">
              <input
                ref={inputRef}
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className="input"
                placeholder="name@example.com"
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
                  disabled={!email || !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)}
                  loading={sendMutation.isPending}
                  className="flex-1"
                  icon={<ArrowRight className="w-4 h-4" />}
                >
                  继续
                </Button>
              </div>
            </form>
          </>
        )}

        {/* Step: Verify Code */}
        {step === 'verify' && (
          <>
            <h2 className="text-lg font-semibold text-center text-gray-900 mb-1">
              输入验证码
            </h2>
            <p className="text-sm text-center text-gray-500 mb-5">
              已发送到 <span className="font-medium text-gray-700">{email}</span>
            </p>

            <form onSubmit={handleVerifyCode} className="space-y-4">
              <input
                ref={codeRef}
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
                <Button type="button" variant="secondary" onClick={() => setStep('input')} className="flex-1">
                  返回
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

              <button
                type="button"
                onClick={handleResend}
                disabled={sendMutation.isPending}
                className="w-full text-sm text-apple-blue hover:text-blue-700 transition-colors disabled:opacity-50 py-2"
              >
                {sendMutation.isPending ? '发送中...' : '重新发送验证码'}
              </button>
            </form>
          </>
        )}

        {/* Step: Success */}
        {step === 'success' && (
          <>
            <h2 className="text-lg font-semibold text-center text-gray-900 mb-1">
              添加成功
            </h2>
            <p className="text-sm text-center text-gray-500">
              <span className="font-medium text-gray-700">{email}</span> 已添加为备用邮箱
            </p>
          </>
        )}
      </div>
    </Modal>
  )
}
