package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
