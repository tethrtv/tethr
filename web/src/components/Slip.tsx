import { type PointerEvent, type ReactNode, useEffect, useLayoutEffect, useRef, useState } from 'react'
import styles from './Slip.module.css'

const ease = 'cubic-bezier(.22, 1, .36, 1)'
const reduced = () => matchMedia('(prefers-reduced-motion: reduce)').matches
const narrow = () => matchMedia('(max-width: 719px)').matches

type Props = {
  open: boolean
  onClose: () => void
  title: ReactNode
  children: ReactNode
  step?: string
}

export function SlipBody({ children }: { children: ReactNode }) {
  return <div className={styles.body}>{children}</div>
}

export function SlipFoot({ children }: { children: ReactNode }) {
  return <div className={styles.foot}>{children}</div>
}

export function Slip({ open, onClose, title, children, step = '' }: Props) {
  const dialog = useRef<HTMLDialogElement>(null)
  const paper = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  const [shown, setShown] = useState(open)
  const [closing, setClosing] = useState(false)
  const close = useRef(onClose)
  close.current = onClose

  useEffect(() => {
    const d = dialog.current
    if (!d) return
    if (open) {
      setShown(true)
      setClosing(false)
      if (!d.open) d.showModal()
      return
    }
    if (!d.open) return
    setClosing(true)
    const done = setTimeout(
      () => {
        d.close()
        setClosing(false)
        setShown(false)
      },
      reduced() ? 120 : 220,
    )
    return () => clearTimeout(done)
  }, [open])

  const first = useRef(step)
  const last = useRef({ step, height: 0 })
  useLayoutEffect(() => {
    const p = paper.current
    if (!p) {
      last.current = { step, height: 0 }
      return
    }
    const height = p.offsetHeight
    const before = last.current
    last.current = { step, height }
    if (before.step === step || !before.height) return
    const moving = [content.current, p.querySelector('h2')]
    if (reduced()) {
      for (const el of moving) el?.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 160 })
      return
    }
    const dir = step === first.current ? -1 : 1
    p.animate([{ height: `${before.height}px` }, { height: `${height}px` }], { duration: 300, easing: ease })
    for (const el of moving) {
      el?.animate(
        [
          { opacity: 0, transform: `translateX(${dir * 28}px)` },
          { opacity: 1, transform: 'none' },
        ],
        {
          duration: 280,
          easing: ease,
        },
      )
    }
  })

  const drag = useRef<{ y: number; t: number } | null>(null)
  const grab = (e: PointerEvent<HTMLDivElement>) => {
    if (!narrow() || (e.target as HTMLElement).closest('button, a, input, textarea, select, label')) return
    drag.current = { y: e.clientY, t: performance.now() }
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  const pull = (e: PointerEvent<HTMLDivElement>) => {
    if (!drag.current || !paper.current) return
    paper.current.style.transform = `translateY(${Math.max(0, e.clientY - drag.current.y)}px)`
  }
  const release = (e: PointerEvent<HTMLDivElement>) => {
    const p = paper.current
    if (!drag.current || !p) return
    const dy = e.clientY - drag.current.y
    const fast = dy / (performance.now() - drag.current.t) > 0.6
    drag.current = null
    if (dy > p.offsetHeight * 0.3 || fast) {
      close.current()
      return
    }
    p.animate([{ transform: p.style.transform }, { transform: 'none' }], { duration: 220, easing: ease })
    p.style.transform = ''
  }

  return (
    // biome-ignore lint/a11y/useKeyWithClickEvents: clicking the backdrop closes it, and Escape does the same from the keyboard
    <dialog
      ref={dialog}
      className={closing ? `${styles.slip} ${styles.closing}` : styles.slip}
      aria-label={typeof title === 'string' ? title : undefined}
      onCancel={(e) => {
        e.preventDefault()
        close.current()
      }}
      onClick={(e) => {
        if (e.target === dialog.current) close.current()
      }}
    >
      {shown && (
        <div ref={paper} className={styles.paper}>
          <div
            className={styles.top}
            onPointerDown={grab}
            onPointerMove={pull}
            onPointerUp={release}
            onPointerCancel={release}
          >
            <span className={styles.grab} />
            <div className={styles.head}>
              <h2 className={styles.title}>{title}</h2>
              <button type="button" className={styles.close} onClick={() => close.current()}>
                Close
              </button>
            </div>
            <div className={styles.perf} />
          </div>
          <div ref={content} className={styles.content}>
            {children}
          </div>
        </div>
      )}
    </dialog>
  )
}
