import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'

dayjs.extend(relativeTime)

export function fmtBytes(n: number | undefined | null): string {
  if (n === undefined || n === null) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function fmtTime(iso: string | undefined | null): string {
  return iso ? dayjs(iso).format('YYYY-MM-DD HH:mm:ss') : '-'
}

export function fmtAgo(iso: string | undefined | null): string {
  return iso ? dayjs(iso).fromNow() : '-'
}
