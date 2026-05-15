import { useState, useEffect, useCallback } from 'react'
import { BrowserRouter, Routes, Route, Link, Navigate, useLocation } from 'react-router-dom'
import * as api from './api'
import {
  getStoredThemeMode,
  getStoredTimeRange,
  setStoredThemeMode,
  setStoredTimeRange,
  resolveTheme,
} from './theme'
import UserPanel from './components/UserPanel'
import AdminPanel from './components/AdminPanel'
import './App.css'

export default function App() {
  return (
    <BrowserRouter>
      <AppContent />
    </BrowserRouter>
  )
}

function AppContent() {
  const [setupRequired, setSetupRequired] = useState(false)
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [authChecked, setAuthChecked] = useState(false)
  const [error, setError] = useState(null)
  const [passwordInput, setPasswordInput] = useState('')
  const [passwordConfirm, setPasswordConfirm] = useState('')
  const [tokenInput, setTokenInput] = useState(api.getAuthToken())
  const [themeMode, setThemeModeState] = useState(getStoredThemeMode)
  const [themeTimeRange, setThemeTimeRangeState] = useState(getStoredTimeRange)
  const [resolvedTheme, setResolvedTheme] = useState(() => resolveTheme(getStoredThemeMode(), getStoredTimeRange()))

  const setThemeMode = (mode) => {
    setStoredThemeMode(mode)
    setThemeModeState(mode)
  }

  const setThemeTimeRange = (range) => {
    setStoredTimeRange(range)
    setThemeTimeRangeState(range)
  }

  useEffect(() => {
    const applyTheme = () => {
      const nextTheme = resolveTheme(themeMode, themeTimeRange)
      setResolvedTheme(nextTheme)
      document.documentElement.dataset.theme = nextTheme
    }
    applyTheme()
    const interval = themeMode === 'time' ? window.setInterval(applyTheme, 60_000) : null
    const media = window.matchMedia?.('(prefers-color-scheme: dark)')
    if (themeMode === 'system' && media) {
      media.addEventListener?.('change', applyTheme)
    }
    const onThemeChange = () => {
      setThemeMode(getStoredThemeMode())
      setThemeTimeRange(getStoredTimeRange())
    }
    window.addEventListener('themechange', onThemeChange)
    return () => {
      if (interval) window.clearInterval(interval)
      if (media) media.removeEventListener?.('change', applyTheme)
      window.removeEventListener('themechange', onThemeChange)
    }
  }, [themeMode, themeTimeRange])

  useEffect(() => {
    async function check() {
      try {
        const status = await api.getSetupStatus()
        if (status.setup_required) {
          setSetupRequired(true)
          setAuthChecked(true)
          return
        }
        try {
          await api.getConfig()
          setIsAuthenticated(true)
        } catch (_) {
          setIsAuthenticated(false)
        }
      } catch (e) {
        setError(e.message)
      }
      setAuthChecked(true)
    }
    check()
  }, [])

  const handleSetupSubmit = async (e) => {
    e.preventDefault()
    const password = passwordInput.trim()
    if (password.length < 8) {
      setError('密码至少需要 8 个字符')
      return
    }
    if (password !== passwordConfirm.trim()) {
      setError('两次输入的密码不一致')
      return
    }
    try {
      await api.setupPassword(password)
      setSetupRequired(false)
      setIsAuthenticated(true)
      setPasswordInput('')
      setPasswordConfirm('')
      setError(null)
    } catch (e) {
      setError(e.body || e.message)
    }
  }

  const handleLoginSubmit = async (e) => {
    e.preventDefault()
    try {
      await api.login(passwordInput)
      setIsAuthenticated(true)
      setPasswordInput('')
      setError(null)
    } catch (e) {
      setError('密码错误或登录失败')
    }
  }

  const handleTokenSubmit = async (e) => {
    e.preventDefault()
    api.setAuthToken(tokenInput.trim())
    setIsAuthenticated(true)
    setError(null)
  }

  const handleLogout = async () => {
    api.setAuthToken('')
    setTokenInput('')
    try {
      await api.logout()
    } catch (_) {}
    setIsAuthenticated(false)
  }

  return (
    <div className="app">
      {setupRequired ? (
        <SetupScreen error={error} passwordInput={passwordInput} setPasswordInput={setPasswordInput} passwordConfirm={passwordConfirm} setPasswordConfirm={setPasswordConfirm} onSubmit={handleSetupSubmit} />
      ) : (
        <>
          <AppShell
            isAuthenticated={isAuthenticated}
            onLogout={handleLogout}
            themeMode={themeMode}
            resolvedTheme={resolvedTheme}
            onThemeToggle={() => setThemeMode(resolvedTheme === 'dark' ? 'light' : 'dark')}
          />
          <Routes>
            <Route path="/" element={<UserPanel />} />
            <Route
              path="/admin"
              element={
                isAuthenticated
                  ? <AdminPanel onLogout={handleLogout} />
                  : <Navigate to="/admin/login" replace />
              }
            />
            <Route
              path="/admin/login"
              element={
                isAuthenticated
                  ? <Navigate to="/admin" replace />
                  : <AdminLogin
                      onLogin={handleLoginSubmit}
                      onTokenSubmit={handleTokenSubmit}
                      passwordInput={passwordInput}
                      setPasswordInput={setPasswordInput}
                      tokenInput={tokenInput}
                      setTokenInput={setTokenInput}
                      error={error}
                      setError={setError}
                    />
              }
            />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </>
      )}
    </div>
  )
}

