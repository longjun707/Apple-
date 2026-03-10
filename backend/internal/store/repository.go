package store

import (
	"time"
)

// AdminRepo handles admin user database operations
type AdminRepo struct{}

// NewAdminRepo creates a new admin repository
func NewAdminRepo() *AdminRepo {
	return &AdminRepo{}
}

// FindByUsername finds an admin by username
func (r *AdminRepo) FindByUsername(username string) (*Admin, error) {
	var admin Admin
	err := DB.Where("username = ? AND status = 1", username).First(&admin).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// UpdateLastLogin updates the last login time
func (r *AdminRepo) UpdateLastLogin(adminID uint) error {
	now := time.Now()
	return DB.Model(&Admin{}).Where("id = ?", adminID).Update("last_login", now).Error
}

// FindByID finds an admin by ID
func (r *AdminRepo) FindByID(id uint) (*Admin, error) {
	var admin Admin
	err := DB.First(&admin, id).Error
	if err != nil {
		return nil, err
	}
	return &admin, nil
}

// AccountRepo handles account database operations
type AccountRepo struct{}

// NewAccountRepo creates a new account repository
func NewAccountRepo() *AccountRepo {
	return &AccountRepo{}
}

// FindByAppleID finds an account by Apple ID
func (r *AccountRepo) FindByAppleID(appleID string) (*Account, error) {
	var account Account
	err := DB.Where("apple_id = ?", appleID).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// FindOrCreate finds or creates an account
func (r *AccountRepo) FindOrCreate(appleID string) (*Account, error) {
	var account Account
	err := DB.Where("apple_id = ?", appleID).FirstOrCreate(&account, Account{
		AppleID: appleID,
		Status:  1,
	}).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// UpdateLastLogin updates the last login time
func (r *AccountRepo) UpdateLastLogin(accountID uint) error {
	now := time.Now()
	return DB.Model(&Account{}).Where("id = ?", accountID).Update("last_login", now).Error
}

// UpdateHMECount updates the HME count
func (r *AccountRepo) UpdateHMECount(accountID uint, count int) error {
	return DB.Model(&Account{}).Where("id = ?", accountID).Update("hme_count", count).Error
}

// List returns all accounts
func (r *AccountRepo) List(page, pageSize int) ([]Account, int64, error) {
	var accounts []Account
	var total int64

	DB.Model(&Account{}).Count(&total)

	offset := (page - 1) * pageSize
	err := DB.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&accounts).Error
	return accounts, total, err
}

// HMERepo handles HME record database operations
type HMERepo struct{}

// NewHMERepo creates a new HME repository
func NewHMERepo() *HMERepo {
	return &HMERepo{}
}

// Create creates a new HME record
func (r *HMERepo) Create(record *HMERecord) error {
	return DB.Create(record).Error
}

// BatchCreate creates multiple HME records
func (r *HMERepo) BatchCreate(records []HMERecord) error {
	if len(records) == 0 {
		return nil
	}
	return DB.CreateInBatches(records, 100).Error
}

// FindByAccountID finds all HME records for an account
func (r *HMERepo) FindByAccountID(accountID uint) ([]HMERecord, error) {
	var records []HMERecord
	err := DB.Where("account_id = ?", accountID).Order("created_at DESC").Find(&records).Error
	return records, err
}

// FindByEmail finds an HME record by email address
func (r *HMERepo) FindByEmail(email string) (*HMERecord, error) {
	var record HMERecord
	err := DB.Where("email_address = ?", email).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// FindByHMEID finds an HME record by Apple's HME ID
func (r *HMERepo) FindByHMEID(hmeID string) (*HMERecord, error) {
	var record HMERecord
	err := DB.Where("hme_id = ?", hmeID).First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// Delete soft deletes an HME record
func (r *HMERepo) Delete(id uint) error {
	return DB.Delete(&HMERecord{}, id).Error
}

// DeleteByHMEID deletes by Apple's HME ID
func (r *HMERepo) DeleteByHMEID(hmeID string) error {
	return DB.Where("hme_id = ?", hmeID).Delete(&HMERecord{}).Error
}

// Count returns the total count for an account
func (r *HMERepo) Count(accountID uint) (int64, error) {
	var count int64
	err := DB.Model(&HMERecord{}).Where("account_id = ?", accountID).Count(&count).Error
	return count, err
}

// ListAll returns all HME records with pagination
func (r *HMERepo) ListAll(page, pageSize int) ([]HMERecord, int64, error) {
	var records []HMERecord
	var total int64

	DB.Model(&HMERecord{}).Count(&total)

	offset := (page - 1) * pageSize
	err := DB.Preload("Account").Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&records).Error
	return records, total, err
}

// LoginLogRepo handles login log database operations
type LoginLogRepo struct{}

// NewLoginLogRepo creates a new login log repository
func NewLoginLogRepo() *LoginLogRepo {
	return &LoginLogRepo{}
}

// Create creates a new login log
func (r *LoginLogRepo) Create(log *LoginLog) error {
	return DB.Create(log).Error
}

// FindByAccountID finds all login logs for an account
func (r *LoginLogRepo) FindByAccountID(accountID uint, limit int) ([]LoginLog, error) {
	var logs []LoginLog
	err := DB.Where("account_id = ?", accountID).Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}
