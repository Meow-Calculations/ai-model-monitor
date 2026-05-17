import { useState, useEffect } from 'react'
import * as api from '../api'

const EMPTY_RULE = {
  name: '',
  provider_id: '',
  model_pattern: '',
  metric_type: 'latency',
  condition: 'gt',
  threshold: 8000,
  duration_min: 0,
  notify_channels: [],
  webhook_url: '',
  enabled: true,
}

export default function AlertPanel() {
  const [rules, setRules] = useState([])
  const [events, setEvents] = useState([])
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState(EMPTY_RULE)
  const [tab, setTab] = useState('rules')
  const [error, setError] = useState(null)

  useEffect(() => {
    loadRules()
    loadEvents()
  }, [])

  const loadRules = async () => {
    try {
      setRules(await api.getAlertRules())
    } catch (e) {
      setError(e.body || e.message)
    }
  }

  const loadEvents = async () => {
    try {
      setEvents(await api.getAlertEvents())
    } catch (_) {}
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!form.name || !form.metric_type || !form.condition) return
    try {
      await api.saveAlertRule(form)
      setShowForm(false)
      setForm(EMPTY_RULE)
      await loadRules()
    } catch (e) {
      setError(e.body || e.message)
    }
  }

  const handleDelete = async (id) => {
    if (!window.confirm('确认删除此告警规则？')) return
    try {
      await api.deleteAlertRule(id)
      await loadRules()
    } catch (e) {
      setError(e.body || e.message)
    }
  }

  return (
    <div className="settings-stack">
      <section className="settings-card">
        <div className="settings-card-header">
          <div>
            <h2>告警管理</h2>
            <p>配置告警规则并查看告警事件</p>
          </div>
          <div style={{display:'flex',gap:'8px'}}>
            <button className={`btn-soft ${tab==='rules'?'success':''}`} onClick={() => setTab('rules')}>告警规则</button>
            <button className={`btn-soft ${tab==='events'?'success':''}`} onClick={() => setTab('events')}>告警事件</button>
          </div>
        </div>

        {error && <div style={{padding:'0 24px',marginTop:'12px'}}><div className="auth-error">{error}<button onClick={()=>setError(null)} style={{float:'right',background:'none',border:'none',color:'inherit',cursor:'pointer',fontSize:'18px'}}>&times;</button></div></div>}

        {tab === 'rules' && (
          <div className="settings-card-body">
            <div style={{display:'flex',justifyContent:'flex-end',marginBottom:'16px'}}>
              <button className="btn-soft success" onClick={() => { setForm(EMPTY_RULE); setShowForm(!showForm) }}>
                {showForm ? '收起' : '+ 添加规则'}
              </button>
            </div>

            {showForm && (
              <form className="provider-form" onSubmit={handleSubmit}>
                <div className="form-grid compact">
                  <div className="form-field">
                    <label>规则名称 *</label>
                    <input value={form.name} onChange={e => setForm({...form, name: e.target.value})} placeholder="例: 延迟过高告警" required />
                  </div>
                  <div className="form-field">
                    <label>指标类型 *</label>
                    <select value={form.metric_type} onChange={e => setForm({...form, metric_type: e.target.value})}>
                      <option value="latency">延迟 (ms)</option>
                      <option value="error_count">错误次数</option>
                      <option value="availability">可用性 (%)</option>
                    </select>
                  </div>
                </div>
                <div className="form-grid compact">
                  <div className="form-field">
                    <label>条件</label>
                    <select value={form.condition} onChange={e => setForm({...form, condition: e.target.value})}>
                      <option value="gt">大于 (&gt;)</option>
                      <option value="gte">大于等于 (&gt;=)</option>
                      <option value="lt">小于 (&lt;)</option>
                      <option value="lte">小于等于 (&lt;=)</option>
                    </select>
                  </div>
                  <div className="form-field">
                    <label>阈值</label>
                    <input type="number" value={form.threshold} onChange={e => setForm({...form, threshold: Number(e.target.value)})} required />
                  </div>
                </div>
                <div className="form-grid compact">
                  <div className="form-field">
                    <label>Provider ID (留空=全部)</label>
                    <input value={form.provider_id} onChange={e => setForm({...form, provider_id: e.target.value})} placeholder="留空则匹配所有 Provider" />
                  </div>
                  <div className="form-field">
                    <label>模型匹配模式 (留空=全部)</label>
                    <input value={form.model_pattern} onChange={e => setForm({...form, model_pattern: e.target.value})} placeholder="支持通配符 *，例: gpt-4*" />
                  </div>
                </div>
                <div className="form-grid compact">
                  <div className="form-field">
                    <label>持续时间 (分钟)</label>
                    <input type="number" value={form.duration_min} onChange={e => setForm({...form, duration_min: Number(e.target.value)})} placeholder="0=立即触发" />
                  </div>
                  <div className="form-field">
                    <label>Webhook URL</label>
                    <input value={form.webhook_url} onChange={e => setForm({...form, webhook_url: e.target.value})} placeholder="支持 钉钉/飞书/Slack webhook" />
                  </div>
                </div>
                <div className="form-field" style={{flexDirection:'row',alignItems:'center',gap:'12px'}}>
                  <label style={{margin:0}}>启用</label>
                  <input type="checkbox" className="toggle" checked={form.enabled} onChange={e => setForm({...form, enabled: e.target.checked})} />
                </div>
                <div className="form-actions">
                  <button type="button" className="btn-soft" onClick={() => setShowForm(false)}>取消</button>
                  <button type="submit" className="btn-soft success">保存规则</button>
                </div>
              </form>
            )}

            {rules.length === 0 && !showForm ? (
              <div className="empty-settings-state">暂无告警规则，请点击上方按钮添加。</div>
            ) : (
              <div className="provider-list">
                {rules.map(rule => (
                  <div key={rule.id} className="provider-item">
                    <div className="provider-meta">
                      <div className="provider-title-row">
                        <strong>{rule.name}</strong>
                        <span style={{color: rule.enabled ? 'var(--color-ok)' : 'var(--text-muted)'}}>{rule.enabled ? '启用' : '禁用'}</span>
                      </div>
                      <div className="provider-detail">
                        {rule.metric_type === 'latency' ? '延迟' : rule.metric_type === 'error_count' ? '错误次数' : '可用性'}
                        {' '}{rule.condition === 'gt' ? '>' : rule.condition === 'gte' ? '>=' : rule.condition === 'lt' ? '<' : rule.condition === 'lte' ? '<=' : '='}{' '}{rule.threshold}
                        {rule.metric_type === 'latency' ? ' ms' : rule.metric_type === 'availability' ? '%' : ''}
                        {rule.provider_id ? ` · Provider: ${rule.provider_id}` : ''}
                        {rule.model_pattern ? ` · 模型: ${rule.model_pattern}` : ''}
                        {rule.webhook_url && ' · Webhook 已配置'}
                      </div>
                    </div>
                    <div className="provider-actions">
                      <button onClick={() => handleDelete(rule.id)} className="btn-small danger">删除</button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {tab === 'events' && (
          <div className="settings-card-body">
            {events.length === 0 ? (
              <div className="empty-settings-state">暂无告警事件。</div>
            ) : (
              <div className="provider-list">
                {events.map(event => (
                  <div key={event.id} className="provider-item">
                    <div className="provider-meta">
                      <div className="provider-title-row">
                        <strong>{event.rule_name}</strong>
                        <span style={{
                          color: event.status === 'firing' ? 'var(--color-error)' : 'var(--color-ok)',
                          background: event.status === 'firing' ? 'var(--color-error-bg)' : 'var(--color-ok-bg)',
                          padding: '2px 8px', borderRadius: 'var(--radius-full)',
                          fontSize: '11px', fontWeight: 800
                        }}>
                          {event.status === 'firing' ? '触发中' : '已恢复'}
                        </span>
                      </div>
                      <div className="provider-detail">
                        {event.provider_id}/{event.model}
                        {' · '}值: {event.actual_value}{event.metric_type === 'latency' ? ' ms' : event.metric_type === 'availability' ? '%' : ''}
                        {' · '}阈值: {event.threshold}
                        {' · '}{event.triggered_at}
                        {event.resolved_at ? ` · 恢复: ${event.resolved_at}` : ''}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </section>
    </div>
  )
}
