import React from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './store/AuthContext';
import { ProjectProvider } from './store/ProjectContext';
import useAuth from './store/useAuth';
import AppLayout from './layouts/AppLayout';

// Actual Pages
import LoginPage from './pages/LoginPage';
import RegisterPage from './pages/RegisterPage';
import ProjectListPage from './pages/ProjectListPage';
import ProjectDetailPage from './pages/ProjectDetailPage';
import MyTasksPage from './pages/MyTasksPage';
import Next7DaysPage from './pages/Next7DaysPage';
import CalendarPage from './pages/CalendarPage';
import ProfilePage from './pages/ProfilePage';

function AppRoutes() {
  const { user, loading } = useAuth();

  if (loading) {
    return <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>Loading...</div>;
  }

  return (
    <Routes>
      <Route path="/login" element={!user ? <LoginPage /> : <Navigate to="/" replace />} />
      <Route path="/register" element={!user ? <RegisterPage /> : <Navigate to="/" replace />} />

      {/* Protected Routes encapsulated in AppLayout */}
      <Route
        path="/"
        element={user ? (
          <ProjectProvider>
            <AppLayout />
          </ProjectProvider>
        ) : <Navigate to="/login" replace />}
      >
        <Route index element={<ProjectListPage />} />
        <Route path="tasks/me" element={<MyTasksPage />} />
        <Route path="tasks/next7" element={<Next7DaysPage />} />
        <Route path="calendar" element={<CalendarPage />} />
        <Route path="profile" element={<ProfilePage />} />
        <Route path="projects/:id" element={<ProjectDetailPage />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <AppRoutes />
      </BrowserRouter>
    </AuthProvider>
  );
}

export default App;
