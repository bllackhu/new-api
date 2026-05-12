package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const tokenPoolUsageDataSource = "rolling_redis"

const (
	tokenPoolUsageReasonNoResolvedPool     = "no_resolved_pool"
	tokenPoolUsageReasonRedisRequired      = "redis_required"
	tokenPoolUsageReasonWindowNotRetained  = "window_not_retained"
	tokenPoolUsageReasonTokenScopeDisabled = "token_scope_not_enabled"
	tokenPoolUsageReasonUserScopeOnly      = "user_scope_only"
)

var defaultTokenPoolUsageWindows = []string{"5h", "7d", "30d"}
var buildTokenPoolUsageItemFunc = buildTokenPoolUsageItem

type TokenPoolUsageBatchRequest struct {
	Ids     []int    `json:"ids"`
	Windows []string `json:"windows"`
}

type TokenPoolUsageWindow struct {
	Window        string `json:"window"`
	WindowSeconds int    `json:"window_seconds"`
	Available     bool   `json:"available"`
	Count         *int64 `json:"count,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type TokenPoolUsageItem struct {
	TokenId                int                              `json:"token_id"`
	TokenName              string                           `json:"token_name,omitempty"`
	PoolId                 int                              `json:"pool_id,omitempty"`
	PoolName               string                           `json:"pool_name,omitempty"`
	ScopeType              string                           `json:"scope_type,omitempty"`
	DataSource             string                           `json:"data_source"`
	RetentionWindowSeconds int                              `json:"retention_window_seconds,omitempty"`
	TokenScopeEnabled      bool                             `json:"token_scope_enabled"`
	Usage                  map[string]*TokenPoolUsageWindow `json:"usage"`
}

const tokenPoolLLMTokenDataSource = "consume_logs"

// TokenPoolLLMTokenWindow is rolling-window LLM token totals from consume logs for this API token.
type TokenPoolLLMTokenWindow struct {
	Window           string `json:"window"`
	WindowSeconds    int    `json:"window_seconds"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

// TokenPoolLLMTokenLifetime is all-time LLM token totals from retained consume logs for this API token.
type TokenPoolLLMTokenLifetime struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// TokenPoolLLMTokenUsage bundles per-window and lifetime aggregates for GET /api/usage/token/pool.
type TokenPoolLLMTokenUsage struct {
	DataSource string                              `json:"data_source"`
	ByWindow   map[string]*TokenPoolLLMTokenWindow `json:"by_window"`
	Lifetime   TokenPoolLLMTokenLifetime           `json:"lifetime"`
}

var sumConsumeLogTokensByTokenID = model.SumConsumeLogTokensByTokenID

func buildTokenPoolLLMTokenUsage(tokenId int, windows []string, windowSeconds map[string]int) (*TokenPoolLLMTokenUsage, error) {
	out := &TokenPoolLLMTokenUsage{
		DataSource: tokenPoolLLMTokenDataSource,
		ByWindow:   make(map[string]*TokenPoolLLMTokenWindow, len(windows)),
	}
	now := time.Now().Unix()
	for _, w := range windows {
		sec := windowSeconds[w]
		since := now - int64(sec)
		prompt, completion, err := sumConsumeLogTokensByTokenID(tokenId, since)
		if err != nil {
			return nil, err
		}
		out.ByWindow[w] = &TokenPoolLLMTokenWindow{
			Window:           w,
			WindowSeconds:    sec,
			PromptTokens:     prompt,
			CompletionTokens: completion,
			TotalTokens:      prompt + completion,
		}
	}
	lp, lc, err := sumConsumeLogTokensByTokenID(tokenId, 0)
	if err != nil {
		return nil, err
	}
	out.Lifetime = TokenPoolLLMTokenLifetime{
		PromptTokens:     lp,
		CompletionTokens: lc,
		TotalTokens:      lp + lc,
	}
	return out, nil
}

type tokenPoolUsageBuilderDeps struct {
	resolvePool   func(token *model.Token) (*model.Pool, error)
	loadPolicies  func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error)
	isRedisReady  func() bool
	countByWindow func(redisKey string, windowSeconds int) (int64, error)
}

