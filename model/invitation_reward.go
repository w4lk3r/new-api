package model

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	InvitationRewardTypeSignup   = "signup"
	InvitationRewardTypeRecharge = "recharge"

	InvitationRewardStatusSettled = "settled"
	InvitationRewardStatusFailed  = "failed"
)

type InvitationRewardRecord struct {
	Id              int     `json:"id"`
	InviterId       int     `json:"inviter_id" gorm:"index"`
	InviteeId       int     `json:"invitee_id" gorm:"index"`
	Type            string  `json:"type" gorm:"type:varchar(32);uniqueIndex:idx_invitation_reward_source"`
	SourceId        string  `json:"source_id" gorm:"type:varchar(255);uniqueIndex:idx_invitation_reward_source"`
	BaseQuota       int     `json:"base_quota"`
	Rate            float64 `json:"rate" gorm:"type:decimal(10,6)"`
	RewardQuota     int     `json:"reward_quota"`
	Status          string  `json:"status" gorm:"type:varchar(32);index"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime"`
	SettledAt       int64   `json:"settled_at"`
	Remark          string  `json:"remark" gorm:"type:varchar(255)"`
	InviteeUsername string  `json:"invitee_username" gorm:"column:invitee_username;->;-:migration"`
}

type InvitedUser struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Status      int    `json:"status"`
	CreatedAt   int64  `json:"created_at"`
}

func recordSignupInvitation(inviterId int, inviteeId int, rewardQuota int) error {
	if inviterId == 0 || inviteeId == 0 || inviterId == inviteeId || rewardQuota < 0 {
		return nil
	}

	sourceId := strconv.Itoa(inviteeId)
	return DB.Transaction(func(tx *gorm.DB) error {
		var inviter User
		if err := lockForUpdate(tx).First(&inviter, inviterId).Error; err != nil {
			return err
		}
		if inviter.Status != common.UserStatusEnabled {
			return nil
		}

		now := common.GetTimestamp()
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&InvitationRewardRecord{
			InviterId:   inviterId,
			InviteeId:   inviteeId,
			Type:        InvitationRewardTypeSignup,
			SourceId:    sourceId,
			RewardQuota: rewardQuota,
			Status:      InvitationRewardStatusSettled,
			SettledAt:   now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if inviter.AffQuota > common.MaxQuota-rewardQuota ||
			inviter.AffHistoryQuota > common.MaxQuota-rewardQuota {
			return errors.New("invitation reward quota exceeds storage limit")
		}

		inviter.AffCount++
		inviter.AffQuota += rewardQuota
		inviter.AffHistoryQuota += rewardQuota
		if err := tx.Save(&inviter).Error; err != nil {
			return err
		}
		return nil
	})
}

func settleRechargeInvitationRewardTx(tx *gorm.DB, topUp *TopUp, creditedQuota int) error {
	paymentSetting := operation_setting.GetPaymentSetting()
	rate := paymentSetting.InviteRechargeRewardRate
	if !paymentSetting.InviteRechargeRewardEnabled ||
		!operation_setting.IsPaymentComplianceConfirmed() ||
		rate <= 0 || rate > 100 || creditedQuota <= 0 {
		return nil
	}

	var invitee User
	if err := tx.Select("id", "inviter_id").First(&invitee, topUp.UserId).Error; err != nil {
		return err
	}
	if invitee.InviterId == 0 || invitee.InviterId == invitee.Id {
		return nil
	}

	var inviter User
	if err := lockForUpdate(tx).First(&inviter, invitee.InviterId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if inviter.Status != common.UserStatusEnabled {
		return nil
	}

	rewardDecimal := decimal.NewFromInt(int64(creditedQuota)).
		Mul(decimal.NewFromFloat(rate)).
		Div(decimal.NewFromInt(100))
	rewardQuota, clamp := common.QuotaFromDecimalChecked(rewardDecimal)
	if clamp != nil {
		return fmt.Errorf("recharge invitation reward exceeds storage limit: %w", clamp)
	}
	if rewardQuota <= 0 {
		return nil
	}

	now := common.GetTimestamp()
	record := InvitationRewardRecord{
		InviterId:   inviter.Id,
		InviteeId:   invitee.Id,
		Type:        InvitationRewardTypeRecharge,
		SourceId:    topUp.TradeNo,
		BaseQuota:   creditedQuota,
		Rate:        rate,
		RewardQuota: rewardQuota,
		Status:      InvitationRewardStatusSettled,
		SettledAt:   now,
	}
	if inviter.AffQuota > common.MaxQuota-rewardQuota ||
		inviter.AffHistoryQuota > common.MaxQuota-rewardQuota {
		record.Status = InvitationRewardStatusFailed
		record.SettledAt = 0
		record.Remark = "invitation reward quota exceeds storage limit"
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	if record.Status == InvitationRewardStatusFailed {
		return nil
	}

	if err := tx.Model(&inviter).Updates(map[string]interface{}{
		"aff_quota":   gorm.Expr("aff_quota + ?", rewardQuota),
		"aff_history": gorm.Expr("aff_history + ?", rewardQuota),
	}).Error; err != nil {
		return err
	}
	return nil
}

func GetInvitedUsers(inviterId int, pageInfo *common.PageInfo) ([]*InvitedUser, int64, error) {
	query := DB.Model(&User{}).Where("inviter_id = ?", inviterId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []*InvitedUser
	if err := query.Select("id", "username", "display_name", "status", "created_at").
		Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func GetInvitationRewardRecords(inviterId int, pageInfo *common.PageInfo) ([]*InvitationRewardRecord, int64, error) {
	query := DB.Model(&InvitationRewardRecord{}).Where("invitation_reward_records.inviter_id = ?", inviterId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []*InvitationRewardRecord
	if err := query.
		Select("invitation_reward_records.*, users.username AS invitee_username").
		Joins("LEFT JOIN users ON users.id = invitation_reward_records.invitee_id").
		Order("invitation_reward_records.id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Scan(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func ClearInvitationRewardQuota(userId int) (int, error) {
	clearedQuota := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, userId).Error; err != nil {
			return err
		}
		clearedQuota = user.AffQuota
		if clearedQuota == 0 {
			return nil
		}
		return tx.Model(&user).Update("aff_quota", 0).Error
	})
	return clearedQuota, err
}
