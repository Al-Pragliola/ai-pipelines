const colors: Record<string, string> = {
  Succeeded: 'bg-green-500/15 text-green-400 border-green-500/25',
  Running: 'bg-blue-500/15 text-blue-400 border-blue-500/25',
  Failed: 'bg-red-500/15 text-red-400 border-red-500/25',
  Stopped: 'bg-gray-500/15 text-gray-400 border-gray-500/25',
  Initializing: 'bg-cyan-500/15 text-cyan-400 border-cyan-500/25',
  Pending: 'bg-yellow-500/15 text-yellow-400 border-yellow-500/25',
  WaitingForInput: 'bg-orange-500/15 text-orange-400 border-orange-500/25',
  Skipped: 'bg-gray-500/15 text-gray-400 border-gray-500/25',
  Deleting: 'bg-red-500/15 text-red-400 border-red-500/25',
}

export default function StatusBadge({ status }: { status: string }) {
  const cls = colors[status] ?? colors.Pending
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium border ${cls}`}>
      {(status === 'Running' || status === 'WaitingForInput' || status === 'Initializing' || status === 'Deleting') && (
        <span className="relative flex h-2 w-2">
          <span className={`animate-ping absolute inline-flex h-full w-full rounded-full opacity-75 ${
            status === 'WaitingForInput' ? 'bg-orange-400' : status === 'Initializing' ? 'bg-cyan-400' : status === 'Deleting' ? 'bg-red-400' : 'bg-blue-400'
          }`} />
          <span className={`relative inline-flex rounded-full h-2 w-2 ${
            status === 'WaitingForInput' ? 'bg-orange-400' : status === 'Initializing' ? 'bg-cyan-400' : status === 'Deleting' ? 'bg-red-400' : 'bg-blue-400'
          }`} />
        </span>
      )}
      {status || 'Unknown'}
    </span>
  )
}
