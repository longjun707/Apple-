import { useState, useMemo, useCallback } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, getErrorMessage } from '@/api/client'
import { toast } from '@/stores/toastStore'
import Modal from '@/components/ui/Modal'
import Button from '@/components/ui/Button'
import { Upload, FileText, CheckCircle2, XCircle, AlertTriangle } from 'lucide-react'

interface BatchImportModalProps {
  open: boolean
  onClose: () => void
  onSuccess: () => void
}

interface ParsedAccount {
  appleId: string
  password: string
  remark?: string
}

interface ParsedAccountResult {
  accounts: ParsedAccount[]
  duplicateCount: number
  invalidCount: number
}

/** Parse lines of text into account objects. Supports separators: ----, :, tab, space */
function parseAccounts(text: string): ParsedAccountResult {
  const lines = text
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith('#') && !l.startsWith('//'))

  const results: ParsedAccount[] = []
  const seen = new Set<string>()
  let duplicateCount = 0
  let invalidCount = 0

  for (const line of lines) {
    let parts: string[] = []

    if (line.includes('----')) {
      parts = line.split('----').map((s) => s.trim())
    } else if (line.includes('\t')) {
      parts = line.split('\t').map((s) => s.trim())
    } else if (line.includes(':')) {
      // Be careful: emails contain @, not colons typically, but apple_id:password is common
      // Only split on first colon to handle passwords with colons
      const idx = line.indexOf(':')
      parts = [line.slice(0, idx).trim(), line.slice(idx + 1).trim()]
    } else if (line.includes(' ')) {
      // Split on first space only
      const idx = line.indexOf(' ')
      parts = [line.slice(0, idx).trim(), line.slice(idx + 1).trim()]
    } else {
      invalidCount++
      continue
    }

    const appleId = parts[0]?.toLowerCase()
    const password = parts[1]
    if (!appleId || !password) {
      invalidCount++
      continue
    }
    if (seen.has(appleId)) {
      duplicateCount++
      continue
    }

    seen.add(appleId)
    results.push({
      appleId,
      password,
      remark: parts[2] || undefined,
    })
  }
  return {
    accounts: results,
    duplicateCount,
    invalidCount,
  }
}

