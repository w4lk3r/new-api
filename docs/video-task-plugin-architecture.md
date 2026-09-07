# 视频 Task Plugin 接入与运维技术说明

> 当前状态：已采用 New API 官方 Task Plugin 架构。基线为
> `v1.0.0-rc.31`，当前 Fork 发布版本为
> `v1.0.0-rc.31-video-fix.2`（2026-09-07）。

本文是当前视频接入方案的技术主文档，记录架构决策、渠道配置、路由、计费、
版本升级、测试证据和已知限制。面向调用方的绘帧AI请求示例见
[`task-plugins/huizhen-ai/user-guide.md`](./task-plugins/huizhen-ai/user-guide.md)。

## 1. 架构结论

当前方案直接使用官方 Task Plugin，不再新增私有 `LingjingVideo` 或
`NativeVideo` 渠道类型，也不占用私有渠道编号。

- 官方 `Task Plugin` 渠道类型是 `61`；
- `59` 已被官方 `Sub2API` 使用，不能再分配给私有视频渠道；
- 每个视频平台由独立、带版本号的 JavaScript 插件描述上游协议；
- Token 鉴权、用户分组、模型授权、渠道调度、公共任务 ID、任务归属、轮询、
  日志、预扣费、结算退款和结果代理继续由 New API 宿主统一负责；
- 调用方以 `model` 选择产品能力，普通用户不直接指定 Channel ID；
- 同一模型可配置多个兼容渠道，继续使用 New API 的优先级和权重进行选择。

旧分支 `main` 上的
`docs/native-video-routing-architecture.md`、`LingjingVideo` 和私有
`NativeVideo` 实现是历史方案，目前没有合入本分支。该设计依赖私有渠道类型、
私有路由与适配器注册表，已经被官方 Task Plugin 能力取代，不能作为当前部署
说明直接使用。

## 2. 请求与任务流程

```text
客户端（New API Token + model）
  -> TokenAuth / 分组和模型权限
  -> 筛选绑定该模型的已启用 Task Plugin 渠道
  -> 按优先级、权重选择渠道
  -> 固定插件 key、插件版本、渠道 ID 和计费快照
  -> 插件校验请求并构造上游 HTTP 请求
  -> 使用渠道中保存的上游 API Key 发起请求
  -> 保存 New API 公共任务 ID 与上游任务 ID 的映射
  -> 后台按创建时固定的渠道和插件版本轮询
  -> 成功后通过 New API 代理结果；失败或超时进入任务结算/退款链
```

创建任务一旦被上游接受，就必须固定到原渠道。查询、下载、轮询和退款不能根据
后来修改的默认渠道重新选择平台。对于发送超时等“无法确认上游是否已经受理”
的情况，也不能盲目切换渠道重试，否则可能产生两个收费任务。插件应在上游支持
时使用基于公共任务 ID 的幂等键。

Task Plugin 的边界如下：

| 责任 | Task Plugin | New API 宿主 |
| --- | --- | --- |
| 上游路径、鉴权头和原生请求格式 | 是 | 仅安全执行描述符 |
| 参数及模型能力校验 | 是 | 执行通用请求和计费安全约束 |
| 上游状态映射、结果定位 | 是 | 持久化和公开统一任务状态 |
| 用户 Token、分组、模型权限 | 否 | 是 |
| API Key 保存与脱敏 | 否 | 是 |
| 渠道优先级和权重 | 否 | 是 |
| 预扣费、结算、日志与退款 | 提供用量事实 | 是 |
| 公共任务 ID、任务归属校验 | 否 | 是 |
| 视频流、Range 和条件请求代理 | 描述上游请求 | 是 |

## 3. 对外接口

插件声明 `openai_video` 协议后，使用官方统一接口：

```text
POST /v1/videos
GET  /v1/videos/{public_task_id}
GET  /v1/videos/{public_task_id}/content
HEAD /v1/videos/{public_task_id}/content
```

