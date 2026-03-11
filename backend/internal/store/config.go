package store

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Config holds database configuration
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

// DefaultConfig returns the default database configuration from environment variables
func DefaultConfig() *Config {
	port := 3306
	if p := os.Getenv("DB_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	return &Config{
		Host:     getEnvOrDefault("DB_HOST", "127.0.0.1"),
		Port:     port,
		User:     getEnvOrDefault("DB_USER", "root"),
		Password: os.Getenv("DB_PASSWORD"),
		DBName:   getEnvOrDefault("DB_NAME", "icloud"),
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// InitDB initializes the database connection
func InitDB(cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Initialize encryption for Apple passwords
	InitEncryption()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
	)

	// Use Warn level by default; set DB_DEBUG=1 for verbose SQL logging
	logLevel := logger.Warn
	if os.Getenv("DB_DEBUG") == "1" {
		logLevel = logger.Info
	}

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	log.Println("✅ Database connected successfully")

	// Auto migrate tables
	if err := AutoMigrate(); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	// Start periodic cleanup of old login logs (keep 90 days)
	go cleanupOldLogs()

	return nil
}

// cleanupOldLogs periodically removes login logs older than 90 days
func cleanupOldLogs() {
	// Run once on startup
	time.Sleep(10 * time.Second)
	deleteOldLogs()

	ticker := time.NewTicker(24 * time.Hour)
	for range ticker.C {
		deleteOldLogs()
	}
}

func deleteOldLogs() {
	if DB == nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -90)
	result := DB.Where("created_at < ?", cutoff).Delete(&LoginLog{})
	if result.RowsAffected > 0 {
		log.Printf("🧹 Cleaned up %d old login logs", result.RowsAffected)
	}
}

// AutoMigrate runs database migrations
func AutoMigrate() error {
	err := DB.AutoMigrate(
		&Admin{},
		&Account{},
		&HMERecord{},
		&LoginLog{},
		&SystemSetting{},
	)
	if err != nil {
		return err
	}

	// Ensure phone_numbers column exists (keep migration idempotent and quiet on restart)
	if !DB.Migrator().HasColumn(&Account{}, "PhoneNumbers") {
		if err := DB.Migrator().AddColumn(&Account{}, "PhoneNumbers"); err != nil {
			log.Printf("[Migration] Failed to add phone_numbers column: %v", err)
		} else {
			log.Printf("[Migration] Added phone_numbers column to accounts table")
		}
	}

	// Migrate session_scnt and session_id from VARCHAR to TEXT (fix truncation issue)
	DB.Exec("ALTER TABLE accounts MODIFY COLUMN session_scnt TEXT")
	DB.Exec("ALTER TABLE accounts MODIFY COLUMN session_id TEXT")

	// Create default admin if not exists
	var count int64
	DB.Model(&Admin{}).Count(&count)
	if count == 0 {
		admin := &Admin{
			Username: "admin",
			Nickname: "管理员",
			Role:     "admin",
			Status:   1,
		}
		admin.SetPassword("admin123")
		DB.Create(admin)
		log.Println("📝 Created default admin: admin / admin123")
	}

	return nil
}

// Close closes the database connection
func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
