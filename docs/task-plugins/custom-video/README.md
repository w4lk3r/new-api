# Custom Video Task Plugin

当前可安装插件是 [`custom-video.plugin.js`](./custom-video.plugin.js)，插件 key 为
`custom-video`，版本为 `1.0.1`，对外模型为 `custom-video-30s`。

它使用 New API 官方 Task Plugin 渠道和统一视频接口，不新增渠道类型：

```text
POST /v1/videos
GET  /v1/videos/{public_task_id}
GET  /v1/videos/{public_task_id}/content
```

## 安装与升级

1. 在“系统设置 → Task Plugins”上传插件；
2. 上传后明确激活 `custom-video 1.0.1`；
3. 新建或更新 `Task Plugin` 渠道，选择 `custom-video`；
4. 填写上游 Base URL 和 API Key，启用 `custom-video-30s`；
5. 为模型设置按次价格或计费表达式。

同一 `key + version` 的源码不可覆盖。若后台仍在使用 `1.0.0`，仅上传
`1.0.1` 不会自动切换，必须手工激活新版本。

## 计费语义

上游协议固定生成 30 秒视频，收到的协议资料以整次任务报告扣费，没有按秒或按
分辨率价格表。因此当前按次计费是正确语义。

`1.0.1` 声明了 `usageSchema.calls`，并为每个创建请求返回 `{calls: 1}`。因此
既可以使用普通模型按次价格，也可以用表达式：

```text
tier("base", u("calls") * 你的单次价格)
```

表达式仍然是固定按次价格。`tier("base", ...)` 中的 `tier` 只是账单档位名称，
不表示系统根据时长或其他参数自动选择了阶梯价。插件固定校验 `seconds=30`。

## 测试样本的区别

[`controller/testdata/custom-video`](../../../controller/testdata/custom-video/README.md)
保存的是最初收到的 `1.0.0` 原始文件，作为不可变回归样本。它没有
`usageSchema`，不会被替换为本目录的管理后台安装版本。

当前架构、宿主修复和发布版本见
[`video-task-plugin-architecture.md`](../../video-task-plugin-architecture.md)。
