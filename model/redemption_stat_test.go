package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 兑换码充值不经过支付渠道，不会写入 top_ups，因此使用日志的充值统计需要单独从
// redemptions 表统计，时间口径是 redeemed_time（与日志列表的 created_at 对齐）。

func redemptionStatKey(n int) string {
	return fmt.Sprintf("testkey_%024d", n)
}

func insertRedemptionStatUser(t *testing.T, id int, username string, inviterId int) *User {
	t.Helper()
	user := &User{
		Id:        id,
		Username:  username,
		AffCode:   fmt.Sprintf("AFF%05d", id),
		Status:    common.UserStatusEnabled,
		InviterId: inviterId,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func insertRedemptionStatRecord(t *testing.T, key string, creatorId int, usedUserId int, quota int, status int, redeemedTime int64) {
	t.Helper()
	redemption := &Redemption{
		UserId:       creatorId,
		Key:          key,
		Status:       status,
		Name:         "stat test",
		Quota:        quota,
		CreatedTime:  redeemedTime,
		RedeemedTime: redeemedTime,
		UsedUserId:   usedUserId,
	}
	require.NoError(t, DB.Create(redemption).Error)
}

func TestSumRedeemedQuota_OnlyUsedCodesInTimeRange(t *testing.T) {
	truncateTables(t)

	payer := insertRedemptionStatUser(t, 1, "redeem_stat_payer", 0)
	// 区间内已兑换的两笔
	insertRedemptionStatRecord(t, redemptionStatKey(1), payer.Id, payer.Id, 100, common.RedemptionCodeStatusUsed, 1000)
	insertRedemptionStatRecord(t, redemptionStatKey(2), payer.Id, payer.Id, 250, common.RedemptionCodeStatusUsed, 1500)
	// 区间外已兑换
	insertRedemptionStatRecord(t, redemptionStatKey(3), payer.Id, payer.Id, 999, common.RedemptionCodeStatusUsed, 500)
	// 区间内但未兑换（无到账额度，不应计入）
	insertRedemptionStatRecord(t, redemptionStatKey(4), payer.Id, 0, 777, common.RedemptionCodeStatusEnabled, 0)

	quota, err := SumRedeemedQuota(1000, 2000, "", 0)
	require.NoError(t, err)
	assert.EqualValues(t, 350, quota)

	// 不做时间过滤时统计所有已兑换的额度
	quota, err = SumRedeemedQuota(0, 0, "", 0)
	require.NoError(t, err)
	assert.EqualValues(t, 1349, quota)
}

func TestSumRedeemedQuota_FilterByRedeemerAndInviter(t *testing.T) {
	truncateTables(t)

	inviter := insertRedemptionStatUser(t, 1, "redeem_stat_inviter", 0)
	payer := insertRedemptionStatUser(t, 2, "redeem_stat_payer", inviter.Id)
	other := insertRedemptionStatUser(t, 3, "redeem_stat_other", 0)

	insertRedemptionStatRecord(t, redemptionStatKey(1), payer.Id, payer.Id, 100, common.RedemptionCodeStatusUsed, 1000)
	insertRedemptionStatRecord(t, redemptionStatKey(2), other.Id, other.Id, 700, common.RedemptionCodeStatusUsed, 1000)

	// 按充值到账的用户名过滤（used_user_id 对应的用户，与日志列表口径一致）
	quota, err := SumRedeemedQuota(1000, 2000, "redeem_stat_payer", 0)
	require.NoError(t, err)
	assert.EqualValues(t, 100, quota)

	// 按邀请人过滤
	quota, err = SumRedeemedQuota(1000, 2000, "", inviter.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 100, quota)

	// 无过滤
	quota, err = SumRedeemedQuota(1000, 2000, "", 0)
	require.NoError(t, err)
	assert.EqualValues(t, 800, quota)
}

func TestSumUserRedeemedQuota(t *testing.T) {
	truncateTables(t)

	first := insertRedemptionStatUser(t, 1, "redeem_stat_first", 0)
	second := insertRedemptionStatUser(t, 2, "redeem_stat_second", 0)

	insertRedemptionStatRecord(t, redemptionStatKey(1), first.Id, first.Id, 120, common.RedemptionCodeStatusUsed, 1000)
	insertRedemptionStatRecord(t, redemptionStatKey(2), first.Id, first.Id, 80, common.RedemptionCodeStatusUsed, 3000) // 区间外
	insertRedemptionStatRecord(t, redemptionStatKey(3), second.Id, second.Id, 500, common.RedemptionCodeStatusUsed, 1000)

	quota, err := SumUserRedeemedQuota(first.Id, 500, 2000)
	require.NoError(t, err)
	assert.EqualValues(t, 120, quota)

	quota, err = SumUserRedeemedQuota(second.Id, 500, 2000)
	require.NoError(t, err)
	assert.EqualValues(t, 500, quota)
}
