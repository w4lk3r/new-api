export const meta = {
  apiVersion: 1,
  key: "custom-video",
  name: "Custom Video",
  version: "1.0.1",
  author: { name: "Local Integration" },
  description: {
    en: "30-second video generation through a custom video API",
    zh: "通过 OpenAI Video API 接入自定义 30 秒视频生成接口",
  },
  models: ["custom-video-30s"],
  fetchMode: "per_task",
  protocols: ["openai_video"],
  usageSchema: {
    calls: {
      type: "number",
      unit: "count",
      description: {
        en: "Video generation request count.",
        zh: "视频生成任务次数。",
      },
    },
  },
  routes: [
    {
      method: "GET",
      path: "/custom/v1/videos/:task_id",
      type: "query",
      render: "renderCustomTask",
    },
  ],
};

const ALLOWED_RATIOS = ["16:9", "9:16", "1:1", "3:4", "4:3", "21:9"];
const MAX_IMAGES = 9;
const MAX_IMAGE_BYTES = 20 * 1024 * 1024;

function trimmed(value) {
  return String(value === undefined || value === null ? "" : value).trim();
}

function firstField(fields, name, fallback) {
  const values = (fields || {})[name] || [];
  if (values.length > 1) throw new Error(name + " must be provided once");
  return values.length ? values[0] : fallback;
}

function validatePrompt(prompt) {
  const value = trimmed(prompt);
  const length = Array.from(value).length;
  if (length < 1 || length > 3000) throw new Error("prompt must contain 1 to 3000 characters");
  return value;
}

function validateRatio(ratio) {
  const value = trimmed(ratio);
  if (!ALLOWED_RATIOS.includes(value)) throw new Error("unsupported ratio");
  return value;
}

function validateSeconds(seconds) {
  const value = trimmed(seconds || "30");
  if (value !== "30") throw new Error("seconds must be 30");
  return value;
}

function validateFiles(files) {
  const input = Array.isArray(files) ? files : [];
  if (input.length > MAX_IMAGES) throw new Error("at most 9 images are allowed");

  let totalBytes = 0;
  let repeatedImagesField = 0;
  for (const file of input) {
    const field = trimmed(file.field);
    const accepted = field === "input_reference" || field === "images[]" || /^image_[1-9]$/.test(field);
    if (!accepted) throw new Error("unexpected image field: " + field);
    if (field === "images[]") repeatedImagesField += 1;

    const mimeType = trimmed(file.mimeType).toLowerCase();
    if (mimeType !== "image/jpeg" && mimeType !== "image/png") {
      throw new Error("only JPEG and PNG images are supported");
    }
    totalBytes += Number(file.size || 0);
  }

  // Task Plugin v1 file refs are field-based. Repeating images[] would cause
  // each outbound reference to resolve to the first uploaded file.
  if (repeatedImagesField > 1) {
    throw new Error("use image_1 through image_9 for multiple images");
  }
  if (totalBytes > MAX_IMAGE_BYTES) throw new Error("total image size exceeds 20 MiB");
  return input;
}

function idempotencyKey(publicTaskId) {
  let value = "video-" + trimmed(publicTaskId);
  value = value.replace(/[^A-Za-z0-9._:-]/g, "-");
  if (value.length < 8) value += "-request";
  return value.slice(0, 128);
}

function baseURL(value) {
  return trimmed(value).replace(/\/+$/, "");
}

function normalizedRequest(ctx) {
  if (!ctx.body || (ctx.body.kind !== "json" && ctx.body.kind !== "multipart")) {
    throw new Error("JSON or multipart body required");
  }

  let prompt;
  let ratio;
  let seconds;
  let files = [];
  if (ctx.body.kind === "json") {
    const body = ctx.body.value;
    if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("JSON object required");
    prompt = body.prompt;
    ratio = body.ratio;
    seconds = body.seconds;
  } else {
    prompt = firstField(ctx.body.fields, "prompt", "");
    ratio = firstField(ctx.body.fields, "ratio", "");
    seconds = firstField(ctx.body.fields, "seconds", "30");
    files = validateFiles(ctx.body.files);
  }

  return {
    kind: "submit",
    model: ctx.model,
    action: files.length ? "image_to_video" : "text_to_video",
    requestBody: {
      prompt: validatePrompt(prompt),
      ratio: validateRatio(ratio),
      seconds: validateSeconds(seconds),
    },
  };
}

export function extractUsage() {
  return { calls: 1 };
}

