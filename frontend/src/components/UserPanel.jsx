import { useState, useEffect, useCallback, useRef } from 'react'
import * as api from '../api'
import Dashboard from './Dashboard'

export default function UserPanel() {
  const [report, setReport] = useState(null)
  const [error, setError] = useState(null)
  const esRef = useRef(null)
  const pollingRef = useRef(null)

  const loadStatus = useCallback(async () => {
    try {
      const data = await api.getStatus()
      if (data.providers) {
        setReport(data)
      }
      setError(null)
    } catch (e) {
      if (e.status !== 401) {
        setError(e.message)
      }
    }
  }, [])

  useEffect(() => {
    loadStatus()

    if (import.meta.env.DEV) {
      pollingRef.current = setInterval(loadStatus, 30_000)
    } else {
      const es = new EventSource('/api/events')
      esRef.current = es

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data)
          if (data && data.providers) {
            setReport(data)
            setError(null)
          }
        } catch (_) {
          console.warn('SSE: failed to parse event data')
        }
      }

      es.onerror = () => {
        es.close()
        esRef.current = null
        if (!pollingRef.current) {
          pollingRef.current = setInterval(loadStatus, 30_000)
        }
      }
    }

    return () => {
      if (esRef.current) {
        esRef.current.close()
        esRef.current = null
      }
      if (pollingRef.current) {
        clearInterval(pollingRef.current)
        pollingRef.current = null
      }
    }
  }, [loadStatus])

  return (
    <div className="panel user-panel">
      <div className="panel-header">
        <div className="panel-header-inner">
          <div className="panel-brand">
            <div className="panel-logo" aria-hidden="true">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M5 12.55a11 11 0 0 1 14.08 0" />
                <path d="M1.42 9a16 16 0 0 1 21.16 0" />
                <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
                <line x1="12" y1="20" x2="12.01" y2="20" />
              </svg>
            </div>
            <div>
              <h1 className="panel-title">模型状态监控</h1>
              <p className="panel-subtitle">实时查看 AI 模型服务连通性与性能指标</p>
            </div>
          </div>
          <div className="panel-meta">
            {report && (
              <>
                <span className="meta-chip" role="status" aria-live="polite">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 6 12 12 16 14" />
                  </svg>
                  更新于 {report.generated_at}
                </span>
                <span className="meta-chip">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
                  </svg>
                  耗时 {report.elapsed_ms} ms
                </span>
              </>
            )}
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
        <Dashboard report={report} onProbe={null} probing={false} readOnly />
      </main>
    </div>
  )
}
