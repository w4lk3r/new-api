---
title: "绘帧AI 视频生成 API 用户使用指南"
subtitle: "通过 New API 创建、查询与下载视频"
lang: zh-CN
date: 2026-09-06
---

# 绘帧AI 视频生成 API 用户使用指南

本文面向通过 New API 调用绘帧AI视频生成服务的开发者。文中的请求只使用
New API 用户令牌；用户不需要、也不应接触上游渠道密钥。

> 本文记录的模型能力来自 2026 年 9 月 6 日的接入资料。实际上线的模型、
> 价格及可用能力以服务提供方和 New API 管理员配置为准。

## 1. 快速开始

本章固定使用 `seedance-2.0-mini-2`，以一个 5 秒、720p、16:9 的文生视频
任务演示完整流程。

### 1.1 准备访问地址和令牌

需要准备：

- New API 服务地址，例如 `https://api.example.com`；
- New API 用户令牌，例如 `sk-...`；
- 账户中足够的视频生成额度。

在 macOS 的 Terminal 中设置：

```bash
export NEWAPI_BASE="https://api.example.com"
read -s "NEWAPI_TOKEN?请输入 New API Token: "
echo
export NEWAPI_TOKEN
```

`NEWAPI_BASE` 末尾不要添加 `/`。使用 `read -s` 可以避免令牌显示在终端和
保存在命令历史中。

本文使用 `jq` 格式化 JSON。尚未安装时可以执行：

```bash
brew install jq
```

### 1.2 确认模型可用

```bash
curl -sS "$NEWAPI_BASE/v1/models" \
  -H "Authorization: Bearer $NEWAPI_TOKEN" \
  | jq -e '.data[] | select(.id == "seedance-2.0-mini-2")'
```

命令输出模型信息表示当前令牌可以使用该模型。没有输出时，请联系 New API
管理员检查渠道状态、用户分组、模型授权和模型价格配置。

### 1.3 创建视频任务

创建任务会扣除相应额度：

```bash
CREATE_RESPONSE=$(
  curl -sS -X POST "$NEWAPI_BASE/v1/videos" \
    -H "Authorization: Bearer $NEWAPI_TOKEN" \
    -H "Content-Type: application/json" \
    --data '{
      "model": "seedance-2.0-mini-2",
      "prompt": "清晨的未来城市，阳光穿过云层，电影级平稳运镜",
      "duration": 5,
      "resolution": "720p",
      "aspect_ratio": "16:9"
    }'
)

printf '%s\n' "$CREATE_RESPONSE" | jq .
```

从响应中保存 New API 公共任务 ID：

```bash
TASK_ID=$(printf '%s' "$CREATE_RESPONSE" | jq -r '.id // empty')
test -n "$TASK_ID" && printf '任务创建成功：%s\n' "$TASK_ID"
```

如果 `TASK_ID` 为空，应先查看完整创建响应，不要继续查询。后续所有操作都使
用这个公共任务 ID，不使用上游任务 ID。

### 1.4 查询任务状态

```bash
curl -sS "$NEWAPI_BASE/v1/videos/$TASK_ID" \
  -H "Authorization: Bearer $NEWAPI_TOKEN" \
  | jq .
```

常见状态：

| 状态 | 含义 | 建议操作 |
|---|---|---|
| `queued` | 已排队 | 稍后查询 |
| `in_progress` | 正在生成 | 稍后查询 |
| `completed` | 已完成 | 下载视频 |
| `failed` | 生成失败 | 查看 `error` 信息 |

不要高频轮询。建议至少间隔 20 秒：

```bash
while true; do
  RESPONSE=$(curl -sS "$NEWAPI_BASE/v1/videos/$TASK_ID" \
    -H "Authorization: Bearer $NEWAPI_TOKEN")
  printf '%s\n' "$RESPONSE" | jq .
  STATUS=$(printf '%s' "$RESPONSE" | jq -r '.status // "unknown"')
  case "$STATUS" in
    completed|failed) break ;;
  esac
  sleep 20
done
```

