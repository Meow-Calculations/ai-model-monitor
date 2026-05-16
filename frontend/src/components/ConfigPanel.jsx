import { useState, useRef, useCallback, useEffect } from 'react'

const PROVIDER_TYPES = [
  'openai', 'anthropic', 'deepseek', 'google', 'ollama',
  'zhipu', 'moonshot', 'siliconflow', 'groq', 'openrouter',
  'nvidia', 'azure', 'xai', 'dashscope', 'volcengine',
  'minimax', 'kimi', 'modelscope',
]

const EMPTY_PROVIDER_FORM = {
  name: '',
  type: 'openai',
  api_endpoint: '',
  api_key: '',
  models: '',
  icon: '',
}

export default function ConfigPanel({
  config,
  onSave,
  onAddProvider,
  onRemoveProvider,
  onUpdateProvider,
  themeMode,
  resolvedTheme,
  themeTimeRange,
  onThemeModeChange,
  onThemeTimeRangeChange,
}) {
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState(EMPTY_PROVIDER_FORM)
  const [savingProvider, setSavingProvider] = useState(false)
  const [localConfig, setLocalConfig] = useState(null)
  const saveTimerRef = useRef(null)

  useEffect(() => {
    setLocalConfig(config)
  }, [config])

  useEffect(() => () => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
  }, [])

  const debouncedSave = useCallback((newConfig) => {
    setLocalConfig(newConfig)
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(() => onSave(newConfig), 600)
  }, [onSave])

  if (!config) return null

  const resetForm = () => {
    setForm(EMPTY_PROVIDER_FORM)
    setShowForm(false)
    setEditingId(null)
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    const provider = {
      ...form,
      models: form.models.split(/[,\n]+/).map(s => s.trim()).filter(Boolean),
    }
    if (!provider.name || !provider.api_endpoint || provider.models.length === 0) return

    setSavingProvider(true)
    try {
      if (editingId) {
        await onUpdateProvider(editingId, provider)
      } else {
        if (!provider.id) provider.id = provider.name.toLowerCase().replace(/\s+/g, '_').replace(/[^a-z0-9_]/g, '')
        await onAddProvider(provider)
      }
      resetForm()
    } finally {
      setSavingProvider(false)
    }
  }

  const handleEdit = (provider) => {
    setForm({
      name: provider.name || '',
      type: provider.type || 'openai',
      api_endpoint: provider.api_endpoint || '',
      api_key: provider.api_key || '',
      models: (provider.models || []).join('\n'),
      icon: provider.icon || '',
    })
    setEditingId(provider.id)
    setShowForm(true)
  }

  const handleRemove = async (provider) => {
    if (!window.confirm(`确认删除 Provider「${provider.name || provider.id}」？该操作会移除其模型配置。`)) return
    await onRemoveProvider(provider.id)
  }

  const handleGeneralChange = (key, value) => {
    const source = localConfig || config
    debouncedSave({ ...source, [key]: value })
  }

  const displayConfig = localConfig || config

  return (
    <div className="settings-stack">
      <ThemeSettings
        mode={themeMode}
        resolvedTheme={resolvedTheme}
        timeRange={themeTimeRange}
        onModeChange={onThemeModeChange}
        onTimeRangeChange={onThemeTimeRangeChange}
      />
      <GeneralSettings config={displayConfig} onChange={handleGeneralChange} />
      <ProviderSection
        providers={displayConfig.providers || []}
        showForm={showForm}
        editingId={editingId}
        form={form}
        saving={savingProvider}
        onToggleForm={() => { resetForm(); setShowForm(!showForm) }}
        onCancel={resetForm}
        onSubmit={handleSubmit}
        onFormChange={setForm}
        onEdit={handleEdit}
        onRemove={handleRemove}
      />
    </div>
  )
}

