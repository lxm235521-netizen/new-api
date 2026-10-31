package constant

var StreamingTimeout int
var DifyDebug bool
var MaxFileDownloadMB int
var StreamScannerMaxBufferMB int
var ForceStreamOption bool
var CountToken bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var MaxRequestBodyMB int
var AnonymousRequestBodyLimitKB int
var AzureDefaultAPIVersion string
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
var TaskTimeoutMinutes int

// TaskFetchTimeoutSeconds 单次上游任务状态查询的硬超时（秒）。
// 上游 accept 后不回包时，若不设超时，单线程轮询会被永久挂住。
var TaskFetchTimeoutSeconds int

// TaskChannelFailureLimit 单渠道连续查询超时多少次后跳过本轮剩余任务（防止单个上游拖死所有渠道）。
var TaskChannelFailureLimit int

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string

// TrustedRedirectDomains is a list of trusted domains for redirect URL validation.
// Domains support subdomain matching (e.g., "example.com" matches "sub.example.com").
var TrustedRedirectDomains []string