function SetupScreen({ error, passwordInput, setPasswordInput, passwordConfirm, setPasswordConfirm, onSubmit }) {
  return (
    <>
      <AppShell isAuthenticated={false} onLogout={null} themeMode={null} resolvedTheme={null} onThemeToggle={null} />
      <div className="auth-overlay" role="dialog" aria-label="初始化设置">
        <div className="auth-card glass-card">
          <div className="auth-icon">
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
          </div>
          <h2 className="auth-title">初始化管理密码</h2>
          <p className="auth-desc">首次使用前请设置管理密码，用于访问管理面板</p>
          {error && <div className="auth-error" role="alert">{error}</div>}
          <form className="auth-form" onSubmit={onSubmit}>
            <div className="form-field">
              <label htmlFor="setup-password">管理密码</label>
              <input id="setup-password" type="password" value={passwordInput} onChange={e => setPasswordInput(e.target.value)} placeholder="至少 8 个字符" autoFocus />
            </div>
            <div className="form-field">
              <label htmlFor="setup-confirm">确认密码</label>
              <input id="setup-confirm" type="password" value={passwordConfirm} onChange={e => setPasswordConfirm(e.target.value)} placeholder="再次输入密码" />
            </div>
            <button type="submit" className="btn-primary">完成初始化</button>
          </form>
        </div>
      </div>
    </>
  )
}

function AdminLogin({ onLogin, onTokenSubmit, passwordInput, setPasswordInput, tokenInput, setTokenInput, error, setError }) {
  const [localError, setLocalError] = useState(error)

  useEffect(() => {
    setLocalError(error)
  }, [error])

  return (
    <div className="auth-overlay" role="dialog" aria-label="管理面板登录">
      <div className="auth-card glass-card">
        <div className="auth-icon">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
          </svg>
        </div>
        <h2 className="auth-title">管理面板登录</h2>
        <p className="auth-desc">输入管理密码或令牌以访问管理功能</p>
        {localError && <div className="auth-error" role="alert">{localError}</div>}
        <form className="auth-form" onSubmit={onLogin}>
          <div className="form-field">
            <label htmlFor="admin-password">管理密码</label>
            <input id="admin-password" type="password" value={passwordInput} onChange={e => setPasswordInput(e.target.value)} placeholder="输入管理密码" autoFocus />
          </div>
          <button type="submit" className="btn-primary">登录</button>
        </form>
        <div className="auth-divider"><span>或</span></div>
        <form className="auth-form" onSubmit={onTokenSubmit}>
          <div className="form-field">
            <label htmlFor="admin-token">兼容令牌</label>
            <input id="admin-token" type="password" value={tokenInput} onChange={e => setTokenInput(e.target.value)} placeholder="AI_MODEL_MONITOR_TOKEN" />
          </div>
          <button type="submit" className="btn-secondary">使用令牌</button>
        </form>
        <div className="auth-footer">
          <Link to="/" className="auth-link">返回用户面板</Link>
        </div>
      </div>
    </div>
  )
}

function AppShell({ isAuthenticated, onLogout, themeMode, resolvedTheme, onThemeToggle }) {
  const location = useLocation()
  const isAdmin = location.pathname.startsWith('/admin')

  return (
    <>
      <a className="skip-link" href="#main-content">跳到主要内容</a>
      <nav className="navbar" role="navigation" aria-label="主导航">
        <div className="nav-inner">
        <div className="nav-brand">
          <div className="nav-icon" aria-hidden="true">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="M5 12.55a11 11 0 0 1 14.08 0" />
              <path d="M1.42 9a16 16 0 0 1 21.16 0" />
              <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
              <line x1="12" y1="20" x2="12.01" y2="20" />
            </svg>
          </div>
          <span className="nav-title">AI Model Monitor</span>
        </div>

        <div className="nav-routes">
          <Link
            to="/"
            className={`nav-route ${!isAdmin ? 'active' : ''}`}
            aria-current={!isAdmin ? 'page' : undefined}
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M5 12.55a11 11 0 0 1 14.08 0" />
              <path d="M1.42 9a16 16 0 0 1 21.16 0" />
              <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
              <line x1="12" y1="20" x2="12.01" y2="20" />
            </svg>
            用户面板
          </Link>
          {isAuthenticated ? (
            <Link
              to="/admin"
              className={`nav-route ${isAdmin ? 'active' : ''}`}
              aria-current={isAdmin ? 'page' : undefined}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              </svg>
              管理面板
            </Link>
          ) : (
            <Link
              to="/admin/login"
              className={`nav-route ${isAdmin ? 'active' : ''}`}
              aria-current={isAdmin ? 'page' : undefined}
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                <path d="M7 11V7a5 5 0 0 1 10 0v4" />
              </svg>
              管理面板
            </Link>
          )}
        </div>

        {onThemeToggle && (
          <button
            className="theme-pill"
            onClick={onThemeToggle}
            title={`当前主题：${resolvedTheme === 'dark' ? '夜间' : '日间'}`}
            aria-label={`切换主题，当前为${resolvedTheme === 'dark' ? '夜间' : '日间'}模式`}
          >
            {resolvedTheme === 'dark' ? (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
              </svg>
            ) : (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <circle cx="12" cy="12" r="5" />
                <line x1="12" y1="1" x2="12" y2="3" />
                <line x1="12" y1="21" x2="12" y2="23" />
                <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
                <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
                <line x1="1" y1="12" x2="3" y2="12" />
                <line x1="21" y1="12" x2="23" y2="12" />
                <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
                <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
              </svg>
            )}
          </button>
        )}

        {isAuthenticated && onLogout && (
          <button className="nav-logout" onClick={onLogout} aria-label="退出登录">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
              <polyline points="16 17 21 12 16 7" />
              <line x1="21" y1="12" x2="9" y2="12" />
            </svg>
            退出
          </button>
        )}
        </div>
      </nav>
    </>
  )
}
