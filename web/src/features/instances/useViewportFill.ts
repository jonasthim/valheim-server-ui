// Measures how far down the viewport an element starts so its height can be
// set to fill the rest of the viewport (`100dvh` minus that offset), leaving
// `reserveBottom` px below it for chrome that follows (a prompt row, etc).
// Owned by C-9.
import { useLayoutEffect, useState, type RefObject } from 'react'

/** Offset (px) for `calc(100dvh - <offset>px)`; remeasured on window resize
 *  and on any resize of `document.body` (sidebar collapse, font load, …). */
export function useViewportFill(ref: RefObject<HTMLElement | null>, reserveBottom: number): number {
  const [offset, setOffset] = useState(360)

  useLayoutEffect(() => {
    const measure = () => {
      const el = ref.current
      if (!el) return
      const top = el.getBoundingClientRect().top + window.scrollY
      setOffset(Math.round(top + reserveBottom))
    }
    measure()
    window.addEventListener('resize', measure)
    const observer = new ResizeObserver(measure)
    observer.observe(document.body)
    return () => {
      window.removeEventListener('resize', measure)
      observer.disconnect()
    }
  }, [ref, reserveBottom])

  return offset
}
