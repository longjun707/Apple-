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

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
	)

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
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

	return nil
}

// AutoMigrate runs database migrations
func AutoMigrate() error {
	err := DB.AutoMigrate(
		&Admin{},
		&Account{},
		&HMERecord{},
		&LoginLog{},
	)
	if err != nil {
		return err
	}

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
