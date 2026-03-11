import { useState, useEffect } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, getErrorMessage } from '@/api/client'
import { useAuthStore } from '@/stores/authStore'
import { Lock, Shield, User } from 'lucide-react'
import Button from '@/components/ui/Button'

const LOGIN_PREFERENCES_KEY = 'admin-login-preferences'

interface LoginPreferences {
  username: string
  rememberMe: boolean
}

function getLoginPreferences(): LoginPreferences | null {
  try {
    // Migrate: clear previously stored password data
    localStorage.removeItem('admin-credentials-v2')
    localStorage.removeItem('admin-credentials')
    localStorage.removeItem('admin-username')

    const saved = localStorage.getItem(LOGIN_PREFERENCES_KEY)
    if (!saved) return null

    return JSON.parse(saved) as LoginPreferences
  } catch {
    return null
  }
}

function saveLoginPreferences(username: string, rememberMe: boolean) {
  const data: LoginPreferences = { username, rememberMe }
  localStorage.setItem(LOGIN_PREFERENCES_KEY, JSON.stringify(data))
}

function clearLoginPreferences() {
  localStorage.removeItem(LOGIN_PREFERENCES_KEY)
}

export default function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [rememberMe, setRememberMe] = useState(true)
  const [error, setError] = useState('')

  const { setState, setAdmin } = useAuthStore()

  // Load safe login preferences on mount
  useEffect(() => {
    const saved = getLoginPreferences()
    if (saved?.rememberMe) {
      setUsername(saved.username)
      setRememberMe(saved.rememberMe)
    }
  }, [])

  const loginMutation = useMutation({
    mutationFn: () => api.adminLogin(username, password, rememberMe),
    onSuccess: (res) => {
      if (!res.success) {
        setError(res.error || '登录失败')
        return
      }
      if (res.data) {
        if (rememberMe) {
          saveLoginPreferences(username, true)
        } else {
          clearLoginPreferences()
        }
        setAdmin(res.data)
        setState('authenticated')
      }
    },
    onError: (err) => {
      setError(getErrorMessage(err))
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    loginMutation.mutate()
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4 bg-gradient-to-br from-slate-100 via-blue-50/40 to-indigo-50/30 relative overflow-hidden">
      {/* Decorative blobs */}
      <div className="absolute top-[-20%] right-[-10%] w-[600px] h-[600px] rounded-full bg-blue-200/20 blur-3xl" />
      <div className="absolute bottom-[-20%] left-[-10%] w-[500px] h-[500px] rounded-full bg-indigo-200/20 blur-3xl" />

      <div className="w-full max-w-[420px] animate-fade-in relative z-10">
        <div className="bg-white/80 backdrop-blur-xl rounded-3xl shadow-elevated border border-white/60 p-8">
          {/* Logo */}
          <div className="flex justify-center mb-8">
            <div className="w-[72px] h-[72px] bg-gradient-to-br from-blue-500 to-indigo-600 rounded-2xl flex items-center justify-center shadow-lg shadow-blue-500/25">
              <Shield className="w-10 h-10 text-white" />
            </div>
          </div>

          <h1 className="text-[22px] font-bold text-center text-gray-900 tracking-tight mb-1">
            Apple HME Manager
          </h1>
          <p className="text-center text-gray-500 text-sm mb-8">
            管理员登录
          </p>

          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <label className="block text-[13px] font-medium text-gray-600 mb-1.5">用户名</label>
              <div className="relative">
                <User className="absolute left-3.5 top-1/2 -translate-y-1/2 w-[18px] h-[18px] text-gray-400" />
                <input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  className="input pl-10"
                  placeholder="admin"
                  required
                />
              </div>
            </div>

            <div>
              <label className="block text-[13px] font-medium text-gray-600 mb-1.5">密码</label>
              <div className="relative">
                <Lock className="absolute left-3.5 top-1/2 -translate-y-1/2 w-[18px] h-[18px] text-gray-400" />
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="input pl-10"
                  placeholder="••••••••"
                  required
                />
              </div>
            </div>

            {error && (
              <div className="p-3 bg-red-50 border border-red-100 rounded-xl text-red-600 text-sm animate-fade-in">
                {error}
              </div>
            )}

            <label className="flex items-center gap-2 cursor-pointer select-none">
              <input
                type="checkbox"
                checked={rememberMe}
                onChange={(e) => setRememberMe(e.target.checked)}
                className="w-4 h-4 rounded border-gray-300 text-apple-blue focus:ring-apple-blue/30"
              />
              <span className="text-sm text-gray-600">保持登录</span>
            </label>

            <Button
              type="submit"
              disabled={!username || !password}
              loading={loginMutation.isPending}
              className="w-full !rounded-xl"
              size="lg"
            >
              登录
            </Button>
          </form>

        </div>
      </div>
    </div>
  )
}
