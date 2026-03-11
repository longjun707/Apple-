import { useEffect, useCallback, useState } from 'react'
import { useAuthStore } from '@/stores/authStore'
import { setOnUnauthorized, api, clearSession } from '@/api/client'
import LoginPage from '@/pages/LoginPage'
import DashboardPage from '@/pages/DashboardPage'
import AccountsPage from '@/pages/AccountsPage'
import EmailsPage from '@/pages/EmailsPage'
import SettingsPage from '@/pages/SettingsPage'
import AutoTaskPage from '@/pages/AutoTaskPage'
import Sidebar from '@/components/layout/Sidebar'
import ToastContainer from '@/components/ui/Toast'
import { toast } from '@/stores/toastStore'
import { cn } from '@/lib/cn'
import { Loader2 } from 'lucide-react'

function App() {
  const authState = useAuthStore((s) => s.state)
  const admin = useAuthStore((s) => s.admin)
  const logout = useAuthStore((s) => s.logout)
  const setAdmin = useAuthStore((s) => s.setAdmin)
  const [currentPage, setCurrentPage] = useState('dashboard')
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [sessionChecked, setSessionChecked] = useState(false)

  // Register 401 handler — auto logout when session expires
  useEffect(() => {
    setOnUnauthorized(() => {
      logout()
      clearSession()
      toast.error('会话已过期，请重新登录')
    })
  }, [logout])

  // Validate session on mount — if persisted auth exists, verify it's still valid
  useEffect(() => {
    // 只在组件挂载时执行一次，使用 ref 避免重复执行
    let isMounted = true
    
    const validateSession = async () => {
      if (authState !== 'authenticated') {
        setSessionChecked(true)
        return
      }
      
      try {
        const res = await api.adminInfo()
        if (!isMounted) return
        
        if (!res.success) {
          clearSession()
          logout()
        } else if (res.data) {
          setAdmin(res.data)
        }
      } finally {
        if (isMounted) {
          setSessionChecked(true)
        }
      }
    }
    
    validateSession()
    
    return () => {
      isMounted = false
    }
  }, [authState, logout, setAdmin])

  const handleLogout = useCallback(async () => {
    try {
      await api.adminLogout()
    } finally {
      clearSession()
      logout()
    }
  }, [logout])

  // Show loading spinner while validating session
  if (!sessionChecked) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-[#f8f9fb]">
        <Loader2 className="w-8 h-8 animate-spin text-apple-blue" />
        <ToastContainer />
      </div>
    )
  }

  if (authState !== 'authenticated') {
    return (
      <div className="min-h-screen">
        <LoginPage />
        <ToastContainer />
      </div>
    )
  }

  const renderPage = () => {
    switch (currentPage) {
      case 'dashboard':
        return <DashboardPage onNavigate={setCurrentPage} />
      case 'accounts':
        return <AccountsPage />
      case 'emails':
        return <EmailsPage />
      case 'auto-task':
        return <AutoTaskPage />
      case 'settings':
        return <SettingsPage />
      default:
        return <DashboardPage onNavigate={setCurrentPage} />
    }
  }

  return (
    <div className="min-h-screen bg-[#f8f9fb]">
      <Sidebar
        currentPage={currentPage}
        onPageChange={setCurrentPage}
        onLogout={handleLogout}
        adminName={admin?.username}
        collapsed={sidebarCollapsed}
        onToggleCollapse={() => setSidebarCollapsed((prev) => !prev)}
      />

      <main
        className={cn(
          'transition-all duration-300 min-h-screen',
          sidebarCollapsed ? 'ml-[68px]' : 'ml-[260px]'
        )}
      >
        <div className="max-w-[1400px] mx-auto px-8 py-8">
          {renderPage()}
        </div>
      </main>

      <ToastContainer />
    </div>
  )
}

export default App
