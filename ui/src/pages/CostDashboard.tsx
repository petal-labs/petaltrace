import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { RefreshCw, Coins, Hash, TrendingUp, TrendingDown, Minus } from 'lucide-react'
import {
  AreaChart,
  Area,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  Legend,
} from 'recharts'
import { getCostSummary, getCostTimeseries, type CostSummaryParams, type CostTimeseriesParams } from '@/lib/api'
import { formatCost, formatTokens, cn } from '@/lib/utils'

const COLORS = ['#8b5cf6', '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#6366f1', '#ec4899', '#14b8a6']

type TimeRange = '24h' | '7d' | '30d' | '90d'
type Bucket = 'hour' | 'day' | 'week'

export default function CostDashboard() {
  const [timeRange, setTimeRange] = useState<TimeRange>('7d')
  const [bucket, setBucket] = useState<Bucket>('day')

  const summaryParams: CostSummaryParams = {
    since: timeRange,
  }

  const timeseriesParams: CostTimeseriesParams = {
    since: timeRange,
    bucket,
  }

  const { data: summary, isLoading: summaryLoading, refetch: refetchSummary } = useQuery({
    queryKey: ['cost-summary', summaryParams],
    queryFn: () => getCostSummary(summaryParams),
  })

  const { data: timeseries, isLoading: timeseriesLoading, refetch: refetchTimeseries } = useQuery({
    queryKey: ['cost-timeseries', timeseriesParams],
    queryFn: () => getCostTimeseries(timeseriesParams),
  })

  const refetch = () => {
    refetchSummary()
    refetchTimeseries()
  }

  const isLoading = summaryLoading || timeseriesLoading

  // Calculate trend
  const trend = timeseries && timeseries.length >= 2
    ? ((timeseries[timeseries.length - 1].cost - timeseries[0].cost) / timeseries[0].cost) * 100
    : 0

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Cost Dashboard</h1>
        <div className="flex items-center gap-4">
          {/* Time range selector */}
          <div className="flex rounded-md shadow-sm">
            {(['24h', '7d', '30d', '90d'] as TimeRange[]).map((range) => (
              <button
                key={range}
                onClick={() => setTimeRange(range)}
                className={cn(
                  'px-3 py-1.5 text-sm font-medium border border-gray-300 first:rounded-l-md last:rounded-r-md -ml-px first:ml-0',
                  timeRange === range
                    ? 'bg-primary-50 border-primary-500 text-primary-700 z-10'
                    : 'bg-white text-gray-700 hover:bg-gray-50'
                )}
              >
                {range}
              </button>
            ))}
          </div>

          <button
            onClick={refetch}
            className="btn btn-secondary"
            disabled={isLoading}
          >
            <RefreshCw className={cn('h-4 w-4 mr-2', isLoading && 'animate-spin')} />
            Refresh
          </button>
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-4 gap-4 mb-6">
        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div className="text-sm text-gray-500">Total Cost</div>
            <Coins className="h-5 w-5 text-purple-500" />
          </div>
          <div className="text-2xl font-bold text-gray-900 mt-2">
            {summary ? formatCost(summary.total_cost) : '--'}
          </div>
          <div className="flex items-center mt-2 text-sm">
            {trend > 0 ? (
              <span className="flex items-center text-red-600">
                <TrendingUp className="h-4 w-4 mr-1" />
                +{trend.toFixed(1)}%
              </span>
            ) : trend < 0 ? (
              <span className="flex items-center text-green-600">
                <TrendingDown className="h-4 w-4 mr-1" />
                {trend.toFixed(1)}%
              </span>
            ) : (
              <span className="flex items-center text-gray-500">
                <Minus className="h-4 w-4 mr-1" />
                0%
              </span>
            )}
            <span className="text-gray-400 ml-2">vs. start of period</span>
          </div>
        </div>

        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div className="text-sm text-gray-500">Total Tokens</div>
            <Hash className="h-5 w-5 text-blue-500" />
          </div>
          <div className="text-2xl font-bold text-gray-900 mt-2">
            {summary ? formatTokens(summary.total_tokens) : '--'}
          </div>
          <div className="text-sm text-gray-400 mt-2">
            {summary ? `${formatTokens(summary.input_tokens)} in / ${formatTokens(summary.output_tokens)} out` : '--'}
          </div>
        </div>

        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div className="text-sm text-gray-500">Total Runs</div>
            <RefreshCw className="h-5 w-5 text-green-500" />
          </div>
          <div className="text-2xl font-bold text-gray-900 mt-2">
            {summary?.total_runs ?? '--'}
          </div>
          <div className="text-sm text-gray-400 mt-2">
            {summary ? `${formatCost(summary.total_cost / summary.total_runs)} avg/run` : '--'}
          </div>
        </div>

        <div className="card p-4">
          <div className="flex items-center justify-between">
            <div className="text-sm text-gray-500">Cost per 1K Tokens</div>
            <Coins className="h-5 w-5 text-orange-500" />
          </div>
          <div className="text-2xl font-bold text-gray-900 mt-2">
            {summary && summary.total_tokens > 0
              ? formatCost((summary.total_cost / summary.total_tokens) * 1000)
              : '--'}
          </div>
          <div className="text-sm text-gray-400 mt-2">
            Effective rate
          </div>
        </div>
      </div>

      {/* Charts */}
      <div className="grid grid-cols-2 gap-6 mb-6">
        {/* Cost over time */}
        <div className="card p-4">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-semibold text-gray-900">Cost Over Time</h3>
            <div className="flex rounded-md shadow-sm">
              {(['hour', 'day', 'week'] as Bucket[]).map((b) => (
                <button
                  key={b}
                  onClick={() => setBucket(b)}
                  className={cn(
                    'px-2 py-1 text-xs font-medium border border-gray-300 first:rounded-l-md last:rounded-r-md -ml-px first:ml-0',
                    bucket === b
                      ? 'bg-primary-50 border-primary-500 text-primary-700 z-10'
                      : 'bg-white text-gray-700 hover:bg-gray-50'
                  )}
                >
                  {b}
                </button>
              ))}
            </div>
          </div>
          <div className="h-64">
            {timeseries && timeseries.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={timeseries}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis
                    dataKey="timestamp"
                    tickFormatter={(value) => {
                      const date = new Date(value)
                      return bucket === 'hour'
                        ? date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                        : date.toLocaleDateString([], { month: 'short', day: 'numeric' })
                    }}
                    fontSize={12}
                  />
                  <YAxis
                    tickFormatter={(value) => formatCost(value)}
                    fontSize={12}
                  />
                  <Tooltip
                    formatter={(value: number) => [formatCost(value), 'Cost']}
                    labelFormatter={(label) => new Date(label).toLocaleString()}
                  />
                  <Area
                    type="monotone"
                    dataKey="cost"
                    stroke="#8b5cf6"
                    fill="#8b5cf680"
                  />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No data available
              </div>
            )}
          </div>
        </div>

        {/* Tokens over time */}
        <div className="card p-4">
          <h3 className="font-semibold text-gray-900 mb-4">Tokens Over Time</h3>
          <div className="h-64">
            {timeseries && timeseries.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={timeseries}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis
                    dataKey="timestamp"
                    tickFormatter={(value) => {
                      const date = new Date(value)
                      return bucket === 'hour'
                        ? date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
                        : date.toLocaleDateString([], { month: 'short', day: 'numeric' })
                    }}
                    fontSize={12}
                  />
                  <YAxis
                    tickFormatter={(value) => formatTokens(value)}
                    fontSize={12}
                  />
                  <Tooltip
                    formatter={(value: number) => [formatTokens(value), 'Tokens']}
                    labelFormatter={(label) => new Date(label).toLocaleString()}
                  />
                  <Area
                    type="monotone"
                    dataKey="tokens"
                    stroke="#3b82f6"
                    fill="#3b82f680"
                  />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No data available
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Breakdown charts */}
      <div className="grid grid-cols-3 gap-6">
        {/* By workflow */}
        <div className="card p-4">
          <h3 className="font-semibold text-gray-900 mb-4">Cost by Workflow</h3>
          <div className="h-64">
            {summary?.by_workflow && summary.by_workflow.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={summary.by_workflow}
                    dataKey="total_cost"
                    nameKey="name"
                    cx="50%"
                    cy="50%"
                    outerRadius={80}
                    label={({ name, percent }) => `${name} (${(percent * 100).toFixed(0)}%)`}
                  >
                    {summary.by_workflow.map((_, index) => (
                      <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                    ))}
                  </Pie>
                  <Tooltip formatter={(value: number) => formatCost(value)} />
                </PieChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No data available
              </div>
            )}
          </div>
        </div>

        {/* By provider */}
        <div className="card p-4">
          <h3 className="font-semibold text-gray-900 mb-4">Cost by Provider</h3>
          <div className="h-64">
            {summary?.by_provider && summary.by_provider.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={summary.by_provider} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis type="number" tickFormatter={(value) => formatCost(value)} fontSize={12} />
                  <YAxis type="category" dataKey="name" width={80} fontSize={12} />
                  <Tooltip formatter={(value: number) => formatCost(value)} />
                  <Bar dataKey="total_cost" fill="#8b5cf6" />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No data available
              </div>
            )}
          </div>
        </div>

        {/* By model */}
        <div className="card p-4">
          <h3 className="font-semibold text-gray-900 mb-4">Cost by Model</h3>
          <div className="h-64">
            {summary?.by_model && summary.by_model.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={summary.by_model} layout="vertical">
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis type="number" tickFormatter={(value) => formatCost(value)} fontSize={12} />
                  <YAxis type="category" dataKey="name" width={120} fontSize={10} />
                  <Tooltip formatter={(value: number) => formatCost(value)} />
                  <Legend />
                  <Bar dataKey="total_cost" fill="#3b82f6" name="Cost" />
                </BarChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex items-center justify-center h-full text-gray-500">
                No data available
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Detailed breakdown table */}
      {summary?.by_workflow && summary.by_workflow.length > 0 && (
        <div className="card mt-6 overflow-hidden">
          <div className="px-6 py-4 border-b border-gray-200">
            <h3 className="font-semibold text-gray-900">Workflow Breakdown</h3>
          </div>
          <table className="min-w-full divide-y divide-gray-200">
            <thead className="bg-gray-50">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Workflow
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Runs
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Tokens
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Total Cost
                </th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">
                  Avg Cost/Run
                </th>
              </tr>
            </thead>
            <tbody className="bg-white divide-y divide-gray-200">
              {summary.by_workflow.map((workflow) => (
                <tr key={workflow.name} className="hover:bg-gray-50">
                  <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                    {workflow.name}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-600 text-right">
                    {workflow.run_count}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-600 text-right">
                    {formatTokens(workflow.total_tokens)}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900 text-right font-medium">
                    {formatCost(workflow.total_cost)}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-600 text-right">
                    {formatCost(workflow.total_cost / workflow.run_count)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
