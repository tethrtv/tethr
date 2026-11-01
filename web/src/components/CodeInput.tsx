import { useRef, useState } from 'react'
import styles from './CodeInput.module.css'

export const cleanCode = (raw: string, length = 6, letters = false) =>
  raw
    .toUpperCase()
    .replace(letters ? /[^A-Z0-9]/g : /\D/g, '')
    .slice(0, length)

export function CodeInput({
  value,
  onChange,
  disabled,
  label,
  length = 6,
  letters = false,
}: {
  value: string
  onChange: (code: string) => void
  disabled?: boolean
  label: string
  length?: number
  letters?: boolean
}) {
  const input = useRef<HTMLInputElement>(null)
  const [focused, setFocused] = useState(false)
  return (
    <div className={styles.comb} style={{ gridTemplateColumns: `repeat(${length}, minmax(0, 1fr))` }}>
      <input
        ref={input}
        aria-label={label}
        className={styles.input}
        value={value}
        onChange={(e) => onChange(cleanCode(e.target.value, length, letters))}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        inputMode={letters ? 'text' : 'numeric'}
        autoCapitalize={letters ? 'characters' : undefined}
        autoCorrect="off"
        spellCheck={false}
        autoComplete="one-time-code"
        disabled={disabled}
      />
      {Array.from({ length }, (_, i) => (
        <span
          // biome-ignore lint/suspicious/noArrayIndexKey: the cells never reorder
          key={i}
          aria-hidden="true"
          className={[
            styles.cell,
            i === length / 2 - 1 && styles.half,
            focused && !disabled && i === value.length && styles.here,
          ]
            .filter(Boolean)
            .join(' ')}
        >
          {value[i]}
        </span>
      ))}
    </div>
  )
}
