package model

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func configureInvitationRewardsForTest(t *testing.T, rate float64) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalEnabled := paymentSetting.InviteRechargeRewardEnabled
	originalRate := paymentSetting.InviteRechargeRewardRate
	originalCompliance := paymentSetting.ComplianceConfirmed
	originalVersion := paymentSetting.ComplianceTermsVersion
	paymentSetting.InviteRechargeRewardEnabled = true
	paymentSetting.InviteRechargeRewardRate = rate
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.InviteRechargeRewardEnabled = originalEnabled
		paymentSetting.InviteRechargeRewardRate = originalRate
		paymentSetting.ComplianceConfirmed = originalCompliance
		paymentSetting.ComplianceTermsVersion = originalVersion
	})
}

func TestInvitationRewardWalletLimitDoesNotRollBackRecharge(t *testing.T) {
	for _, fullWallet := range []string{"aff_quota", "aff_history"} {
		t.Run(fullWallet, func(t *testing.T) {
			truncateTables(t)
			configureInvitationRewardsForTest(t, 100)
			previousQuotaPerUnit := common.QuotaPerUnit
			common.QuotaPerUnit = 1
			t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
			inviter := User{Username: "full-inviter", AffCode: "full-inviter", Status: common.UserStatusEnabled}
			require.NoError(t, DB.Create(&inviter).Error)
			require.NoError(t, DB.Model(&inviter).Update(fullWallet, common.MaxWalletQuota).Error)
			invitee := User{Username: "full-invitee", AffCode: "full-invitee", InviterId: inviter.Id}
			require.NoError(t, DB.Create(&invitee).Error)
			topup := TopUp{UserId: invitee.Id, TradeNo: "full-wallet", Amount: 100,
				PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
			require.NoError(t, DB.Create(&topup).Error)
			alreadyDone, err := RechargeEpay(topup.TradeNo, "", "")
			require.NoError(t, err)
			assert.False(t, alreadyDone)
			alreadyDone, err = RechargeEpay(topup.TradeNo, "", "")
			require.NoError(t, err)
			assert.True(t, alreadyDone)
			require.NoError(t, DB.First(&invitee, invitee.Id).Error)
			assert.Equal(t, 100, invitee.Quota)
			var records []InvitationRewardRecord
			require.NoError(t, DB.Find(&records).Error)
			require.Len(t, records, 1)
			assert.Equal(t, InvitationRewardStatusFailed, records[0].Status)
			assert.Equal(t, 100, records[0].RewardQuota)
			assert.Zero(t, records[0].SettledAt)
			assert.NotEmpty(t, records[0].Remark)
			require.NoError(t, DB.First(&inviter, inviter.Id).Error)
			if fullWallet == "aff_quota" {
				assert.Equal(t, common.MaxWalletQuota, inviter.AffQuota)
				assert.Zero(t, inviter.AffHistoryQuota)
			} else {
				assert.Equal(t, common.MaxWalletQuota, inviter.AffHistoryQuota)
				assert.Zero(t, inviter.AffQuota)
			}
		})
	}
}

func TestInvitationRewardRejectsNonFiniteRate(t *testing.T) {
	for _, test := range []struct {
		name string
		rate float64
	}{{"nan", math.NaN()}, {"positive_infinity", math.Inf(1)}, {"negative_infinity", math.Inf(-1)}} {
		t.Run(test.name, func(t *testing.T) {
			truncateTables(t)
			configureInvitationRewardsForTest(t, test.rate)
			require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
				return settleRechargeInvitationRewardTx(tx, &TopUp{UserId: 1, TradeNo: "invalid-rate"}, 100)
			}))
			var count int64
			require.NoError(t, DB.Model(&InvitationRewardRecord{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func TestInvitationRewardTransferRespectsWalletLimit(t *testing.T) {
	truncateTables(t)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	user := User{Username: "transfer-limit", AffCode: "transfer-limit", Quota: common.MaxWalletQuota - 10, AffQuota: 20, AffHistoryQuota: 20}
	require.NoError(t, DB.Create(&user).Error)
	require.ErrorIs(t, user.TransferAffQuotaToQuota(11), ErrWalletQuotaLimitExceeded)
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, common.MaxWalletQuota-10, user.Quota)
	assert.Equal(t, 20, user.AffQuota)
	require.NoError(t, user.TransferAffQuotaToQuota(10))
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, common.MaxWalletQuota, user.Quota)
	assert.Equal(t, 10, user.AffQuota)
	assert.Equal(t, 20, user.AffHistoryQuota)
}

func TestInvitationRewardSQLiteMigrationIsIdempotent(t *testing.T) {
	require.NoError(t, DB.Migrator().DropTable(&InvitationRewardRecord{}))
	require.NoError(t, migrateInvitationRewardRecord(DB))
	require.NoError(t, migrateInvitationRewardRecord(DB))
}

func TestRechargeWaffoPancakeGrantsInvitationCommissionOnce(t *testing.T) {
	truncateTables(t)
	configureInvitationRewardsForTest(t, 10)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	inviter := User{Id: 201, Username: "inviter", AffCode: "invite-201", Status: common.UserStatusEnabled}
	invitee := User{Id: 202, Username: "invitee", AffCode: "invite-202", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	require.NoError(t, DB.Create(&TopUp{
		UserId:          invitee.Id,
		Amount:          10,
		TradeNo:         "invitation-reward-topup",
		PaymentMethod:   PaymentMethodWaffoPancake,
		PaymentProvider: PaymentProviderWaffoPancake,
		Status:          common.TopUpStatusPending,
	}).Error)

	require.NoError(t, RechargeWaffoPancake("invitation-reward-topup"))
	require.NoError(t, RechargeWaffoPancake("invitation-reward-topup"))

	require.NoError(t, DB.First(&inviter, inviter.Id).Error)
	require.NoError(t, DB.First(&invitee, invitee.Id).Error)
	assert.Equal(t, 1000, invitee.Quota)
	assert.Equal(t, 100, inviter.AffQuota)
	assert.Equal(t, 100, inviter.AffHistoryQuota)

	var records []InvitationRewardRecord
	require.NoError(t, DB.Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, InvitationRewardTypeRecharge, records[0].Type)
	assert.Equal(t, 1000, records[0].BaseQuota)
	assert.Equal(t, 10.0, records[0].Rate)
	assert.Equal(t, 100, records[0].RewardQuota)
}

func TestSignupInvitationRewardIsIdempotent(t *testing.T) {
	truncateTables(t)
	originalReward := common.QuotaForInviter
	common.QuotaForInviter = 75
	t.Cleanup(func() { common.QuotaForInviter = originalReward })

	inviter := User{Id: 301, Username: "signup-inviter", AffCode: "invite-301", Status: common.UserStatusEnabled}
	invitee := User{Id: 302, Username: "signup-invitee", AffCode: "invite-302", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)

	require.NoError(t, recordSignupInvitation(inviter.Id, invitee.Id, common.QuotaForInviter))
	require.NoError(t, recordSignupInvitation(inviter.Id, invitee.Id, common.QuotaForInviter))
	require.NoError(t, DB.First(&inviter, inviter.Id).Error)
	assert.Equal(t, 1, inviter.AffCount)
	assert.Equal(t, 75, inviter.AffQuota)
	assert.Equal(t, 75, inviter.AffHistoryQuota)

	var count int64
	require.NoError(t, DB.Model(&InvitationRewardRecord{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestSignupInvitationIsCountedWhenFixedRewardIsZero(t *testing.T) {
	truncateTables(t)
	inviter := User{Id: 311, Username: "zero-reward-inviter", AffCode: "invite-311", Status: common.UserStatusEnabled}
	invitee := User{Id: 312, Username: "zero-reward-invitee", AffCode: "invite-312", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)

	require.NoError(t, recordSignupInvitation(inviter.Id, invitee.Id, 0))
	require.NoError(t, recordSignupInvitation(inviter.Id, invitee.Id, 0))
	require.NoError(t, DB.First(&inviter, inviter.Id).Error)
	assert.Equal(t, 1, inviter.AffCount)
	assert.Equal(t, 0, inviter.AffQuota)
	assert.Equal(t, 0, inviter.AffHistoryQuota)
}

func TestInvitationRewardsSkipDisabledInviter(t *testing.T) {
	truncateTables(t)
	configureInvitationRewardsForTest(t, 10)
	originalReward := common.QuotaForInviter
	common.QuotaForInviter = 75
	t.Cleanup(func() { common.QuotaForInviter = originalReward })

	inviter := User{Id: 321, Username: "disabled-inviter", AffCode: "invite-321", Status: common.UserStatusDisabled}
	invitee := User{Id: 322, Username: "disabled-invitee", AffCode: "invite-322", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)

	require.NoError(t, recordSignupInvitation(inviter.Id, invitee.Id, common.QuotaForInviter))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return settleRechargeInvitationRewardTx(tx, &TopUp{UserId: invitee.Id, TradeNo: "disabled-inviter-topup"}, 1000)
	}))

	require.NoError(t, DB.First(&inviter, inviter.Id).Error)
	assert.Equal(t, 0, inviter.AffCount)
	assert.Equal(t, 0, inviter.AffQuota)
	assert.Equal(t, 0, inviter.AffHistoryQuota)

	var count int64
	require.NoError(t, DB.Model(&InvitationRewardRecord{}).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestRechargeInvitationRewardIsIdempotentWithoutTopUpGuard(t *testing.T) {
	truncateTables(t)
	configureInvitationRewardsForTest(t, 10)

	inviter := User{Id: 331, Username: "direct-inviter", AffCode: "invite-331", Status: common.UserStatusEnabled}
	invitee := User{Id: 332, Username: "direct-invitee", AffCode: "invite-332", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	topUp := &TopUp{UserId: invitee.Id, TradeNo: "direct-idempotent-topup"}

	for i := 0; i < 2; i++ {
		require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
			return settleRechargeInvitationRewardTx(tx, topUp, 1000)
		}))
	}

	require.NoError(t, DB.First(&inviter, inviter.Id).Error)
	assert.Equal(t, 100, inviter.AffQuota)
	assert.Equal(t, 100, inviter.AffHistoryQuota)

	var count int64
	require.NoError(t, DB.Model(&InvitationRewardRecord{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestUserInvitationViewsRedactSensitiveDetails(t *testing.T) {
	truncateTables(t)
	configureInvitationRewardsForTest(t, 10)

	inviter := User{Id: 341, Username: "privacy-inviter", AffCode: "invite-341", Status: common.UserStatusEnabled}
	invitee := User{Id: 342, Username: "private-user", AffCode: "invite-342", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, DB.Create(&inviter).Error)
	require.NoError(t, DB.Create(&invitee).Error)
	require.NoError(t, recordSignupInvitation(inviter.Id, invitee.Id, 75))
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return settleRechargeInvitationRewardTx(tx, &TopUp{UserId: invitee.Id, TradeNo: "privacy-topup"}, 1000)
	}))

	pageInfo := &common.PageInfo{Page: 1, PageSize: 20}
	invitedUsers, invitedTotal, err := GetUserInvitedUsers(inviter.Id, pageInfo)
	require.NoError(t, err)
	require.Equal(t, int64(1), invitedTotal)
	require.Len(t, invitedUsers, 1)
	assert.Equal(t, "p***r", invitedUsers[0].Invitee)
	assert.NotEmpty(t, invitedUsers[0].RegisteredDate)

	rewardRecords, rewardTotal, err := GetUserInvitationRewardRecords(inviter.Id, pageInfo)
	require.NoError(t, err)
	require.Equal(t, int64(2), rewardTotal)
	require.Len(t, rewardRecords, 2)
	for _, record := range rewardRecords {
		assert.Equal(t, "p***r", record.Invitee)
		assert.NotEmpty(t, record.RewardDate)
	}
}

func TestMaskInvitationUsernameHidesShortUnicodeNames(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{name: "", expected: "-"},
		{name: "a", expected: "***"},
		{name: "ab", expected: "***"},
		{name: "甲", expected: "***"},
		{name: "甲乙", expected: "***"},
		{name: "abc", expected: "a***c"},
		{name: "用户名称", expected: "用***称"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, maskInvitationUsername(test.name), test.name)
	}
}

func TestClearInvitationRewardQuotaPreservesBalanceAndHistory(t *testing.T) {
	truncateTables(t)
	user := User{
		Id:              401,
		Username:        "reward-payout-user",
		AffCode:         "invite-401",
		Status:          common.UserStatusEnabled,
		Quota:           1000,
		AffQuota:        250,
		AffHistoryQuota: 500,
	}
	require.NoError(t, DB.Create(&user).Error)

	cleared, err := ClearInvitationRewardQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 250, cleared)

	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 0, user.AffQuota)
	assert.Equal(t, 500, user.AffHistoryQuota)
	assert.Equal(t, 1000, user.Quota)
}
