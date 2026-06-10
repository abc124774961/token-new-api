package upstreamerror

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
)

type Signal struct {
	StatusCode int
	ErrorCode  string
	ErrorType  string
	Message    string
	Metadata   string
	Header     string
}

type Result struct {
	Matched           bool
	Kind              string
	MatchedRuleID     string
	ErrorCategory     string
	SchedulerAction   string
	AvoidanceReason   string
	AvoidanceSeconds  int
	RetryAfterSeconds int
}

type Classifier interface {
	Classify(signal Signal) Result
}

type RuleMatcher interface {
	Match(signal Signal) bool
}

type KindPolicy interface {
	Resolve(kind string) Policy
}

type RuleCompiler interface {
	Compile(enabled bool, rules []scheduler_setting.UpstreamErrorRule) Classifier
}

type Policy struct {
	Kind            string
	ErrorCategory   string
	DefaultAction   string
	AvoidanceReason string
}

type ClassifierManager struct {
	compiler RuleCompiler
	current  atomic.Value
}

func NewClassifierManager(compiler RuleCompiler) *ClassifierManager {
	if compiler == nil {
		compiler = DefaultRuleCompiler()
	}
	manager := &ClassifierManager{compiler: compiler}
	manager.Reload(scheduler_setting.GetSetting())
	return manager
}

func (m *ClassifierManager) Reload(setting scheduler_setting.SchedulerSetting) {
	if m == nil {
		return
	}
	compiled := m.compiler.Compile(setting.UpstreamErrorClassificationEnabled, setting.UpstreamErrorRules)
	m.current.Store(compiled)
}

func (m *ClassifierManager) Classify(signal Signal) Result {
	if m == nil {
		return Result{}
	}
	value := m.current.Load()
	classifier, ok := value.(Classifier)
	if !ok || classifier == nil {
		return Result{}
	}
	return classifier.Classify(signal)
}

var defaultManager = NewClassifierManager(DefaultRuleCompiler())

func init() {
	scheduler_setting.AddChangeHook(func(before scheduler_setting.SchedulerSetting, after scheduler_setting.SchedulerSetting) {
		if upstreamErrorSettingSignature(before) == upstreamErrorSettingSignature(after) {
			return
		}
		defaultManager.Reload(after)
	})
}

func SyncDefaultManager() {
	defaultManager.Reload(scheduler_setting.GetSetting())
}

func Classify(signal Signal) Result {
	return defaultManager.Classify(signal)
}

func ClassifyAPIError(err *types.NewAPIError) Result {
	if err == nil {
		return Result{}
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeChannelConcurrencyLimit,
		types.ErrorCodeChannelResponseTimeExceeded:
		return Result{}
	case types.ErrorCodeInsufficientUserQuota:
		if types.IsSkipRetryError(err) {
			return Result{}
		}
	}
	return Classify(SignalFromAPIError(err))
}

func SignalFromAPIError(err *types.NewAPIError) Signal {
	if err == nil {
		return Signal{}
	}
	openAIError := err.ToOpenAIError()
	errorCode := strings.TrimSpace(fmt.Sprint(openAIError.Code))
	if errorCode == "" || errorCode == "<nil>" {
		errorCode = string(err.GetErrorCode())
	}
	errorType := strings.TrimSpace(openAIError.Type)
	if errorType == "" {
		errorType = string(err.GetErrorType())
	}
	message := strings.Join(uniqueNonEmptyStrings([]string{
		err.Error(),
		openAIError.Message,
	}), " ")
	metadata := string(err.Metadata)
	return Signal{
		StatusCode: err.StatusCode,
		ErrorCode:  errorCode,
		ErrorType:  errorType,
		Message:    message,
		Metadata:   metadata,
		Header:     metadata,
	}
}

func DefaultRuleCompiler() RuleCompiler {
	return ruleCompiler{policy: defaultKindPolicy{}}
}

type ruleCompiler struct {
	policy KindPolicy
}

