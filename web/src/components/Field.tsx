import { type InputHTMLAttributes, type ReactNode, useId } from 'react'
import styles from './Field.module.css'

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, 'onChange'> & {
  label: string
  hint?: ReactNode
  error?: string
  onChange: (value: string) => void
}

export function Field({ label, hint, error, onChange, ...input }: Props) {
  const id = useId()
  const note = error ?? hint
  return (
    <div className={error ? `${styles.field} ${styles.bad}` : styles.field}>
      <label htmlFor={id}>{label}</label>
      <input
        id={id}
        className={styles.input}
        aria-invalid={!!error}
        aria-describedby={note ? `${id}-note` : undefined}
        onChange={(e) => onChange(e.target.value)}
        {...input}
      />
      {note && (
        <p id={`${id}-note`} className={styles.note}>
          {note}
        </p>
      )}
    </div>
  )
}
