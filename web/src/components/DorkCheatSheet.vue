<template>
  <section class="dork-cheat" :class="{ open: embedded || expanded, embedded }">
    <!-- 嵌入式（弹层）模式：精简头部 + 关闭按钮，主体常驻展开 -->
    <div v-if="embedded" class="dc-embed-head">
      <span class="dc-icon" v-html="icons.book"></span>
      <h3 class="dc-title">检索语法手册</h3>
      <button class="dc-close" type="button" @click="emit('close')" title="关闭">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
        </svg>
      </button>
    </div>
    <!-- 独立模式：可点击头部展开/收起 -->
    <header v-else class="dc-header" @click="expanded = !expanded">
      <span class="dc-icon" v-html="icons.book"></span>
      <div class="dc-title-wrap">
        <h3 class="dc-title">检索语法手册</h3>
        <p class="dc-sub">点击示例可直接填入检索框 · 支持字段 / 运算符 / 逻辑组合</p>
      </div>
      <span class="dc-toggle">
        <span class="dc-toggle-text">{{ expanded ? '收起' : '展开' }}</span>
        <svg class="dc-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="6,9 12,15 18,9" />
        </svg>
      </span>
    </header>

    <!-- 可折叠主体：grid-rows 动画 -->
    <div class="dc-body">
      <div class="dc-body-inner">
        <!-- 运算符 -->
        <div class="dc-section">
          <div class="dc-sec-head">
            <span class="dc-sec-tag tag-amber">运算符</span>
            <span class="dc-sec-title">运算符</span>
          </div>
          <div class="op-grid">
            <div v-for="o in operators" :key="o.sym" class="op-card op-amber">
              <code class="op-sym">{{ o.sym }}</code>
              <div class="op-meta">
                <span class="op-name">{{ o.name }}</span>
                <span class="op-desc">{{ o.desc }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 逻辑 -->
        <div class="dc-section">
          <div class="dc-sec-head">
            <span class="dc-sec-tag tag-violet">逻辑</span>
            <span class="dc-sec-title">逻辑组合</span>
          </div>
          <div class="op-grid op-grid--logic">
            <div v-for="l in logic" :key="l.sym" class="op-card op-violet">
              <code class="op-sym">{{ l.sym }}</code>
              <div class="op-meta">
                <span class="op-name">{{ l.name }}</span>
                <span class="op-desc">{{ l.desc }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- 字段：按类别分组 -->
        <div class="dc-section">
          <div class="dc-sec-head">
            <span class="dc-sec-tag tag-cyan">字段</span>
            <span class="dc-sec-title">常用字段</span>
          </div>
          <div class="field-groups">
            <div v-for="g in fieldGroups" :key="g.cat" class="field-group">
              <span class="fg-label">{{ g.cat }}</span>
              <div class="field-tags">
                <code v-for="f in g.items" :key="f" class="field-tag">{{ f }}</code>
              </div>
            </div>
          </div>
        </div>

        <!-- 示例 -->
        <div class="dc-section">
          <div class="dc-sec-head">
            <span class="dc-sec-tag tag-green">示例</span>
            <span class="dc-sec-title">使用示例</span>
          </div>
          <ul class="ex-list">
            <li v-for="(ex, i) in examples" :key="ex.q" class="ex-row">
              <div class="ex-info">
                <p class="ex-desc">{{ ex.desc }}</p>
                <button class="ex-code" @click.stop="apply(ex.q)" :title="'填入检索框：' + ex.q">
                  <code v-html="highlight(ex.q)"></code>
                </button>
              </div>
              <button class="ex-copy" :class="{ done: copied === i }" @click.stop="copy(ex.q, i)" :title="copied === i ? '已复制' : '复制'">
                <svg v-if="copied !== i" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15V5a2 2 0 0 1 2-2h10" />
                </svg>
                <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
                  <polyline points="20,6 9,17 4,12" />
                </svg>
              </button>
            </li>
          </ul>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { ref } from 'vue'

const props = defineProps({
  embedded: { type: Boolean, default: false }
})
const emit = defineEmits(['apply', 'close'])

const expanded = ref(true)
const copied = ref(-1)

const icons = {
  book: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/></svg>'
}

const operators = [
  { sym: '=', name: '包含', desc: '字段包含指定值' },
  { sym: '==', name: '精确', desc: '完全相等匹配' },
  { sym: '!=', name: '排除', desc: '字段不等于 / 不含' },
  { sym: '*=', name: '模糊', desc: '通配 / 正则模糊' }
]

const logic = [
  { sym: '&&', name: '与', desc: '同时满足' },
  { sym: '||', name: '或', desc: '满足其一' },
  { sym: '()', name: '分组', desc: '改变优先级' }
]

const fieldGroups = [
  { cat: '网络', items: ['ip', 'port', 'protocol', 'base_protocol'] },
  { cat: '服务', items: ['server', 'banner', 'title', 'body', 'header'] },
  { cat: '资产', items: ['os', 'org', 'asn', 'country', 'region', 'city'] },
  { cat: '主机/域名', items: ['host', 'domain', 'app', 'product'] },
  { cat: '其他', items: ['cert', 'is_ipv6'] }
]

const examples = [
  { q: 'ip="1.1.1.1"', desc: '查询指定 IP 的资产' },
  { q: 'ip="1.1.1.1" && port="443"', desc: '指定 IP 的 443 端口' },
  { q: 'server="nginx" && (port="80" || port="443")', desc: '运行 nginx 的 80 / 443 端口' },
  { q: 'title="登录" && country="CN"', desc: '标题含“登录”且位于中国' },
  { q: 'is_ipv6=true', desc: '仅 IPv6 资产' },
  { q: 'domain="example.com"', desc: '指定域名' },
  { q: 'host="admin.example.com" && port="443"', desc: '管理后台子域名' },
  { q: 'protocol="tcp" && banner*="SSH"', desc: 'TCP 且 banner 含 SSH' }
]

function escapeHtml(t) {
  return t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// 轻量 Dork 语法高亮：字段 / 字符串 / 运算符 / 逻辑 / 括号
function highlight(q) {
  const re = /("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')|(&&|\|\||==|!=|\*=|\(|\))|([a-zA-Z_][a-zA-Z0-9_]*)|(\s+)|([^\s]+)/g
  let out = ''
  let m
  while ((m = re.exec(q))) {
    if (m[1]) out += `<span class="tk-str">${escapeHtml(m[1])}</span>`
    else if (m[2]) {
      if (m[2] === '&&' || m[2] === '||') out += `<span class="tk-logic">${escapeHtml(m[2])}</span>`
      else if (m[2] === '(' || m[2] === ')') out += `<span class="tk-paren">${escapeHtml(m[2])}</span>`
      else out += `<span class="tk-op">${escapeHtml(m[2])}</span>`
    } else if (m[3]) out += `<span class="tk-field">${escapeHtml(m[3])}</span>`
    else if (m[4]) out += m[4]
    else out += escapeHtml(m[5])
  }
  return out
}

function apply(q) {
  emit('apply', q)
}

async function copy(q, i) {
  try {
    await navigator.clipboard.writeText(q)
    copied.value = i
    setTimeout(() => { if (copied.value === i) copied.value = -1 }, 1500)
  } catch (e) {
    /* 剪贴板不可用时静默 */
  }
}
</script>

<style scoped>
.dork-cheat {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-lg);
  overflow: hidden;
}

/* ===== 头部 ===== */
.dc-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 13px 16px;
  cursor: pointer;
  user-select: none;
  background: linear-gradient(90deg, rgba(0, 212, 255, 0.05) 0%, transparent 60%);
  transition: background 0.25s ease;
}
.dc-header:hover { background: linear-gradient(90deg, rgba(0, 212, 255, 0.09) 0%, transparent 60%); }
.dc-icon {
  display: flex; align-items: center; justify-content: center;
  width: 34px; height: 34px; flex: none;
  background: rgba(0, 212, 255, 0.08);
  border: 1px solid rgba(0, 212, 255, 0.2);
  border-radius: var(--radius-md);
  color: var(--accent-cyan);
}
.dc-icon svg { width: 18px; height: 18px; }
.dc-title-wrap { flex: 1; min-width: 0; }
.dc-title {
  margin: 0;
  font-family: var(--font-heading);
  font-size: 13px; font-weight: 700; letter-spacing: 0.1em;
  color: var(--text-primary);
  text-transform: uppercase;
}
.dc-sub {
  margin: 2px 0 0;
  font-family: var(--font-body);
  font-size: 11px; color: var(--text-muted);
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.dc-toggle {
  display: inline-flex; align-items: center; gap: 5px;
  flex: none;
  padding: 5px 11px;
  border: 1px solid var(--border-subtle);
  border-radius: 999px;
  color: var(--text-secondary);
  font-family: var(--font-heading); font-size: 11px; font-weight: 600; letter-spacing: 0.06em;
  transition: all 0.2s ease;
}
.dc-header:hover .dc-toggle { border-color: rgba(0, 212, 255, 0.4); color: var(--accent-cyan); }
.dc-chevron { width: 14px; height: 14px; transition: transform 0.3s ease; }
.dork-cheat.open .dc-chevron { transform: rotate(180deg); }

/* ===== 可折叠主体（grid-rows 动画） ===== */
.dc-body {
  display: grid;
  grid-template-rows: 0fr;
  transition: grid-template-rows 0.38s cubic-bezier(0.4, 0, 0.2, 1);
}
.dork-cheat.open .dc-body { grid-template-rows: 1fr; }
.dc-body-inner { overflow: hidden; min-height: 0; }
.dc-body-inner > * { padding-left: 16px; padding-right: 16px; }
.dc-body-inner { padding-bottom: 16px; }

/* ===== 区块 ===== */
.dc-section { padding-top: 16px; }
.dc-sec-head { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.dc-sec-tag {
  font-family: var(--font-heading); font-size: 9px; font-weight: 700; letter-spacing: 0.12em;
  padding: 3px 8px; border-radius: 3px;
}
.tag-amber { color: var(--accent-amber); background: rgba(245, 166, 35, 0.1); border: 1px solid rgba(245, 166, 35, 0.22); }
.tag-violet { color: var(--accent-violet); background: rgba(123, 97, 255, 0.1); border: 1px solid rgba(123, 97, 255, 0.22); }
.tag-cyan { color: var(--accent-cyan); background: rgba(0, 212, 255, 0.08); border: 1px solid rgba(0, 212, 255, 0.2); }
.tag-green { color: var(--accent-green); background: rgba(0, 230, 118, 0.08); border: 1px solid rgba(0, 230, 118, 0.2); }
.dc-sec-title { font-family: var(--font-heading); font-size: 12px; font-weight: 600; color: var(--text-secondary); letter-spacing: 0.04em; }

/* ===== 运算符 / 逻辑卡片 ===== */
.op-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 8px; }
.op-grid--logic { grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); }
.op-card {
  display: flex; align-items: center; gap: 10px;
  padding: 9px 11px;
  background: var(--bg-input);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  transition: border-color 0.2s ease, transform 0.15s ease;
}
.op-card:hover { transform: translateY(-1px); }
.op-amber:hover { border-color: rgba(245, 166, 35, 0.4); }
.op-violet:hover { border-color: rgba(123, 97, 255, 0.4); }
.op-sym {
  font-family: var(--font-mono); font-size: 15px; font-weight: 700;
  padding: 3px 9px; border-radius: 4px; flex: none;
  background: rgba(245, 166, 35, 0.12); color: var(--accent-amber);
}
.op-violet .op-sym { background: rgba(123, 97, 255, 0.12); color: var(--accent-violet); }
.op-meta { display: flex; flex-direction: column; min-width: 0; }
.op-name { font-family: var(--font-heading); font-size: 12px; font-weight: 600; color: var(--text-primary); }
.op-desc { font-family: var(--font-body); font-size: 11px; color: var(--text-muted); line-height: 1.3; }

/* ===== 字段分组 ===== */
.field-groups { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 12px 16px; }
.field-group { display: flex; flex-direction: column; gap: 6px; }
.fg-label {
  font-family: var(--font-heading); font-size: 10px; font-weight: 700; letter-spacing: 0.08em;
  color: var(--text-muted); text-transform: uppercase;
}
.field-tags { display: flex; flex-wrap: wrap; gap: 5px; }
.field-tag {
  font-family: var(--font-mono); font-size: 11px;
  padding: 2px 8px; border-radius: 3px;
  color: var(--accent-cyan); background: rgba(0, 212, 255, 0.06);
  border: 1px solid rgba(0, 212, 255, 0.14);
}

/* ===== 示例 ===== */
.ex-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 7px; }
.ex-row {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 10px;
  background: rgba(0, 230, 118, 0.03);
  border: 1px solid var(--border-subtle);
  border-left: 2px solid rgba(0, 230, 118, 0.35);
  border-radius: var(--radius-md);
  transition: border-color 0.2s ease, background 0.2s ease;
}
.ex-row:hover { border-color: rgba(0, 230, 118, 0.3); background: rgba(0, 230, 118, 0.06); }
.ex-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.ex-desc { margin: 0; font-family: var(--font-body); font-size: 12px; color: var(--text-secondary); }
.ex-code {
  align-self: flex-start; max-width: 100%;
  padding: 5px 10px;
  background: var(--bg-input);
  border: 1px dashed rgba(0, 230, 118, 0.25);
  border-radius: 5px;
  cursor: pointer; text-align: left;
  font-family: var(--font-mono); font-size: 12.5px; color: var(--text-primary);
  overflow-x: auto; white-space: nowrap;
  transition: border-color 0.2s ease;
}
.ex-code:hover { border-color: rgba(0, 230, 118, 0.55); }
.ex-copy {
  flex: none;
  display: flex; align-items: center; justify-content: center;
  width: 30px; height: 30px;
  background: transparent; border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  color: var(--text-muted); cursor: pointer;
  transition: all 0.2s ease;
}
.ex-copy svg { width: 15px; height: 15px; }
.ex-copy:hover { color: var(--accent-cyan); border-color: rgba(0, 212, 255, 0.4); }
.ex-copy.done { color: var(--accent-green); border-color: rgba(0, 230, 118, 0.4); }

/* ===== 语法高亮 token ===== */
.tk-field { color: var(--accent-cyan); }
.tk-str { color: var(--accent-amber); }
.tk-op { color: var(--accent-violet); font-weight: 700; }
.tk-logic { color: var(--accent-green); font-weight: 700; }
.tk-paren { color: var(--text-muted); }

/* ===== 嵌入式（弹层）模式 ===== */
.dc-embed-head {
  display: flex; align-items: center; gap: 10px;
  padding: 12px 14px;
  border-bottom: 1px solid var(--border-subtle);
  background: linear-gradient(90deg, rgba(0, 212, 255, 0.05) 0%, transparent 60%);
}
.dc-close {
  margin-left: auto;
  display: flex; align-items: center; justify-content: center;
  width: 28px; height: 28px;
  background: transparent; border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  color: var(--text-muted); cursor: pointer;
  transition: all 0.2s ease;
}
.dc-close svg { width: 15px; height: 15px; }
.dc-close:hover { color: var(--accent-red); border-color: rgba(255, 71, 87, 0.4); }
.dork-cheat.embedded {
  background: transparent;
  border: none;
  border-radius: 0;
  overflow: visible;
}

/* ===== 响应式 ===== */
@media (max-width: 640px) {
  .dc-sub { display: none; }
  .op-grid, .op-grid--logic, .field-groups { grid-template-columns: 1fr 1fr; }
  .ex-desc { font-size: 11px; }
}
@media (max-width: 420px) {
  .op-grid, .op-grid--logic, .field-groups { grid-template-columns: 1fr; }
}
</style>
