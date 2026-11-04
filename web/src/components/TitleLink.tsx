import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'

export function TitleLink({
  type,
  slug,
  className,
  onClick,
  children,
}: {
  type: string
  slug?: string
  className?: string
  onClick?: () => void
  children: ReactNode
}) {
  if (!slug) return <span className={className}>{children}</span>
  return (
    <Link
      to={type === 'movie' ? '/films/$slug' : '/shows/$slug'}
      params={{ slug }}
      className={className}
      onClick={onClick}
    >
      {children}
    </Link>
  )
}
