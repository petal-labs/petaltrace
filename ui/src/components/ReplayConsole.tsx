import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Play, RefreshCw, CheckCircle, XCircle, GitCompare } from 'lucide-react'
import { triggerReplay } from '@/lib/api'
import { cn } from '@/lib/utils'

interface ReplayConsoleProps {
  runId: string
}

type ReplayMode = 'live' | 'mocked' | 'hybrid'

export default function ReplayConsole({ runId }: ReplayConsoleProps) {
  const navigate = useNavigate()
  const [mode, setMode] = useState<ReplayMode>('live')
  const [model, setModel] = useState('')
  const [temperature, setTemperature] = useState('')
  const [autoDiff, setAutoDiff] = useState(true)
  const [tags, setTags] = useState('')

  const { mutate, isPending, isSuccess, isError, error, data } = useMutation({
    mutationFn: triggerReplay,
    onSuccess: (result) => {
      if (result.new_run_id) {
        // Navigate to the new run after a short delay
        setTimeout(() => {
          navigate(`/runs/${result.new_run_id}`)
        }, 1000)
      }
    },
  })

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()

    const parsedTags: Record<string, string> = {}
    if (tags) {
      tags.split(',').forEach(pair => {
        const [key, value] = pair.split('=').map(s => s.trim())
        if (key && value) {
          parsedTags[key] = value
        }
      })
    }

    mutate({
      source_run_id: runId,
      mode,
      model: model || undefined,
      temperature: temperature ? parseFloat(temperature) : undefined,
      auto_diff: autoDiff,
      tags: Object.keys(parsedTags).length > 0 ? parsedTags : undefined,
      sync: true,
    })
  }

  return (
    <div className="card">
      <div className="p-4 border-b border-gray-200">
        <h3 className="text-lg font-semibold text-gray-900">Replay Run</h3>
        <p className="text-sm text-gray-500 mt-1">
          Re-execute this workflow with different parameters
        </p>
      </div>

      <form onSubmit={handleSubmit} className="p-4 space-y-4">
        {/* Mode selection */}
        <div>
          <label className="label">Replay Mode</label>
          <div className="grid grid-cols-3 gap-2">
            {(['live', 'mocked', 'hybrid'] as ReplayMode[]).map((m) => (
              <button
                key={m}
                type="button"
                onClick={() => setMode(m)}
                className={cn(
                  'px-4 py-2 rounded-md text-sm font-medium border transition-colors',
                  mode === m
                    ? 'bg-primary-50 border-primary-500 text-primary-700'
                    : 'bg-white border-gray-300 text-gray-700 hover:bg-gray-50'
                )}
              >
                {m.charAt(0).toUpperCase() + m.slice(1)}
              </button>
            ))}
          </div>
          <p className="text-xs text-gray-500 mt-2">
            {mode === 'live' && 'Re-execute against real LLM providers'}
            {mode === 'mocked' && 'Use captured responses for deterministic replay'}
            {mode === 'hybrid' && 'Live LLM calls with mocked tool results'}
          </p>
        </div>

        {/* Model override */}
        <div>
          <label className="label">Model Override (optional)</label>
          <input
            type="text"
            className="input"
            placeholder="e.g., claude-3-opus-20240229"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            disabled={mode === 'mocked'}
          />
        </div>

        {/* Temperature override */}
        <div>
          <label className="label">Temperature Override (optional)</label>
          <input
            type="number"
            className="input"
            placeholder="0.0 - 2.0"
            min="0"
            max="2"
            step="0.1"
            value={temperature}
            onChange={(e) => setTemperature(e.target.value)}
            disabled={mode === 'mocked'}
          />
        </div>

        {/* Tags */}
        <div>
          <label className="label">Tags (optional)</label>
          <input
            type="text"
            className="input"
            placeholder="key=value, key2=value2"
            value={tags}
            onChange={(e) => setTags(e.target.value)}
          />
        </div>

        {/* Auto diff */}
        <div className="flex items-center gap-2">
          <input
            type="checkbox"
            id="autoDiff"
            checked={autoDiff}
            onChange={(e) => setAutoDiff(e.target.checked)}
            className="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          />
          <label htmlFor="autoDiff" className="text-sm text-gray-700">
            Auto-diff against source run after completion
          </label>
        </div>

        {/* Submit */}
        <button
          type="submit"
          className="btn btn-primary w-full"
          disabled={isPending}
        >
          {isPending ? (
            <>
              <RefreshCw className="h-4 w-4 mr-2 animate-spin" />
              Replaying...
            </>
          ) : (
            <>
              <Play className="h-4 w-4 mr-2" />
              Start Replay
            </>
          )}
        </button>
      </form>

      {/* Result */}
      {isSuccess && data && (
        <div className="p-4 border-t border-gray-200 bg-green-50">
          <div className="flex items-center gap-2 text-green-700 mb-2">
            <CheckCircle className="h-5 w-5" />
            <span className="font-medium">Replay completed</span>
          </div>
          <p className="text-sm text-green-600 mb-2">
            New run ID: <code className="font-mono">{data.new_run_id}</code>
          </p>
          {data.diff_id && (
            <button
              onClick={() => navigate(`/diff/${runId}/${data.new_run_id}`)}
              className="btn btn-secondary"
            >
              <GitCompare className="h-4 w-4 mr-2" />
              View Diff
            </button>
          )}
        </div>
      )}

      {isError && (
        <div className="p-4 border-t border-gray-200 bg-red-50">
          <div className="flex items-center gap-2 text-red-700">
            <XCircle className="h-5 w-5" />
            <span className="font-medium">Replay failed</span>
          </div>
          <p className="text-sm text-red-600 mt-1">
            {(error as Error).message}
          </p>
        </div>
      )}
    </div>
  )
}
