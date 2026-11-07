import { createContext, type ReactNode, useCallback, useContext, useEffect, useRef, useState } from 'react'
import styles from './Toast.module.css'

type Toast = {
  text: string
  commit?: () => Promise<unknown> | undefined
  undo?: () => void
}

const ToastContext = createContext<(t: Toast) => void>(() => {})

export const useToast = () => useContext(ToastContext)

export function Toasts({ children }: { children: ReactNode }) {
  const [shown, setShown] = useState<{ text: string; undo: boolean } | null>(null)
  const pending = useRef<Toast | null>(null)
  const timer = useRef(0)

  const commit = useCallback(() => {
    const t = pending.current
    pending.current = null
    if (!t?.commit) return
    Promise.resolve(t.commit()).catch(() => {
      t.undo?.()
      setShown({ text: 'That didn’t go through, so it’s back. Try again in a moment.', undo: false })
    })
  }, [])

  const show = useCallback(
    (t: Toast) => {
      clearTimeout(timer.current)
      commit()
      pending.current = t
      setShown({ text: t.text, undo: !!t.undo })
      timer.current = window.setTimeout(() => {
        commit()
        setShown(null)
      }, 5000)
    },
    [commit],
  )

  const undo = () => {
    clearTimeout(timer.current)
    pending.current?.undo?.()
    pending.current = null
    setShown(null)
  }

  useEffect(() => {
    addEventListener('pagehide', commit)
    return () => removeEventListener('pagehide', commit)
  }, [commit])

  return (
    <ToastContext.Provider value={show}>
      {children}
      <div className={shown ? `${styles.toast} ${styles.on}` : styles.toast} role="status">
        {shown && (
          <>
            <span>{shown.text}</span>
            {shown.undo && (
              <button type="button" className={styles.undo} onClick={undo}>
                Undo
              </button>
            )}
          </>
        )}
      </div>
    </ToastContext.Provider>
  )
}
