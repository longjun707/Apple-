import { Mail, SearchX } from 'lucide-react'

interface EmptyStateProps {
  isFiltered: boolean
}

export default function EmptyState({ isFiltered }: EmptyStateProps) {
  return (
    <div className="py-16 text-center">
      {isFiltered ? (
        <>
          <SearchX className="w-12 h-12 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 font-medium">没有匹配的邮箱</p>
          <p className="text-sm text-gray-400 mt-1">试试修改搜索条件</p>
        </>
      ) : (
        <>
          <Mail className="w-12 h-12 text-gray-300 mx-auto mb-3" />
          <p className="text-gray-500 font-medium">暂无隐藏邮箱</p>
          <p className="text-sm text-gray-400 mt-1">点击「创建邮箱」开始</p>
        </>
      )}
    </div>
  )
}
