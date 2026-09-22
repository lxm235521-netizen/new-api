# 定向请求审计（Prompt Audit）

默认关闭的运维排查功能：把**指定用户 + 指定模型**的请求/响应正文追加写入 JSONL 文件，用于风控、违规取证和故障定位。

- 未命中过滤条件的请求不产生任何 IO，也不写数据库日志。
- 只影响 `relay/` 中转路径，不改动计费与日志表结构。
- 输出文件包含用户原始输入，属敏感数据，请限制文件权限与访问范围。

## 实现位置

| 内容 | 位置 |
|---|---|
| 审计器本体 | `service/prompt_audit.go` |
| 配置项（后台可改） | `setting/prompt_audit_setting/prompt_audit_setting.go`，键名 `prompt_audit_setting.*` |
| 变更留痕 | `controller/option.go`（更新 `prompt_audit_setting.*` 时写一条 `LogTypeManage` 日志） |
| 后台界面（经典前端） | `web/classic/src/components/settings/PromptAuditSetting.jsx`、`web/classic/src/pages/Setting/Operation/SettingsPromptAudit.jsx`、`web/classic/src/pages/Setting/index.jsx`（「请求审计」tab，仅 root 可见） |
| 输入钩子（7 处，均为原有 debug 日志旁） | `relay/compatible_handler.go`、`relay/responses_handler.go`、`relay/claude_handler.go`、`relay/gemini_handler.go`、`relay/image_handler.go`、`relay/embedding_handler.go`、`relay/rerank_handler.go` |
| 输入钩子（透传 passthrough，记录客户端原始 body） | `compatible_handler.go`、`responses_handler.go`、`claude_handler.go`、`gemini_handler.go`、`image_handler.go`、`rerank_handler.go` 的 passthrough 分支，统一走 `service.AuditRequestBodyStorage` |
| 输入钩子（chat completions → Responses 模式） | `compatible_handler.go`、`claude_handler.go` 的 `chatCompletionsViaResponses` 分支 |
| 输出钩子（流式，所有渠道通用） | `relay/helper/stream_scanner.go` |
| 输出钩子（非流式） | `relay/channel/openai/relay-openai.go`、`relay/channel/gemini/relay-gemini.go`、`relay/channel/gemini/relay-gemini-native.go`、`relay/channel/claude/relay-claude.go` |

钩子挂在"请求体已转换完成"的位置，因此记录的是**实际上游请求体**（已包含渠道系统提示词注入与参数覆盖的结果）；若渠道开启 passthrough，则记录**客户端原始请求体**。

## 配置方式：后台设置（推荐）

登录 root → **系统设置 → 请求审计**，可配置：

| 表单项 | 选项键 | 说明 |
|---|---|---|
| 启用请求审计 | `prompt_audit_setting.enabled` | 总开关，默认关闭 |
| 监控用户 | `prompt_audit_setting.users` | 用户名或用户 id，逗号分隔，留空=所有用户 |
| 监控模型 | `prompt_audit_setting.models` | 逗号分隔，支持 `*` 通配，不带 `*` 按前缀匹配，留空=所有模型 |
| 单条上限 (字节) | `prompt_audit_setting.max_bytes` | `0` = 不截断（完整输入） |
| 只记录输入 | `prompt_audit_setting.skip_output` | 开启后不记输出与流式分片 |

保存后由 `updateOptionMap` 直接刷新内存，**无需重启容器**；每次修改都会写入一条管理日志，便于追溯"谁在什么时候监控了谁"。

只想监控单个用户 + 单个模型的完整输入，填：

```
启用请求审计：开
监控用户：ccx
监控模型：gemini-3.1-pro*
单条上限：0
```

`监控模型` 会同时比对客户端请求的模型名（`OriginModelName`）与映射后的上游模型名（`UpstreamModelName`），大小写不敏感。

`单条上限=0` 时正文完整落盘，代价是**含 base64 图片/音频/大文件的长上下文请求会产生很大的单行记录**，建议配合磁盘监控与定期清理；如果只想看文本，可填 `2097152`（2MB）之类的上限。

## 环境变量（应急覆盖）

环境变量**只要显式设置**就覆盖同名后台设置，用于应急开关或自动化部署；生产建议只留后台配置。

| 变量 | 覆盖的选项 | 说明 |
|---|---|---|
| `PROMPT_AUDIT_ENABLED` | `enabled` | `true`/`false` |
| `PROMPT_AUDIT_USERS` | `users` | 非空时生效，逗号分隔 |
| `PROMPT_AUDIT_MODELS` | `models` | 非空时生效，逗号分隔 |
| `PROMPT_AUDIT_MAX_BYTES` | `max_bytes` | `<=0` 表示不截断 |
| `PROMPT_AUDIT_SKIP_OUTPUT` | `skip_output` | `true` 时只记输入 |
| `PROMPT_AUDIT_FILE` | — | 输出文件路径；默认 `<LOG_DIR>/prompt-audit-YYYYMMDD.jsonl` |