func (c ruleCompiler) Compile(enabled bool, rules []scheduler_setting.UpstreamErrorRule) Classifier {
	if c.policy == nil {
		c.policy = defaultKindPolicy{}
	}
	if len(rules) == 0 {
		rules = scheduler_setting.DefaultUpstreamErrorRules()
	}
	compiled := make([]compiledRule, 0, len(rules))
	for index, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Kind = normalizeToken(rule.Kind)
		rule.SchedulerAction = normalizeToken(rule.SchedulerAction)
		if rule.ID == "" || rule.Kind == "" || !rule.Enabled {
			continue
		}
		matcher := compileRuleMatcher(rule)
		if matcher == nil {
			continue
		}
		compiled = append(compiled, compiledRule{
			rule:          rule,
			matcher:       matcher,
			originalIndex: index,
		})
	}
	sort.SliceStable(compiled, func(i, j int) bool {
		if compiled[i].rule.Priority == compiled[j].rule.Priority {
			return compiled[i].originalIndex < compiled[j].originalIndex
		}
		return compiled[i].rule.Priority > compiled[j].rule.Priority
	})
	return &compiledClassifier{
		enabled: enabled,
		rules:   compiled,
		policy:  c.policy,
	}
}

type compiledClassifier struct {
	enabled bool
	rules   []compiledRule
	policy  KindPolicy
}

func (c *compiledClassifier) Classify(signal Signal) Result {
	if c == nil || !c.enabled || signal.empty() {
		return Result{}
	}
	for _, rule := range c.rules {
		if !rule.matcher.Match(signal) {
			continue
		}
		policy := c.policy.Resolve(rule.rule.Kind)
		if policy.Kind == "" {
			continue
		}
		action := normalizeToken(rule.rule.SchedulerAction)
		if action == "" {
			action = policy.DefaultAction
		}
		result := Result{
			Matched:           true,
			Kind:              policy.Kind,
			MatchedRuleID:     rule.rule.ID,
			ErrorCategory:     policy.ErrorCategory,
			SchedulerAction:   action,
			AvoidanceReason:   policy.AvoidanceReason,
			AvoidanceSeconds:  rule.rule.AvoidanceSeconds,
			RetryAfterSeconds: extractRetryAfterSeconds(signal.Metadata),
		}
		if result.AvoidanceReason == "" && result.Kind != "" {
			result.AvoidanceReason = "upstream_" + result.Kind
		}
		return result
	}
	return Result{}
}

type compiledRule struct {
	rule          scheduler_setting.UpstreamErrorRule
	matcher       RuleMatcher
	originalIndex int
}

type compiledRuleMatcher struct {
	statusCodes map[int]struct{}
	code        []string
	errorType   []string
	message     []string
	metadata    []string
	header      []string
	hasKeyword  bool
}

func compileRuleMatcher(rule scheduler_setting.UpstreamErrorRule) RuleMatcher {
	matcher := &compiledRuleMatcher{
		statusCodes: make(map[int]struct{}, len(rule.StatusCodes)),
		code:        normalizeKeywords(rule.Keywords.Code),
		errorType:   normalizeKeywords(rule.Keywords.Type),
		message:     normalizeKeywords(rule.Keywords.Message),
		metadata:    normalizeKeywords(rule.Keywords.Metadata),
		header:      normalizeKeywords(rule.Keywords.Header),
	}
	for _, statusCode := range rule.StatusCodes {
		if statusCode <= 0 {
			continue
		}
		matcher.statusCodes[statusCode] = struct{}{}
	}
	matcher.hasKeyword = len(matcher.code)+len(matcher.errorType)+len(matcher.message)+len(matcher.metadata)+len(matcher.header) > 0
	if len(matcher.statusCodes) == 0 && !matcher.hasKeyword {
		return nil
	}
	return matcher
}

func (m *compiledRuleMatcher) Match(signal Signal) bool {
	if m == nil {
		return false
	}
	if len(m.statusCodes) > 0 {
		if _, ok := m.statusCodes[signal.StatusCode]; !ok {
			return false
		}
	}
	if !m.hasKeyword {
		return true
	}
	return containsAnyKeyword(signal.ErrorCode, m.code) ||
		containsAnyKeyword(signal.ErrorType, m.errorType) ||
		containsAnyKeyword(signal.Message, m.message) ||
		containsAnyKeyword(signal.Metadata, m.metadata) ||
		containsAnyKeyword(signal.Header, m.header)
}

type defaultKindPolicy struct{}

