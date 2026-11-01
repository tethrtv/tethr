import { useRef, useState } from 'react'
import styles from './CodeInput.module.css'

export const cleanCode = (raw: string) => raw.replace(/\D/g, '').slice(0, 6)

export function CodeInput({
  value,
  onChange,
  disabled,
  label,
}: {
  value: string
  onChange: (code: string) => void
  disabled?: boolean
  label: string
}) {
  const input = useRef<HTMLInputElement>(null)
  const [focused, setFocused] = useState(false)
  return (
    <div className={styles.comb}>
      <input
        ref={input}
        aria-label={label}
        className={styles.input}
        value={value}
        onChange={(e) => onChange(cleanCode(e.target.value))}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        inputMode="numeric"
        autoComplete="one-time-code"
        disabled={disabled}
      />
      {Array.from({ length: 6 }, (_, i) => (
        <span
          // biome-ignore lint/suspicious/noArrayIndexKey: the six cells never reorder
          key={i}
          aria-hidden="true"
          className={focused && !disabled && i === value.length ? `${styles.cell} ${styles.here}` : styles.cell}
        >
          {value[i]}
        </span>
      ))}
    </div>
  )
}
