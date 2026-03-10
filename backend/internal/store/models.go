package store

import (
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Admin represents an admin user for the management system
type Admin struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Username  string         `gorm:"uniqueIndex;size:100" json:"username"`
	Password  string         `gorm:"size:255" json:"-"`
	Nickname  string         `gorm:"size:100" json:"nickname"`
	Role      string         `gorm:"size:50;default:admin" json:"role"`
	Status    int            `gorm:"default:1" json:"status"`
	LastLogin *time.Time     `json:"lastLogin"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// SetPassword hashes and sets the password
func (a *Admin) SetPassword(password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	a.Password = string(hash)
	return nil
}

// CheckPassword verifies the password
func (a *Admin) CheckPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(a.Password), []byte(password))
	return err == nil
}

// TableName sets the table name for Admin
func (Admin) TableName() string {
	return "admins"
}

// Account represents an Apple ID account
type Account struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	AppleID   string         `gorm:"uniqueIndex;size:255" json:"appleId"`
	Password  string         `gorm:"size:255" json:"-"` // Encrypted password
	Remark    string         `gorm:"size:500" json:"remark"`
	Status    int            `gorm:"default:1" json:"status"` // 1=active, 0=disabled, 2=locked
	HMECount  int            `gorm:"default:0" json:"hmeCount"`
	LastLogin *time.Time     `json:"lastLogin"`
	LastError string         `gorm:"size:500" json:"lastError"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Apple session persistence (survives server restart)
	SessionToken   string     `gorm:"type:text" json:"-"`
	SessionSCNT    string     `gorm:"size:500" json:"-"`
	SessionID      string     `gorm:"size:500" json:"-"`
	SessionCookies string     `gorm:"type:text" json:"-"`
	SessionSavedAt *time.Time `json:"sessionSavedAt"`

	// Family sharing info
	IsFamilyOrganizer bool   `gorm:"default:false" json:"isFamilyOrganizer"`
	FamilyMemberCount int    `gorm:"default:0" json:"familyMemberCount"`
	FamilyRole        string `gorm:"size:50" json:"familyRole"` // organizer, parent, adult, child

	// Profile info
	FullName           string `gorm:"size:255" json:"fullName"`
	Birthday           string `gorm:"size:50" json:"birthday"`
	Country            string `gorm:"size:10" json:"country"`          // CHN, USA, etc.
	AlternateEmails    string `gorm:"type:text" json:"alternateEmails"` // JSON array
	PhoneNumbers       string `gorm:"type:text" json:"phoneNumbers"`    // JSON array of phone numbers
	TrustedDeviceCount int    `gorm:"default:0" json:"trustedDeviceCount"`
	TwoFactorEnabled   bool   `gorm:"default:false" json:"twoFactorEnabled"`

	// Relations
	HMERecords []HMERecord `gorm:"foreignKey:AccountID" json:"hmeRecords,omitempty"`
	LoginLogs  []LoginLog  `gorm:"foreignKey:AccountID" json:"loginLogs,omitempty"`
}

// HMERecord represents a Hide My Email address record
type HMERecord struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	AccountID      uint           `gorm:"index" json:"accountId"`
	HMEID          string         `gorm:"size:100;index" json:"hmeId"`
	EmailAddress   string         `gorm:"size:255;uniqueIndex" json:"emailAddress"`
	Label          string         `gorm:"size:255" json:"label"`
	Note           string         `gorm:"size:500" json:"note"`
	ForwardToEmail string         `gorm:"size:255" json:"forwardToEmail"`
	Active         bool           `gorm:"default:true" json:"active"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// LoginLog represents a login history record
type LoginLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AccountID uint      `gorm:"index" json:"accountId"`
	IP        string    `gorm:"size:50" json:"ip"`
	UserAgent string    `gorm:"size:500" json:"userAgent"`
	Status    string    `gorm:"size:50" json:"status"` // success, failed, 2fa_required
	Message   string    `gorm:"size:500" json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// TableName sets the table name for Account
func (Account) TableName() string {
	return "accounts"
}

// TableName sets the table name for HMERecord
func (HMERecord) TableName() string {
	return "hme_records"
}

// TableName sets the table name for LoginLog
func (LoginLog) TableName() string {
	return "login_logs"
}
