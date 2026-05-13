package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTokenPoolSubscriptionCheckoutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Pool{}, &model.PoolBinding{}))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestRequestTokenPoolSubscriptionWechatCheckout_PoolIdMustMatchResolved(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	token := seedToken(t, db, 42, "co-tok", "checkout-key-12345678")
	resolved := &model.Pool{Name: "resolved-p", Status: model.PoolStatusEnabled, MonthlyPriceCny: 40}
	require.NoError(t, db.Create(resolved).Error)
	other := &model.Pool{Name: "other-p", Status: model.PoolStatusEnabled, MonthlyPriceCny: 40}
	require.NoError(t, db.Create(other).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       resolved.Id,
		Enabled:      true,
	}).Error)

	body, _ := json.Marshal(map[string]int{"pool_id": other.Id})
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/usage/token/pool/subscription/wechat/checkout", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	RequestTokenPoolSubscriptionWechatCheckout(ctx)
	require.Equal(t, 200, rec.Code)
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "resolved pool")
}

func TestRequestTokenPoolSubscriptionWechatCheckout_WeChatNotConfigured(t *testing.T) {
	db := setupTokenPoolSubscriptionCheckoutTestDB(t)
	token := seedToken(t, db, 43, "co-tok2", "checkout-key-abcdefgh")
	resolved := &model.Pool{Name: "priced-p", Status: model.PoolStatusEnabled, MonthlyPriceCny: 12.34}
	require.NoError(t, db.Create(resolved).Error)
	require.NoError(t, db.Create(&model.PoolBinding{
		BindingType:  model.PoolBindingTypeToken,
		BindingValue: strconv.Itoa(token.Id),
		PoolId:       resolved.Id,
		Enabled:      true,
	}).Error)

	body, _ := json.Marshal(map[string]int{"pool_id": resolved.Id})
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/usage/token/pool/subscription/wechat/checkout", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("token_id", token.Id)
	ctx.Set("id", token.UserId)

	RequestTokenPoolSubscriptionWechatCheckout(ctx)
	require.Equal(t, 200, rec.Code)
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "wechat pay is not configured")
}