### 1.5 下载视频

状态为 `completed` 后执行：

```bash
curl -fL "$NEWAPI_BASE/v1/videos/$TASK_ID/content" \
  -H "Authorization: Bearer $NEWAPI_TOKEN" \
  --output "video-$TASK_ID.mp4"
```

检查并在 macOS 中打开视频：

```bash
ls -lh "video-$TASK_ID.mp4"
file "video-$TASK_ID.mp4"
open "video-$TASK_ID.mp4"
```

### 1.6 使用参考图片生成视频

`image_refs` 接收公网可访问的 HTTP/HTTPS 图片地址，不接收本地文件路径、
`file://` 地址或仅在局域网可访问的地址。

```bash
curl -sS -X POST "$NEWAPI_BASE/v1/videos" \
  -H "Authorization: Bearer $NEWAPI_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{
    "model": "seedance-2.0-mini-2",
    "prompt": "人物缓慢转身看向镜头，背景灯光自然变化，电影级运镜",
    "duration": 5,
    "resolution": "720p",
    "aspect_ratio": "16:9",
    "image_refs": [
      "https://assets.example.com/reference.png"
    ]
  }' | jq .
```

## 2. 通用请求参数

推荐使用以下规范字段：

| 参数 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `model` | string | 是 | 模型名称，必须与管理员开放的名称完全一致 |
| `prompt` | string | 是 | 视频描述，最多 3000 个字符 |
| `duration` | integer | 建议填写 | 视频时长，单位为秒，必须是模型支持的值 |
| `resolution` | string | 建议填写 | 输出清晰度，当前为 `480p` 或 `720p` |
| `aspect_ratio` | string | 否 | 画面比例，使用 `宽:高` 格式，例如 `16:9` |
| `image_refs` | string[] | 否 | 公网图片 URL，最多 9 个 |
| `video_refs` | string[] | 否 | 公网视频 URL，最多 9 个 |
| `audio_refs` | string[] | 否 | 公网音频 URL，最多 9 个 |
| `compliance_enabled` | boolean | 否 | 是否启用上游支持的合规处理选项 |

兼容字段如下。新项目仍建议使用上表中的规范字段：

| 兼容字段 | 对应规范字段 | 注意事项 |
|---|---|---|
| `seconds` | `duration` | 两者同时提供时必须相等 |
| `aspectRatio`、`ratio` | `aspect_ratio` | 多个比例字段同时提供时必须相等 |
| `size` | `resolution` | 使用 `宽x高`，且短边只能对应 480p 或 720p |

参考素材的格式、大小、数量以及不同素材组合是否受支持，仍可能因模型而异。
模型明确拒绝某种输入时，应以接口返回的错误信息为准。

## 3. 视频模型列表

### 3.1 模型能力总览

| 模型 | 支持时长（秒） | 支持分辨率 |
|---|---|---|
| `sd2-c6-10s` | 5、8、10 | 720p |
| `sd2.5` | 30 | 720p |
| `wan-3.0-3` | 5、10、15、20、25、30 | 720p |
| `seedance-2.0-mini-2` | 5、8、10、12、15 | 480p、720p |
| `seedance-2.0` | 5、8、10、12、15 | 720p |
| `seedance-2.0-fast` | 5、8、10、12、15 | 720p |
| `seedance-2.5` | 5、8、10、12、15、20 | 720p |
| `grok-imagine-video-1.5（按次）` | 5、8、10、12、15 | 720p |
| `sd2.0 满血` | 5、8、10、12、15 | 720p |

模型名称区分字符。特别是下面两个名称包含全角括号、中文或空格，调用时必须
原样填写：

```text
grok-imagine-video-1.5（按次）
sd2.0 满血
```

### 3.2 `sd2-c6-10s`

- 支持时长：5、8、10 秒；
- 支持分辨率：720p；
- 请求示例中的 `duration` 只能填写上述三个整数之一。

### 3.3 `sd2.5`

