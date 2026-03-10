import { Search } from 'lucide-react'
import { cn } from '@/lib/cn'

export type FilterStatus = 'all' | 'active' | 'inactive'

interface SearchBarProps {
  query: string
  onQueryChange: (q: string) => void
  filter: FilterStatus
  onFilterChange: (f: FilterStatus) => void
  total: number
  filtered: number
}

const filters: { value: FilterStatus; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'active', label: '启用' },
  { value: 'inactive', label: '停用' },
]

export default function SearchBar({
  query,
  onQueryChange,
  filter,
  onFilterChange,
  total,
  filtered,
}: SearchBarProps) {
  return (
    <div className="flex flex-col sm:flex-row gap-3 items-stretch sm:items-center">
      {/* Search input */}
      <div className="relative flex-1">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
        <input
          type="text"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          placeholder="搜索邮箱地址或标签..."
          className="w-full pl-9 pr-4 py-2.5 text-sm border border-gray-200 rounded-xl bg-white focus:outline-none focus:ring-2 focus:ring-apple-blue/20 focus:border-apple-blue transition-all placeholder:text-gray-400"
        />
      </div>

      {/* Filter tabs */}
      <div className="flex bg-gray-100 rounded-lg p-0.5 self-start">
        {filters.map((f) => (
          <button
            key={f.value}
            onClick={() => onFilterChange(f.value)}
            className={cn(
              'px-3 py-1.5 text-xs font-medium rounded-md transition-all',
              filter === f.value
                ? 'bg-white text-gray-900 shadow-sm'
                : 'text-gray-500 hover:text-gray-700',
            )}
          >
            {f.label}
          </button>
        ))}
      </div>

      {/* Count */}
      {(query || filter !== 'all') && (
        <span className="text-xs text-gray-400 whitespace-nowrap self-center">
          {filtered}/{total}
        </span>
      )}
    </div>
  )
}
