import {
  Shield,
  Users,
  Mail,
  Settings,
  LogOut,
  LayoutDashboard,
  PanelLeftClose,
  PanelLeftOpen,
  Timer,
} from 'lucide-react'
import { cn } from '@/lib/cn'

interface MenuItem {
  id: string
  label: string
  icon: React.ReactNode
}

interface SidebarProps {
  currentPage: string
  onPageChange: (page: string) => void
  onLogout: () => void
  adminName?: string
  collapsed?: boolean
  onToggleCollapse?: () => void
}

const menuItems: MenuItem[] = [
  { id: 'dashboard', label: '仪表盘', icon: <LayoutDashboard className="w-[18px] h-[18px]" /> },
  { id: 'accounts', label: 'Apple 账户', icon: <Users className="w-[18px] h-[18px]" /> },
  { id: 'emails', label: 'HME 邮箱', icon: <Mail className="w-[18px] h-[18px]" /> },
  { id: 'auto-task', label: '自动任务', icon: <Timer className="w-[18px] h-[18px]" /> },
  { id: 'settings', label: '系统设置', icon: <Settings className="w-[18px] h-[18px]" /> },
]

export default function Sidebar({
  currentPage,
  onPageChange,
  onLogout,
  adminName = 'Admin',
  collapsed = false,
  onToggleCollapse,
}: SidebarProps) {
  return (
    <aside
      className={cn(
        'fixed left-0 top-0 h-full bg-sidebar text-white transition-all duration-300 z-50 flex flex-col',
        collapsed ? 'w-[68px]' : 'w-[260px]'
      )}
    >
      {/* Logo */}
      <div className={cn('h-16 flex items-center border-b border-sidebar-border', collapsed ? 'px-3 justify-center' : 'px-5')}>
        <div className="flex items-center gap-3">
          <div className="w-9 h-9 bg-gradient-to-br from-blue-500 to-indigo-600 rounded-xl flex items-center justify-center flex-shrink-0 shadow-lg shadow-blue-500/20">
            <Shield className="w-5 h-5" />
          </div>
          {!collapsed && <span className="font-bold text-[15px] tracking-tight">HME Manager</span>}
        </div>
      </div>

      {/* Toggle */}
      <div className={cn('px-3 pt-3', collapsed && 'flex justify-center')}>
        <button
          onClick={onToggleCollapse}
          title={collapsed ? '展开侧栏' : '收起侧栏'}
          className="p-2 rounded-lg text-gray-500 hover:text-gray-300 hover:bg-sidebar-hover transition-colors"
        >
          {collapsed ? <PanelLeftOpen className="w-4 h-4" /> : <PanelLeftClose className="w-4 h-4" />}
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex-1 py-2 px-3 overflow-y-auto">
        <div className="space-y-1">
          {menuItems.map((item) => {
            const isActive = currentPage === item.id
            return (
              <button
                key={item.id}
                onClick={() => onPageChange(item.id)}
                title={collapsed ? item.label : undefined}
                className={cn(
                  'w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-[13px] font-medium transition-all duration-150 relative',
                  isActive
                    ? 'bg-white/10 text-white shadow-sm'
                    : 'text-gray-400 hover:bg-white/5 hover:text-gray-200',
                  collapsed && 'justify-center px-0'
                )}
              >
                {isActive && (
                  <div className="absolute left-0 top-1/2 -translate-y-1/2 w-[3px] h-5 bg-apple-blue rounded-r-full" />
                )}
                <span className={cn(collapsed ? 'ml-0' : 'ml-1')}>{item.icon}</span>
                {!collapsed && <span>{item.label}</span>}
              </button>
            )
          })}
        </div>
      </nav>

      {/* User & Logout */}
      <div className="border-t border-sidebar-border p-3">
        {!collapsed && (
          <div className="flex items-center gap-3 mb-2 px-2 py-2">
            <div className="w-8 h-8 bg-gradient-to-br from-gray-600 to-gray-700 rounded-full flex items-center justify-center ring-2 ring-white/10">
              <span className="text-xs font-semibold">
                {adminName.charAt(0).toUpperCase()}
              </span>
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium truncate">{adminName}</p>
              <p className="text-[11px] text-gray-500">管理员</p>
            </div>
          </div>
        )}
        <button
          onClick={onLogout}
          title={collapsed ? '退出登录' : undefined}
          className={cn(
            'w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-[13px] font-medium text-gray-400 hover:bg-red-500/10 hover:text-red-400 transition-all',
            collapsed && 'justify-center'
          )}
        >
          <LogOut className="w-[18px] h-[18px]" />
          {!collapsed && <span>退出登录</span>}
        </button>
      </div>
    </aside>
  )
}
