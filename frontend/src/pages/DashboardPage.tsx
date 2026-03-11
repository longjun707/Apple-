import { useQuery } from '@tanstack/react-query'
import { api, getErrorMessage, unwrapResponse } from '@/api/client'
import { Users, Mail, AlertCircle, CheckCircle, Activity, Plus, Layers, Download, ArrowRight, Loader2 } from 'lucide-react'

interface DashboardPageProps {
  onNavigate?: (page: string) => void
}

export default function DashboardPage({ onNavigate }: DashboardPageProps) {
  const {
    data: statsData,
    isLoading: statsLoading,
    isError: statsQueryError,
    error: statsError,
  } = useQuery({
    queryKey: ['admin-stats'],
    queryFn: async () => unwrapResponse(await api.getStats(), '获取统计信息失败'),
  })

  const {
    data: accountsData,
    isLoading: accountsLoading,
    isError: accountsQueryError,
    error: accountsError,
  } = useQuery({
    queryKey: ['accounts', 1],
    queryFn: async () => unwrapResponse(await api.listAccounts(1, 5), '获取最近账户失败'),
  })

  const isLoading = statsLoading || accountsLoading
  const accounts = accountsData?.list || []

  const s = statsData
  const stats = [
    {
      label: '账户总数',
      value: s?.totalAccounts ?? 0,
      icon: <Users className="w-6 h-6" />,
      color: 'bg-blue-500',
    },
    {
      label: '正常账户',
      value: s?.activeAccounts ?? 0,
      icon: <CheckCircle className="w-6 h-6" />,
      color: 'bg-green-500',
    },
    {
      label: '异常账户',
      value: s?.errorAccounts ?? 0,
      icon: <AlertCircle className="w-6 h-6" />,
      color: 'bg-red-500',
    },
    {
      label: 'HME 总数',
      value: s?.totalHME ?? 0,
      icon: <Mail className="w-6 h-6" />,
      color: 'bg-purple-500',
    },
  ]

  const quickActions = [
    {
      icon: <Plus className="w-5 h-5" />,
      title: '添加新账户',
      desc: '添加 Apple ID 到系统',
      color: 'text-blue-600 bg-blue-50',
      onClick: () => onNavigate?.('accounts'),
    },
    {
      icon: <Layers className="w-5 h-5" />,
      title: '批量创建 HME',
      desc: '为账户批量生成隐藏邮箱',
      color: 'text-purple-600 bg-purple-50',
      onClick: () => onNavigate?.('accounts'),
    },
    {
      icon: <Download className="w-5 h-5" />,
      title: '导出数据',
      desc: '导出 HME 列表到 CSV',
      color: 'text-green-600 bg-green-50',
      onClick: () => onNavigate?.('emails'),
    },
  ]

  return (
    <div className="animate-fade-in">
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-gray-900 tracking-tight">仪表盘</h1>
        <p className="text-sm text-gray-500 mt-1">系统概览与快速操作</p>
      </div>

      {(statsQueryError || accountsQueryError) && (
        <div className="mb-5 rounded-2xl border border-yellow-200 bg-yellow-50 px-4 py-3 text-sm text-yellow-700">
          {statsQueryError && <div>统计数据加载失败：{getErrorMessage(statsError, '获取统计信息失败')}</div>}
          {accountsQueryError && <div>最近账户加载失败：{getErrorMessage(accountsError, '获取最近账户失败')}</div>}
        </div>
      )}

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5 mb-8">
        {stats.map((stat, index) => (
          <div
            key={index}
            className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-5 flex items-center gap-4 hover:shadow-card-hover transition-all duration-200 group"
          >
            <div className={`${stat.color} text-white p-3 rounded-xl shadow-sm group-hover:scale-105 transition-transform duration-200`}>
              {stat.icon}
            </div>
            <div>
              {isLoading ? (
                <Loader2 className="w-6 h-6 animate-spin text-gray-300" />
              ) : (
                <p className="text-2xl font-bold text-gray-900 tabular-nums tracking-tight">{stat.value}</p>
              )}
              <p className="text-[13px] text-gray-500">{stat.label}</p>
            </div>
          </div>
        ))}
      </div>

      {/* Content Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
        {/* Recent Accounts */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-6">
          <div className="flex items-center justify-between mb-5">
            <div className="flex items-center gap-2.5">
              <div className="w-8 h-8 bg-blue-50 rounded-lg flex items-center justify-center">
                <Activity className="w-4 h-4 text-blue-600" />
              </div>
              <h2 className="text-[15px] font-semibold text-gray-900">最近账户</h2>
            </div>
            <button
              onClick={() => onNavigate?.('accounts')}
              className="text-[13px] text-apple-blue hover:text-blue-700 font-medium flex items-center gap-1 transition-colors"
            >
              查看全部 <ArrowRight className="w-3.5 h-3.5" />
            </button>
          </div>
          {isLoading ? (
            <div className="flex justify-center py-8">
              <Loader2 className="w-6 h-6 animate-spin text-gray-300" />
            </div>
          ) : accountsQueryError ? (
            <div className="text-center py-10 text-sm text-red-500">
              {getErrorMessage(accountsError, '获取最近账户失败')}
            </div>
          ) : accounts.length === 0 ? (
            <div className="text-center py-10">
              <div className="w-12 h-12 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-3">
                <Users className="w-6 h-6 text-gray-300" />
              </div>
              <p className="text-gray-500 font-medium text-sm">暂无账户</p>
              <button
                onClick={() => onNavigate?.('accounts')}
                className="mt-2 text-[13px] text-apple-blue hover:text-blue-700 font-medium"
              >
                添加第一个账户
              </button>
            </div>
          ) : (
            <div className="space-y-0.5">
              {accounts.slice(0, 5).map((account) => (
                <div
                  key={account.id}
                  className="flex items-center justify-between py-2.5 px-3 -mx-1 rounded-xl hover:bg-gray-50/80 transition-colors cursor-pointer group"
                  onClick={() => onNavigate?.('accounts')}
                >
                  <div className="flex items-center gap-3">
                    <div className={`w-2 h-2 rounded-full ${account.status === 1 ? 'bg-emerald-500' : 'bg-red-500'}`} />
                    <span className="text-sm font-medium text-gray-900 truncate max-w-[200px] group-hover:text-apple-blue transition-colors">
                      {account.appleId}
                    </span>
                  </div>
                  <span className="text-[13px] text-gray-400 tabular-nums">{account.hmeCount} HME</span>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Quick Actions */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-6">
          <div className="flex items-center gap-2.5 mb-5">
            <div className="w-8 h-8 bg-purple-50 rounded-lg flex items-center justify-center">
              <Mail className="w-4 h-4 text-purple-600" />
            </div>
            <h2 className="text-[15px] font-semibold text-gray-900">快速操作</h2>
          </div>
          <div className="space-y-2.5">
            {quickActions.map((action, index) => (
              <button
                key={index}
                onClick={action.onClick}
                className="w-full flex items-center gap-4 px-4 py-3.5 bg-gray-50/80 hover:bg-gray-100/80 rounded-xl transition-all group border border-transparent hover:border-gray-100"
              >
                <div className={`p-2.5 rounded-xl ${action.color}`}>
                  {action.icon}
                </div>
                <div className="flex-1 text-left">
                  <span className="text-sm font-medium text-gray-900">{action.title}</span>
                  <p className="text-[13px] text-gray-500">{action.desc}</p>
                </div>
                <ArrowRight className="w-4 h-4 text-gray-300 group-hover:text-gray-500 group-hover:translate-x-0.5 transition-all" />
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
