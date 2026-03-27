import { useEffect, useState } from 'react'

function format(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 0) return 'just now'
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

export default function TimeAgo({ date }: { date: string | null }) {
  const [, setTick] = useState(0)

  useEffect(() => {
    if (!date) return
    const id = setInterval(() => setTick(t => t + 1), 10_000)
    return () => clearInterval(id)
  }, [date])

  if (!date) return <span className="text-gray-500">—</span>
  return <span title={new Date(date).toLocaleString()}>{format(date)}</span>
}
