import { getProviderIcon, getProviderColor } from '../api'

export default function Dashboard({ report, onProbe, probing }) {
  if (!report || !report.providers || report.providers.length === 0) {
    return (
      <div className="empty-state">
        <div className="empty-icon">
          <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M5 12.55a11 11 0 0 1 14.08 0" />
            <path d="M1.42 9a16 16 0 0 1 21.16 0" />
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
            <line x1="12" y1="20" x2="12.01" y2="20" />
          </svg>
        </div>
        <div className="empty-title">尚未检测</div>
        <div className="empty-desc">
          {report && report.overall_status === 'NO_MODELS'
            ? '未配置任何模型，请先在「配置管理」中添加 Provider 和模型'
            : '点击右上角「开始探测」按钮检测模型连通性'}
        </div>
      </div>
    )
  }

  const overallClass = report.overall_class || 'ok'

  return (
    <div>
      <header className="dashboard-header">
        <div>
          <h1 className="dashboard-title">{report.title || '模型连通性'}</h1>
          <div className="summary">
            <span className="pill ok"><span className="dot" />{report.ok_count} 正常</span>
            <span className="pill slow"><span className="dot" />{report.slow_count} 较慢</span>
            <span className="pill error"><span className="dot" />{report.error_count} 错误</span>
            <span className="pill">{report.provider_count} 个 Provider</span>
            <span className="pill">{report.total} 个模型</span>
          </div>
        </div>
        <div className="right-meta">
          <div className={`overall ${overallClass}`}>
            <span className="dot" />
            {report.overall_status}
          </div>
          <div className="meta-line">
            更新于 {report.generated_at} · 耗时 {report.elapsed_ms} ms
          </div>
          <div className="meta-line">
            全局并发 {report.global_concurrency} · 单 Provider {report.provider_concurrency} · 统计 {report.stats_window_days} 天 · 历史 {report.history_size} 次
          </div>
        </div>
      </header>

      <section className="provider-grid">
        {report.providers.map((provider) => (
          <ProviderCard key={provider.provider_id} provider={provider} historySize={report.history_size} />
        ))}
      </section>
    </div>
  )
}

function ProviderCard({ provider, historySize }) {
  const iconUrl = provider.provider_logo || getProviderIcon(provider.provider_type)
  const initial = (provider.provider_name || 'P')[0].toUpperCase()
  const bgColor = getProviderColor(provider.provider_type)

  return (
    <article className="provider-card">
      <header className="provider-head">
        <div className="provider-icon" style={iconUrl ? {} : { background: bgColor }}>
          {iconUrl ? (
            <img src={iconUrl} alt="" onError={(e) => { e.target.style.display = 'none'; e.target.nextElementSibling && (e.target.nextElementSibling.style.display = 'flex') }} />
          ) : null}
          <span style={{ display: iconUrl ? 'none' : 'flex', width: '100%', height: '100%', alignItems: 'center', justifyContent: 'center' }}>{initial}</span>
        </div>
        <div className="provider-info">
          <h2>{provider.provider_name}</h2>
          <p>{provider.provider_type} · {provider.provider_id} · {provider.model_count} models</p>
        </div>
        <div className={`provider-status ${provider.status}`}>{provider.status_label}</div>
      </header>

      <div className="models-section">
        {provider.results.map((model, idx) => (
          <ModelRow key={`${model.provider_id}-${model.model}`} model={model} historySize={historySize} />
        ))}
      </div>
    </article>
  )
}

function ModelRow({ model, historySize }) {
  const statusClass = model.status || 'ok'
  const availClass = `avail-${statusClass}`

  return (
    <div className="model-row">
      <div className="model-top">
        <div className="model-name">
          <span className={`model-dot ${statusClass}`} />
          <span>{model.model}</span>
        </div>
        <div className={`status-badge ${statusClass}`}>{model.status_label}</div>
      </div>

      <div className="metric-grid">
        <div className="metric">
          <label>当前延迟</label>
          <strong>{model.latency_ms} ms</strong>
        </div>
        <div className="metric">
          <label>24h平均</label>
          <strong>{model.avg_latency_24h}</strong>
        </div>
        <div className="metric">
          <label>可用性</label>
          <strong className={availClass}>{model.availability}</strong>
        </div>
        <div className="metric">
          <label>周成功次数</label>
          <strong className={availClass}>{model.weekly_success_text}</strong>
        </div>
      </div>

      {model.show_curve_chart && model.latency_curve && model.latency_curve.length > 1 && (
        <div className="curve-container">
          <svg className="curve-chart" viewBox="0 0 100 40" preserveAspectRatio="none">
            <defs>
              <linearGradient id={`curve-grad-${model.model.replace(/[^a-zA-Z0-9]/g, '')}`} x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stopColor="rgba(139, 92, 246, 0.4)" />
                <stop offset="100%" stopColor="rgba(139, 92, 246, 0.0)" />
              </linearGradient>
            </defs>
            {generateCurveSVG(model.latency_curve, model.model)}
          </svg>
          {model.time_labels && model.time_labels.length > 0 && (
            <div className="time-axis">
              {model.time_labels.map((label, i) => (
                <span key={i} style={{ left: `${label.x_pct}%` }}>{label.text}</span>
              ))}
            </div>
          )}
        </div>
      )}

      {model.history && model.history.length > 0 && (
        <div className="history-bars" title={`最近 ${historySize} 次检测`}>
          {model.history.map((status, i) => (
            <span key={i} className={`bar ${status}`} />
          ))}
        </div>
      )}

      {model.error && (
        <div className="error-text">{model.error}</div>
      )}
    </div>
  )
}

function generateCurveSVG(latencies, modelKey) {
  if (!latencies || latencies.length < 2) return null

  const maxLat = Math.max(...latencies.filter(l => l > 0), 1000)
  const n = latencies.length
  const width = 100
  const height = 40
  const step = width / (n - 1)

  const points = latencies.map((lat, i) => ({
    x: i * step,
    y: height - (lat / maxLat * height * 0.9)
  }))

  let areaPath = `M 0,${height} `
  let linePath = `M ${points[0].x.toFixed(1)},${points[0].y.toFixed(1)} `

  for (let i = 1; i < points.length; i++) {
    const prev = points[i - 1]
    const curr = points[i]
    const cx1 = prev.x + step / 2
    const cx2 = curr.x - step / 2
    linePath += `C ${cx1.toFixed(1)},${prev.y.toFixed(1)} ${cx2.toFixed(1)},${curr.y.toFixed(1)} ${curr.x.toFixed(1)},${curr.y.toFixed(1)} `
  }

  areaPath += linePath.replace(/^M /, 'L ') + ` L ${width},${height} Z`

  const safeId = `curve-grad-${modelKey.replace(/[^a-zA-Z0-9]/g, '')}`

  return (
    <>
      <path d={areaPath} fill={`url(#${safeId})`} />
      <path d={linePath} fill="none" stroke="#8b5cf6" strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </>
  )
}
