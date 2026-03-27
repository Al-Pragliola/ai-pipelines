import { useState } from 'react'

export default function StopButton({ namespace, name, small, onStopped }: { namespace: string; name: string; small?: boolean; onStopped?: () => void }) {
  const [stopping, setStopping] = useState(false)

  const stop = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setStopping(true)
    try {
      const res = await fetch(`/api/runs/${namespace}/${name}/stop`, { method: 'POST' })
      if (!res.ok) throw new Error(await res.text())
      onStopped?.()
    } catch {
      setStopping(false)
    }
  }

  const size = small ? 'w-4 h-4' : 'w-5 h-5'

  return (
    <button
      onClick={stop}
      disabled={stopping}
      title="Stop"
      className={`p-1.5 rounded text-gray-400 hover:text-red-400 hover:bg-red-500/10 transition-colors disabled:opacity-50`}
    >
      <svg className={size} fill="currentColor" viewBox="0 0 24 24">
        <rect x="6" y="6" width="12" height="12" rx="1" />
      </svg>
    </button>
  )
}
