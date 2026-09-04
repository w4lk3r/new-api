package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// This is the released private schema, before wallet-sized ledger columns.
type legacyInvitationRewardRecord struct {
	Id          int
	InviterId   int    `gorm:"index"`
	InviteeId   int    `gorm:"index"`
	Type        string `gorm:"type:varchar(32);uniqueIndex:idx_invitation_reward_source"`
	SourceId    string `gorm:"type:varchar(255);uniqueIndex:idx_invitation_reward_source"`
	BaseQuota   int
	Rate        float64 `gorm:"type:decimal(10,6)"`
	RewardQuota int
	Status      string `gorm:"type:varchar(32);index"`
	CreatedAt   int64  `gorm:"autoCreateTime"`
	SettledAt   int64
	Remark      string `gorm:"type:varchar(255)"`
}

func (legacyInvitationRewardRecord) TableName() string { return "invitation_reward_records" }

func TestInvitationRewardDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var dialector gorm.Dialector
			switch dialect {
			case "sqlite":
				dialector = sqlite.Open(":memory:")
			case "mysql":
				dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured; MySQL validation was not run")
				}
				dialector = mysql.Open(dsn)
			case "postgres":
				dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured; PostgreSQL validation was not run")
				}
				dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			if dialect != "sqlite" {
				var databaseName, version string
				query := "SELECT DATABASE()"
				if dialect == "postgres" {
					query = "SELECT current_database()"
				}
				require.NoError(t, db.Raw(query).Scan(&databaseName).Error)
				require.Contains(t, strings.ToLower(databaseName), "test", "DSN must point to a dedicated test database")
				require.NoError(t, db.Raw("SELECT version()").Scan(&version).Error)
				t.Logf("database=%s version=%s", databaseName, version)
			} else {
				var version string
				require.NoError(t, db.Raw("SELECT sqlite_version()").Scan(&version).Error)
				t.Logf("SQLite version=%s", version)
			}
			for _, scenario := range []string{"fresh", "private_schema_upgrade"} {
				t.Run(scenario, func(t *testing.T) {
					// Refuse an occupied database, even one named "test". Cleanup
					// only removes the exact tables created by this test.
					tables := []any{&InvitationRewardRecord{}, &TopUp{}, &Log{}, &User{}}
					for _, table := range tables {
						require.False(t, db.Migrator().HasTable(table), "test table already exists; refusing to modify it")
					}
					t.Cleanup(func() {
						for _, table := range tables {
							require.NoError(t, db.Migrator().DropTable(table))
						}
					})
					previousDB, previousLogDB := DB, LOG_DB
					previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
					previousRedis := common.RedisEnabled
					DB, LOG_DB = db, db
					common.SetDatabaseTypes(common.DatabaseType(dialect), common.DatabaseType(dialect))
					common.RedisEnabled = false
					initCol()
					t.Cleanup(func() {
						DB, LOG_DB = previousDB, previousLogDB
						common.SetDatabaseTypes(previousMainType, previousLogType)
						common.RedisEnabled = previousRedis
						initCol()
					})
					require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}))
					preservedUser := User{Username: "legacy-inviter", AffCode: "legacy-code", Quota: 800, AffQuota: 250, AffHistoryQuota: 500}
					require.NoError(t, db.Create(&preservedUser).Error)
					if scenario == "private_schema_upgrade" {
						require.NoError(t, db.AutoMigrate(&legacyInvitationRewardRecord{}))
						require.NoError(t, db.Create(&legacyInvitationRewardRecord{
							InviterId: preservedUser.Id, InviteeId: 9, Type: InvitationRewardTypeRecharge,
							SourceId: "preserved-trade", BaseQuota: 2500, RewardQuota: 250, Rate: 10,
							Status: InvitationRewardStatusSettled, CreatedAt: 100, SettledAt: 101, Remark: "preserve audit",
						}).Error)
					}
					for range 2 {
						require.NoError(t, migrateInvitationRewardRecord(db))
					}
					if scenario == "private_schema_upgrade" {
						var preserved InvitationRewardRecord
						require.NoError(t, db.First(&preserved, "source_id = ?", "preserved-trade").Error)
						assert.Equal(t, 2500, preserved.BaseQuota)
						assert.Equal(t, 250, preserved.RewardQuota)
						assert.Equal(t, 10.0, preserved.Rate)
						assert.Equal(t, int64(100), preserved.CreatedAt)
						assert.Equal(t, int64(101), preserved.SettledAt)
						assert.Equal(t, "preserve audit", preserved.Remark)
						preserved.Id = 0
						require.Error(t, db.Create(&preserved).Error, "the source uniqueness constraint must survive migration")
					}
					var user User
					require.NoError(t, db.First(&user, preservedUser.Id).Error)
					assert.Equal(t, 800, user.Quota)
					assert.Equal(t, 250, user.AffQuota)
					assert.Equal(t, 500, user.AffHistoryQuota)
					if dialect != "sqlite" {
						columns, err := db.Migrator().ColumnTypes(&InvitationRewardRecord{})
						require.NoError(t, err)
						for _, column := range columns {
							if column.Name() == "base_quota" || column.Name() == "reward_quota" {
								assert.True(t, is64BitIntegerType(common.DatabaseType(dialect), column.DatabaseTypeName()), column.Name())
							}
						}
					}
					testInvitationPaymentPaths(t)
				})
			}
		})
	}
}

