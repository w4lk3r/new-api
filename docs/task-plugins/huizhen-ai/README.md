# 绘帧AI Task Plugin

此目录提供可通过 New API 管理后台上传的自定义 Task Plugin。它不是新的
Go 渠道类型，不占用渠道编号。

面向最终用户的创建、查询、下载、模型参数和 PDF 导出说明见
[`user-guide.md`](./user-guide.md)。

上游协议依据 `绘帧AI 接入文档.pdf`（2026-09-06）实现：

- `POST /openapi/v1/videos`
- `GET /openapi/v1/videos/{task_id}`
- `GET /openapi/v1/videos/{task_id}/content`

## 安装

1. 在“系统设置 → Task Plugins”上传 `huizhen-ai.plugin.js`。
2. 新建“Task Plugin”渠道，选择插件 `huizhen-ai`。
3. Base URL 填 `https://hz.codelba.cn`，密钥填平台创建的 `sk-hz-...`。
4. 渠道模型选择插件声明的、且该 API Key 实际可用的模型。
5. 为这些模型配置 New API 自己向用户收取的价格。

客户端使用 New API 的 `POST /v1/videos` 创建任务，使用返回的 New API
公共任务 ID 查询和下载。上游任务 ID 与渠道 API Key 不会暴露给客户端。

## 计费

2026-09-07 实际查询的 9 个视频模型全部返回 `price_type: per_call`，且
`resolution_prices` 为空。因此当前应使用按次基础价格，不应仅因一个模型支持
多个时长或分辨率就推导为阶梯价。上游积分按 `1 积分 = ¥0.10` 换算后的成本见
[`video-task-plugin-architecture.md`](../../video-task-plugin-architecture.md)。

插件声明 `seconds` 和 `resolution` 两个经过校验的计费事实，是为了保留未来动态
计费能力；用量事实本身不会自动决定价格。若上游今后明确提供每秒价，才使用：

```text
tier("base", u("seconds") * 每秒价格)
```

若 480p 和 720p 今后出现不同价格，可再按 `u("resolution")` 建立枚举档位。
普通 `ModelPrice` 表示整次任务的固定价格，不会自动乘以视频秒数。

上游返回的 `metadata.points_charged` 只作为渠道侧诊断信息，插件不会用它直接
覆盖 New API 对用户的扣费金额。上游任务进入 `failed` 后会映射为 New API
`FAILURE`，由现有异步任务结算链退款。

## 请求约定

插件接受 JSON 请求，支持：

- `model`、`prompt`；
- `duration`，或兼容字段 `seconds`；
- `resolution`、`aspect_ratio`；
- 兼容字段 `aspectRatio`、`ratio`、`size`；
- `image_refs`、`video_refs`、`audio_refs`；
- `compliance_enabled`。

参考素材必须是上游服务器可访问的公网 HTTP/HTTPS URL。插件不会上传本地
文件。下载由 New API 流式代理，并转发 Range 和条件请求头。

## 当前限制

- Task Plugin 的 `meta.models` 是静态声明。上游 `/openapi/v1/models` 新增、
  删除模型或改变能力后，需要更新插件版本及能力快照。
- 当前模型、时长和清晰度来自 2026-09-06 文档；比例和具体素材数量仍由
  上游进行模型级校验。插件对每类参考 URL 额外设置了 9 个的安全上限。
- 当前 New API 的异步任务调度周期为 15 秒，而上游文档建议查询间隔至少
  20 秒。Task Plugin v1 不能单独指定轮询周期；上线前需向上游确认 15 秒
  是否可接受，或后续在宿主层增加按插件配置轮询间隔的能力。
- 已用测试账户完成一次真实创建：上游返回 HTTP 202、`queued` 和扣除 8 积分。
  这只验证了创建请求和鉴权；上线前仍需完成文生视频或图生视频直至成功、失败
  退款、HEAD、Range 及内容下载的真实全链路验证。
