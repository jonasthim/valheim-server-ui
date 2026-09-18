// Where the explored part of the world is, as fractions of the map image,
// derived in the browser from the fog mask (alpha 255 = unexplored). The
// Overview's hero map zooms to this box so a freshly explored island fills
// the frame instead of sitting as a speck in a parchment square.
import { useEffect, useState } from 'react'
import { fogMaskUrl } from './useMap'

export interface ExploredBounds {
  u0: number
  v0: number
  u1: number
  v1: number
}

/** A square viewport (top-left corner + side, as fractions of the map) framing the bounds. */
export interface MapFrame {
  u0: number
  v0: number
  side: number
}

const PADDING = 0.12 // of the box's larger side, on every edge
const MIN_SIDE = 0.25 // never zoom past 4×: a tiny explored patch would become pixels
const MAX_SIDE = 1

/**
 * Reads the fog mask for `maskVersion` and returns the explored bounding box,
 * or null while loading, when there is no mask (no agent, no exploration yet)
 * or when nothing is explored.
 */
export function useExploredBounds(id: string, maskVersion: number | undefined): ExploredBounds | null {
  // Keyed by the mask version it was read from, so a stale result is never
  // shown for a newer mask (and no state write is needed to "reset").
  const [result, setResult] = useState<{ version: number; bounds: ExploredBounds | null } | null>(null)

  useEffect(() => {
    if (maskVersion === undefined) return
    let cancelled = false
    const img = new Image()
    img.onload = () => {
      if (cancelled) return
      const w = img.naturalWidth
      const h = img.naturalHeight
      if (!w || !h) return
      const canvas = document.createElement('canvas')
      canvas.width = w
      canvas.height = h
      const ctx = canvas.getContext('2d', { willReadFrequently: true })
      if (!ctx) return
      ctx.drawImage(img, 0, 0)
      const data = ctx.getImageData(0, 0, w, h).data
      let minX = w
      let minY = h
      let maxX = -1
      let maxY = -1
      for (let y = 0; y < h; y++) {
        for (let x = 0; x < w; x++) {
          if (data[(y * w + x) * 4 + 3] < 255) {
            if (x < minX) minX = x
            if (x > maxX) maxX = x
            if (y < minY) minY = y
            if (y > maxY) maxY = y
          }
        }
      }
      const bounds = maxX < 0 ? null : { u0: minX / w, v0: minY / h, u1: (maxX + 1) / w, v1: (maxY + 1) / h }
      setResult({ version: maskVersion, bounds })
    }
    img.onerror = () => {
      if (!cancelled) setResult({ version: maskVersion, bounds: null })
    }
    img.src = fogMaskUrl(id, maskVersion)
    return () => {
      cancelled = true
    }
  }, [id, maskVersion])

  return maskVersion !== undefined && result?.version === maskVersion ? result.bounds : null
}

/** The square viewport that frames `bounds` with padding, clamped to the map and to a 4× zoom. */
export function frameFor(bounds: ExploredBounds): MapFrame {
  const w = bounds.u1 - bounds.u0
  const h = bounds.v1 - bounds.v0
  const side = Math.min(MAX_SIDE, Math.max(MIN_SIDE, Math.max(w, h) * (1 + 2 * PADDING)))
  const cu = (bounds.u0 + bounds.u1) / 2
  const cv = (bounds.v0 + bounds.v1) / 2
  const clamp = (c: number) => Math.min(1 - side, Math.max(0, c - side / 2))
  return { u0: clamp(cu), v0: clamp(cv), side }
}

/** CSS transform that shows `frame` of an image filling its (square) container. */
export function frameTransform(frame: MapFrame): string {
  return `scale(${1 / frame.side}) translate(-${frame.u0 * 100}%, -${frame.v0 * 100}%)`
}