func normalizeTokenPoolUsageWindows(input []string) ([]string, map[string]int, error) {
	if len(input) == 0 {
		input = defaultTokenPoolUsageWindows
	}
	seen := make(map[string]struct{}, len(input))
	windows := make([]string, 0, len(input))
	windowSeconds := make(map[string]int, len(input))
	for _, item := range input {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seconds, err := parseRollingWindow(item)
		if err != nil {
			return nil, nil, err
		}
		seen[item] = struct{}{}
		windows = append(windows, item)
		windowSeconds[item] = seconds
	}
	if len(windows) == 0 {
		return nil, nil, errors.New("at least one valid window is required")
	}
	sort.SliceStable(windows, func(i, j int) bool {
		return windowSeconds[windows[i]] < windowSeconds[windows[j]]
	})
	return windows, windowSeconds, nil
}

func filterValidRequestCountPolicies(policies []*model.PoolQuotaPolicy) ([]*model.PoolQuotaPolicy, int) {
	validPolicies := make([]*model.PoolQuotaPolicy, 0, len(policies))
	maxWindowSeconds := 0
	for _, policy := range policies {
		if policy == nil || !policy.Enabled || policy.WindowSeconds <= 0 || policy.LimitCount <= 0 {
			continue
		}
		validPolicies = append(validPolicies, policy)
		if policy.WindowSeconds > maxWindowSeconds {
			maxWindowSeconds = policy.WindowSeconds
		}
	}
	return validPolicies, maxWindowSeconds
}

func buildUnavailableTokenPoolUsage(item *TokenPoolUsageItem, windows []string, windowSeconds map[string]int, reason string) *TokenPoolUsageItem {
	item.Usage = make(map[string]*TokenPoolUsageWindow, len(windows))
	for _, window := range windows {
		item.Usage[window] = &TokenPoolUsageWindow{
			Window:        window,
			WindowSeconds: windowSeconds[window],
			Available:     false,
			Reason:        reason,
		}
	}
	return item
}

func countRollingWindowByRedisKey(ctx context.Context, redisKey string, windowSeconds int) (int64, error) {
	nowMs := time.Now().UnixMilli()
	startMs := nowMs - int64(windowSeconds)*1000
	return common.RDB.ZCount(ctx, redisKey, fmt.Sprintf("(%d", startMs), "+inf").Result()
}

func buildTokenPoolUsageItem(token *model.Token, windows []string, windowSeconds map[string]int) (*TokenPoolUsageItem, error) {
	deps := tokenPoolUsageBuilderDeps{
		resolvePool: func(token *model.Token) (*model.Pool, error) {
			return model.ResolvePoolForContext(token.UserId, token.Id, token.Group)
		},
		loadPolicies: func(poolId int, scopeType string) ([]*model.PoolQuotaPolicy, error) {
			return model.GetPoolQuotaPolicies(poolId, model.PoolQuotaMetricRequestCount, scopeType)
		},
		isRedisReady: func() bool {
			return common.RedisEnabled && common.RDB != nil
		},
		countByWindow: func(redisKey string, windowSeconds int) (int64, error) {
			return countRollingWindowByRedisKey(context.Background(), redisKey, windowSeconds)
		},
	}
	return buildTokenPoolUsageItemWithDeps(token, windows, windowSeconds, deps)
}