function ThemeSettings({ mode, resolvedTheme, timeRange, onModeChange, onTimeRangeChange }) {
  const modes = [
    { value: 'system', label: '跟随系统', description: '使用操作系统深浅色偏好' },
    { value: 'time', label: '按时间', description: '按夜间时间段自动切换' },
    { value: 'light', label: '日间', description: '固定浅色界面' },
    { value: 'dark', label: '夜间', description: '固定深色界面' },
  ]

  return (
    <section className="settings-card">
      <div className="settings-card-header">
        <div>
          <h2>主题设置</h2>
          <p>当前生效：{resolvedTheme === 'dark' ? '夜间主题' : '日间主题'}，偏好仅保存在当前浏览器</p>
        </div>
      </div>
      <div className="settings-card-body">
        <div className="theme-mode-grid">
          {modes.map(item => (
            <button
              key={item.value}
              type="button"
              className={`theme-mode-card ${mode === item.value ? 'active' : ''}`}
              onClick={() => onModeChange(item.value)}
              aria-pressed={mode === item.value}
              aria-label={`${item.label}：${item.description}`}
            >
              <strong>{item.label}</strong>
              <span>{item.description}</span>
            </button>
          ))}
        </div>
        {mode === 'time' && (
          <div className="form-grid compact">
            <FormField label="夜间开始" type="time" value={timeRange.darkStart} onChange={darkStart => onTimeRangeChange({ ...timeRange, darkStart })} />
            <FormField label="夜间结束" type="time" value={timeRange.darkEnd} onChange={darkEnd => onTimeRangeChange({ ...timeRange, darkEnd })} help="支持跨午夜，例如 18:00 到 07:00。" />
          </div>
        )}
      </div>
    </section>
  )
}

function GeneralSettings({ config, onChange }) {
  const fields = [
    { key: 'title', label: '看板标题', type: 'text', default: '模型连通性' },
    { key: 'timeout_seconds', label: '探测超时 (秒)', type: 'number', default: 30, min: 1 },
    { key: 'slow_threshold_ms', label: '慢响应阈值 (ms)', type: 'number', default: 8000, min: 1 },
    { key: 'concurrency', label: '全局最大并发', type: 'number', default: 3, min: 1 },
    { key: 'provider_concurrency', label: '单 Provider 并发', type: 'number', default: 1, min: 1 },
    { key: 'history_size', label: '历史条长度', type: 'number', default: 30, min: 1 },
    { key: 'stats_window_days', label: '统计窗口天数', type: 'number', default: 7, min: 1 },
    { key: 'probe_prompt', label: '探测提示词', type: 'text', default: '只回复 OK 两个字母。' },
    { key: 'probe_system_prompt', label: '探测系统提示词', type: 'text', default: '你是一个模型连通性探针。请只回复 OK，不要解释。' },
    { key: 'show_curve_chart', label: '显示延迟曲线', type: 'checkbox', default: true },
    { key: 'show_error_detail', label: '显示错误详情', type: 'checkbox', default: true },
    { key: 'auto_check_interval_seconds', label: '自动检测间隔 (秒, 0=关闭)', type: 'number', default: 0, min: 0 },
    { key: 'port', label: '服务端口', type: 'number', default: 8080, min: 1, max: 65535 },
  ]

  return (
    <section className="settings-card">
      <div className="settings-card-header">
        <div>
          <h2>通用设置</h2>
          <p>修改后自动保存并即时生效</p>
        </div>
      </div>
      <div className="settings-card-body form-grid">
        {fields.map(field => (
          <div key={field.key} className={`form-field${field.type === 'checkbox' ? ' form-field--toggle' : ''}`}>
            <label>{field.label}</label>
            {field.type === 'checkbox' ? (
              <label className="toggle-row">
                <input
                  type="checkbox"
                  className="toggle"
                  checked={config[field.key] ?? field.default}
                  onChange={e => onChange(field.key, e.target.checked)}
                />
              </label>
            ) : (
              <input
                type={field.type}
                value={config[field.key] ?? field.default}
                min={field.min}
                max={field.max}
                onChange={e => {
                  let value = field.type === 'number' ? Number(e.target.value) : e.target.value
                  if (field.type === 'number') {
                    if (Number.isNaN(value)) value = field.default
                    if (field.min !== undefined && value < field.min) value = field.min
                    if (field.max !== undefined && value > field.max) value = field.max
                  }
                  onChange(field.key, value)
                }}
                placeholder={String(field.default)}
              />
            )}
          </div>
        ))}
      </div>
    </section>
  )
}