创建和查询必须携带 New API 用户 Token。结果内容也经过宿主的任务归属或短期
结果访问能力校验，客户端不会得到上游 API Key。`public_task_id` 是 New API
任务 ID，不是上游任务 ID。

Task Plugin 还提供与平台无关的通用任务接口：

```text
POST /v1/tasks/{plugin_key}
GET  /v1/tasks/{public_task_id}
GET  /v1/tasks/{public_task_id}/artifacts
GET|HEAD /v1/tasks/{public_task_id}/artifacts/{artifact_key}/content
```

当前视频客户端优先使用 `/v1/videos`，不要恢复旧的
`/v1/lingjing/*` 私有路由。

## 4. 多平台和默认模型

普通调用方只传逻辑模型名。管理员通过以下配置决定实际平台：

1. 在各 Task Plugin 渠道中启用平台实际支持的模型；
2. 使用渠道模型映射把统一的对外模型名映射为不同上游模型；
3. 使用分组限制不同用户可以使用的模型和渠道；
4. 使用渠道优先级、权重和状态控制同一模型的候选渠道。

若要提供 `video-default`，推荐把它作为管理员维护的逻辑模型别名，并在所有语义、
参数能力和定价相容的候选渠道上配置映射。平台专有字段、上传得到的平台临时文件
ID，或不同收费规则的能力不应混在同一个默认模型中，应公开独立模型名。

新增视频平台时，通常只需要新增一个插件目录，而不需要新增 Go 渠道常量：

```text
docs/task-plugins/{provider}/
  {provider}.plugin.js
  {provider}.fixture.json
  README.md
```

插件至少要声明稳定 `key`、不可变语义版本、模型列表、`openai_video` 协议、
创建/查询/内容请求构建、状态解析、产物声明以及必要的用量事实。

## 5. 已接入平台

### 5.1 绘帧AI

- 插件：[`huizhen-ai.plugin.js`](./task-plugins/huizhen-ai/huizhen-ai.plugin.js)
- 插件 key / 版本：`huizhen-ai` / `1.0.0`
- 建议 Base URL：`https://hz.codelba.cn`
- 上游创建：`POST /openapi/v1/videos`
- 上游查询：`GET /openapi/v1/videos/{task_id}`
- 上游内容：`GET /openapi/v1/videos/{task_id}/content`
- 上游鉴权：`Authorization: Bearer {渠道 API Key}`

当前插件包含下列 9 个上游模型：

| 模型 | 上游计价 | 积分/次 | 成本人民币/次（1 积分 = ¥0.10） |
| --- | --- | ---: | ---: |
| `sd2-c6-10s` | 按次 | 10 | ¥1.00 |
| `sd2.5` | 按次 | 20 | ¥2.00 |
| `wan-3.0-3` | 按次 | 35 | ¥3.50 |
| `seedance-2.0-mini-2` | 按次 | 12 | ¥1.20 |
| `seedance-2.0` | 按次 | 22 | ¥2.20 |
| `seedance-2.0-fast` | 按次 | 14 | ¥1.40 |
| `seedance-2.5` | 按次 | 55 | ¥5.50 |
| `grok-imagine-video-1.5（按次）` | 按次 | 8 | ¥0.80 |
| `sd2.0满血` | 按次 | 25 | ¥2.50 |

以上价格来自 2026-09-06/07 的上游模型接口结果。当前所有列出的模型都是
`price_type: per_call`，`resolution_prices` 为空；时长和分辨率是模型能力参数，
不是当前上游价格阶梯。上游今后调整价格或返回动态价格结构时，需要重新核对。

插件仍公开 `seconds` 与 `resolution` 用量事实，以便未来上游改为按时长或分辨率
计费时使用表达式，而不需要重写任务系统。用量事实本身不会自动决定价格。

已用测试账户发起过一次真实创建：上游返回 HTTP 202、状态 `queued`、任务
`video-24944`，并报告扣除 8 积分。它证明创建鉴权和请求协议可达；尚未据此宣称
真实任务的完成轮询、内容下载和失败退款已经全链路验收。

### 5.2 Custom Video（Dola 协议样本）