func (defaultKindPolicy) Resolve(kind string) Policy {
	kind = normalizeToken(kind)
	switch kind {
	case scheduler_setting.UpstreamErrorKindBalanceQuota:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryBalanceOrQuota, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindRateLimit:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryRateLimit, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindConcurrencyLimit:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryUpstreamConcurrencyLimit, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindCapacityOverload:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryOverloadSkip, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindModelPoolUnavailable,
		scheduler_setting.UpstreamErrorKindToolEndpointUnavailable:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryUpstreamError, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindAuthAccount,
		scheduler_setting.UpstreamErrorKindAccessRegion:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryAuthConfigError, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindRequestLimit,
		scheduler_setting.UpstreamErrorKindPolicySafety:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryClientRequestError, DefaultAction: scheduler_setting.UpstreamErrorActionStop}
	case scheduler_setting.UpstreamErrorKindTransportTimeout:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryTimeout, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindBadResponse:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryServerError, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	case scheduler_setting.UpstreamErrorKindStreamInterrupted:
		return Policy{Kind: kind, ErrorCategory: core.ErrorCategoryStreamInterrupted, DefaultAction: scheduler_setting.UpstreamErrorActionSwitchChannel}
	default:
		return Policy{}
	}
}

func IsKnownKind(kind string) bool {
	kind = normalizeToken(kind)
	for _, value := range scheduler_setting.UpstreamErrorKinds() {
		if kind == value {
			return true
		}
	}
	return false
}

func IsKnownAction(action string) bool {
	action = normalizeToken(action)
	for _, value := range scheduler_setting.UpstreamErrorActions() {
		if action == value {
			return true
		}
	}
	return false
}

func ShouldSwitchChannel(result Result) bool {
	return result.Matched && result.SchedulerAction == scheduler_setting.UpstreamErrorActionSwitchChannel
}

func ShouldStop(result Result) bool {
	return result.Matched && result.SchedulerAction == scheduler_setting.UpstreamErrorActionStop
}

func ShouldRecordOnly(result Result) bool {
	return result.Matched && result.SchedulerAction == scheduler_setting.UpstreamErrorActionRecordOnly
}

func normalizeKeywords(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func containsAnyKeyword(value string, keywords []string) bool {
	if value == "" || len(keywords) == 0 {
		return false
	}
	value = strings.ToLower(value)
	for _, keyword := range keywords {
		if keyword != "" && strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}

func (s Signal) empty() bool {
	return s.StatusCode == 0 &&
		strings.TrimSpace(s.ErrorCode) == "" &&
		strings.TrimSpace(s.ErrorType) == "" &&
		strings.TrimSpace(s.Message) == "" &&
		strings.TrimSpace(s.Metadata) == "" &&
		strings.TrimSpace(s.Header) == ""
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func extractRetryAfterSeconds(metadata string) int {
	metadata = strings.TrimSpace(metadata)
	if metadata == "" {
		return 0
	}
	lower := strings.ToLower(metadata)
	key := `"retry_after_seconds"`
	idx := strings.Index(lower, key)
	if idx < 0 {
		return 0
	}
	tail := lower[idx+len(key):]
	colon := strings.Index(tail, ":")
	if colon < 0 {
		return 0
	}
	tail = strings.TrimLeft(tail[colon+1:], " \t\r\n")
	end := 0
	for end < len(tail) && tail[end] >= '0' && tail[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	seconds, err := strconv.Atoi(tail[:end])
	if err != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

func upstreamErrorSettingSignature(setting scheduler_setting.SchedulerSetting) string {
	builder := strings.Builder{}
	builder.WriteString(strconv.FormatBool(setting.UpstreamErrorClassificationEnabled))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(setting.UpstreamErrorRuleVersion))
	builder.WriteByte('|')
	for _, rule := range setting.UpstreamErrorRules {
		builder.WriteString(rule.ID)
		builder.WriteByte(':')
		builder.WriteString(strconv.FormatBool(rule.Enabled))
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(rule.Priority))
		builder.WriteByte(':')
		builder.WriteString(rule.Kind)
		builder.WriteByte(':')
		builder.WriteString(rule.SchedulerAction)
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(rule.AvoidanceSeconds))
		builder.WriteByte(':')
		for _, status := range rule.StatusCodes {
			builder.WriteString(strconv.Itoa(status))
			builder.WriteByte(',')
		}
		builder.WriteByte(':')
		writeKeywordsSignature(&builder, rule.Keywords.Code)
		writeKeywordsSignature(&builder, rule.Keywords.Type)
		writeKeywordsSignature(&builder, rule.Keywords.Message)
		writeKeywordsSignature(&builder, rule.Keywords.Metadata)
		writeKeywordsSignature(&builder, rule.Keywords.Header)
		builder.WriteByte('|')
	}
	return builder.String()
}

func writeKeywordsSignature(builder *strings.Builder, values []string) {
	if builder == nil {
		return
	}
	for _, value := range values {
		builder.WriteString(value)
		builder.WriteByte(',')
	}
	builder.WriteByte(';')
}