- 支持时长：固定 30 秒；
- 支持分辨率：720p；
- `duration` 应填写 `30`。

### 3.4 `wan-3.0-3`

- 支持时长：5、10、15、20、25、30 秒；
- 支持分辨率：720p；
- 适合需要较多时长档位的调用场景。

### 3.5 `seedance-2.0-mini-2`

- 支持时长：5、8、10、12、15 秒；
- 支持分辨率：480p、720p；
- 是当前列表中可以明确选择 480p 或 720p 的模型。

### 3.6 `seedance-2.0`

- 支持时长：5、8、10、12、15 秒；
- 支持分辨率：720p。

### 3.7 `seedance-2.0-fast`

- 支持时长：5、8、10、12、15 秒；
- 支持分辨率：720p。

### 3.8 `seedance-2.5`

- 支持时长：5、8、10、12、15、20 秒；
- 支持分辨率：720p；
- 在 Seedance 系列当前列表中支持最长 20 秒。

### 3.9 `grok-imagine-video-1.5（按次）`

- 支持时长：5、8、10、12、15 秒；
- 支持分辨率：720p；
- 模型名称中的 `（按次）` 是名称的一部分；
- 实际收费方式和价格以 New API 管理员配置为准。

### 3.10 `sd2.0 满血`

- 支持时长：5、8、10、12、15 秒；
- 支持分辨率：720p；
- 模型名称中包含一个空格，不能删除或替换。

## 4. 错误排查

| 现象 | 常见原因 | 处理方式 |
|---|---|---|
| HTTP 401 | New API Token 无效或缺失 | 检查 `Authorization: Bearer ...` |
| `/v1/models` 中没有模型 | 分组、模型权限、渠道或价格未配置 | 联系 New API 管理员检查配置 |
| `unsupported model` | 模型名错误 | 从 `/v1/models` 复制模型名 |
| `duration is not supported` | 时长不在模型支持列表 | 按本文模型表修改 `duration` |
| `resolution is not supported` | 模型不支持该分辨率 | 改用模型支持的分辨率 |
| 参考素材 URL 被拒绝 | 使用了本地地址或非 HTTP/HTTPS 地址 | 换成上游可访问的公网 URL |
| 任务长时间未结束 | 上游排队或生成耗时较长 | 保持至少 20 秒查询间隔并联系管理员 |
| 下载返回未完成错误 | 任务尚未 `completed` | 等待成功后再访问 `/content` |

创建任务可能产生费用。重复提交同一请求通常会创建多个独立任务，并分别计费；
客户端发生超时时，应先确认是否已收到任务 ID，不要立即无条件重试。

## 5. 安全建议

- 只在 HTTPS 网络中调用正式服务；
- 不要把 New API Token 写入前端网页、公开仓库、截图或聊天记录；
- 不要尝试获取或传递渠道上游 API Key；
- 只保存 New API 返回的公共任务 ID；
- 服务端应用应通过环境变量或密钥管理服务保存 Token；
- 不同用户只能查询和下载自己创建的任务。

## 6. 导出为 PDF

这份文档使用标准 Markdown 表格和代码块，可以通过以下方式导出：

1. 使用 Typora 打开本文，选择“文件 → 导出 → PDF”；
2. 使用 Obsidian 打开本文，选择“导出为 PDF”；
3. 在支持 Markdown 预览的编辑器中打开预览，再使用 macOS 的“打印 →
   存储为 PDF”；
4. 使用 Pandoc 和 XeLaTeX 从命令行生成 PDF。

Pandoc 示例：

```bash
pandoc docs/task-plugins/huizhen-ai/user-guide.md \
  --pdf-engine=xelatex \
  -V CJKmainfont="PingFang SC" \
  -V geometry:margin=22mm \
  -o huizhen-ai-video-api-user-guide.pdf
```

命令行方案需要预先安装 `pandoc` 和带有 `xelatex` 的 TeX 环境。如果只是
临时导出，Typora、Obsidian或浏览器打印通常更方便。