func buildTokenPoolUsageItemWithDeps(token *model.Token, windows []string, windowSeconds map[string]int, deps tokenPoolUsageBuilderDeps) (*TokenPoolUsageItem, error) {
	item := &TokenPoolUsageItem{
		TokenId:    token.Id,
		TokenName:  token.Name,
		DataSource: tokenPoolUsageDataSource,
		Usage:      make(map[string]*TokenPoolUsageWindow, len(windows)),
	}
	pool, err := deps.resolvePool(token)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonNoResolvedPool), nil
		}
		return nil, err
	}
	if pool == nil {
		return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonNoResolvedPool), nil
	}
	item.PoolId = pool.Id
	item.PoolName = pool.Name

	tokenPolicies, err := deps.loadPolicies(pool.Id, model.PoolQuotaScopeToken)
	if err != nil {
		return nil, err
	}
	validTokenPolicies, maxWindowSeconds := filterValidRequestCountPolicies(tokenPolicies)
	if len(validTokenPolicies) == 0 || maxWindowSeconds <= 0 {
		userPolicies, userErr := deps.loadPolicies(pool.Id, model.PoolQuotaScopeUser)
		if userErr != nil {
			return nil, userErr
		}
		validUserPolicies, _ := filterValidRequestCountPolicies(userPolicies)
		item.ScopeType = model.PoolQuotaScopeToken
		if len(validUserPolicies) > 0 {
			item.ScopeType = model.PoolQuotaScopeUser
			return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonUserScopeOnly), nil
		}
		return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonTokenScopeDisabled), nil
	}

	item.ScopeType = model.PoolQuotaScopeToken
	item.TokenScopeEnabled = true
	item.RetentionWindowSeconds = maxWindowSeconds
	if !deps.isRedisReady() {
		return buildUnavailableTokenPoolUsage(item, windows, windowSeconds, tokenPoolUsageReasonRedisRequired), nil
	}

	redisKey := fmt.Sprintf("pool:rq:events:%d:token:%d", pool.Id, token.Id)
	for _, window := range windows {
		result := &TokenPoolUsageWindow{
			Window:        window,
			WindowSeconds: windowSeconds[window],
		}
		if result.WindowSeconds > maxWindowSeconds {
			result.Available = false
			result.Reason = tokenPoolUsageReasonWindowNotRetained
			item.Usage[window] = result
			continue
		}
		count, countErr := deps.countByWindow(redisKey, result.WindowSeconds)
		if countErr != nil {
			return nil, countErr
		}
		result.Available = true
		result.Count = &count
		item.Usage[window] = result
	}
	return item, nil
}

func GetTokenPoolUsageBatch(c *gin.Context) {
	userId := c.GetInt("id")
	if userId <= 0 {
		common.ApiErrorMsg(c, "invalid user")
		return
	}

	req := TokenPoolUsageBatchRequest{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	windows, windowSeconds, err := normalizeTokenPoolUsageWindows(req.Windows)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	uniqueIds := make([]int, 0, len(req.Ids))
	seen := make(map[int]struct{}, len(req.Ids))
	for _, id := range req.Ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIds = append(uniqueIds, id)
	}
	if len(uniqueIds) == 0 {
		common.ApiSuccess(c, gin.H{"items": []*TokenPoolUsageItem{}})
		return
	}

	tokens, err := model.GetUserTokensByIds(userId, uniqueIds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tokenById := make(map[int]*model.Token, len(tokens))
	for _, token := range tokens {
		if token == nil {
			continue
		}
		tokenById[token.Id] = token
	}

	items := make([]*TokenPoolUsageItem, 0, len(uniqueIds))
	for _, id := range uniqueIds {
		token := tokenById[id]
		if token == nil {
			continue
		}
		item, buildErr := buildTokenPoolUsageItem(token, windows, windowSeconds)
		if buildErr != nil {
			common.ApiError(c, buildErr)
			return
		}
		items = append(items, item)
	}

	common.ApiSuccess(c, gin.H{
		"items":       items,
		"windows":     windows,
		"data_source": tokenPoolUsageDataSource,
	})
}

func GetTokenPoolUsageSelf(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId <= 0 {
		common.ApiErrorMsg(c, "invalid token")
		return
	}

	windows, windowSeconds, err := normalizeTokenPoolUsageWindows(nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	token, err := model.GetTokenById(tokenId)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	item, err := buildTokenPoolUsageItemFunc(token, windows, windowSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	llmUsage, err := buildTokenPoolLLMTokenUsage(tokenId, windows, windowSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"item":            item,
		"windows":         windows,
		"data_source":     tokenPoolUsageDataSource,
		"llm_token_usage": llmUsage,
	})
}