export function buildSubmitRequest(ctx) {
  const request = ctx.requestBody || {};
  const files = validateFiles(ctx.files);
  const parts = [
    { name: "prompt", value: validatePrompt(request.prompt) },
    { name: "ratio", value: validateRatio(request.ratio) },
    { name: "seconds", value: validateSeconds(request.seconds) },
  ];
  for (const file of files) {
    parts.push({
      name: "images[]",
      fileRef: file.ref,
      filename: file.filename,
    });
  }

  return {
    url: baseURL(ctx.baseUrl) + "/api/v1/videos",
    method: "POST",
    headers: {
      Authorization: "Bearer " + ctx.apiKey,
      Accept: "application/json",
      "Idempotency-Key": idempotencyKey(ctx.publicTaskId),
    },
    bodyType: "multipart",
    parts: parts,
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  if (response.statusCode < 200 || response.statusCode >= 300 || String(body.code) !== "1") {
    throw new Error(trimmed(body.message) || "target video submission failed");
  }
  const taskId = trimmed(body.task_id);
  if (!taskId) throw new Error("target response is missing task_id");
  return {
    taskId: taskId,
    taskData: {
      code: String(body.code),
      task_id: taskId,
      status: trimmed(body.status) || "queued",
    },
  };
}

export function buildQueryRequest(ctx) {
  const taskId = trimmed(ctx.taskId);
  if (!taskId) throw new Error("task_id is required");
  return {
    url: baseURL(ctx.baseUrl) + "/api/v1/videos/" + encodeURIComponent(taskId),
    method: "GET",
    headers: {
      Authorization: "Bearer " + ctx.apiKey,
      Accept: "application/json",
    },
  };
}

export function parseTaskResult(ctx, body) {
  const task = body && body.task;
  if (!task || typeof task !== "object" || Array.isArray(task)) {
    throw new Error(trimmed(body && body.message) || "target query response is missing task");
  }

  const statusMap = {
    queued: "QUEUED",
    processing: "IN_PROGRESS",
    succeeded: "SUCCESS",
    failed: "FAILURE",
  };
  const status = statusMap[trimmed(task.status)] || "UNKNOWN";
  const result = { status: status };
  if (status === "QUEUED") result.progress = "0%";
  if (status === "SUCCESS" || status === "FAILURE") result.progress = "100%";
  if (status === "SUCCESS" && trimmed(task.url)) result.url = trimmed(task.url);
  if (status === "FAILURE") result.reason = trimmed(task.error) || trimmed(body.message) || "video generation failed";
  return result;
}

function storedTargetTask(data) {
  if (!data || typeof data !== "object" || Array.isArray(data)) return {};
  if (data.task && typeof data.task === "object" && !Array.isArray(data.task)) return data.task;
  if (data.data && data.data.task && typeof data.data.task === "object" && !Array.isArray(data.data.task)) return data.data.task;
  return {};
}

export function listArtifacts(task) {
  const targetTask = storedTargetTask(task && task.data);
  return String(task && task.status).toUpperCase() === "SUCCESS" && trimmed(targetTask.url)
    ? [{ key: "video", type: "video", mimeType: "video/mp4" }]
    : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const targetTask = storedTargetTask(ctx.data);
  const url = trimmed(targetTask.url);
  if (!url) throw new Error("artifact_not_found");
  return {
    url: url,
    method: ctx.clientRequest.method,
    credentialless: true,
  };
}

export const protocols = {
  openai_video: {
    decodeRequest: normalizedRequest,
    render: function (ctx, task) {
      return {
        id: task.task_id,
        object: "video",
        model: (task.properties || {}).origin_model_name || "custom-video-30s",
        status: "queued",
        progress: 0,
        created_at: Number(task.created_at || 0),
        seconds: "30",
      };
    },
  },
};

function publicStatus(status) {
  const values = {
    NOT_START: "queued",
    SUBMITTED: "queued",
    QUEUED: "queued",
    IN_PROGRESS: "processing",
    SUCCESS: "succeeded",
    FAILURE: "failed",
  };
  return values[String(status || "").toUpperCase()] || "processing";
}

export const native = {
  renderCustomTask: function (ctx, task) {
    const targetTask = storedTargetTask(task.data);
    const status = publicStatus(task.status);
    const output = {
      task_id: task.task_id,
      status: status,
    };
    for (const key of ["estimated_wait", "created_at", "updated_at"]) {
      if (targetTask[key] !== undefined) output[key] = targetTask[key];
    }
    if (status === "succeeded" && trimmed(targetTask.url)) output.url = trimmed(targetTask.url);
    if (status === "failed") output.error = trimmed(task.fail_reason) || trimmed(targetTask.error) || "video generation failed";

    const messages = {
      queued: "任务排队中",
      processing: "视频生成中",
      succeeded: "生成成功",
      failed: "任务生成失败",
    };
    return {
      code: status === "succeeded" ? "1" : "0",
      message: messages[status],
      task: output,
    };
  },
  error: function (ctx, error) {
    return { code: "0", message: error.message };
  },
};
