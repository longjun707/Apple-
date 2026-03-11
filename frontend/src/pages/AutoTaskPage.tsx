import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, getErrorMessage, unwrapResponse, type PhoneNumber } from '@/api/client'
import { toast } from '@/stores/toastStore'
import {
  Loader2,
  Play,
  Pause,
  RefreshCw,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Info,
  Settings,
  Zap,
  Users,
  Phone,
} from 'lucide-react'
import Button from '@/components/ui/Button'


interface EligibleAccount {
  appleId: string
  hmeCount: number
  phoneNumbers: string // JSON string of PhoneNumber[]
}

interface AutoHMEStatus {
  enabled: boolean
  running: boolean
  intervalMinutes: number
  countPerAccount: number
  lastRunTime: string | null
  nextRunTime: string | null
  currentAccount: string
  currentAccountIndex: number
  currentProgress: number
  totalAccounts: number
  processedAccounts: number
  totalCreated: number
  totalFailed: number
  eligibleAccounts: EligibleAccount[]
}

interface LogEntry {
  time: string
  level: 'info' | 'success' | 'error' | 'warning'
  message: string
}

export default function AutoTaskPage() {
  const queryClient = useQueryClient()
  const [showSettings, setShowSettings] = useState(false)
  const [intervalMinutes, setIntervalMinutes] = useState(30)
  const [countPerAccount, setCountPerAccount] = useState(20)

  // Fetch status
  const {
    data: statusData,
    isError: statusQueryError,
    error: statusError,
  } = useQuery({
    queryKey: ['auto-hme-status'],
    queryFn: async () => {
      return unwrapResponse(await api.getAutoHMEStatus(), '获取自动任务状态失败') as AutoHMEStatus
    },
    refetchInterval: 3000, // Poll every 3 seconds
  })

  // Fetch logs
  const {
    data: logsData,
    isLoading: logsLoading,
    isError: logsQueryError,
    error: logsError,
  } = useQuery({
    queryKey: ['auto-hme-logs'],
    queryFn: async () => {
      return unwrapResponse(await api.getAutoHMELogs(), '获取自动任务日志失败') as LogEntry[]
    },
    refetchInterval: 5000, // Poll every 5 seconds
  })

  // Toggle enabled
  const toggleMutation = useMutation({
    mutationFn: (enabled: boolean) => api.updateAutoHMESettings({ enabled }),
    onSuccess: (res) => {
      if (res.success) {
        queryClient.invalidateQueries({ queryKey: ['auto-hme-status'] })
        toast.success(statusData?.enabled ? '已禁用自动任务' : '已启用自动任务')
      } else {
        toast.error(res.error || '操作失败')
      }
    },
    onError: (mutationError) => toast.error(getErrorMessage(mutationError)),
  })

  // Update settings
  const updateSettingsMutation = useMutation({
    mutationFn: () => api.updateAutoHMESettings({ intervalMinutes, countPerAccount }),
    onSuccess: (res) => {
      if (res.success) {
        queryClient.invalidateQueries({ queryKey: ['auto-hme-status'] })
        toast.success('设置已保存')
        setShowSettings(false)
      } else {
        toast.error(res.error || '保存失败')
      }
    },
    onError: (mutationError) => toast.error(getErrorMessage(mutationError)),
  })

  // Trigger manually
  const triggerMutation = useMutation({
    mutationFn: () => api.triggerAutoHME(),
    onSuccess: (res) => {
      if (res.success) {
        queryClient.invalidateQueries({ queryKey: ['auto-hme-status'] })
        toast.success('任务已触发')
      } else {
        toast.error(res.error || '触发失败')
      }
    },
    onError: (mutationError) => toast.error(getErrorMessage(mutationError)),
  })

  const formatTime = (timeStr: string | null) => {
    if (!timeStr) return '-'
    const date = new Date(timeStr)
    return date.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  }

  const getLevelIcon = (level: string) => {
    switch (level) {
      case 'success':
        return <CheckCircle2 className="w-4 h-4 text-green-500" />
      case 'error':
        return <XCircle className="w-4 h-4 text-red-500" />
      case 'warning':
        return <AlertCircle className="w-4 h-4 text-yellow-500" />
      default:
        return <Info className="w-4 h-4 text-blue-500" />
    }
  }

  const getLevelBg = (level: string) => {
    switch (level) {
      case 'success':
        return 'bg-green-50'
      case 'error':
        return 'bg-red-50'
      case 'warning':
        return 'bg-yellow-50'
      default:
        return 'bg-blue-50'
    }
  }

  const currentAccountIndex =
    statusData?.running && statusData.currentAccount
      ? statusData.currentAccountIndex || Math.min(statusData.processedAccounts + 1, statusData.totalAccounts)
      : 0

  const overallProgressPercent =
    statusData && statusData.totalAccounts > 0
      ? Math.min(
          100,
          ((statusData.processedAccounts +
            (statusData.running && statusData.countPerAccount > 0
              ? statusData.currentProgress / statusData.countPerAccount
              : 0)) /
            statusData.totalAccounts) *
            100
        )
      : 0

  return (
    <div className="animate-fade-in">
      {/* Header */}
      <div className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 tracking-tight">自动任务</h1>
          <p className="text-sm text-gray-500 mt-1">自动创建 HME 邮箱定时任务管理</p>
        </div>
        <div className="flex items-center gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setIntervalMinutes(statusData?.intervalMinutes || 30)
              setCountPerAccount(statusData?.countPerAccount || 20)
              setShowSettings(!showSettings)
            }}
            icon={<Settings className="w-4 h-4" />}
          >
            设置
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              queryClient.invalidateQueries({ queryKey: ['auto-hme-status'] })
              queryClient.invalidateQueries({ queryKey: ['auto-hme-logs'] })
            }}
            icon={<RefreshCw className="w-4 h-4" />}
          >
            刷新
          </Button>
        </div>
      </div>

      {(statusQueryError || logsQueryError) && (
        <div className="mb-6 rounded-2xl border border-yellow-200 bg-yellow-50 px-4 py-3 text-sm text-yellow-700">
          {statusQueryError && <div>任务状态加载失败：{getErrorMessage(statusError, '获取自动任务状态失败')}</div>}
          {logsQueryError && <div>任务日志加载失败：{getErrorMessage(logsError, '获取自动任务日志失败')}</div>}
        </div>
      )}

      {/* Settings Panel */}
      {showSettings && (
        <div className="mb-6 bg-white rounded-2xl shadow-card border border-gray-100/80 p-5">
          <h3 className="text-sm font-semibold text-gray-900 mb-4">任务设置</h3>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1.5">
                执行间隔（分钟）
              </label>
              <input
                type="number"
                min={5}
                max={1440}
                value={intervalMinutes}
                onChange={(e) => setIntervalMinutes(Number(e.target.value))}
                className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-apple-blue/20 focus:border-apple-blue"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-gray-600 mb-1.5">
                每账户创建数量
              </label>
              <input
                type="number"
                min={1}
                max={100}
                value={countPerAccount}
                onChange={(e) => setCountPerAccount(Number(e.target.value))}
                className="w-full px-3 py-2 border border-gray-200 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-apple-blue/20 focus:border-apple-blue"
              />
            </div>
          </div>
          <div className="mt-4 flex justify-end gap-2">
            <Button variant="outline" size="sm" onClick={() => setShowSettings(false)}>
              取消
            </Button>
            <Button
              size="sm"
              onClick={() => updateSettingsMutation.mutate()}
              loading={updateSettingsMutation.isPending}
            >
              保存设置
            </Button>
          </div>
        </div>
      )}

      {/* Status Cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        {/* Enable/Disable Card */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-5">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-xs font-medium text-gray-500 mb-1">任务状态</p>
              <p className={`text-lg font-bold ${statusData?.enabled ? 'text-green-600' : 'text-gray-400'}`}>
                {statusData?.enabled ? '已启用' : '已禁用'}
              </p>
            </div>
            <button
              onClick={() => toggleMutation.mutate(!statusData?.enabled)}
              disabled={toggleMutation.isPending}
              className={`w-12 h-12 rounded-xl flex items-center justify-center transition-all ${
                statusData?.enabled
                  ? 'bg-green-100 text-green-600 hover:bg-green-200'
                  : 'bg-gray-100 text-gray-400 hover:bg-gray-200'
              }`}
            >
              {statusData?.enabled ? <Pause className="w-5 h-5" /> : <Play className="w-5 h-5" />}
            </button>
          </div>
        </div>

        {/* Running Status */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-5">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-xs font-medium text-gray-500 mb-1">执行状态</p>
              <p className={`text-lg font-bold ${statusData?.running ? 'text-apple-blue' : 'text-gray-600'}`}>
                {statusData?.running ? '运行中' : '空闲'}
              </p>
              {statusData?.running && statusData.currentAccount && (
                <p className="text-xs text-gray-400 mt-0.5 truncate max-w-[150px]">
                  {statusData.currentAccount}
                </p>
              )}
            </div>
            {statusData?.running ? (
              <div className="w-12 h-12 rounded-xl bg-blue-100 flex items-center justify-center">
                <Loader2 className="w-5 h-5 text-apple-blue animate-spin" />
              </div>
            ) : (
              <button
                onClick={() => triggerMutation.mutate()}
                disabled={triggerMutation.isPending || statusData?.running}
                className="w-12 h-12 rounded-xl bg-apple-blue/10 text-apple-blue hover:bg-apple-blue/20 flex items-center justify-center transition-all disabled:opacity-50"
                title="立即执行"
              >
                <Zap className="w-5 h-5" />
              </button>
            )}
          </div>
        </div>

        {/* Progress */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-5">
          <p className="text-xs font-medium text-gray-500 mb-1">当前进度</p>
          {statusData?.running ? (
            <>
              <p className="text-lg font-bold text-gray-900">
                {currentAccountIndex}/{statusData.totalAccounts} 账户
              </p>
              <div className="mt-2 h-1.5 bg-gray-100 rounded-full overflow-hidden">
                <div
                  className="h-full bg-apple-blue rounded-full transition-all"
                  style={{
                    width: `${overallProgressPercent}%`,
                  }}
                />
              </div>
              <p className="text-xs text-gray-400 mt-1">
                已完成账户: {statusData.processedAccounts}/{statusData.totalAccounts}
              </p>
              <p className="text-xs text-gray-400 mt-0.5">
                当前账户进度: {statusData.currentProgress}/{statusData.countPerAccount}
              </p>
            </>
          ) : (
            <p className="text-lg font-bold text-gray-400">-</p>
          )}
        </div>

        {/* Results */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-5">
          <p className="text-xs font-medium text-gray-500 mb-1">本轮结果</p>
          <div className="flex items-baseline gap-3">
            <div>
              <span className="text-lg font-bold text-green-600">{statusData?.totalCreated || 0}</span>
              <span className="text-xs text-gray-400 ml-1">成功</span>
            </div>
            <div>
              <span className="text-lg font-bold text-red-500">{statusData?.totalFailed || 0}</span>
              <span className="text-xs text-gray-400 ml-1">失败</span>
            </div>
          </div>
        </div>
      </div>

      {/* Time Info */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-4 flex items-center gap-4">
          <div className="w-10 h-10 rounded-xl bg-gray-100 flex items-center justify-center">
            <Clock className="w-5 h-5 text-gray-500" />
          </div>
          <div>
            <p className="text-xs font-medium text-gray-500">上次执行</p>
            <p className="text-sm font-semibold text-gray-900">{formatTime(statusData?.lastRunTime || null)}</p>
          </div>
        </div>
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-4 flex items-center gap-4">
          <div className="w-10 h-10 rounded-xl bg-blue-100 flex items-center justify-center">
            <Clock className="w-5 h-5 text-apple-blue" />
          </div>
          <div>
            <p className="text-xs font-medium text-gray-500">下次执行</p>
            <p className="text-sm font-semibold text-gray-900">{formatTime(statusData?.nextRunTime || null)}</p>
          </div>
        </div>
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-4 flex items-center gap-4">
          <div className="w-10 h-10 rounded-xl bg-green-100 flex items-center justify-center">
            <Users className="w-5 h-5 text-green-600" />
          </div>
          <div>
            <p className="text-xs font-medium text-gray-500">可用账户</p>
            <p className="text-sm font-semibold text-gray-900">{statusData?.eligibleAccounts?.length || 0} 个</p>
          </div>
        </div>
      </div>

      {/* Eligible Accounts */}
      {statusData?.eligibleAccounts && statusData.eligibleAccounts.length > 0 && (
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 overflow-hidden mb-6">
          <div className="px-5 py-4 border-b border-gray-100 flex items-center justify-between">
            <h3 className="text-sm font-semibold text-gray-900">可用账户列表</h3>
            <span className="text-xs text-gray-400">已登录且 Session 有效</span>
          </div>
          <div className="max-h-[200px] overflow-y-auto">
            <div className="divide-y divide-gray-50">
              {statusData.eligibleAccounts.map((account, idx) => {
                // Parse phone numbers from JSON string
                let phones: PhoneNumber[] = []
                if (account.phoneNumbers) {
                  try {
                    phones = JSON.parse(account.phoneNumbers)
                  } catch {
                    // ignore parse errors
                  }
                }
                return (
                  <div key={idx} className="px-5 py-3 flex items-center justify-between hover:bg-gray-50">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-green-100 flex items-center justify-center">
                        <CheckCircle2 className="w-4 h-4 text-green-600" />
                      </div>
                      <div>
                        <span className="text-sm text-gray-800">{account.appleId}</span>
                        {phones.length > 0 && (
                          <div className="flex items-center gap-1 mt-0.5">
                            <Phone className="w-3 h-3 text-gray-400" />
                            <span className="text-xs text-gray-400">
                              {phones.map(p => p.fullNumberWithCountryPrefix || p.numberWithDialCode || '').join(', ')}
                            </span>
                          </div>
                        )}
                      </div>
                    </div>
                    <span className="text-xs text-gray-400">{account.hmeCount} 个 HME</span>
                  </div>
                )
              })}
            </div>
          </div>
        </div>
      )}

      {/* Logs */}
      <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 overflow-hidden">
        <div className="px-5 py-4 border-b border-gray-100 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-gray-900">执行日志</h3>
          <span className="text-xs text-gray-400">{logsData?.length || 0} 条记录</span>
        </div>
        <div className="max-h-[400px] overflow-y-auto">
          {logsLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="w-6 h-6 animate-spin text-gray-400" />
            </div>
          ) : logsData && logsData.length > 0 ? (
            <div className="divide-y divide-gray-50">
              {logsData.map((log, idx) => (
                <div key={idx} className={`px-5 py-3 flex items-start gap-3 ${getLevelBg(log.level)}`}>
                  <div className="mt-0.5">{getLevelIcon(log.level)}</div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-gray-800">{log.message}</p>
                    <p className="text-xs text-gray-400 mt-0.5">{formatTime(log.time)}</p>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="py-12 text-center text-gray-400 text-sm">暂无日志</div>
          )}
        </div>
      </div>
    </div>
  )
}
