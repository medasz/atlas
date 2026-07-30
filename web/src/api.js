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
  getHostDetail: (ip) => request('GET', '/api/hosts/' + encodeURIComponent(ip) + '/detail'),
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
}
