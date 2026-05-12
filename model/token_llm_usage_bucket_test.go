package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpsertTokenLLMUsageBucket_AndIncrementLifetime(t *testing.T) {
	truncateTables(t)
	DB.Exec("DELETE FROM token_llm_usage_buckets")

	key := "0123456789012345678901234567890123456789012345678" // 48 chars
	tok := &Token{
		UserId:         1,
		Name:           "rollup-t",
		Key:            key,
		Status:         1,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    1,
		UnlimitedQuota: true,
		Group:          "default",
	}
	require.NoError(t, DB.Create(tok).Error)

	bucketStart := int64(1_700_000_000)
	require.NoError(t, UpsertTokenLLMUsageBucket(tok.Id, bucketStart, 10, 20, 1))
	require.NoError(t, UpsertTokenLLMUsageBucket(tok.Id, bucketStart, 1, 2, 1))

	prompt, completion, err := SumTokenLLMUsageBucketsByTokenSince(tok.Id, 0)
	require.NoError(t, err)
	require.EqualValues(t, 11, prompt)
	require.EqualValues(t, 22, completion)

	since := bucketStart + 1
	p2, c2, err := SumTokenLLMUsageBucketsByTokenSince(tok.Id, since)
	require.NoError(t, err)
	require.EqualValues(t, 0, p2)
	require.EqualValues(t, 0, c2)

	require.NoError(t, IncrementTokenLLMTokenTotals(tok.Id, 5, 7))
	var loaded Token
	require.NoError(t, DB.First(&loaded, tok.Id).Error)
	require.EqualValues(t, 5, loaded.LlmPromptTokensTotal)
	require.EqualValues(t, 7, loaded.LlmCompletionTokensTotal)
}
