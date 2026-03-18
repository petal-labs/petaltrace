import { useState } from 'react'
import { X, Copy, Check, ChevronDown, ChevronRight, Hash, Clock } from 'lucide-react'
import { formatDuration, formatTokens, formatJSON, cn } from '@/lib/utils'
import type { Span, LLMMessage } from '@/types'

interface PromptInspectorProps {
  runId: string
  span: Span
  onClose: () => void
}

export default function PromptInspector({ span, onClose }: PromptInspectorProps) {
  const [copied, setCopied] = useState(false)
  const [showCompletion, setShowCompletion] = useState(true)

  const llm = span.llm
  if (!llm) {
    return (
      <div className="card p-4">
        <p className="text-gray-500">No LLM data available</p>
      </div>
    )
  }

  const copyToClipboard = async (text: string) => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const copyAsJSON = () => {
    const data = {
      provider: llm.provider,
      model: llm.model,
      system_prompt: llm.system_prompt,
      messages: llm.messages,
      completion: llm.completion,
    }
    copyToClipboard(formatJSON(data))
  }

  return (
    <div className="card flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between p-4 border-b border-gray-200">
        <div>
          <h3 className="font-semibold text-gray-900">{span.name}</h3>
          <div className="text-sm text-gray-500">
            {llm.provider} / {llm.model}
          </div>
        </div>
        <button
          onClick={onClose}
          className="p-1 hover:bg-gray-100 rounded"
        >
          <X className="h-5 w-5 text-gray-500" />
        </button>
      </div>

      {/* Stats */}
      <div className="flex gap-4 p-4 border-b border-gray-200 bg-gray-50 text-sm">
        <div className="flex items-center gap-1 text-gray-600">
          <Hash className="h-4 w-4" />
          <span>{formatTokens(llm.tokens.input_tokens)} in</span>
        </div>
        <div className="flex items-center gap-1 text-gray-600">
          <Hash className="h-4 w-4" />
          <span>{formatTokens(llm.tokens.output_tokens)} out</span>
        </div>
        <div className="flex items-center gap-1 text-gray-600">
          <Clock className="h-4 w-4" />
          <span>{formatDuration(llm.total_latency_ms)}</span>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto p-4 space-y-4">
        {/* System prompt */}
        {llm.system_prompt && (
          <div>
            <div className="text-xs font-medium text-gray-500 uppercase mb-2">System Prompt</div>
            <div className="bg-blue-50 rounded-md p-3 text-sm text-gray-800 whitespace-pre-wrap">
              {llm.system_prompt}
            </div>
          </div>
        )}

        {/* Messages */}
        {llm.messages && llm.messages.length > 0 && (
          <div>
            <div className="text-xs font-medium text-gray-500 uppercase mb-2">Messages</div>
            <div className="space-y-2">
              {llm.messages.map((message, index) => (
                <MessageBlock key={index} message={message} />
              ))}
            </div>
          </div>
        )}

        {/* Completion */}
        <div>
          <button
            onClick={() => setShowCompletion(!showCompletion)}
            className="flex items-center gap-1 text-xs font-medium text-gray-500 uppercase mb-2 hover:text-gray-700"
          >
            {showCompletion ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
            Completion
            {llm.stop_reason && (
              <span className="ml-2 text-gray-400">({llm.stop_reason})</span>
            )}
          </button>
          {showCompletion && (
            <div className="bg-green-50 rounded-md p-3 text-sm text-gray-800 whitespace-pre-wrap">
              {llm.completion.text_content || formatJSON(llm.completion.content)}
            </div>
          )}
        </div>
      </div>

      {/* Actions */}
      <div className="p-4 border-t border-gray-200 flex gap-2">
        <button
          onClick={copyAsJSON}
          className="btn btn-secondary flex-1"
        >
          {copied ? <Check className="h-4 w-4 mr-2" /> : <Copy className="h-4 w-4 mr-2" />}
          {copied ? 'Copied!' : 'Copy as JSON'}
        </button>
      </div>
    </div>
  )
}

function MessageBlock({ message }: { message: LLMMessage }) {
  const roleColors: Record<string, string> = {
    user: 'bg-gray-100 border-gray-200',
    assistant: 'bg-green-50 border-green-200',
    system: 'bg-blue-50 border-blue-200',
    tool: 'bg-orange-50 border-orange-200',
  }

  const roleLabels: Record<string, string> = {
    user: 'User',
    assistant: 'Assistant',
    system: 'System',
    tool: 'Tool Result',
  }

  const content = typeof message.content === 'string'
    ? message.content
    : formatJSON(message.content)

  return (
    <div className={cn('rounded-md p-3 border', roleColors[message.role] ?? 'bg-gray-50 border-gray-200')}>
      <div className="text-xs font-medium text-gray-500 mb-1">
        {roleLabels[message.role] ?? message.role}
      </div>
      <div className="text-sm text-gray-800 whitespace-pre-wrap">
        {content}
      </div>
    </div>
  )
}