func testInvitationPaymentPaths(t *testing.T) {
	configureInvitationRewardsForTest(t, 100)
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })
	for _, provider := range []string{PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake, "manual"} {
		t.Run(provider, func(t *testing.T) {
			inviter := User{Username: "i-" + provider, AffCode: "i-" + provider, Status: common.UserStatusEnabled}
			require.NoError(t, DB.Create(&inviter).Error)
			invitee := User{Username: "e-" + provider, AffCode: "e-" + provider, Status: common.UserStatusEnabled, InviterId: inviter.Id}
			require.NoError(t, DB.Create(&invitee).Error)
			credited := common.MaxQuota + 100
			topup := TopUp{UserId: invitee.Id, Amount: int64(credited), Money: float64(credited), TradeNo: "matrix-" + provider,
				PaymentProvider: provider, PaymentMethod: provider, Status: common.TopUpStatusPending}
			require.NoError(t, DB.Create(&topup).Error)
			var settle func() error
			switch provider {
			case PaymentProviderEpay:
				settle = func() error { _, err := RechargeEpay(topup.TradeNo, provider, ""); return err }
			case PaymentProviderStripe:
				settle = func() error { return Recharge(topup.TradeNo, "customer", "") }
			case PaymentProviderCreem:
				settle = func() error { return RechargeCreem(topup.TradeNo, "", "", "") }
			case PaymentProviderWaffo:
				settle = func() error { return RechargeWaffo(topup.TradeNo, "") }
			case PaymentProviderWaffoPancake:
				settle = func() error { return RechargeWaffoPancake(topup.TradeNo) }
			case "manual":
				settle = func() error { return ManualCompleteTopUp(topup.TradeNo, "") }
			}
			require.NoError(t, settle())
			// Stripe and Creem retain their existing duplicate-callback error
			// contract. Regardless of response, money and commission stay once.
			_ = settle()
			require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
				return settleRechargeInvitationRewardTx(tx, &topup, credited)
			}), "ledger uniqueness must also protect retries without the top-up status guard")
			require.NoError(t, DB.First(&invitee, invitee.Id).Error)
			require.NoError(t, DB.First(&inviter, inviter.Id).Error)
			assert.Equal(t, credited, invitee.Quota)
			assert.Equal(t, credited, inviter.AffQuota)
			assert.Equal(t, credited, inviter.AffHistoryQuota)
			var records []InvitationRewardRecord
			require.NoError(t, DB.Where("source_id = ?", topup.TradeNo).Find(&records).Error)
			require.Len(t, records, 1)
			assert.Equal(t, credited, records[0].BaseQuota)
			assert.Equal(t, credited, records[0].RewardQuota)
			assert.Equal(t, InvitationRewardStatusSettled, records[0].Status)
		})
	}
}
