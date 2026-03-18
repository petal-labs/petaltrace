import { Routes, Route } from 'react-router-dom'
import Layout from '@/components/Layout'
import RunList from '@/pages/RunList'
import RunDetail from '@/pages/RunDetail'
import CostDashboard from '@/pages/CostDashboard'
import DiffView from '@/pages/DiffView'

function App() {
  return (
    <Layout>
      <Routes>
        <Route path="/" element={<RunList />} />
        <Route path="/runs" element={<RunList />} />
        <Route path="/runs/:runId" element={<RunDetail />} />
        <Route path="/cost" element={<CostDashboard />} />
        <Route path="/diff" element={<DiffView />} />
        <Route path="/diff/:baseId/:compareId" element={<DiffView />} />
      </Routes>
    </Layout>
  )
}

export default App
