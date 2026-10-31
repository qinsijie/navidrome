import React, { useCallback, useEffect, useRef } from 'react'
import {
  Pagination as RAPagination,
  useListPaginationContext,
} from 'react-admin'
import { useMediaQuery, CircularProgress, Typography } from '@material-ui/core'
import { setStoredPerPage, defaultRowsPerPageOptions } from './perPageStore'

const MobileInfiniteScroll = ({ perPage, total, loading, setPerPage }) => {
  const sentinelRef = useRef(null)
  const hasMore = Boolean(total && perPage < total)
  const loadingRef = useRef(loading)
  loadingRef.current = loading
  const hasMoreRef = useRef(hasMore)
  hasMoreRef.current = hasMore
  const perPageRef = useRef(perPage)
  perPageRef.current = perPage

  const loadMore = useCallback(() => {
    if (!loadingRef.current && hasMoreRef.current) {
      setPerPage(perPageRef.current + 30)
    }
  }, [setPerPage])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel || typeof IntersectionObserver === 'undefined') return

    const observer = new IntersectionObserver(
      (entries) => {
        const first = entries[0]
        if (first.isIntersecting) {
          loadMore()
        }
      },
      { rootMargin: '250px 0px' },
    )

    observer.observe(sentinel)
    return () => {
      observer.disconnect()
    }
  }, [loadMore])

  return (
    <div
      ref={sentinelRef}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '16px 0',
        width: '100%',
        minHeight: 48,
      }}
    >
      {loading ? (
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <CircularProgress size={20} />
          <Typography variant="caption" color="textSecondary">
            正在载入更多...
          </Typography>
        </div>
      ) : hasMore ? (
        <Typography variant="caption" color="textSecondary" style={{ opacity: 0.6 }}>
          下滑自动加载更多 ({perPage} / {total})
        </Typography>
      ) : total > 0 ? (
        <Typography variant="caption" color="textSecondary" style={{ opacity: 0.6 }}>
          已加载全部 {total} 项
        </Typography>
      ) : null}
    </div>
  )
}

export const Pagination = ({
  rowsPerPageOptions = defaultRowsPerPageOptions,
  ...props
}) => {
  const { resource, perPage, total, loading, setPerPage } = useListPaginationContext()
  const isMobile = useMediaQuery('(max-width: 768px)', {
    noSsr: true,
    defaultMatches: false,
  })

  // Persist only a selector-driven change: mount, URL params and responsive
  // fallbacks never call setPerPage, so they can't overwrite the preference.
  const handleSetPerPage = useCallback(
    (value) => {
      if (resource) setStoredPerPage(resource, value)
      setPerPage(value)
    },
    [resource, setPerPage],
  )

  if (isMobile) {
    return (
      <MobileInfiniteScroll
        perPage={perPage}
        total={total}
        loading={loading}
        setPerPage={setPerPage}
      />
    )
  }

  return (
    <RAPagination
      rowsPerPageOptions={rowsPerPageOptions}
      {...props}
      setPerPage={handleSetPerPage}
    />
  )
}
