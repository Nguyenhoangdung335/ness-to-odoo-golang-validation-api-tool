package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"ness-to-odoo-golang-validation-api-tool/api/services"
	"ness-to-odoo-golang-validation-api-tool/utils"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ValidateEmailsRequest represents the request structure for email validation
type ValidateEmailsRequest struct {
	// No body parameters as we're using multipart form
}

// ValidationResult represents the response structure for email validation
type ValidationResult struct {
	MatchingEmails      []string                   `json:"matchingEmails"`
	MissingInFirstFile  []string                   `json:"missingInFirstFile"`
	MissingInSecondFile []string                   `json:"missingInSecondFile"`
	OutputFileURL       string                     `json:"outputFileURL"`
	Summary             services.ValidationSummary `json:"summary"`
}

// ValidateEmails godoc
// @Summary Validate emails from two files
// @Description Upload two CSV/Excel files containing emails and get validation results
// @Tags emails
// @Accept multipart/form-data
// @Produce json
// @Param firstFile formData file true "First CSV/Excel file containing emails"
// @Param secondFile formData file true "Second CSV/Excel file containing emails"
// @Param outputFormat formData string false "Output format (csv or excel, default: csv)"
// @Success 200 {file} file
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /validate-emails [post]
func ValidateEmails(c *gin.Context) {
	logger := utils.GetLogger()
	defer utils.LogExecutionTime("ValidateEmails handler")()
	logger.Info("Processing email validation request")

	// Get files from request
	firstFile, err := c.FormFile("firstFile")
	if err != nil {
		logger.Warn("First file is missing from request")
		c.JSON(http.StatusBadRequest, gin.H{"error": "First file is required"})
		return
	}
	logger.Info("Received first file: %s (size: %.2f MB)", firstFile.Filename, float64(firstFile.Size)/(1024*1024))

	secondFile, err := c.FormFile("secondFile")
	if err != nil {
		logger.Warn("Second file is missing from request")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Second file is required"})
		return
	}
	logger.Info("Received second file: %s (size: %.2f MB)", secondFile.Filename, float64(secondFile.Size)/(1024*1024))

	// Get output format (default to CSV)
	outputFormat := c.DefaultPostForm("outputFormat", "csv")
	logger.Info("Output format: %s", outputFormat)
	if outputFormat != "csv" && outputFormat != "excel" {
		logger.Warn("Invalid output format: %s", outputFormat)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Output format must be 'csv' or 'excel'"})
		return
	}

	// Validate file extensions
	firstFileExt := filepath.Ext(firstFile.Filename)
	secondFileExt := filepath.Ext(secondFile.Filename)
	logger.Debug("File extensions: %s, %s", firstFileExt, secondFileExt)

	validExts := map[string]bool{
		".csv":  true,
		".xlsx": true,
		".xls":  true,
	}

	if !validExts[firstFileExt] || !validExts[secondFileExt] {
		logger.Warn("Invalid file format: %s, %s", firstFileExt, secondFileExt)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid file format. Only CSV and Excel files are supported",
		})
		return
	}

	// Save uploaded files temporarily
	firstFilePath := fmt.Sprintf("./temp/%s", firstFile.Filename)
	secondFilePath := fmt.Sprintf("./temp/%s", secondFile.Filename)
	logger.Debug("Saving files to: %s, %s", firstFilePath, secondFilePath)

	startTime := time.Now()
	if err := c.SaveUploadedFile(firstFile, firstFilePath); err != nil {
		logger.Error("Failed to save first file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save first file"})
		return
	}
	logger.Debug("First file saved in %s", utils.FormatDuration(time.Since(startTime)))

	startTime = time.Now()
	if err := c.SaveUploadedFile(secondFile, secondFilePath); err != nil {
		logger.Error("Failed to save second file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save second file"})
		return
	}
	logger.Debug("Second file saved in %s", utils.FormatDuration(time.Since(startTime)))

	// Process files and validate emails
	logger.Info("Starting email validation process")
	startTime = time.Now()
	result, err := services.ValidateEmails(firstFilePath, secondFilePath, outputFormat)
	if err != nil {
		logger.Error("Email validation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logger.Info("Email validation completed in %s", utils.FormatDuration(time.Since(startTime)))

	logger.Info("Returning validation result: %d matching, %d missing in first, %d missing in second",
		len(result.MatchingEmails), len(result.MissingInFirstFile), len(result.MissingInSecondFile))

	filePath := filepath.Join("./temp", result.FileName)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Set appropriate content type based on file extension
	ext := filepath.Ext(result.FileName)
	contentType := "application/octet-stream"

	switch ext {
	case ".csv":
		contentType = "text/csv"
	case ".xlsx", ".xls":
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", result.FileName))
	c.Header("Content-Type", contentType)

	c.File(filePath)

	//c.JSON(http.StatusOK, result)
}
