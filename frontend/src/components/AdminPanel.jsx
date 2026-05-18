import { useState, useEffect, useCallback, lazy, Suspense } from 'react'
import * as api from '../api'
import { getExportDownloadURL } from '../api'
import {
  getStoredThemeMode,
  getStoredTimeRange,
  setStoredThemeMode,
  setStoredTimeRange,
  resolveTheme,
} from '../theme'
import Dashboard from './Dashboard'

const ConfigPanel = lazy(() => import('./ConfigPanel'))
const AlertPanel = lazy(() => import('./AlertPanel'))

export default function AdminPanel() {
  const [tab, setTab] = useState('dashboard')
  const [report, setReport] = useState(null)
  const [config, setConfig] = useState(null)
  const [probing, setProbing] = useState(false)
  const [error, setError] = useState(null)

  const handleAPIError = useCallback((e) => {
    if (e.status === 401) {
      setError('认证失败，请重新登录')
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
      if (e.status !== 401) {
        setError(e.message)
      }
    }
  }, [])

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
      if (!provider.models || provider.models.length === 0) {
        await api.fetchProviderModels(provider.id || provider.name).catch(() => {})
      }
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
      if (!provider.models || provider.models.length === 0) {
        await api.fetchProviderModels(id).catch(() => {})
      }
      await loadConfig()
    } catch (e) {
      handleAPIError(e)
    }
  }

  return (
    <div className="panel admin-panel">
      <div className="panel-header">
        <div className="panel-header-inner">
          <div className="panel-brand">
            <div className="panel-logo admin-logo" aria-hidden="true">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              </svg>
            </div>
            <div>
              <h1 className="panel-title">管理面板</h1>
              <p className="panel-subtitle">配置管理、探测触发与系统设置</p>
            </div>
          </div>
          <div className="panel-meta">
            <div className="panel-tabs" role="tablist" aria-label="管理面板导航">
              <button
                className={`panel-tab ${tab === 'dashboard' ? 'active' : ''}`}
                onClick={() => setTab('dashboard')}
                role="tab"
                aria-selected={tab === 'dashboard'}
                aria-controls="admin-tab-dashboard"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <rect x="3" y="3" width="7" height="7" />
                  <rect x="14" y="3" width="7" height="7" />
                  <rect x="14" y="14" width="7" height="7" />
                  <rect x="3" y="14" width="7" height="7" />
                </svg>
                状态看板
              </button>
              <button
                className={`panel-tab ${tab === 'config' ? 'active' : ''}`}
                onClick={() => setTab('config')}
                role="tab"
                aria-selected={tab === 'config'}
                aria-controls="admin-tab-config"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <circle cx="12" cy="12" r="3" />
                  <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                </svg>
                配置管理
              </button>
              <button
                className={`panel-tab ${tab === 'alert' ? 'active' : ''}`}
                onClick={() => setTab('alert')}
                role="tab"
                aria-selected={tab === 'alert'}
                aria-controls="admin-tab-alert"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
                  <path d="M13.73 21a2 2 0 0 1-3.46 0" />
                </svg>
                告警管理
              </button>
            </div>
            <div className="panel-actions">
              <button
                className="btn-probe"
                onClick={handleProbe}
                disabled={probing}
                aria-label={probing ? '正在探测' : '开始探测'}
              >
                {probing ? (
                  <>
                    <span className="spinner" aria-hidden="true" />
                    检测中...
                  </>
                ) : (
                  <>
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                      <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
                    </svg>
                    开始探测
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      </div>

      {error && (
        <div className="error-banner" role="alert">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="error-close" aria-label="关闭错误提示">&times;</button>
        </div>
      )}

      <main id="main-content" className="panel-content" tabIndex="-1">
        <div
          id="admin-tab-dashboard"
          role="tabpanel"
          style={{ display: tab === 'dashboard' ? 'block' : 'none' }}
        >
          <Dashboard report={report} onProbe={handleProbe} probing={probing} />
        </div>
        <div
          id="admin-tab-config"
          role="tabpanel"
          style={{ display: tab === 'config' ? 'block' : 'none' }}
        >
          <Suspense fallback={<div className="panel-content" style={{textAlign:'center',padding:'48px',color:'var(--text-muted)'}}>配置加载中...</div>}>
          <ConfigPanel
            config={config}
            onSave={handleSaveConfig}
            onAddProvider={handleAddProvider}
            onRemoveProvider={handleRemoveProvider}
            onUpdateProvider={handleUpdateProvider}
            themeMode={getStoredThemeMode()}
            resolvedTheme={resolveTheme(getStoredThemeMode(), getStoredTimeRange())}
            themeTimeRange={getStoredTimeRange()}
            onThemeModeChange={(mode) => { setStoredThemeMode(mode); window.dispatchEvent(new CustomEvent('themechange')) }}
            onThemeTimeRangeChange={(range) => { setStoredTimeRange(range); window.dispatchEvent(new CustomEvent('themechange')) }}
          />
          </Suspense>
        </div>
        <div
          id="admin-tab-alert"
          role="tabpanel"
          style={{ display: tab === 'alert' ? 'block' : 'none' }}
        >
          <Suspense fallback={<div className="panel-content" style={{textAlign:'center',padding:'48px',color:'var(--text-muted)'}}>加载中...</div>}>
            <AlertPanel />
          </Suspense>
        </div>
      </main>
    </div>
  )
}
