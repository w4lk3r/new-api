export const meta = {
  apiVersion: 1,
  key: "huizhen-ai",
  name: "绘帧AI",
  version: "1.0.0",
  author: { name: "Local Integration" },
  description: {
    en: "Video generation through the Huizhen AI unified video API",
    zh: "通过绘帧AI统一视频 API 生成视频",
  },
  models: [
    "sd2-c6-10s",
    "sd2.5",
    "wan-3.0-3",
    "seedance-2.0-mini-2",
    "seedance-2.0",
    "seedance-2.0-fast",
    "seedance-2.5",
    "grok-imagine-video-1.5（按次）",
    "sd2.0满血",
  ],
  fetchMode: "per_task",
  auth: "api_key",
  protocols: ["openai_video"],
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Requested video duration in seconds.", zh: "请求的视频时长（秒）。" },
    },
    resolution: {
      enum: ["480p", "720p"],
      description: { en: "Requested public video resolution.", zh: "请求的公共视频清晰度。" },
    },
  },
};

const MODEL_CAPABILITIES = {
  "sd2-c6-10s": { durations: [10, 8, 5], resolutions: ["720p"] },
  "sd2.5": { durations: [30], resolutions: ["720p"] },
  "wan-3.0-3": { durations: [30, 25, 20, 15, 10, 5], resolutions: ["720p"] },
  "seedance-2.0-mini-2": { durations: [15, 12, 10, 8, 5], resolutions: ["480p", "720p"] },
  "seedance-2.0": { durations: [15, 12, 10, 8, 5], resolutions: ["720p"] },
  "seedance-2.0-fast": { durations: [15, 12, 10, 8, 5], resolutions: ["720p"] },
  "seedance-2.5": { durations: [20, 15, 12, 10, 8, 5], resolutions: ["720p"] },
  "grok-imagine-video-1.5（按次）": { durations: [15, 12, 10, 8, 5], resolutions: ["720p"] },
  "sd2.0满血": { durations: [15, 12, 10, 8, 5], resolutions: ["720p"] },
};

const MAX_PROMPT_CHARACTERS = 3000;
const MAX_REFERENCES_PER_TYPE = 9;
const MAX_REFERENCE_URL_LENGTH = 4096;

function trimmed(value) {
  return String(value === undefined || value === null ? "" : value).trim();
}

function baseURL(value) {
  return trimmed(value).replace(/\/+$/, "");
}

function validatePrompt(value) {
  const prompt = trimmed(value);
  if (!prompt) throw new Error("prompt is required");
  if (Array.from(prompt).length > MAX_PROMPT_CHARACTERS) throw new Error("prompt must not exceed 3000 characters");
  return prompt;
}

function validateModel(value) {
  const model = trimmed(value);
  if (!Object.prototype.hasOwnProperty.call(MODEL_CAPABILITIES, model)) throw new Error("unsupported model");
  return model;
}

function validateDuration(value, model) {
  const supported = MODEL_CAPABILITIES[model].durations;
  const duration = value === undefined || value === null || value === "" ? supported[0] : Number(value);
  if (!Number.isInteger(duration) || !supported.includes(duration)) throw new Error("duration is not supported by model " + model);
  return duration;
}

function resolutionFromSize(value) {
  const match = /^(\d+)x(\d+)$/.exec(trimmed(value));
  if (!match) return "";
  const shorterEdge = Math.min(Number(match[1]), Number(match[2]));
  return shorterEdge === 480 || shorterEdge === 720 ? shorterEdge + "p" : "";
}

function validateResolution(value, size, model) {
  const supported = MODEL_CAPABILITIES[model].resolutions;
  let resolution = trimmed(value);
  const sizedResolution = resolutionFromSize(size);
  if (size && !sizedResolution) throw new Error("size must describe a supported 480p or 720p frame");
  if (resolution && sizedResolution && resolution !== sizedResolution) throw new Error("resolution and size must be consistent");
  if (!resolution && sizedResolution) resolution = sizedResolution;
  if (!resolution) resolution = supported[0];
  if (!supported.includes(resolution)) throw new Error("resolution is not supported by model " + model);
  return resolution;
}

