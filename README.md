# AI Model Monitor

一个独立的网页版 AI 大模型状态监控程序，用于实时监控 AI 大模型的连通性和响应状态。

## 功能特性

- **模型连通性探测** — 向每个模型发送探针请求，检测响应状态和延迟
- **多 API 协议支持** — OpenAI 兼容 API（OpenAI / DeepSeek / Ollama / Groq 等）+ Anthropic 原生 Messages API
- **智能重试** — 对超时、429 限流、5xx 服务端错误自动重试（最多 2 次，指数退避）
- **并发控制** — 全局并发 + 单 Provider 并发，防止 API 过载
- **历史记录** — SQLite 持久化，支持可用性、24h 平均延迟、统计窗口成功率
- **延迟曲线** — SVG 平滑贝塞尔曲线展示延迟趋势
- **自动检测** — 可配置间隔自动定期探测
- **Provider 管理** — Web UI 动态增删改 Provider 和模型
- **面板分离** — 用户面板（公开只读）与管理面板（需认证）独立路由，互不干扰
- **双列瀑布流** — Provider 卡片 CSS Grid 双列布局，不同高度顶部对齐
- **首次初始化密码** — 首次运行必须在网页设置管理密码后才能使用管理功能
- **管理接口鉴权** — `/api/admin/*` 强制登录态鉴权，兼容环境变量 Token
- **主题切换** — 支持日间、夜间、跟随系统和按时间自动切换，全局导航栏一键切换
- **毛玻璃主题** — 网格背景 + 辐射渐变 + 毛玻璃效果 + 深浅色双主题
- **开关控件** — 配置项使用 iOS 风格开关按钮替代传统复选框

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.26+ |
| 数据库 | SQLite (modernc.org/sqlite，纯 Go 无需 CGO) |
| 前端 | React 19 + Vite 6 + react-router-dom v6 |
| 样式 | CSS 自定义属性 + 毛玻璃效果 + 深浅色双主题 |

## 项目结构

后端直接位于仓库根目录，前端位于 `frontend/`：

```text
ai-model-monitor/
├── main.go                      # 入口：HTTP 服务、静态文件托管、自动检测
├── api.go                       # REST API 路由、CORS、鉴权与初始化接口
├── auth.go                      # 管理密码哈希、会话创建与校验
├── models.go                    # 数据模型定义
├── config.go                    # SQLite 配置读写、Provider CRUD、配置校验
├── probe.go                     # 模型探测逻辑（含重试机制）
├── history.go                   # 历史记录持久化、统计计算
├── migrate.go                   # YAML → SQLite 迁移辅助
├── config.yaml                  # 默认配置（可选，启动时自动迁移到 SQLite）
├── go.mod
├── go.sum
├── README.md
├── .gitignore
└── frontend/                    # React 前端
    ├── src/
    │   ├── App.jsx              # 主应用：路由、初始化、登录、导航栏、主题管理
    │   ├── App.css              # 全局样式（深浅色双主题 + 毛玻璃效果 + 响应式）
    │   ├── api.js               # API 客户端、会话 Cookie、兼容 Token、Provider 图标映射
    │   ├── theme.js             # 主题模式管理（system/time/light/dark）
    │   ├── main.jsx             # 入口
    │   └── components/
    │       ├── Dashboard.jsx    # 状态看板：Provider 卡片、模型指标、SVG 曲线
    │       ├── AdminPanel.jsx   # 管理面板：看板 + 配置管理（需认证）
    │       ├── UserPanel.jsx    # 用户面板：只读状态看板（公开访问）
    │       └── ConfigPanel.jsx  # 配置管理：Provider CRUD、通用设置、主题设置
    ├── public/                  # 静态源资源
    ├── index.html
    ├── vite.config.js
    ├── package.json
    └── package-lock.json
```

以下内容是本地安装、构建或运行生成的产物，不提交到 Git：

- `frontend/node_modules/` — npm 依赖目录
- `static/` — Vite 构建后的前端静态资源，由 Go 后端托管
- `data/` — SQLite 运行时数据目录
- `ai-model-monitor` / `ai-model-monitor.exe` — Go 编译产物