export default function BatchImportModal({ open, onClose, onSuccess }: BatchImportModalProps) {
  const [text, setText] = useState('')
  const [result, setResult] = useState<{ created: number; skipped: number; errors: string[] } | null>(null)

  const parsed = useMemo(() => parseAccounts(text), [text])

  const importMutation = useMutation({
    mutationFn: () => api.batchCreateAccounts(parsed.accounts),
    onSuccess: (res) => {
      if (res.success && res.data) {
        setResult(res.data)
        if (res.data.created > 0) {
          toast.success(`成功导入 ${res.data.created} 个账户`)
        }
      } else {
        toast.error(res.error || '导入失败')
      }
    },
    onError: (mutationError) => toast.error(getErrorMessage(mutationError)),
  })

  const handleClose = useCallback(() => {
    if (importMutation.isPending) return
    if (result && result.created > 0) {
      onSuccess()
    }
    setText('')
    setResult(null)
    importMutation.reset()
    onClose()
  }, [importMutation, result, onClose, onSuccess])

  const handleImport = () => {
    if (parsed.accounts.length === 0) return
    setResult(null)
    importMutation.mutate()
  }

  return (
    <Modal open={open} onClose={handleClose} disableClose={importMutation.isPending} className="max-w-lg">
      <div className="p-6">
        <div className="flex items-center gap-3 mb-5">
          <div className="w-10 h-10 rounded-full bg-purple-50 flex items-center justify-center">
            <Upload className="w-5 h-5 text-purple-600" />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-gray-900">批量导入账户</h2>
            <p className="text-xs text-gray-500">每行一个账户，支持多种格式</p>
          </div>
        </div>

        {!result ? (
          <>
            {/* Format hints */}
            <div className="mb-4 p-3 bg-gray-50 rounded-xl text-xs text-gray-500 space-y-1">
              <div className="flex items-center gap-1.5 font-medium text-gray-600">
                <FileText className="w-3.5 h-3.5" /> 支持格式（每行一个）
              </div>
              <code className="block text-gray-400">apple_id----password</code>
              <code className="block text-gray-400">apple_id:password</code>
              <code className="block text-gray-400">apple_id{'\t'}password</code>
              <code className="block text-gray-400">apple_id password</code>
              <p className="text-gray-400 pt-1"># 开头的行会被忽略。重复的 Apple ID 自动去重。</p>
            </div>

            {/* Textarea */}
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={10}
              className="w-full px-3 py-2.5 text-sm border border-gray-200 rounded-xl bg-white focus:outline-none focus:ring-2 focus:ring-apple-blue/20 focus:border-apple-blue transition-all placeholder:text-gray-400 font-mono resize-none"
              placeholder={"example@icloud.com----MyP@ssw0rd\nuser2@icloud.com:password123\n# 这是注释行"}
            />

            {/* Preview */}
            {text.trim() && (
              <div className="mt-3 space-y-1 text-sm">
                {parsed.accounts.length > 0 ? (
                  <span className="text-green-600 font-medium flex items-center gap-1">
                    <CheckCircle2 className="w-4 h-4" /> 识别到 {parsed.accounts.length} 个账户
                  </span>
                ) : (
                  <span className="text-yellow-600 font-medium flex items-center gap-1">
                    <AlertTriangle className="w-4 h-4" /> 未识别到有效账户
                  </span>
                )}
                {(parsed.duplicateCount > 0 || parsed.invalidCount > 0) && (
                  <div className="text-xs text-gray-500">
                    {parsed.duplicateCount > 0 && <span>重复跳过 {parsed.duplicateCount} 行</span>}
                    {parsed.duplicateCount > 0 && parsed.invalidCount > 0 && <span> · </span>}
                    {parsed.invalidCount > 0 && <span>格式无效 {parsed.invalidCount} 行</span>}
                  </div>
                )}
              </div>
            )}

            {/* Actions */}
            <div className="flex gap-3 pt-4">
              <Button
                type="button"
                variant="secondary"
                onClick={handleClose}
                disabled={importMutation.isPending}
                className="flex-1"
              >
                取消
              </Button>
              <Button
                onClick={handleImport}
                loading={importMutation.isPending}
                disabled={parsed.accounts.length === 0}
                className="flex-1"
              >
                导入 {parsed.accounts.length > 0 && `(${parsed.accounts.length})`}
              </Button>
            </div>
          </>
        ) : (
          <>
            {/* Result */}
            <div className="space-y-4">
              <div className="grid grid-cols-3 gap-3">
                <div className="bg-green-50 rounded-xl p-4 text-center">
                  <div className="text-2xl font-bold text-green-600">{result.created}</div>
                  <div className="text-xs text-green-600 font-medium mt-1">成功导入</div>
                </div>
                <div className="bg-yellow-50 rounded-xl p-4 text-center">
                  <div className="text-2xl font-bold text-yellow-600">{result.skipped}</div>
                  <div className="text-xs text-yellow-600 font-medium mt-1">已存在跳过</div>
                </div>
                <div className="bg-red-50 rounded-xl p-4 text-center">
                  <div className="text-2xl font-bold text-red-500">{result.errors.length}</div>
                  <div className="text-xs text-red-500 font-medium mt-1">失败</div>
                </div>
              </div>

              {result.errors.length > 0 && (
                <div className="bg-red-50 rounded-xl p-3 max-h-32 overflow-y-auto">
                  <div className="flex items-center gap-1.5 text-xs font-medium text-red-600 mb-1.5">
                    <XCircle className="w-3.5 h-3.5" /> 错误详情
                  </div>
                  {result.errors.map((err, i) => (
                    <div key={i} className="text-xs text-red-500 py-0.5 font-mono">{err}</div>
                  ))}
                </div>
              )}

              <Button onClick={handleClose} className="w-full">
                完成
              </Button>
            </div>
          </>
        )}
      </div>
    </Modal>
  )
}
