package model

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
)

type Redemption struct {
	Id           int            `json:"id"`
	UserId       int            `json:"user_id"`
	Key          string         `json:"key" gorm:"type:char(32);uniqueIndex"`
	Status       int            `json:"status" gorm:"default:1"`
	Name         string         `json:"name" gorm:"index"`
	Quota        int            `json:"quota" gorm:"default:100"`
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`
	RedeemedTime int64          `json:"redeemed_time" gorm:"bigint"`
	Count        int            `json:"count" gorm:"-:all"` // only for api request
	UsedUserId   int            `json:"used_user_id"`
	CreatorName  string         `json:"creator_name" gorm:"->"`
	UsedUserName string         `json:"used_user_name" gorm:"->"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
	ExpiredTime  int64          `json:"expired_time" gorm:"bigint"` // 过期时间，0 表示不过期
}

func GetRedemptionAudit(userId int, isAdmin bool, creatorId int, creatorName string, usedUserId int, usedUserName string, startTimestamp int64, endTimestamp int64, status int, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	query := DB.Model(&Redemption{}).
		Select("redemptions.id, redemptions.user_id, redemptions.status, redemptions.name, redemptions.quota, redemptions.created_time, redemptions.redeemed_time, redemptions.used_user_id, redemptions.expired_time, creator.username AS creator_name, used_user.username AS used_user_name").
		Joins("JOIN users AS creator ON creator.id = redemptions.user_id").
		Joins("LEFT JOIN users AS used_user ON used_user.id = redemptions.used_user_id")
	if !isAdmin {
		query = query.Where("redemptions.user_id = ?", userId)
	} else if creatorId > 0 {
		query = query.Where("redemptions.user_id = ?", creatorId)
	}
	if creatorName != "" {
		query = query.Where("creator.username LIKE ? OR creator.display_name LIKE ?", "%"+creatorName+"%", "%"+creatorName+"%")
	}
	if usedUserId > 0 {
		query = query.Where("redemptions.used_user_id = ?", usedUserId)
	}
	if usedUserName != "" {
		query = query.Where("used_user.username LIKE ? OR used_user.display_name LIKE ?", "%"+usedUserName+"%", "%"+usedUserName+"%")
	}
	if startTimestamp != 0 {
		query = query.Where("redemptions.created_time >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		query = query.Where("redemptions.created_time <= ?", endTimestamp)
	}
	if status != 0 {
		query = query.Where("redemptions.status = ?", status)
	}
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = query.Order("redemptions.created_time desc, redemptions.id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error; err != nil {
		return nil, 0, err
	}
	return redemptions, total, nil
}

type RedemptionAuditStat struct {
	CreatedQuota  int64 `json:"created_quota"`
	RedeemedQuota int64 `json:"redeemed_quota"`
	UnusedQuota   int64 `json:"unused_quota"`
	CreatedCount  int64 `json:"created_count"`
	RedeemedCount int64 `json:"redeemed_count"`
	UnusedCount   int64 `json:"unused_count"`
}

func GetRedemptionAuditKeys(ids []int, userId int, isAdmin bool) (redemptions []*Redemption, err error) {
	query := DB.Model(&Redemption{}).Select("id, name, key").Where("id IN ?", ids)
	if !isAdmin {
		query = query.Where("user_id = ?", userId)
	}
	err = query.Order("id asc").Find(&redemptions).Error
	return redemptions, err
}

type RedemptionAuditDeleteRejection struct {
	Id     int    `json:"id"`
	Reason string `json:"reason"`
}

func DeleteRedemptionsForAudit(ids []int, userId int, isAdmin bool) (deletedIds []int, rejected []RedemptionAuditDeleteRejection, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var redemptions []Redemption
	redemptionQuery := tx.Where("id IN ?", ids)
	if !common.UsingSQLite {
		redemptionQuery = redemptionQuery.Set("gorm:query_option", "FOR UPDATE")
	}
	if err = redemptionQuery.Find(&redemptions).Error; err != nil {
		tx.Rollback()
		return nil, nil, err
	}
	found := make(map[int]Redemption, len(redemptions))
	for _, redemption := range redemptions {
		found[redemption.Id] = redemption
	}
	for _, id := range ids {
		redemption, ok := found[id]
		if !ok {
			rejected = append(rejected, RedemptionAuditDeleteRejection{Id: id, Reason: "not_found"})
			continue
		}
		if !isAdmin && redemption.UserId != userId {
			rejected = append(rejected, RedemptionAuditDeleteRejection{Id: id, Reason: "forbidden"})
			continue
		}
		if redemption.Status == common.RedemptionCodeStatusUsed {
			rejected = append(rejected, RedemptionAuditDeleteRejection{Id: id, Reason: "used"})
			continue
		}
		result := tx.Where("id = ? AND status <> ?", id, common.RedemptionCodeStatusUsed).Delete(&Redemption{})
		if result.Error != nil {
			tx.Rollback()
			return nil, nil, result.Error
		}
		if result.RowsAffected == 0 {
			var current Redemption
			lookupErr := tx.Where("id = ?", id).First(&current).Error
			if lookupErr != nil {
				if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
					rejected = append(rejected, RedemptionAuditDeleteRejection{Id: id, Reason: "not_found"})
					continue
				}
				tx.Rollback()
				return nil, nil, lookupErr
			}
			reason := "not_found"
			if current.Status == common.RedemptionCodeStatusUsed {
				reason = "used"
			}
			rejected = append(rejected, RedemptionAuditDeleteRejection{Id: id, Reason: reason})
			continue
		}
		deletedIds = append(deletedIds, id)
	}
	if err = tx.Commit().Error; err != nil {
		return nil, nil, err
	}
	return deletedIds, rejected, nil
}

