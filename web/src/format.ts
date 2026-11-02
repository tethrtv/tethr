export const ep = (season: number, episode: number) => `S${season} E${episode}`

export const clock = (iso: string) => new Date(iso).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })

export const date = (iso: string) =>
  new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' })

export function ago(iso: string) {
  const minutes = (Date.now() - Date.parse(iso)) / 60000
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${Math.round(minutes)} min ago`
  if (minutes < 60 * 24) return `${Math.round(minutes / 60)} hr ago`
  const days = Math.round(minutes / 1440)
  if (days === 1) return 'yesterday'
  if (days < 7) return `${days} days ago`
  if (days < 30) return Math.round(days / 7) === 1 ? 'last week' : `${Math.round(days / 7)} weeks ago`
  if (days < 365) return Math.round(days / 30) === 1 ? 'last month' : `${Math.round(days / 30)} months ago`
  return date(iso)
}

export function dayLabel(iso: string) {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const day = new Date(iso)
  day.setHours(0, 0, 0, 0)
  const diff = Math.round((today.getTime() - day.getTime()) / 86400000)
  if (diff === 0) return 'Today'
  if (diff === 1) return 'Yesterday'
  if (diff < 7) return day.toLocaleDateString('en-GB', { weekday: 'long' })
  return day.toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' })
}

export const plural = (n: number, one: string, many = `${one}s`) =>
  `${n.toLocaleString('en-GB')} ${n === 1 ? one : many}`
