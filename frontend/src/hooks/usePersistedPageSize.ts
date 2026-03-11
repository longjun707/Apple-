import { useEffect, useState } from 'react'

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 20, 50, 100] as const

function getInitialPageSize(
  storageKey: string,
  fallback: number,
  allowedPageSizes: readonly number[],
) {
  try {
    const storedValue = localStorage.getItem(storageKey)
    if (!storedValue) return fallback

    const parsedValue = Number(storedValue)
    return allowedPageSizes.includes(parsedValue) ? parsedValue : fallback
  } catch {
    return fallback
  }
}

export function usePersistedPageSize(
  storageKey: string,
  fallback = 20,
  allowedPageSizes: readonly number[] = DEFAULT_PAGE_SIZE_OPTIONS,
) {
  const [pageSize, setPageSizeState] = useState(() =>
    getInitialPageSize(storageKey, fallback, allowedPageSizes),
  )

  useEffect(() => {
    if (!allowedPageSizes.includes(pageSize)) {
      setPageSizeState(fallback)
      return
    }

    try {
      localStorage.setItem(storageKey, String(pageSize))
    } catch {
      // Ignore storage write failures.
    }
  }, [allowedPageSizes, fallback, pageSize, storageKey])

  const setPageSize = (nextPageSize: number) => {
    setPageSizeState(allowedPageSizes.includes(nextPageSize) ? nextPageSize : fallback)
  }

  return [pageSize, setPageSize] as const
}
