import { Plus, Layers, ClipboardCopy, Download } from 'lucide-react'
import Button from '@/components/ui/Button'

interface StatsBarProps {
  total: number | undefined
  isCreating: boolean
  onCreate: () => void
  onBatchCreate: () => void
  onCopyAll: () => void
  onExport: () => void
}

export default function StatsBar({
  total,
  isCreating,
  onCreate,
  onBatchCreate,
  onCopyAll,
  onExport,
}: StatsBarProps) {
  const hasEmails = total !== undefined && total > 0

  return (
    <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      {/* Stats */}
      <div className="bg-white rounded-xl px-5 py-3 shadow-card border border-gray-100/80">
        <p className="text-[11px] text-gray-500 uppercase tracking-wider font-semibold">总数量</p>
        <p className="text-2xl font-bold text-gray-900 tabular-nums tracking-tight">
          {total ?? '—'}
        </p>
      </div>

      {/* Actions */}
      <div className="flex flex-wrap gap-2">
        {hasEmails && (
          <>
            <Button
              variant="ghost"
              size="sm"
              onClick={onCopyAll}
              icon={<ClipboardCopy className="w-4 h-4" />}
            >
              复制全部
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={onExport}
              icon={<Download className="w-4 h-4" />}
            >
              导出
            </Button>
          </>
        )}
        <Button
          variant="secondary"
          onClick={onBatchCreate}
          icon={<Layers className="w-4 h-4" />}
        >
          批量创建
        </Button>
        <Button
          onClick={onCreate}
          loading={isCreating}
          icon={<Plus className="w-4 h-4" />}
        >
          创建邮箱
        </Button>
      </div>
    </div>
  )
}
