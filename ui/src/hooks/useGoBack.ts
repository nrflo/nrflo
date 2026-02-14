import { useCallback } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'

/**
 * Returns a goBack() function that navigates to the previous page if browser
 * history exists, otherwise navigates to fallbackPath.
 *
 * Uses react-router's location.key — it equals "default" when there is no
 * preceding in-app navigation entry.
 */
export function useGoBack(fallbackPath: string) {
  const navigate = useNavigate()
  const location = useLocation()

  return useCallback(() => {
    if (location.key !== 'default') {
      navigate(-1)
    } else {
      navigate(fallbackPath, { replace: true })
    }
  }, [navigate, location.key, fallbackPath])
}