- 插件：[`custom-video.plugin.js`](./task-plugins/custom-video/custom-video.plugin.js)
- 插件 key / 版本：`custom-video` / `1.0.1`
- 模型：`custom-video-30s`
- 上游创建：`POST /api/v1/videos`（multipart）
- 上游查询：`GET /api/v1/videos/{task_id}`
- 兼容查询：`GET /custom/v1/videos/{public_task_id}`

该协议当前固定为 30 秒。收到的原始资料按整次返回扣费信息，没有提供按秒价格
表，所以应按次计费。版本 `1.0.1` 增加 `usageSchema.calls`，并固定返回
`{calls: 1}`；这既可使用普通按次模型价格，也可使用如下表达式：

```text
tier("base", u("calls") * 你的单次价格)
```

这里的 `tier("base", ...)` 只给账单结果命名，不代表上游存在时长阶梯。

## 6. 计费配置

### 6.1 当前推荐

绘帧AI当前是固定按次价格，推荐直接为每个模型配置“按次”的基础模型价格，价格
填上表中的人民币成本。当前运营配置为：

- New API 的余额和价格按人民币理解，内部显示汇率采用 1:1；
- `default` 分组倍率为 `2`，即用户价格为基础价格的 2 倍；
- `vip` 若为 `1`，用户按基础成本价收费；
- `svip` 若为 `0.5`，用户只付一半基础成本，通常会亏损，除非这是明确补贴。

因此当前 `default` 用户的绘帧AI价格为：

| 模型 | 基础价格 | `default × 2` |
| --- | ---: | ---: |
| `sd2-c6-10s` | ¥1.00 | ¥2.00 |
| `sd2.5` | ¥2.00 | ¥4.00 |
| `wan-3.0-3` | ¥3.50 | ¥7.00 |
| `seedance-2.0-mini-2` | ¥1.20 | ¥2.40 |
| `seedance-2.0` | ¥2.20 | ¥4.40 |
| `seedance-2.0-fast` | ¥1.40 | ¥2.80 |
| `seedance-2.5` | ¥5.50 | ¥11.00 |
| `grok-imagine-video-1.5（按次）` | ¥0.80 | ¥1.60 |
| `sd2.0满血` | ¥2.50 | ¥5.00 |

充值倍率与消费分组倍率用途不同：充值倍率影响充值换得的余额，分组倍率影响每次
消费。当前保持充值倍率 `1`，只用 `default = 2` 做销售加价，计算最直观。

### 6.2 表达式计费边界

插件的 `usageSchema` 只声明宿主可以信任哪些计费事实。管理员的表达式才把事实
换算为价格。例如未来确实采用每秒价格时，可配置：

```text
tier("base", u("seconds") * 每秒价格)
```

若价格依赖分辨率，可依据 `u("resolution")` 建枚举档位。当前绘帧AI接口明确
返回按次价，因此不应仅因为模型支持多个时长或分辨率就自行推导阶梯价格。

创建任务只预扣一次；查询和下载不重复扣生成费用。任务正常进入 `FAILURE` 或被
宿主超时终止后，走现有异步任务结算链退款。插件展示的上游积分只用于诊断，不能
直接覆盖 New API 对用户的扣费数额。

## 7. 已修复的宿主兼容问题

### 7.1 multipart Content-Type（`video-fix.1`）

官方 rc.31 生成 multipart 请求体后，没有把包含 boundary 的 Content-Type 保留
到实际发出的上游请求，导致上游报告“请求体无效或图片总大小超过限制”。提交
`e2c74bf4` 修复了 multipart boundary 透传，同时支持 JSON 入站生成纯文本
multipart 上游请求。

### 7.2 异步创建返回 HTTP 202（`video-fix.2`）

异步服务常用 `202 Accepted` 表示已接收任务。宿主原先只把 `200 OK` 交给插件
解析，因此绘帧AI实际已经创建并扣费，New API 却返回
`fail_to_fetch_task`。提交 `4120a8ee` 将成功条件改为全部 `2xx`，随后再由插件
解析响应。这也是当前镜像必须使用 `video-fix.2` 的原因。

