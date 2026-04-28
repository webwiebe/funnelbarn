import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Funnels from './pages/Funnels'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route path="/dashboard" element={<Dashboard />} />
        <Route path="/dashboard/:projectId" element={<Dashboard />} />
        <Route path="/funnels" element={<Funnels />} />
        <Route path="/funnels/:projectId" element={<Funnels />} />
      </Routes>
    </BrowserRouter>
  )
}

export default App