func GetRedemptionAuditStat(userId int, isAdmin bool, creatorId int, creatorName string, usedUserId int, usedUserName string, startTimestamp int64, endTimestamp int64, status int) (stat RedemptionAuditStat, err error) {
	query := DB.Model(&Redemption{}).
		Select(`
			COALESCE(SUM(redemptions.quota), 0) AS created_quota,
			COALESCE(SUM(CASE WHEN redemptions.status = ? THEN redemptions.quota ELSE 0 END), 0) AS redeemed_quota,
			COALESCE(SUM(CASE WHEN redemptions.status <> ? THEN redemptions.quota ELSE 0 END), 0) AS unused_quota,
			COUNT(*) AS created_count,
			COALESCE(SUM(CASE WHEN redemptions.status = ? THEN 1 ELSE 0 END), 0) AS redeemed_count,
			COALESCE(SUM(CASE WHEN redemptions.status <> ? THEN 1 ELSE 0 END), 0) AS unused_count`,
			common.RedemptionCodeStatusUsed,
			common.RedemptionCodeStatusUsed,
			common.RedemptionCodeStatusUsed,
			common.RedemptionCodeStatusUsed,
		).
		Joins("JOIN users AS creator ON creator.id = redemptions.user_id").
		Joins("LEFT JOIN users AS used_user ON used_user.id = redemptions.used_user_id")
	if !isAdmin {
		query = query.Where("redemptions.user_id = ?", userId)
	} else if creatorId > 0 {
		query = query.Where("redemptions.user_id = ?", creatorId)
	}
	if creatorName != "" {
		query = query.Where("creator.username LIKE ? OR creator.display_name LIKE ?", "%"+creatorName+"%", "%"+creatorName+"%")
	}
	if usedUserId > 0 {
		query = query.Where("redemptions.used_user_id = ?", usedUserId)
	}
	if usedUserName != "" {
		query = query.Where("used_user.username LIKE ? OR used_user.display_name LIKE ?", "%"+usedUserName+"%", "%"+usedUserName+"%")
	}
	if startTimestamp != 0 {
		query = query.Where("redemptions.created_time >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		query = query.Where("redemptions.created_time <= ?", endTimestamp)
	}
	if status != 0 {
		query = query.Where("redemptions.status = ?", status)
	}
	err = query.Scan(&stat).Error
	return stat, err
}

func GetAllRedemptions(startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取总数
	err = tx.Model(&Redemption{}).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func SearchRedemptions(keyword string, startIdx int, num int) (redemptions []*Redemption, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Build query based on keyword type
	query := tx.Model(&Redemption{})

	// Only try to convert to ID if the string represents a valid integer
	if id, err := strconv.Atoi(keyword); err == nil {
		query = query.Where("id = ? OR name LIKE ?", id, keyword+"%")
	} else {
		query = query.Where("name LIKE ?", keyword+"%")
	}

	// Get total count
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated data
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&redemptions).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return redemptions, total, nil
}

func GetRedemptionById(id int) (*Redemption, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	var err error = nil
	err = DB.First(&redemption, "id = ?", id).Error
	return &redemption, err
}

func Redeem(key string, userId int) (quota int, err error) {
	if key == "" {
		return 0, errors.New("未提供兑换码")
	}
	if userId == 0 {
		return 0, errors.New("无效的 user id")
	}
	redemption := &Redemption{}

	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	common.RandomSleep()
	err = DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Set("gorm:query_option", "FOR UPDATE").Where(keyCol+" = ?", key).First(redemption).Error
		if err != nil {
			return errors.New("无效的兑换码")
		}
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("该兑换码已被使用")
		}
		if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
			return errors.New("该兑换码已过期")
		}
		err = tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", redemption.Quota)).Error
		if err != nil {
			return err
		}
		redemption.RedeemedTime = common.GetTimestamp()
		redemption.Status = common.RedemptionCodeStatusUsed
		redemption.UsedUserId = userId
		err = tx.Save(redemption).Error
		return err
	})
	if err != nil {
		common.SysError("redemption failed: " + err.Error())
		return 0, ErrRedeemFailed
	}
	RecordLog(userId, LogTypeTopup, fmt.Sprintf("通过兑换码充值 %s，兑换码ID %d", logger.LogQuota(redemption.Quota), redemption.Id))
	return redemption.Quota, nil
}