## 8. 插件和镜像版本管理

插件版本是不可变的。同一 `key + version` 上传不同源码会被拒绝。修改插件后必须：

1. 增加 `meta.version`；
2. 上传新文件；
3. 在管理后台明确激活新版本；
4. 确认渠道仍绑定正确的插件 key 和模型；
5. 刷新价格页面并等待缓存更新。

只上传新版本不会自动替换当前激活版本。`custom-video` 当前应激活 `1.0.1`，否则
旧 `1.0.0` 没有 `calls` 用量事实，后台仍只会显示原来的按次价格形态。

当前发布镜像：

```text
ghcr.io/w4lk3r/new-api:v1.0.0-rc.31-video-fix.2
```

镜像版本由 Git tag 驱动；源码中的 `VERSION` 保持构建流程约定，由 GitHub
Actions 在发布时写入 tag。生产环境升级前应备份数据库，并固定 tag，避免直接
使用浮动的 `latest`。

## 9. 配置检查清单

1. 管理后台已经上传并激活目标插件版本；
2. 新建渠道类型选择 `Task Plugin`，而不是 `Sub2API` 或旧私有类型；
3. Base URL 只填上游根地址，不附加插件中已经拼接的 API 路径；
4. 上游 API Key 只保存在渠道密钥中；
5. 渠道启用正确模型、分组、优先级和权重；
6. 模型存在基础价格或有效的计费表达式；
7. 用普通用户 Token 检查 `/v1/models` 中能看到目标模型；
8. 用低价模型完成创建、轮询、下载，并核对消费日志和余额；
9. 人工制造一次上游失败，确认只扣一次且退款只发生一次；
10. 多渠道同模型上线前验证优先级、权重和故障切换边界。

## 10. 测试证据和未完成验收

仓库当前包含以下自动化验证：

```bash
go test ./controller -run '^TestHuizhenVideoPluginContracts$' -count=1 -v
go test ./controller -run '^TestCustomVideoPluginSubmitLifecycle$' -count=1 -v
go test ./relay/channel/task/jsplugin/... ./router ./service
git diff --check
```

- 绘帧AI fixture 覆盖插件元数据、请求、202 创建响应、状态映射、用量和内容请求；
- Custom Video 集成测试覆盖 Token 鉴权、渠道 Key、JSON/multipart、任务归属、
  状态、视频流、失败退款和不重复计费；
- 202 回归测试保证所有成功 `2xx` 可以进入插件解析；
- multipart 回归测试保证 boundary 被发送到真实上游请求。

目前仍需在生产前补齐：

- 绘帧AI真实任务从 `queued` 到 `completed` 的完整轮询；
- 绘帧AI MP4、HEAD、Range、302/内容代理的真实上游验证；
- 绘帧AI真实失败、取消、超时以及上游积分退款与 New API 退款的对账；
- 上游 15 秒宿主轮询间隔是否满足绘帧AI建议的至少 20 秒；
- MySQL、PostgreSQL 专用测试库的升级矩阵（本轮视频改动未新增数据库结构，但生产
  合并仍应按项目规则验收）。

## 11. 相关文档位置

| 文档 | 用途 |
| --- | --- |
| 本文 `docs/video-task-plugin-architecture.md` | 当前视频架构、配置和运维主文档 |
| `docs/task-plugins/huizhen-ai/README.md` | 绘帧AI插件安装和开发说明 |
| `docs/task-plugins/huizhen-ai/user-guide.md` | 面向调用方的 API 使用指南，可导出 PDF |
| `docs/task-plugins/custom-video/README.md` | Custom Video 插件安装和计费说明 |
| `docs/plugin-api/v1.md` | 官方 Task Plugin v1 开发协议 |
| `docs/upstream-sync-rc31.md` | 从旧 Fork 合并官方 rc.31 的历史验证记录 |
| `controller/testdata/custom-video/README.md` | 收到的原始插件样本及回归测试证据 |
