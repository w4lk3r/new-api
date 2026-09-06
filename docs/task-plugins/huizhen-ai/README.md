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

插件声明 `seconds` 和 `resolution` 两个经过校验的计费事实。推荐使用任务
计费表达式，例如每秒 0.10 美元：

```text
tier("base", u("seconds") * 0.10)
```

按次模型可以使用固定表达式：

```text
tier("fixed", 0.50)
```

若 480p 和 720p 单价不同，可按 `u("resolution")` 建立枚举档位。普通
`ModelPrice` 仍表示整次任务的固定价格，不会自动乘以视频秒数。

上游返回的 `metadata.points_charged` 是绘帧AI账户积分，不等同于 New API
的美元计价单位，因此插件不会直接把它作为用户扣费金额。上游任务进入
`failed` 后会映射为 New API `FAILURE`，由现有异步任务结算链退款。

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
- 未使用真实 API Key 发起收费任务；上线前应以测试账户完成一次文生视频、
  图生视频、失败退款和 Range 下载验证。
