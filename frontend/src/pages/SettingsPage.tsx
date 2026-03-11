import { useState, useEffect } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/api/client'
import { useAuthStore } from '@/stores/authStore'
import { toast } from '@/stores/toastStore'
import { Lock, User, Activity, CheckCircle2, XCircle, Globe } from 'lucide-react'
import Button from '@/components/ui/Button'

export default function SettingsPage() {
  const admin = useAuthStore((s) => s.admin)
  const queryClient = useQueryClient()

  // ---- Proxy Settings ----
  const [proxyUrl, setProxyUrl] = useState('')
  const [proxyError, setProxyError] = useState('')

  const { data: settingsData } = useQuery({
    queryKey: ['systemSettings'],
    queryFn: () => api.getSystemSettings(),
  })

  useEffect(() => {
    if (settingsData?.data?.proxyUrl !== undefined) {
      setProxyUrl(settingsData.data.proxyUrl)
    }
  }, [settingsData])

  const saveProxyMutation = useMutation({
    mutationFn: () => api.updateSystemSettings({ proxyUrl }),
    onSuccess: (res) => {
      if (res.success) {
        toast.success('代理设置已保存')
        setProxyError('')
        queryClient.invalidateQueries({ queryKey: ['systemSettings'] })
      } else {
        setProxyError(res.error || '保存失败')
      }
    },
    onError: () => setProxyError('网络错误'),
  })

  const handleProxySubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setProxyError('')
    saveProxyMutation.mutate()
  }

  // ---- Change Password ----
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [pwdError, setPwdError] = useState('')

  const changePwdMutation = useMutation({
    mutationFn: () => api.changePassword(oldPassword, newPassword),
    onSuccess: (res) => {
      if (res.success) {
        toast.success('密码修改成功')
        setOldPassword('')
        setNewPassword('')
        setConfirmPassword('')
        setPwdError('')
      } else {
        setPwdError(res.error || '修改失败')
      }
    },
    onError: () => setPwdError('网络错误'),
  })

  const handlePasswordSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setPwdError('')
    if (newPassword.length < 6) {
      setPwdError('新密码至少6位')
      return
    }
    if (newPassword !== confirmPassword) {
      setPwdError('两次输入的密码不一致')
      return
    }
    changePwdMutation.mutate()
  }

  // ---- Health Check ----
  const { data: healthData, isLoading: healthLoading } = useQuery({
    queryKey: ['health'],
    queryFn: () => api.health(),
    refetchInterval: 30000,
  })

  const isHealthy = healthData?.success === true

  return (
    <div className="animate-fade-in">
      <div className="mb-8">
        <h2 className="text-2xl font-bold text-gray-900 tracking-tight">系统设置</h2>
        <p className="text-sm text-gray-500 mt-1">管理账户安全与系统信息</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
        {/* Proxy Settings */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-6">
          <div className="flex items-center gap-2.5 mb-5">
            <div className="w-8 h-8 bg-orange-50 rounded-lg flex items-center justify-center">
              <Globe className="w-4 h-4 text-orange-600" />
            </div>
            <h3 className="text-[15px] font-semibold text-gray-900">代理设置</h3>
          </div>

          <form onSubmit={handleProxySubmit} className="space-y-4">
            <div>
              <label className="block text-[13px] font-medium text-gray-600 mb-1.5">代理地址</label>
              <input
                type="text"
                value={proxyUrl}
                onChange={(e) => setProxyUrl(e.target.value)}
                className="input"
                placeholder="http://127.0.0.1:7890 或 socks5://127.0.0.1:1080"
              />
              <p className="text-xs text-gray-400 mt-1.5">留空表示不使用代理，支持 HTTP/HTTPS/SOCKS5</p>
            </div>

            {proxyError && (
              <div className="p-3 bg-red-50 border border-red-100 rounded-xl text-red-600 text-sm animate-fade-in">
                {proxyError}
              </div>
            )}

            <Button
              type="submit"
              loading={saveProxyMutation.isPending}
              className="w-full"
            >
              保存代理设置
            </Button>
          </form>
        </div>

        {/* Change Password */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-6">
          <div className="flex items-center gap-2.5 mb-5">
            <div className="w-8 h-8 bg-blue-50 rounded-lg flex items-center justify-center">
              <Lock className="w-4 h-4 text-blue-600" />
            </div>
            <h3 className="text-[15px] font-semibold text-gray-900">修改密码</h3>
          </div>

          <form onSubmit={handlePasswordSubmit} className="space-y-4">
            <div>
              <label className="block text-[13px] font-medium text-gray-600 mb-1.5">当前密码</label>
              <input
                type="password"
                value={oldPassword}
                onChange={(e) => setOldPassword(e.target.value)}
                className="input"
                placeholder="输入当前密码"
                required
              />
            </div>
            <div>
              <label className="block text-[13px] font-medium text-gray-600 mb-1.5">新密码</label>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                className="input"
                placeholder="至少6位"
                required
                minLength={6}
              />
            </div>
            <div>
              <label className="block text-[13px] font-medium text-gray-600 mb-1.5">确认新密码</label>
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="input"
                placeholder="再次输入新密码"
                required
              />
            </div>

            {pwdError && (
              <div className="p-3 bg-red-50 border border-red-100 rounded-xl text-red-600 text-sm animate-fade-in">
                {pwdError}
              </div>
            )}

            <Button
              type="submit"
              loading={changePwdMutation.isPending}
              disabled={!oldPassword || !newPassword || !confirmPassword}
              className="w-full"
            >
              修改密码
            </Button>
          </form>
        </div>

        {/* Admin Info */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-6">
            <div className="flex items-center gap-2.5 mb-5">
              <div className="w-8 h-8 bg-purple-50 rounded-lg flex items-center justify-center">
                <User className="w-4 h-4 text-purple-600" />
              </div>
              <h3 className="text-[15px] font-semibold text-gray-900">管理员信息</h3>
            </div>
            <div className="space-y-3">
              <div className="flex items-center justify-between py-2 border-b border-gray-50">
                <span className="text-sm text-gray-500">用户名</span>
                <span className="text-sm font-medium text-gray-900">{admin?.username || '—'}</span>
              </div>
              <div className="flex items-center justify-between py-2 border-b border-gray-50">
                <span className="text-sm text-gray-500">昵称</span>
                <span className="text-sm font-medium text-gray-900">{admin?.nickname || '—'}</span>
              </div>
              <div className="flex items-center justify-between py-2">
                <span className="text-sm text-gray-500">角色</span>
                <span className="text-sm font-medium text-gray-900">{admin?.role || 'admin'}</span>
              </div>
            </div>
          </div>

        {/* System Health */}
        <div className="bg-white rounded-2xl shadow-card border border-gray-100/80 p-6">
            <div className="flex items-center gap-2.5 mb-5">
              <div className="w-8 h-8 bg-green-50 rounded-lg flex items-center justify-center">
                <Activity className="w-4 h-4 text-green-600" />
              </div>
              <h3 className="text-[15px] font-semibold text-gray-900">系统状态</h3>
            </div>
            <div className="space-y-3">
              <div className="flex items-center justify-between py-2 border-b border-gray-50">
                <span className="text-sm text-gray-500">API 服务</span>
                {healthLoading ? (
                  <span className="text-sm text-gray-400">检测中...</span>
                ) : isHealthy ? (
                  <span className="inline-flex items-center gap-1.5 text-sm font-medium text-emerald-600">
                    <CheckCircle2 className="w-4 h-4" /> 正常
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 text-sm font-medium text-red-600">
                    <XCircle className="w-4 h-4" /> 异常
                  </span>
                )}
              </div>
              <div className="flex items-center justify-between py-2 border-b border-gray-50">
                <span className="text-sm text-gray-500">前端版本</span>
                <span className="text-sm font-medium text-gray-900">1.0.0</span>
              </div>
              <div className="flex items-center justify-between py-2">
                <span className="text-sm text-gray-500">自动刷新</span>
                <span className="text-sm text-gray-400">每 30 秒</span>
              </div>
            </div>
          </div>
      </div>
    </div>
  )
}
