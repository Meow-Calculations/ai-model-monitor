import { useState, useEffect, useCallback } from 'react'
import * as api from './api'
import {
  getStoredThemeMode,
  getStoredTimeRange,
  setStoredThemeMode,
  setStoredTimeRange,
  resolveTheme,
} from './theme'
import Dashboard from './components/Dashboard'
import ConfigPanel from './components/ConfigPanel'
import './App.css'

const TAB_DASHBOARD = 'dashboard'
const TAB_CONFIG = 'config'

export default function App() {
  const [tab, setTab] = useState(TAB_DASHBOARD)
  const [report, setReport] = useState(null)
  const [config, setConfig] = useState(null)
  const [probing, setProbing] = useState(false)
  const [error, setError] = useState(null)
  const [authRequired, setAuthRequired] = useState(false)
  const [tokenInput, setTokenInput] = useState(api.getAuthToken())
  const [themeMode, setThemeModeState] = useState(getStoredThemeMode)
  const [themeTimeRange, setThemeTimeRangeState] = useState(getStoredTimeRange)
  const [resolvedTheme, setResolvedTheme] = useState(() => resolveTheme(getStoredThemeMode(), getStoredTimeRange()))

  const handleAPIError = useCallback((e) => {
    if (e.status === 401) {
      setAuthRequired(true)
      setError('管理接口未授权，请确认服务端已设置 AI_MODEL_MONITOR_TOKEN，并输入正确令牌')
    } else {
      setError(e.message)
    }
  }, [])

  const loadStatus = useCallback(async () => {
    try {
      const data = await api.getStatus()
      if (data.providers) {
        setReport(data)
      }
    } catch (e) {
      if (e.status === 401) setAuthRequired(true)
    }
  }, [])

  const handleTokenSubmit = async (e) => {
    e.preventDefault()
    api.setAuthToken(tokenInput.trim())
    setAuthRequired(false)
    setError(null)
    await loadStatus()
    await loadConfig()
  }

  const handleClearToken = () => {
    api.setAuthToken('')
    setTokenInput('')
    setAuthRequired(true)
    setConfig(null)
    setReport(null)
  }

  const loadConfig = useCallback(async () => {
    try {
      const data = await api.getConfig()
      setConfig(data)
    } catch (e) {
      handleAPIError(e)
    }
  }, [handleAPIError])

  useEffect(() => {
    loadStatus()
    loadConfig()
  }, [loadStatus, loadConfig])

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

    return () => {
      if (interval) window.clearInterval(interval)
      if (media) media.removeEventListener?.('change', applyTheme)
    }
  }, [themeMode, themeTimeRange])

  const setThemeMode = (mode) => {
    setStoredThemeMode(mode)
    setThemeModeState(mode)
  }

  const setThemeTimeRange = (range) => {
    setStoredTimeRange(range)
    setThemeTimeRangeState(range)
  }

  const handleProbe = async () => {
    setProbing(true)
    setError(null)
    try {
      const data = await api.runProbe()
      if (data.providers) {
        setReport(data)
      } else if (data.status === 'already_probing') {
        setError('探测正在进行中，请稍后')
      } else if (data.status === 'NO_MODELS') {
        setError('未配置任何模型，请先在「配置管理」中添加 Provider 和模型')
        setReport(data)
      }
    } catch (e) {
      handleAPIError(e)
    } finally {
      setProbing(false)
    }
  }

  const handleSaveConfig = async (newConfig) => {
    try {
      const saved = await api.updateConfig(newConfig)
      setConfig(saved)
    } catch (e) {
      handleAPIError(e)
    }
  }

  const handleAddProvider = async (provider) => {
    try {
      await api.addProvider(provider)
      await loadConfig()
    } catch (e) {
      handleAPIError(e)
    }
  }

  const handleRemoveProvider = async (id) => {
    try {
      await api.removeProvider(id)
      await loadConfig()
    } catch (e) {
      handleAPIError(e)
    }
  }

  const handleUpdateProvider = async (id, provider) => {
    try {
      await api.updateProvider(id, provider)
      await loadConfig()
    } catch (e) {
      handleAPIError(e)
    }
  }

  return (
    <div className="app">
      <nav className="navbar">
        <div className="nav-brand">
          <div className="nav-icon">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="M5 12.55a11 11 0 0 1 14.08 0" />
              <path d="M1.42 9a16 16 0 0 1 21.16 0" />
              <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
              <line x1="12" y1="20" x2="12.01" y2="20" />
            </svg>
          </div>
          <span className="nav-title">AI Model Monitor</span>
        </div>
        <div className="nav-tabs">
          <button
            className={`nav-tab ${tab === TAB_DASHBOARD ? 'active' : ''}`}
            onClick={() => setTab(TAB_DASHBOARD)}
          >
            状态看板
          </button>
          <button
            className={`nav-tab ${tab === TAB_CONFIG ? 'active' : ''}`}
            onClick={() => setTab(TAB_CONFIG)}
          >
            配置管理
          </button>
        </div>
        <div className="nav-actions">
          <button
            className="theme-pill"
            onClick={() => setThemeMode(resolvedTheme === 'dark' ? 'light' : 'dark')}
            title={`当前主题：${resolvedTheme === 'dark' ? '夜间' : '日间'}`}
          >
            {resolvedTheme === 'dark' ? '夜间' : '日间'}
          </button>
          {api.getAuthToken() && (
            <button className="nav-token-clear" onClick={handleClearToken}>清除令牌</button>
          )}
          <button
            className="btn-probe"
            onClick={handleProbe}
            disabled={probing || authRequired}
          >
            {probing ? (
              <>
                <span className="spinner" />
                检测中...
              </>
            ) : (
              <>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
                </svg>
                开始探测
              </>
            )}
          </button>
        </div>
      </nav>

      {error && (
        <div className="error-banner">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="error-close">&times;</button>
        </div>
      )}

      {authRequired && (
        <form className="auth-panel" onSubmit={handleTokenSubmit}>
          <label>访问令牌</label>
          <input
            type="password"
            value={tokenInput}
            onChange={e => setTokenInput(e.target.value)}
            placeholder="输入 AI_MODEL_MONITOR_TOKEN"
            autoFocus
          />
          <button type="submit">保存并重试</button>
        </form>
      )}

      <main className="main-content">
        {tab === TAB_DASHBOARD ? (
          <Dashboard report={report} onProbe={handleProbe} probing={probing} />
        ) : (
          <ConfigPanel
            config={config}
            onSave={handleSaveConfig}
            onAddProvider={handleAddProvider}
            onRemoveProvider={handleRemoveProvider}
            onUpdateProvider={handleUpdateProvider}
            themeMode={themeMode}
            resolvedTheme={resolvedTheme}
            themeTimeRange={themeTimeRange}
            onThemeModeChange={setThemeMode}
            onThemeTimeRangeChange={setThemeTimeRange}
          />
        )}
      </main>
    </div>
  )
}
