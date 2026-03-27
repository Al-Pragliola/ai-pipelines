import { useState } from 'react'
import { useNavigate } from 'react-router-dom'

export default function RetryButton({ namespace, name, small }: { namespace: string; name: string; small?: boolean }) {
  const navigate = useNavigate()
  const [retrying, setRetrying] = useState(false)

  const retry = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setRetrying(true)
    try {
      const res = await fetch(`/api/runs/${namespace}/${name}/retry`, { method: 'POST' })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      navigate(`/runs/${data.namespace}/${data.name}`)
    } catch {
      setRetrying(false)
    }
  }

  const size = small ? 'w-4 h-4' : 'w-5 h-5'

  return (
    <button
      onClick={retry}
      disabled={retrying}
      title="Retry"
      className={`p-1.5 rounded text-gray-400 hover:text-blue-400 hover:bg-blue-500/10 transition-colors disabled:opacity-50 ${retrying ? 'animate-spin' : ''}`}
    >
      <svg className={size} fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.992 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182M20.985 4.356v4.992" />
      </svg>
    </button>
  )
}