## 输出格式

每行一个 JSON 对象，文件权限 `0600`，目录自动创建：

```json
{
  "time": "2026-02-01T12:00:00.123+08:00",
  "direction": "input",
  "request_id": "20260201120000-abcdef",
  "user_id": 42,
  "username": "alice",
  "token_name": "prod",
  "channel_id": 3,
  "group": "default",
  "model": "gemini-3.1-pro",
  "upstream_model": "gemini-3.1-pro-high",
  "is_stream": true,
  "payload_bytes": 8123,
  "truncated": false,
  "payload": "{\"model\":\"gemini-3.1-pro\",\"messages\":[...]}"
}
```

`direction` 取值：

- `input`：客户端请求（已转换的上游请求体）
- `output`：非流式响应正文
- `output_chunk`：流式响应分片（上游 SSE 原始行，推理模型的思考内容在这里）

## 查询

```bash
# 某用户全部记录
jq -c 'select(.username=="alice")' prompt-audit.jsonl

# 某次请求的输入 + 输出 + 思考分片
jq -r 'select(.request_id=="20260201120000-abcdef") | .direction + "\t" + .payload' prompt-audit.jsonl

# 只提取模型思考分片里的文本（Gemini 的 thought part）
jq -r 'select(.direction=="output_chunk") | .payload' prompt-audit.jsonl | grep -o '"thought":true'
```

## 注意事项

1. **无法回溯**：未开启审计时的请求正文不会保留，事后无法补查。
2. **输入是转换后的 body**：要看客户端原始 body，需要渠道开启 passthrough（`PassThroughRequestEnabled` / `PassThroughBodyEnabled`）。
3. **异步任务类接口不需要本功能**：绘图/视频等 `Task` 请求的 `data` 本来就入库，管理员任务日志直接可见。
4. **日志表的 `po` 字段不会泄露正文**：参数覆盖审计记录的是配置里的 `from` 模式，不是匹配到的用户文本。
5. **合规**：处理用户通信内容应限于风控/审计目的，建议告知用户、控制访问权限与留存期限，必要时脱敏（参考 `common/str.go` 的 `MaskSensitiveInfo`）。

## 验证记录

已在隔离环境（独立容器 + SQLite + 模拟上游，不接触线上库/线上容器）完成端到端验证，100 项断言全部通过：

| 场景 | 覆盖点 | 结果 |
|---|---|---|
| OpenAI 类型渠道（type 1） | input / output / output_chunk 三类记录，流式思考内容落盘，负例模型不记录，`0600` 权限 | 24 PASS |
| Gemini 类型渠道（type 24，OpenAI 风格客户端） | 记录的是**转换后的 Gemini 请求体**（`contents/parts`），非流式走 `GeminiChatHandler` 钩子，流式思考分片落盘 | 12 PASS |
| 用户过滤 + 超长截断 | `PROMPT_AUDIT_USERS=root` 时其他用户的请求成功但零记录；`PROMPT_AUDIT_MAX_BYTES=64` 时 `truncated=true`、`payload_bytes` 保留原长、尾部不入库 | 17 PASS |
| 渠道 passthrough + 不截断 | `setting={"pass_through_body_enabled":true}` 时落盘为**客户端原始 body**（`messages` 而非 `contents`）；`PROMPT_AUDIT_MAX_BYTES=0` 时 2116 字节正文完整落盘、`truncated=false`、尾部标记仍在 | 15 PASS |
| 透传钩子回归 | 透传渠道下用户过滤仍生效：root 完整记录、其他用户请求成功但零记录 | 11 PASS |
| 后台设置驱动（无任何环境变量） | 通过 `PUT /api/option/` 写 `prompt_audit_setting.*` 后**不重启即生效**；只记录监控用户、非监控用户零记录、完整输入落盘、输出同时记录；关闭后立即停止；5 次设置变更各留一条管理日志；重启后环境变量可覆盖后台配置 | 21 PASS |

其中「后台设置驱动」这一轮与页面保存走的是同一个接口（`PUT /api/option/`），经典前端页面本身通过 `rsbuild build` 全量构建验证。

复现要点（用管理接口搭建环境时容易踩的坑）：

- 建渠道必须带外层包装：`POST /api/channel/` body 为 `{"mode":"single","channel":{...}}`；漏掉 `channel` 包装会触发 `model/channel.go` `ValidateSettings()` 的空指针 panic。
- 给用户加额度用 `POST /api/user/manage`（body `{"id":N,"action":"add_quota","mode":"add","value":50000000}`）；`PUT /api/user/` 是整用户更新，只传部分字段会把其他字段清空，甚至报 `UNIQUE constraint failed: users.username`。
- 管理接口除会话 Cookie 外还需要 `New-Api-User: <用户id>` 请求头。
- 想让新注册用户可直接中转，先设置 `QuotaForNewUser` 选项。
- 另外 `Register` 的用户名校验只有 `max=20`，没有最小长度限制，值得单独修。