func CreateRedemptions(userId int, redemption *Redemption) (keys []string, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < redemption.Count; i++ {
			key := common.GetUUID()
			cleanRedemption := Redemption{
				UserId:      userId,
				Name:        redemption.Name,
				Key:         key,
				CreatedTime: common.GetTimestamp(),
				Quota:       redemption.Quota,
				ExpiredTime: redemption.ExpiredTime,
			}
			if err := tx.Create(&cleanRedemption).Error; err != nil {
				return err
			}
			keys = append(keys, key)
		}
		return nil
	})
	return keys, err
}

func (redemption *Redemption) Insert() error {
	var err error
	err = DB.Create(redemption).Error
	return err
}

func (redemption *Redemption) SelectUpdate() error {
	// This can update zero values
	return DB.Model(redemption).Select("redeemed_time", "status").Updates(redemption).Error
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (redemption *Redemption) Update() error {
	var err error
	err = DB.Model(redemption).Select("name", "status", "quota", "redeemed_time", "expired_time").Updates(redemption).Error
	return err
}

func (redemption *Redemption) Delete() error {
	if redemption.Status == common.RedemptionCodeStatusUsed {
		return errors.New("已使用的兑换码不能删除")
	}
	result := DB.Where("status <> ?", common.RedemptionCodeStatusUsed).Delete(redemption)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("已使用的兑换码不能删除")
	}
	return nil
}

func DeleteRedemptionById(id int) (err error) {
	if id == 0 {
		return errors.New("id 为空！")
	}
	redemption := Redemption{Id: id}
	err = DB.Where(redemption).First(&redemption).Error
	if err != nil {
		return err
	}
	if redemption.Status == common.RedemptionCodeStatusUsed {
		return errors.New("已使用的兑换码不能删除")
	}
	return redemption.Delete()
}

func DeleteInvalidRedemptions() (int64, error) {
	now := common.GetTimestamp()
	result := DB.Where("status = ? OR (status = ? AND expired_time != 0 AND expired_time < ?)", common.RedemptionCodeStatusDisabled, common.RedemptionCodeStatusEnabled, now).Delete(&Redemption{})
	return result.RowsAffected, result.Error
}
