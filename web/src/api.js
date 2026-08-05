// 统一 API 调用封装：携带凭据，401 跳转登录
const BASE = ''

async function request(method, path, body) {
  const opts = {
    method,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' }
  }
  if (body !== undefined) opts.body = JSON.stringify(body)
  const resp = await fetch(BASE + path, opts)
  if (resp.status === 401) {
    localStorage.removeItem('atlas_authed')
    if (location.hash !== '#/login') location.hash = '#/login'
    throw new Error('unauthorized')
  }
  const text = await resp.text()
  const data = text ? JSON.parse(text) : {}
  if (!resp.ok) throw new Error(data.error || resp.statusText)
  return data
}

export const api = {
  login: (password) => request('POST', '/api/login', { password }),
  logout: () => request('POST', '/api/logout'),
  searchAssets: (q, type, page = 1, pageSize = 20, aggregated = false) =>
    request('GET', '/api/assets?q=' + encodeURIComponent(q || '') + '&type=' + (type || '') +
      '&page=' + page + '&page_size=' + pageSize + (aggregated ? '&aggregated=true' : '')),
  getHost: (ip) => request('GET', '/api/hosts/' + encodeURIComponent(ip)),
  getHostAggregate: (ip) => request('GET', '/api/hosts/' + encodeURIComponent(ip) + '/aggregate'),
  listHostPorts: (ip, page = 1, pageSize = 50, state = '', sort = 'port_asc') => {
    const params = new URLSearchParams({ page: String(page), page_size: String(pageSize), sort })
    if (state) params.set('state', state)
    return request('GET', '/api/hosts/' + encodeURIComponent(ip) + '/ports?' + params.toString())
  },
  getHostPort: (ip, port) => request('GET', '/api/hosts/' + encodeURIComponent(ip) + '/ports/' + encodeURIComponent(port)),
  getHostDetail: (ip) => request('GET', '/api/hosts/' + encodeURIComponent(ip) + '/detail'),
  deleteAsset: ({ ip, port, domain }) => {
    const params = new URLSearchParams()
    if (ip) params.set('ip', ip)
    if (port) params.set('port', String(port))
    if (domain) params.set('domain', domain)
    return request('DELETE', '/api/assets?' + params.toString())
  },
  deleteHostAssets: (ip) => request('DELETE', '/api/hosts/' + encodeURIComponent(ip)),
  listTasks: () => request('GET', '/api/tasks'),
  getTask: (id) => request('GET', '/api/tasks/' + encodeURIComponent(id)),
  createTask: (payload) => request('POST', '/api/tasks', payload),
  resumeTask: (id) => request('POST', '/api/tasks/' + encodeURIComponent(id) + '/resume'),
  pauseTask: (id) => request('POST', '/api/tasks/' + encodeURIComponent(id) + '/pause'),
  listBlacklist: () => request('GET', '/api/blacklist'),
  addBlacklist: (item) => request('POST', '/api/blacklist', item),
  deleteBlacklist: (type, value) =>
    request('DELETE', '/api/blacklist?type=' + encodeURIComponent(type) + '&value=' + encodeURIComponent(value)),
  reloadFingerprint: () => request('POST', '/api/fingerprint/reload'),
  getConfig: () => request('GET', '/api/config'),
  updateConfig: (payload) => request('PUT', '/api/config', payload),
  getInterfaces: () => request('GET', '/api/config/interfaces'),
  listVulns: (asset) =>
    request('GET', '/api/vulns' + (asset ? '?asset=' + encodeURIComponent(asset) : '')),
  listTemplates: () => request('GET', '/api/templates'),
  addTemplate: (content) => request('POST', '/api/templates', { content }),
  deleteTask: (id) => request('DELETE', '/api/tasks/' + encodeURIComponent(id)),
  getAuditLogs: (q = '', page = 1, pageSize = 20) =>
    request('GET', '/api/audit/logs?q=' + encodeURIComponent(q || '') + '&page=' + page + '&page_size=' + pageSize),
}
