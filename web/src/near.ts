import { useEffect, useState } from 'react'

const waiting = new Map<Element, () => void>()
let watch: IntersectionObserver | undefined

function observer() {
  watch ??= new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue
        waiting.get(e.target)?.()
        waiting.delete(e.target)
        watch?.unobserve(e.target)
      }
    },
    { rootMargin: '100% 100%' },
  )
  return watch
}

export function useNear<T extends Element>() {
  const [el, setEl] = useState<T | null>(null)
  const [near, setNear] = useState(() => !('IntersectionObserver' in window))
  useEffect(() => {
    if (!el || near) return
    const o = observer()
    waiting.set(el, () => setNear(true))
    o.observe(el)
    return () => {
      waiting.delete(el)
      o.unobserve(el)
    }
  }, [el, near])
  return [setEl, near] as const
}
