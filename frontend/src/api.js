const API_BASE = '/api'
const ADMIN_API_BASE = '/api/admin'
const TOKEN_KEY = 'ai_model_monitor_token'

export function getAuthToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setAuthToken(token) {
  if (token) {
    localStorage.setItem(TOKEN_KEY, token)
  } else {
    localStorage.removeItem(TOKEN_KEY)
  }
}

async function request(path, options = {}, base = API_BASE) {
  const url = `${base}${path}`
  const token = getAuthToken()
  const headers = { 'Content-Type': 'application/json', ...options.headers }
  if (token) {
    headers.Authorization = `Bearer ${token}`
  }
  const res = await fetch(url, {
    ...options,
    headers,
    credentials: 'include',
  })
  if (!res.ok) {
    const text = await res.text()
    const err = new Error(`API error ${res.status}: ${text}`)
    err.status = res.status
    err.body = text
    throw err
  }
  return res.json()
}

function adminRequest(path, options = {}) {
  return request(path, options, ADMIN_API_BASE)
}

export function getSetupStatus() {
  return request('/setup-status')
}

export function setupPassword(password) {
  return request('/setup', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function login(password) {
  return request('/login', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })
}

export function logout() {
  return request('/logout', { method: 'POST' })
}

export function getConfig() {
  return adminRequest('/config')
}

export function updateConfig(config) {
  return adminRequest('/config', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

export function getProviders() {
  return adminRequest('/providers')
}

export function addProvider(provider) {
  return adminRequest('/providers', {
    method: 'POST',
    body: JSON.stringify(provider),
  })
}

export function updateProvider(id, provider) {
  return adminRequest(`/providers?id=${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(provider),
  })
}

export function removeProvider(id) {
  return adminRequest(`/providers?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function runProbe() {
  return adminRequest('/probe', { method: 'POST' })
}

export function getStatus() {
  return request('/status')
}

export function getHistory(key) {
  const params = key ? `?key=${encodeURIComponent(key)}` : ''
  return request(`/history${params}`)
}

export function getAlertRules() {
  return adminRequest('/alerts/rules')
}

export function saveAlertRule(rule) {
  return adminRequest('/alerts/rules', {
    method: 'POST',
    body: JSON.stringify(rule),
  })
}

export function deleteAlertRule(id) {
  return adminRequest(`/alerts/rules?id=${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

export function getAlertEvents() {
  return adminRequest('/alerts/events')
}

export function getExportDownloadURL(format) {
  return `${ADMIN_API_BASE}/export/${format}`
}

// CDN icons with fallback to inline SVG
const PROVIDER_ICONS = {
  openai: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/openai.svg',
  azure: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/azure.svg',
  xai: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/xai.svg',
  anthropic: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/anthropic.svg',
  ollama: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/ollama.svg',
  google: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/gemini-color.svg',
  deepseek: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/deepseek.svg',
  modelscope: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/modelscope.svg',
  zhipu: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/zhipu.svg',
  nvidia: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/nvidia-color.svg',
  siliconflow: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/siliconcloud.svg',
  moonshot: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi.svg',
  kimi: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/kimi.svg',
  groq: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/groq.svg',
  openrouter: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/openrouter.svg',
  minimax: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/minimax.svg',
  volcengine: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/volcengine.svg',
  dashscope: 'https://cdn.jsdelivr.net/npm/@lobehub/icons-static-svg@latest/icons/alibabacloud-color.svg',
}

// Color map for fallback initials when CDN is unavailable
const PROVIDER_COLORS = {
  openai: '#10a37f',
  azure: '#0078d4',
  xai: '#000000',
  anthropic: '#d4a574',
  ollama: '#626262',
  google: '#4285f4',
  deepseek: '#4d6bfe',
  modelscope: '#6b5ce7',
  zhipu: '#3b68d6',
  nvidia: '#76b900',
  siliconflow: '#5c6bc0',
  moonshot: '#6c5ce7',
  kimi: '#6c5ce7',
  groq: '#f55036',
  openrouter: '#6d28d9',
  minimax: '#2563eb',
  volcengine: '#ff6a00',
  dashscope: '#ff6a00',
}

export function getProviderIcon(type) {
  if (!type) return ''
  return PROVIDER_ICONS[type.toLowerCase()] || ''
}

export function getProviderColor(type) {
  if (!type) return '#64748b'
  return PROVIDER_COLORS[type.toLowerCase()] || '#64748b'
}