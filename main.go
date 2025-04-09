package main

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"log"
	"os"
	"path/filepath"

	"ness-to-odoo-golang-validation-api-tool/api/handlers"
	"ness-to-odoo-golang-validation-api-tool/api/middleware"
	_ "ness-to-odoo-golang-validation-api-tool/docs" // Import generated swagger docs
	"ness-to-odoo-golang-validation-api-tool/utils"
)

// @title Email Validation API
// @version 1.0
// @description API for validating and comparing emails from two different sources
// @host localhost:8080
// @BasePath /api/v1
func main() {
	// Initialize directories
	dirs := []string{"./temp", "./logs"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			log.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Initialize logger
	logDir := filepath.Join(".", "logs")
	if err := utils.InitLogger(utils.DEBUG, logDir, "2006-01-02 15:04:05.000"); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	logger := utils.GetLogger()
	logger.Info("Email Validation API starting up")

	// Set Gin to release mode in production
	// gin.SetMode(gin.ReleaseMode)

	// Create a new Gin router with default middleware
	r := gin.New()

	// Add recovery middleware to handle panics
	r.Use(gin.Recovery())

	// Add custom logger middleware
	r.Use(middleware.Logger())

	// API v1 routes
	v1 := r.Group("/api/v1")
	{
		v1.POST("/validate-emails", handlers.ValidateEmails)
		v1.GET("/download/:filename", handlers.DownloadFile)
	}

	// Swagger documentation
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	port := ":8080"
	logger.Info("Server starting on %s", port)
	logger.Info("Swagger documentation available at http://localhost%s/swagger/index.html", port)

	if err := r.Run(port); err != nil {
		logger.Fatal("Failed to start server: %v", err)
	}
}
