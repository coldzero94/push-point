import { getRouteApi, Navigate } from '@tanstack/react-router'

const route = getRouteApi('/links/$id/edit')

// The edit screen is retired (11 §0): editing is inline in the inspector, and
// PATCH is one request. /links/$id/edit redirects to /links/$id for bookmark
// compatibility — the migration the spec calls for.
export function LinkEditScreen() {
  const { id } = route.useParams()
  return <Navigate to="/links/$id" params={{ id }} replace />
}