function validateAspectRatio(value) {
  const ratio = trimmed(value);
  if (ratio && !/^[1-9]\d{0,2}:[1-9]\d{0,2}$/.test(ratio)) throw new Error("aspect_ratio must use W:H format");
  return ratio;
}

function validateReferences(value, name) {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value)) throw new Error(name + " must be an array");
  if (value.length > MAX_REFERENCES_PER_TYPE) throw new Error(name + " must not contain more than 9 URLs");
  return value.map(function (item) {
    const url = trimmed(item);
    if (!/^https?:\/\/[^\s]+$/i.test(url) || url.length > MAX_REFERENCE_URL_LENGTH) {
      throw new Error(name + " must contain public HTTP(S) URLs");
    }
    return url;
  });
}

function normalizedRequest(request, model) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("request body must be an object");
  const upstreamModel = validateModel(model);
  if (request.duration !== undefined && request.seconds !== undefined && Number(request.duration) !== Number(request.seconds)) {
    throw new Error("duration and seconds must be consistent");
  }
  const durationValue = request.duration === undefined ? request.seconds : request.duration;
  const ratioValues = [request.aspect_ratio, request.aspectRatio, request.ratio]
    .filter(function (value) {
      return value !== undefined && value !== null && trimmed(value) !== "";
    })
    .map(trimmed);
  if (ratioValues.some(function (value) { return value !== ratioValues[0]; })) {
    throw new Error("aspect_ratio aliases must be consistent");
  }
  const imageRefs = validateReferences(request.image_refs, "image_refs");
  const videoRefs = validateReferences(request.video_refs, "video_refs");
  const audioRefs = validateReferences(request.audio_refs, "audio_refs");
  if (request.compliance_enabled !== undefined && typeof request.compliance_enabled !== "boolean") {
    throw new Error("compliance_enabled must be a boolean");
  }
  const size = trimmed(request.size);
  const resolution = validateResolution(request.resolution, size, upstreamModel);
  const body = {
    model: upstreamModel,
    prompt: validatePrompt(request.prompt),
    duration: validateDuration(durationValue, upstreamModel),
  };
  if (trimmed(request.resolution)) body.resolution = resolution;
  else if (!size) body.resolution = resolution;
  if (size) body.size = size;
  const aspectRatio = validateAspectRatio(ratioValues[0]);
  if (aspectRatio) body.aspect_ratio = aspectRatio;
  if (imageRefs.length) body.image_refs = imageRefs;
  if (videoRefs.length) body.video_refs = videoRefs;
  if (audioRefs.length) body.audio_refs = audioRefs;
  if (request.compliance_enabled !== undefined) body.compliance_enabled = request.compliance_enabled;
  return { body: body, resolution: resolution };
}

export function buildSubmitRequest(ctx) {
  const normalized = normalizedRequest(ctx.requestBody, ctx.upstreamModel || ctx.model);
  return {
    url: baseURL(ctx.baseUrl) + "/openapi/v1/videos",
    method: "POST",
    headers: {
      Authorization: "Bearer " + ctx.apiKey,
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: normalized.body,
  };
}

export function parseSubmitResponse(_ctx, response) {
  const body = response.body || {};
  if (response.statusCode < 200 || response.statusCode >= 300) {
    throw new Error(trimmed(body.error && body.error.message) || trimmed(body.message) || "video submission failed");
  }
  const taskId = trimmed(body.id);
  if (!taskId) throw new Error("video submission response is missing id");
  return { taskId: taskId, taskData: body };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const normalized = normalizedRequest(ctx.requestBody, ctx.upstreamModel || ctx.model);
  return { seconds: normalized.body.duration, resolution: normalized.resolution };
}

export function extractUsageOnComplete(_task, _result, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) return null;
  const facts = {};
  const seconds = Number(body.seconds);
  if (Number.isFinite(seconds) && seconds > 0 && seconds <= 3600) facts.seconds = seconds;
  const resolution = resolutionFromSize(body.size);
  if (resolution === "480p" || resolution === "720p") facts.resolution = resolution;
  return Object.keys(facts).length ? facts : null;
}

