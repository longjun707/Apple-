import { memo, useState } from 'react'
import { Mail, Copy, Check, Trash2 } from 'lucide-react'
import type { HMEEmail } from '@/api/client'
import { cn } from '@/lib/cn'

interface EmailItemProps {
  email: HMEEmail
  onCopy: (email: string) => void
  onDelete?: (email: HMEEmail) => void
}

export default memo(function EmailItem({ email, onCopy, onDelete }: EmailItemProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    onCopy(email.emailAddress)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="px-5 py-3.5 hover:bg-blue-50/30 transition-colors flex items-center justify-between group">
      <div className="flex items-center gap-3 min-w-0 flex-1">
        <div
          className={cn(
            'w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0',
            email.active ? 'bg-green-50 text-green-600' : 'bg-gray-100 text-gray-400',
          )}
        >
          <Mail className="w-4 h-4" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="font-medium text-gray-900 truncate text-sm">{email.emailAddress}</p>
          <p className="text-xs text-gray-500 truncate">
            {email.label || '无标签'}
            {email.forwardToEmail && <span className="text-gray-400"> · {email.forwardToEmail}</span>}
          </p>
        </div>
      </div>

      {/* Always visible on mobile, hover on desktop */}
      <div className="flex items-center gap-1 sm:opacity-0 sm:group-hover:opacity-100 transition-opacity ml-2">
        <button
          onClick={handleCopy}
          className={cn(
            'p-2 rounded-lg transition-colors',
            copied
              ? 'text-green-500 bg-green-50'
              : 'text-gray-400 hover:text-apple-blue hover:bg-apple-blue/10',
          )}
          title="复制邮箱"
        >
          {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
        </button>
        {onDelete && (
          <button
            onClick={() => onDelete(email)}
            className="p-2 text-gray-400 hover:text-red-500 hover:bg-red-50 rounded-lg transition-colors"
            title="删除"
          >
            <Trash2 className="w-4 h-4" />
          </button>
        )}
      </div>
    </div>
  )
})
