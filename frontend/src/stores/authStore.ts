import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { AdminInfo } from '@/api/client'

type AuthState = 'idle' | 'authenticated'

interface AuthStore {
  state: AuthState
  admin: AdminInfo | null
  setState: (state: AuthState) => void
  setAdmin: (admin: AdminInfo | null) => void
  logout: () => void
}

export const useAuthStore = create<AuthStore>()(
  persist(
    (set) => ({
      state: 'idle',
      admin: null,
      setState: (state) => set({ state }),
      setAdmin: (admin) => set({ admin }),
      logout: () => set({ state: 'idle', admin: null }),
    }),
    {
      name: 'admin-auth-storage',
    }
  )
)
