package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"apple-hme-manager/internal/api"
	"apple-hme-manager/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Println("ℹ️  No .env file found, using environment variables")
	}

	port := flag.Int("port", 8080, "Server port")
	debug := flag.Bool("debug", false, "Enable debug mode")
	noDb := flag.Bool("no-db", false, "Disable database")
	flag.Parse()

	if !*debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize database
	if !*noDb {
		if err := store.InitDB(nil); err != nil {
			log.Printf("⚠️  Database connection failed: %v", err)
			log.Println("   Running without database persistence...")
		} else {
			defer store.Close()
		}
	}

	r := gin.Default()

	// CORS configuration
	allowedOrigins := []string{"http://localhost:5173", "http://localhost:3000", "http://127.0.0.1:5173"}
	if extra := os.Getenv("CORS_ORIGINS"); extra != "" {
		allowedOrigins = strings.Split(extra, ",")
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "X-Session-ID"},
		ExposeHeaders:    []string{"X-Session-ID"},
		AllowCredentials: true,
	}))

	// Create API server
	server := api.NewServer()

	// Apply session middleware to all routes
	r.Use(server.SessionMiddleware())

	// Routes
	apiGroup := r.Group("/api")
	{
		// Public routes (no auth required)
		apiGroup.GET("/health", server.Health)
		apiGroup.POST("/admin/login", server.AdminLogin)

		// Protected routes (require admin auth)
		protected := apiGroup.Group("")
		protected.Use(server.AdminAuthMiddleware())
		{
			// Admin routes
			admin := protected.Group("/admin")
			{
				admin.POST("/logout", server.AdminLogout)
				admin.GET("/info", server.AdminInfo)
				admin.GET("/stats", server.AdminStats)
				admin.GET("/hme", server.AdminListAllHME)
				admin.PUT("/password", server.AdminChangePassword)
			}

			// Apple Account management routes
			accounts := protected.Group("/accounts")
			{
				accounts.GET("", server.ListAccounts)
				accounts.POST("", server.CreateAccount)
				accounts.PUT("/:id", server.UpdateAccount)
				accounts.DELETE("/:id", server.DeleteAccount)
				accounts.POST("/:id/login", server.LoginAppleAccount)
				accounts.POST("/:id/2fa", server.Verify2FAForAccount)
				accounts.POST("/:id/request-sms", server.RequestSMSForAccount)
				accounts.GET("/:id/hme", server.GetAccountHME)
				accounts.POST("/:id/hme", server.CreateAccountHME)
				accounts.POST("/:id/hme/batch", server.BatchCreateAccountHME)
				accounts.DELETE("/:id/hme/:hmeId", server.DeleteAccountHME)
				accounts.GET("/:id/forward-emails", server.GetAccountForwardEmails)
			}
		}

		// Legacy auth routes (for backward compatibility)
		auth := apiGroup.Group("/auth")
		{
			auth.POST("/login", server.Login)
			auth.POST("/2fa", server.Verify2FA)
			auth.POST("/sms", server.RequestSMS)
			auth.POST("/logout", server.Logout)
		}

		// Legacy account routes
		apiGroup.GET("/account", server.GetAccount)

		// Legacy HME routes
		hme := apiGroup.Group("/hme")
		{
			hme.GET("", server.ListHME)
			hme.POST("", server.CreateHME)
			hme.POST("/batch", server.BatchCreateHME)
			hme.DELETE("/:id", server.DeleteHME)
			hme.GET("/forward-emails", server.GetForwardEmails)
		}
	}

	// Serve static files (frontend) in production
	if _, err := os.Stat("./static"); err == nil {
		r.Static("/assets", "./static/assets")
		r.StaticFile("/", "./static/index.html")
		r.NoRoute(func(c *gin.Context) {
			c.File("./static/index.html")
		})
	}

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("🚀 Server starting on http://localhost%s", addr)
	log.Printf("   API: http://localhost%s/api", addr)

	if err := r.Run(addr); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
