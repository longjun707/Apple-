package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"apple-hme-manager/internal/api"
	"apple-hme-manager/internal/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
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
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000", "http://127.0.0.1:5173"},
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
		// Admin auth routes
		admin := apiGroup.Group("/admin")
		{
		admin.POST("/login", server.AdminLogin)
			admin.POST("/logout", server.AdminLogout)
			admin.GET("/info", server.AdminInfo)
			admin.GET("/stats", server.AdminStats)
			admin.GET("/hme", server.AdminListAllHME)
			admin.PUT("/password", server.AdminChangePassword)
		}

		// Apple Account management routes
		accounts := apiGroup.Group("/accounts")
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

		// Health check
		apiGroup.GET("/health", server.Health)
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
