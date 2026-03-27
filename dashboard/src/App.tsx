import { Routes, Route, Link } from 'react-router-dom'
import PipelineList from './pages/PipelineList'
import PipelineCreate from './pages/PipelineCreate'
import PipelineDetail from './pages/PipelineDetail'
import RunDetail from './pages/RunDetail'
import IssueHistory from './pages/IssueHistory'
import OperatorLogs from './pages/OperatorLogs'

export default function App() {
  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      <nav className="border-b border-gray-800 bg-gray-900/50 backdrop-blur sticky top-0 z-10">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center h-14">
            <Link to="/" className="flex items-center gap-2 text-white font-semibold text-lg">
              <svg className="w-6 h-6 text-indigo-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904 9 18.75l-.813-2.846a4.5 4.5 0 0 0-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 0 0 3.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 0 0 3.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 0 0-3.09 3.09ZM18.259 8.715 18 9.75l-.259-1.035a3.375 3.375 0 0 0-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 0 0 2.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 0 0 2.455 2.456L21.75 6l-1.036.259a3.375 3.375 0 0 0-2.455 2.456Z" />
              </svg>
              AI Pipelines
            </Link>
            <div className="ml-8 flex items-center gap-4">
              <Link to="/" className="text-sm text-gray-400 hover:text-white transition-colors">Pipelines</Link>
              <Link to="/history" className="text-sm text-gray-400 hover:text-white transition-colors">History</Link>
              <Link to="/logs" className="text-sm text-gray-400 hover:text-white transition-colors">Logs</Link>
            </div>
          </div>
        </div>
      </nav>
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <Routes>
          <Route path="/" element={<PipelineList />} />
          <Route path="/pipelines/new" element={<PipelineCreate />} />
          <Route path="/pipelines/:namespace/:name" element={<PipelineDetail />} />
          <Route path="/runs/:namespace/:name" element={<RunDetail />} />
          <Route path="/history" element={<IssueHistory />} />
          <Route path="/logs" element={<OperatorLogs />} />
        </Routes>
      </main>
    </div>
  )
}
