import { useQuery } from '@tanstack/react-query'
import {
  api,
  getErrorMessage,
  isAppleAccountLoginRequiredError,
  unwrapResponse,
  type ForwardEmailOption,
} from '@/api/client'

export interface ForwardEmailOptionsResult {
  availableEmails: ForwardEmailOption[]
  currentEmail: string | null
  needsLogin: boolean
}

export function useForwardEmailOptions(accountId: number, enabled = true) {
  return useQuery({
    queryKey: ['account-forward-email-options', accountId],
    queryFn: async (): Promise<ForwardEmailOptionsResult> => {
      const response = await api.getForwardEmailOptions(accountId)

      if (!response.success) {
        if (isAppleAccountLoginRequiredError(response.error)) {
          return {
            availableEmails: [],
            currentEmail: null,
            needsLogin: true,
          }
        }

        throw new Error(getErrorMessage(response.error, '获取转发邮箱失败'))
      }

      const data = unwrapResponse(response, '获取转发邮箱失败')

      return {
        availableEmails: data.forwardToOptions.availableEmails,
        currentEmail: data.forwardToOptions.forwardToEmail?.address ?? null,
        needsLogin: false,
      }
    },
    enabled,
    retry: false,
    staleTime: 60_000,
  })
}
