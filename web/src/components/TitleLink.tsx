import { Link } from '@tanstack/react-router'
import type { MouseEventHandler, ReactNode } from 'react'

export function TitleLink({
  type,
  slug,
  className,
  onClick,
  onMouseDown,
  children,
}: {
  type: string
  slug?: string
  className?: string
  onClick?: () => void
  onMouseDown?: MouseEventHandler
  children: ReactNode
}) {
  if (!slug) return <span className={className}>{children}</span>
  return (
    <Link
      to={type === 'movie' ? '/films/$slug' : '/shows/$slug'}
      params={{ slug }}
      className={className}
      onClick={onClick}
      onMouseDown={onMouseDown}
    >
      {children}
    </Link>
  )
}