export function buildQueryRequest(ctx) {
  const taskId = trimmed(ctx.taskId);
  if (!taskId) throw new Error("task id is required");
  return {
    url: baseURL(ctx.baseUrl) + "/openapi/v1/videos/" + encodeURIComponent(taskId),
    method: "GET",
    headers: { Authorization: "Bearer " + ctx.apiKey, Accept: "application/json" },
  };
}

export function parseTaskResult(_ctx, body) {
  if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("invalid task response");
  const statuses = { queued: "QUEUED", in_progress: "IN_PROGRESS", completed: "SUCCESS", failed: "FAILURE" };
  const status = statuses[trimmed(body.status)];
  if (!status) return { status: "UNKNOWN", reason: "unrecognized task status: " + trimmed(body.status) };
  const result = { status: status };
  const progress = Number(body.progress);
  if (Number.isFinite(progress) && progress >= 0 && progress <= 100) result.progress = progress + "%";
  if (status === "FAILURE") {
    result.reason = trimmed(body.error && body.error.message) || trimmed(body.error && body.error.code) || "video generation failed";
  }
  return result;
}

export function listArtifacts(task) {
  return String(task && task.status).toUpperCase() === "SUCCESS" ? [{ key: "video", type: "video", mimeType: "video/mp4" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const taskId = trimmed(ctx.upstreamTaskId);
  if (!taskId) throw new Error("artifact_not_found");
  const headers = { Authorization: "Bearer " + ctx.apiKey };
  for (const name of ["Range", "If-Range", "If-None-Match", "If-Modified-Since"]) {
    const value = trimmed(ctx.clientRequest && ctx.clientRequest.headers && ctx.clientRequest.headers[name]);
    if (value) headers[name] = value;
  }
  return {
    url: baseURL(ctx.baseUrl) + "/openapi/v1/videos/" + encodeURIComponent(taskId) + "/content",
    method: ctx.clientRequest.method,
    headers: headers,
  };
}

function publicStatus(status) {
  const values = {
    NOT_START: "queued",
    SUBMITTED: "queued",
    QUEUED: "queued",
    IN_PROGRESS: "in_progress",
    SUCCESS: "completed",
    FAILURE: "failed",
  };
  return values[String(status || "").toUpperCase()] || "unknown";
}

function renderVideo(_ctx, task) {
  const output = {
    id: task.task_id,
    object: "video",
    model: (task.properties || {}).origin_model_name || "",
    status: publicStatus(task.status),
    progress: Number(String(task.progress || "0").replace("%", "")),
    created_at: Number(task.created_at || 0),
  };
  const completedAt = Number(task.finished_at || task.updated_at || 0);
  if (completedAt > 0) output.completed_at = completedAt;
  const data = task.data && typeof task.data === "object" && !Array.isArray(task.data) ? task.data : {};
  if (data.seconds !== undefined) output.seconds = String(data.seconds);
  if (trimmed(data.size)) output.size = trimmed(data.size);
  if (String(task.status).toUpperCase() === "FAILURE") {
    output.error = { code: "video_generation_failed", message: trimmed(task.fail_reason) || "The video generation task failed." };
  }
  return output;
}

export const protocols = {
  openai_video: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const normalized = normalizedRequest(ctx.body.value, ctx.model);
      const hasReferences = normalized.body.image_refs || normalized.body.video_refs || normalized.body.audio_refs;
      return {
        kind: "submit",
        model: ctx.model,
        action: hasReferences ? "image_to_video" : "text_to_video",
        requestBody: normalized.body,
      };
    },
    render: renderVideo,
  },
};