## 快速开始

### 方式 1：直接运行

```bash
# 如果尚未生成前端资源，请先执行“构建部署”里的前端构建步骤
go build -o ai-model-monitor.exe .
./ai-model-monitor.exe
```

浏览器打开 `http://localhost:8080`。首次运行会进入初始化页面，需要先设置管理密码；之后使用该密码登录管理界面。

### 方式 2：开发模式

```bash
# 终端 1：启动后端
go run .

# 终端 2：启动前端（自动代理 API 到 8080）
cd frontend
npm ci
npm run dev
```

浏览器打开 `http://localhost:5173`

## 构建部署

```bash
# 1. 构建前端（输出到根目录 static/）
cd frontend
npm ci
npm run build

# 2. 编译后端
cd ..
go build -o ai-model-monitor.exe .

# 3. 部署 — 只需以下文件/目录
#   ai-model-monitor.exe
#   static/     (npm run build 生成的前端资源)
#   config.yaml (可选，仅用于首次迁移)
```

`static/` 是构建生成目录，不提交到 Git。每次部署前请先执行 `npm run build` 重新生成静态资源。`data/` 是运行时数据库目录，也不提交到 Git。

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/setup-status` | 查询是否需要首次初始化 |
| POST | `/api/setup` | 首次设置管理密码，成功后写入登录会话 |
| POST | `/api/login` | 使用管理密码登录 |
| POST | `/api/logout` | 退出登录并清除会话 |
| GET | `/api/admin/config` | 获取全局配置（API Key 已脱敏，需鉴权） |
| PUT | `/api/admin/config` | 更新全局配置（需鉴权） |
| GET | `/api/admin/providers` | 获取 Provider 列表（API Key 已脱敏，需鉴权） |
| POST | `/api/admin/providers` | 添加 Provider（需鉴权） |
| PUT | `/api/admin/providers?id=xxx` | 更新 Provider（需鉴权） |
| DELETE | `/api/admin/providers?id=xxx` | 删除 Provider（需鉴权） |
| POST | `/api/admin/probe` | 手动触发探测（需鉴权） |
| GET | `/api/status` | 获取最新探测报告 |
| GET | `/api/history?key=providerID::model` | 查询历史记录 |

兼容说明：旧的 `/api/config`、`/api/providers`、`/api/probe` 路径仍保留为兼容别名，但同样需要完成首次初始化并通过管理鉴权；新接入请使用 `/api/admin/*`。

## 配置项

通过 Web UI「配置管理」页面或 API 修改，存储在 SQLite 中。后端会对关键数值配置做最小值校验，避免并发数为 0 或负数导致阻塞或 panic。

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `title` | 模型连通性 | 看板标题 |
| `timeout_seconds` | 30 | 探测超时时间，最小 1 |
| `slow_threshold_ms` | 8000 | 慢响应阈值（毫秒），最小 1 |
| `concurrency` | 3 | 全局最大并发数，最小 1 |
| `provider_concurrency` | 1 | 单 Provider 并发数，最小 1 |
| `history_size` | 30 | 历史条显示长度，最小 1 |
| `stats_window_days` | 7 | 统计窗口天数，最小 1 |
| `probe_prompt` | 只回复 OK 两个字母 | 探测用户提示词 |
| `probe_system_prompt` | 你是一个模型连通性探针… | 探测系统提示词 |
| `show_curve_chart` | true | 显示延迟曲线 |
| `show_error_detail` | true | 显示错误详情 |
| `auto_check_interval_seconds` | 0 | 自动检测间隔（0 = 关闭） |
| `port` | 8080 | 服务端口，范围 1-65535 |

## 支持的 Provider 类型

openai · anthropic · deepseek · google · ollama · groq · openrouter · nvidia · azure · xai · dashscope · volcengine · minimax · kimi · modelscope

## API Key 处理

API Key 仅用于服务端向模型供应商发起探测请求。读取配置或 Provider 列表时，后端会将非空 API Key 返回为 `********`。

编辑 Provider 时：

- 保持 `********`：不修改原 API Key
- 清空 API Key 字段：删除原 API Key
- 输入新值：替换为新 API Key

## 首次初始化与访问鉴权

首次运行时，后端 SQLite 中还没有管理员密码。浏览器打开页面后会先进入初始化界面，必须设置管理密码后才能使用配置管理、Provider 管理和手动探测等管理功能。

初始化完成后：

- 管理密码只以 PBKDF2-SHA256 哈希形式保存在 SQLite 中，不保存明文。
- 登录成功后，后端通过 HttpOnly Cookie 保存会话。
- 退出登录会清除服务端会话。
- 管理接口 `/api/admin/*` 和旧管理兼容路径都必须通过登录态或兼容 Token 鉴权。

### 面板路由

| 路由 | 面板 | 认证 |
|------|------|------|
| `/` | 用户面板 — 只读状态看板，30 秒自动轮询 | 无需登录 |
| `/admin` | 管理面板 — 看板 + 配置管理 + 手动探测 | 需登录 |
| `/admin/login` | 管理员登录页 | 未认证时访问 |

用户面板始终公开可访问，无需任何登录操作。管理面板未认证时自动重定向到登录页。

`AI_MODEL_MONITOR_TOKEN` 仍作为自动化或兼容接入方式保留，但不能绕过首次网页初始化；首次设置管理密码后，可继续使用 Bearer Token 或 `X-API-Key` 访问管理接口：

```bash
AI_MODEL_MONITOR_TOKEN=your-random-token ./ai-model-monitor
curl -H "Authorization: Bearer your-random-token" http://localhost:8080/api/admin/config
curl -H "X-API-Key: your-random-token" http://localhost:8080/api/admin/config
```

`/api/status`、`/api/history` 是核心监控读接口，保持公开兼容；如需暴露到公网，建议放在 HTTPS 反向代理后面。

## 主题模式

前端支持四种主题模式：

- **跟随系统**：根据浏览器/系统的 `prefers-color-scheme` 自动切换。
- **按时间**：默认 18:00 到次日 07:00 使用夜间主题，其余时间使用日间主题，可在配置管理中调整。
- **日间**：固定浅色主题。
- **夜间**：固定深色主题。

主题偏好保存在当前浏览器的 `localStorage`，不会写入 SQLite，也不会影响其他浏览器或用户。

## 测试验证

```bash
# 前端生产构建，输出到 static/
npm --prefix frontend run build

# 后端测试
go test ./...

# 后端编译
go build -o ai-model-monitor.exe .
```

建议手动验证：

- 首次启动空数据库时，网页只显示设置管理密码界面。
- 未完成首次初始化时，`/api/admin/config` 返回 401。
- 设置管理密码并登录后可访问配置管理。
- 首次初始化完成后，正确 Bearer Token 或 `X-API-Key` 可访问 `/api/admin/config`。
- 用户面板 `/` 无需登录即可查看状态看板，30 秒自动轮询更新。
- 管理面板 `/admin` 未登录时自动重定向到登录页。
- 配置管理中可新增、编辑、删除 Provider。
- 编辑 Provider 时保持 `********` 不会覆盖原 API Key，清空字段才删除。
- 手动探测、历史记录、自动检测仍能正常工作。
- 导航栏主题切换按钮可在任意面板一键切换日间/夜间模式。
- 日间、夜间、跟随系统、按时间主题模式均可切换并持久化到浏览器。
- 配置管理中的开关按钮（显示延迟曲线、显示错误详情）可正常切换。

## 数据存储

所有运行时数据存储在 `data/monitor.db`（SQLite），包含以下主要数据：

- **config** — 全局配置键值对，也保存管理员密码哈希
- **providers** — Provider 信息
- **history** — 探测历史记录
- **sessions** — 管理登录会话哈希与过期时间

首次启动时，如存在旧的 `config.yaml` 会自动迁移到 SQLite。请不要将包含真实 API Key 的本地配置文件提交到 Git。

## License

MIT
