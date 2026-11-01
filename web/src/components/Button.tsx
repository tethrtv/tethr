import type { ButtonHTMLAttributes } from 'react'
import styles from './Button.module.css'

type Props = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'plain' | 'text' | 'danger' }

export function Button({ variant = 'plain', className, ...rest }: Props) {
  return <button className={[styles[variant], className].filter(Boolean).join(' ')} {...rest} />
}