function ProviderSection({ providers, showForm, editingId, form, saving, onToggleForm, onCancel, onSubmit, onFormChange, onEdit, onRemove }) {
  return (
    <section className="settings-card">
      <div className="settings-card-header">
        <div>
          <h2>Provider 与模型管理</h2>
          <p>添加、编辑或删除 LLM API Provider，并维护每个 Provider 的模型列表</p>
        </div>
        <button className="btn-soft success" onClick={onToggleForm}>{showForm ? '收起表单' : '+ 添加 Provider'}</button>
      </div>

      {showForm && (
        <ProviderForm
          form={form}
          editing={Boolean(editingId)}
          saving={saving}
          onSubmit={onSubmit}
          onCancel={onCancel}
          onChange={onFormChange}
        />
      )}

      <div className="settings-card-body">
        {providers.length === 0 ? (
          <div className="empty-settings-state">暂无 Provider，请点击上方按钮添加。</div>
        ) : (
          <div className="provider-list">
            {providers.map(provider => (
              <ProviderItem
                key={provider.id}
                provider={provider}
                onEdit={() => onEdit(provider)}
                onRemove={() => onRemove(provider)}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  )
}

function ProviderForm({ form, editing, saving, onSubmit, onCancel, onChange }) {
  return (
    <form className="provider-form" onSubmit={onSubmit}>
      <div className="form-grid compact">
        <FormField label="Provider 名称" value={form.name} onChange={name => onChange({ ...form, name })} placeholder="例: OpenAI" required />
        <div className="form-field">
          <label>类型</label>
          <select value={form.type} onChange={e => onChange({ ...form, type: e.target.value })}>
            {PROVIDER_TYPES.map(t => <option key={t} value={t}>{t}</option>)}
          </select>
        </div>
      </div>
      <FormField label="API Endpoint" value={form.api_endpoint} onChange={api_endpoint => onChange({ ...form, api_endpoint })} placeholder="例: https://api.openai.com/v1" required />
      <FormField label="API Key" value={form.api_key} onChange={api_key => onChange({ ...form, api_key })} placeholder="sk-..." type="password" help="编辑时保持 ******** 表示不修改；清空表示删除密钥。" />
      <FormField label="模型列表" value={form.models} onChange={models => onChange({ ...form, models })} placeholder="每行一个模型，或用逗号分隔" required multiline help="例：gpt-4o、gpt-4o-mini、claude-sonnet-4-6" />
      <FormField label="图标 URL (可选)" value={form.icon} onChange={icon => onChange({ ...form, icon })} placeholder="留空则使用默认图标" />
      <div className="form-actions">
        <button type="button" className="btn-soft" onClick={onCancel} disabled={saving}>取消</button>
        <button type="submit" className="btn-soft success" disabled={saving}>{saving ? '保存中...' : (editing ? '保存修改' : '添加 Provider')}</button>
      </div>
    </form>
  )
}

function ProviderItem({ provider, onEdit, onRemove }) {
  return (
    <div className="provider-item">
      <div className="provider-avatar">{(provider.name || 'P')[0].toUpperCase()}</div>
      <div className="provider-meta">
        <div className="provider-title-row">
          <strong>{provider.name || provider.id}</strong>
          <span>{provider.type}</span>
        </div>
        <div className="provider-detail">{provider.models?.length || 0} 个模型 · {provider.api_endpoint || '未配置 Endpoint'}</div>
        {provider.models?.length > 0 && (
          <div className="model-chip-row">
            {provider.models.slice(0, 6).map((model, i) => <span key={`${model}-${i}`}>{model}</span>)}
            {provider.models.length > 6 && <span>+{provider.models.length - 6}</span>}
          </div>
        )}
      </div>
      <div className="provider-actions">
        <button onClick={onEdit} className="btn-small">编辑</button>
        <button onClick={onRemove} className="btn-small danger">删除</button>
      </div>
    </div>
  )
}

function FormField({ label, value, onChange, placeholder, type = 'text', required, multiline, help }) {
  return (
    <div className="form-field">
      <label>{label}{required && ' *'}</label>
      {multiline ? (
        <textarea value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder} rows={4} required={required} />
      ) : (
        <input type={type} value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder} required={required} />
      )}
      {help && <span className="field-help">{help}</span>}
    </div>
  )
}
