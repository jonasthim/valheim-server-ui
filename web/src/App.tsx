import { Navigate, Route, Routes } from 'react-router-dom'
import { RequireAuth, RequireRole } from './auth/guards'
import { Shell } from './layout/Shell'
import { LoginPage } from './features/auth/LoginPage'
import { SetupPage } from './features/auth/SetupPage'
import { DashboardPage } from './features/dashboard/DashboardPage'
import { CreateInstancePage } from './features/instances/CreateInstancePage'
import { InstancePage } from './features/instances/InstancePage'
import { JobsPage } from './features/jobs/JobsPage'
import { UsersPage } from './features/users/UsersPage'
import { SettingsPage } from './features/settings/SettingsPage'
import { AuditPage } from './features/audit/AuditPage'
import { AccountPage } from './features/account/AccountPage'

// Route map from docs/ARCHITECTURE.md §15. Tabs inside an instance are handled
// by InstancePage via the :tab param.
export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/setup" element={<SetupPage />} />
      <Route element={<RequireAuth />}>
        <Route element={<Shell />}>
          <Route index element={<DashboardPage />} />
          <Route path="/instances/new" element={<CreateInstancePage />} />
          <Route path="/instances/:id" element={<Navigate to="overview" replace />} />
          <Route path="/instances/:id/:tab" element={<InstancePage />} />
          <Route path="/jobs" element={<JobsPage />} />
          <Route path="/account" element={<AccountPage />} />
          <Route element={<RequireRole min="admin" />}>
            <Route path="/users" element={<UsersPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/audit" element={<AuditPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Route>
    </Routes>
  )
}
