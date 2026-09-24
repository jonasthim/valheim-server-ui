import { lazy } from 'react'
import { createBrowserRouter, createRoutesFromElements, Navigate, Route, RouterProvider } from 'react-router-dom'
import { RequireAuth, RequireRole } from './auth/guards'
import { Shell } from './layout/Shell'
import { RouteErrorPage } from './layout/RouteErrorPage'
import { LoginPage } from './features/auth/LoginPage'
import { SetupPage } from './features/auth/SetupPage'
import { NotFoundPage } from './features/system'

// Route-level pages are code-split: each loads on first navigation, behind the
// Suspense boundary in Shell. LoginPage/SetupPage stay eager (first paint).
const DashboardPage = lazy(() => import('./features/dashboard/DashboardPage').then((m) => ({ default: m.DashboardPage })))
const CreateInstancePage = lazy(() => import('./features/instances/CreateInstancePage').then((m) => ({ default: m.CreateInstancePage })))
const InstancePage = lazy(() => import('./features/instances/InstancePage').then((m) => ({ default: m.InstancePage })))
const JobsPage = lazy(() => import('./features/jobs/JobsPage').then((m) => ({ default: m.JobsPage })))
const UsersPage = lazy(() => import('./features/users/UsersPage').then((m) => ({ default: m.UsersPage })))
const SettingsPage = lazy(() => import('./features/settings/SettingsPage').then((m) => ({ default: m.SettingsPage })))
const AuditPage = lazy(() => import('./features/audit/AuditPage').then((m) => ({ default: m.AuditPage })))
const AccountPage = lazy(() => import('./features/account/AccountPage').then((m) => ({ default: m.AccountPage })))

// Route map from docs/ARCHITECTURE.md §15. Tabs inside an instance are handled
// by InstancePage via the :tab param. A pathless root route carries one
// errorElement over login/setup and the shell alike.
const router = createBrowserRouter(
  createRoutesFromElements(
    <Route errorElement={<RouteErrorPage />}>
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
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Route>
    </Route>,
  ),
)

export default function App() {
  return <RouterProvider router={router} />
}
