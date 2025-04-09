package services

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
	"ness-to-odoo-golang-validation-api-tool/utils"
)

// EmailEntry represents an email entry with validation details
type EmailEntry struct {
	Email           string `json:"email"`
	Source          string `json:"source"`
	IsValid         bool   `json:"isValid"`
	IsDisposable    bool   `json:"isDisposable"`
	NormalizedEmail string `json:"normalizedEmail"`
	Status          string `json:"status"`
	Reason          string `json:"reason,omitempty"`
}

// ValidationResult represents the result of email validation
type ValidationResult struct {
	MatchingEmails      []string          `json:"matchingEmails"`
	MissingInFirstFile  []string          `json:"missingInFirstFile"`
	MissingInSecondFile []string          `json:"missingInSecondFile"`
	OutputFileURL       string            `json:"outputFileURL"`
	FileName            string            `json:"fileName"`
	Summary             ValidationSummary `json:"summary"`
}

// ValidationSummary contains summary statistics of the validation
type ValidationSummary struct {
	TotalEmailsFirstFile  int     `json:"totalEmailsFirstFile"`
	TotalEmailsSecondFile int     `json:"totalEmailsSecondFile"`
	ValidEmailsFirstFile  int     `json:"validEmailsFirstFile"`
	ValidEmailsSecondFile int     `json:"validEmailsSecondFile"`
	MatchingCount         int     `json:"matchingCount"`
	MissingInFirstCount   int     `json:"missingInFirstCount"`
	MissingInSecondCount  int     `json:"missingInSecondCount"`
	DisposableEmailsCount int     `json:"disposableEmailsCount"`
	ProcessingTimeSeconds float64 `json:"processingTimeSeconds"`
}

// ValidateEmails processes two files containing emails and returns validation results
// This version uses concurrent processing for better performance
func ValidateEmails(firstFilePath, secondFilePath, outputFormat string) (*ValidationResult, error) {
	logger := utils.GetLogger()
	logger.Info("Starting email validation process for files: %s and %s", firstFilePath, secondFilePath)
	startTime := time.Now()
	// Create temp directory if it doesn't exist
	if err := os.MkdirAll("./temp", os.ModePerm); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Use a WaitGroup to process both files concurrently
	wg := sync.WaitGroup{}
	wg.Add(2)

	// Channels for results and errors
	type extractResult struct {
		emails []string
		err    error
	}
	firstFileCh := make(chan extractResult, 1)
	secondFileCh := make(chan extractResult, 1)

	// Extract emails from first file concurrently
	go func() {
		defer wg.Done()
		emails, err := extractEmails(firstFilePath)
		firstFileCh <- extractResult{emails, err}
	}()

	// Extract emails from second file concurrently
	go func() {
		defer wg.Done()
		emails, err := extractEmails(secondFilePath)
		secondFileCh <- extractResult{emails, err}
	}()

	// Wait for both goroutines to complete
	wg.Wait()

	// Get results from channels
	firstResult := <-firstFileCh
	secondResult := <-secondFileCh

	// Check for errors
	if firstResult.err != nil {
		return nil, fmt.Errorf("failed to extract emails from first file: %w", firstResult.err)
	}
	if secondResult.err != nil {
		return nil, fmt.Errorf("failed to extract emails from second file: %w", secondResult.err)
	}

	// Process both files concurrently
	wg.Add(2)

	// Channels for validation results
	type validationResult struct {
		entries []EmailEntry
	}
	firstValidationCh := make(chan validationResult, 1)
	secondValidationCh := make(chan validationResult, 1)

	// Validate first file emails concurrently
	go func() {
		defer wg.Done()
		entries := validateEmailList(firstResult.emails, "First File")
		firstValidationCh <- validationResult{entries}
	}()

	// Validate second file emails concurrently
	go func() {
		defer wg.Done()
		entries := validateEmailList(secondResult.emails, "Second File")
		secondValidationCh <- validationResult{entries}
	}()

	// Wait for validation to complete
	wg.Wait()

	// Get validation results
	firstFileEntries := (<-firstValidationCh).entries
	secondFileEntries := (<-secondValidationCh).entries

	// Compare emails using normalized versions for better matching
	matchingEmails, missingInFirst, missingInSecond, summary := compareEmailEntries(firstFileEntries, secondFileEntries)

	// Generate output file
	// Add processing time to summary
	processingTime := time.Since(startTime)
	summary.ProcessingTimeSeconds = processingTime.Seconds()

	if outputFormat == "excel" {
		outputFormat = "xlsx"
	}
	outputFileName := fmt.Sprintf("validation_result_%s.%s", time.Now().Format("20060102_150405"), outputFormat)
	outputFilePath := filepath.Join("./temp", outputFileName)

	logger.Info("Generating output file: %s", outputFilePath)
	if err := generateEnhancedOutputFile(outputFilePath, firstFileEntries, secondFileEntries, matchingEmails, missingInFirst, missingInSecond, summary); err != nil {
		logger.Error("Failed to generate output file: %v", err)
		return nil, fmt.Errorf("failed to generate output file: %w", err)
	}

	// Extract just the email strings for the API response
	logger.Debug("Preparing API response")
	matchingEmailStrings := make([]string, len(matchingEmails))
	missingInFirstStrings := make([]string, len(missingInFirst))
	missingInSecondStrings := make([]string, len(missingInSecond))

	for i, entry := range matchingEmails {
		matchingEmailStrings[i] = entry.Email
	}

	for i, entry := range missingInFirst {
		missingInFirstStrings[i] = entry.Email
	}

	for i, entry := range missingInSecond {
		missingInSecondStrings[i] = entry.Email
	}

	// Return results
	result := &ValidationResult{
		MatchingEmails:      matchingEmailStrings,
		MissingInFirstFile:  missingInFirstStrings,
		MissingInSecondFile: missingInSecondStrings,
		OutputFileURL:       fmt.Sprintf("/api/v1/download/%s", outputFileName),
		FileName:            outputFileName,
		Summary:             summary,
	}

	totalTime := time.Since(startTime)
	logger.Info("Email validation completed in %s. Results: %d matching, %d missing in first, %d missing in second",
		utils.FormatDuration(totalTime),
		len(matchingEmailStrings),
		len(missingInFirstStrings),
		len(missingInSecondStrings))

	return result, nil
}

// extractEmails extracts emails from a CSV or Excel file
func extractEmails(filePath string) ([]string, error) {
	logger := utils.GetLogger()
	defer utils.LogExecutionTime(fmt.Sprintf("extractEmails(%s)", filePath))()

	ext := strings.ToLower(filepath.Ext(filePath))
	logger.Info("Extracting emails from %s (format: %s)", filePath, ext)

	var emails []string
	var err error

	switch ext {
	case ".csv":
		emails, err = extractEmailsFromCSV(filePath)
	case ".xlsx", ".xls":
		emails, err = extractEmailsFromExcel(filePath)
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}

	if err != nil {
		logger.Error("Failed to extract emails from %s: %v", filePath, err)
		return nil, err
	}

	logger.Info("Successfully extracted %d emails from %s", len(emails), filePath)
	return emails, nil
}

// extractEmailsFromCSV extracts emails from a CSV file
// This version is optimized for large files with streaming processing
func extractEmailsFromCSV(filePath string) ([]string, error) {
	logger := utils.GetLogger()
	defer utils.LogExecutionTime("extractEmailsFromCSV")()
	logger.Debug("Starting CSV extraction from %s", filePath)
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create a buffered reader for better performance
	reader := csv.NewReader(file)

	// Read header row
	_, err = reader.Read()
	if err != nil {
		return nil, err
	}

	// Pre-allocate emails slice with a reasonable capacity
	// This avoids repeated slice growth and memory reallocation
	emails := make([]string, 0, 1000) // Start with capacity for 1000 emails

	// Process records one at a time to avoid loading the entire file into memory
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		// Extract email from the first column if it's valid
		if len(record) > 0 && record[0] != "" {
			// Only perform basic validation here for speed
			// The detailed validation will happen later
			if strings.Contains(record[0], "@") {
				emails = append(emails, record[0])
			}
		}
	}

	logger.Debug("CSV extraction completed, found %d potential emails", len(emails))
	return emails, nil
}

// extractEmailsFromExcel extracts emails from an Excel file
// This version is optimized for large files with streaming processing
func extractEmailsFromExcel(filePath string) ([]string, error) {
	logger := utils.GetLogger()
	defer utils.LogExecutionTime("extractEmailsFromExcel")()
	logger.Debug("Starting Excel extraction from %s", filePath)
	// Open the Excel file with streaming mode for better performance with large files
	f, err := excelize.OpenFile(filePath, excelize.Options{
		RawCellValue: true, // Get raw values for better performance
	})
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get the first sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in Excel file")
	}

	// Pre-allocate emails slice with a reasonable capacity
	emails := make([]string, 0, 1000) // Start with capacity for 1000 emails

	// Use rows iterator for streaming large files
	rows, err := f.Rows(sheets[0])
	if err != nil {
		return nil, err
	}

	// Skip header row
	if rows.Next() {
		_, err := rows.Columns()
		if err != nil {
			return nil, err
		}
	}

	// Process each row
	for rows.Next() {
		row, err := rows.Columns()
		if err != nil {
			return nil, err
		}

		// Extract email from the first column if it exists
		if len(row) > 0 && row[0] != "" {
			// Only perform basic validation here for speed
			// The detailed validation will happen later
			if strings.Contains(row[0], "@") {
				emails = append(emails, row[0])
			}
		}
	}

	logger.Debug("Excel extraction completed, found %d potential emails", len(emails))
	return emails, nil
}

// validateEmailList validates a list of emails and returns detailed validation results
// This version uses batch processing for better performance
func validateEmailList(emails []string, source string) []EmailEntry {
	logger := utils.GetLogger()
	defer utils.LogExecutionTime(fmt.Sprintf("validateEmailList(%s)", source))()

	logger.Info("Validating %d emails from %s", len(emails), source)

	// Use batch validation for better performance
	validationResults := utils.ValidateEmailsBatch(emails)

	// Convert validation results to email entries
	result := make([]EmailEntry, len(emails))
	for i, validationResult := range validationResults {
		status := "Invalid"
		if validationResult.IsValid {
			status = "Valid"
		}

		result[i] = EmailEntry{
			Email:           validationResult.Email,
			Source:          source,
			IsValid:         validationResult.IsValid,
			IsDisposable:    validationResult.IsDisposable,
			NormalizedEmail: validationResult.NormalizedEmail,
			Status:          status,
			Reason:          validationResult.Reason,
		}
	}

	logger.Info("Completed validation of %d emails from %s", len(emails), source)
	return result
}

// compareEmailEntries compares two lists of email entries and returns matching and missing emails
// This version is optimized for performance with pre-allocated slices and single-pass processing
func compareEmailEntries(firstEntries, secondEntries []EmailEntry) (matching, missingInFirst, missingInSecond []EmailEntry, summary ValidationSummary) {
	logger := utils.GetLogger()
	defer utils.LogExecutionTime("compareEmailEntries")()
	logger.Info("Comparing %d emails from first file with %d emails from second file", len(firstEntries), len(secondEntries))
	// Pre-allocate maps with appropriate capacity to avoid rehashing
	firstMap := make(map[string]EmailEntry, len(firstEntries))
	secondMap := make(map[string]EmailEntry, len(secondEntries))

	// Pre-allocate result slices with estimated capacities
	// This avoids repeated slice growth and memory reallocation
	estimatedMatchCount := min(len(firstEntries), len(secondEntries)) / 2
	estimatedMissingCount := len(firstEntries) / 4

	matching = make([]EmailEntry, 0, estimatedMatchCount)
	missingInFirst = make([]EmailEntry, 0, estimatedMissingCount)
	missingInSecond = make([]EmailEntry, 0, estimatedMissingCount)

	// Initialize summary
	summary = ValidationSummary{
		TotalEmailsFirstFile:  len(firstEntries),
		TotalEmailsSecondFile: len(secondEntries),
	}

	// Process first file entries
	for _, entry := range firstEntries {
		// Count valid emails
		if entry.IsValid {
			summary.ValidEmailsFirstFile++
		}

		// Use normalized email for comparison
		firstMap[entry.NormalizedEmail] = entry
	}

	// Process second file entries and find matches/missing in one pass
	for _, entry := range secondEntries {
		// Count valid emails
		if entry.IsValid {
			summary.ValidEmailsSecondFile++
		}

		// Check if this email exists in first file
		if firstEntry, exists := firstMap[entry.NormalizedEmail]; exists {
			// It's a match
			matching = append(matching, firstEntry)
		} else {
			// Missing in first file
			missingInFirst = append(missingInFirst, entry)
		}

		// Store in second map for finding missing in second file
		secondMap[entry.NormalizedEmail] = entry
	}

	// Find emails missing in second file
	for normalizedEmail, entry := range firstMap {
		if _, exists := secondMap[normalizedEmail]; !exists {
			missingInSecond = append(missingInSecond, entry)
		}
	}

	// Update summary counts
	summary.MatchingCount = len(matching)
	summary.MissingInFirstCount = len(missingInFirst)
	summary.MissingInSecondCount = len(missingInSecond)

	logger.Info("Comparison completed: %d matching, %d missing in first, %d missing in second",
		summary.MatchingCount, summary.MissingInFirstCount, summary.MissingInSecondCount)

	return matching, missingInFirst, missingInSecond, summary
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateEnhancedOutputFile generates an enhanced output file with detailed validation results
func generateEnhancedOutputFile(outputPath string, firstEntries, secondEntries, matching, missingInFirst, missingInSecond []EmailEntry, summary ValidationSummary) error {
	ext := strings.ToLower(filepath.Ext(outputPath))

	switch ext {
	case ".csv":
		return generateEnhancedCSVOutput(outputPath, firstEntries, secondEntries, matching, missingInFirst, missingInSecond, summary)
	case ".xlsx", ".xls":
		return generateEnhancedExcelOutput(outputPath, firstEntries, secondEntries, matching, missingInFirst, missingInSecond, summary)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

// generateEnhancedCSVOutput generates an enhanced CSV output file with detailed validation results
func generateEnhancedCSVOutput(outputPath string, firstEntries, secondEntries, matching, missingInFirst, missingInSecond []EmailEntry, summary ValidationSummary) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{
		"Email",
		"Normalized Email",
		"Source",
		"Status",
		"Valid",
		"Reason",
	}); err != nil {
		return err
	}

	// Write matching emails
	for _, entry := range matching {
		if err := writer.Write([]string{
			entry.Email,
			entry.NormalizedEmail,
			"Both",
			"Matching",
			fmtBool(entry.IsValid),
			entry.Reason,
		}); err != nil {
			return err
		}
	}

	// Write emails missing in first file
	for _, entry := range missingInFirst {
		if err := writer.Write([]string{
			entry.Email,
			entry.NormalizedEmail,
			"Second File Only",
			"Missing in First File",
			fmtBool(entry.IsValid),
			entry.Reason,
		}); err != nil {
			return err
		}
	}

	// Write emails missing in second file
	for _, entry := range missingInSecond {
		if err := writer.Write([]string{
			entry.Email,
			entry.NormalizedEmail,
			"First File Only",
			"Missing in Second File",
			fmtBool(entry.IsValid),
			entry.Reason,
		}); err != nil {
			return err
		}
	}

	// Write summary
	if err := writer.Write([]string{""}); err != nil {
		return err
	}

	if err := writer.Write([]string{"Summary"}); err != nil {
		return err
	}

	if err := writer.Write([]string{"Metric", "Value"}); err != nil {
		return err
	}

	// Write summary statistics
	summaryData := [][]string{
		{"Total Emails in First File", fmt.Sprintf("%d", summary.TotalEmailsFirstFile)},
		{"Total Emails in Second File", fmt.Sprintf("%d", summary.TotalEmailsSecondFile)},
		{"Valid Emails in First File", fmt.Sprintf("%d", summary.ValidEmailsFirstFile)},
		{"Valid Emails in Second File", fmt.Sprintf("%d", summary.ValidEmailsSecondFile)},
		{"Matching Emails", fmt.Sprintf("%d", summary.MatchingCount)},
		{"Emails Missing in First File", fmt.Sprintf("%d", summary.MissingInFirstCount)},
		{"Emails Missing in Second File", fmt.Sprintf("%d", summary.MissingInSecondCount)},
		{"Disposable Emails", fmt.Sprintf("%d", summary.DisposableEmailsCount)},
	}

	for _, row := range summaryData {
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// fmtBool formats a boolean value as "Yes" or "No"
func fmtBool(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// generateEnhancedExcelOutput generates an enhanced Excel output file with detailed validation results
func generateEnhancedExcelOutput(outputPath string, firstEntries, secondEntries, matching, missingInFirst, missingInSecond []EmailEntry, summary ValidationSummary) error {
	f := excelize.NewFile()

	// Create a new sheet for validation results
	resultsSheet := "Validation Results"
	index, err := f.NewSheet(resultsSheet)
	if err != nil {
		return err
	}
	f.SetActiveSheet(index)

	// Create styles
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"#DDEBF7"}, Pattern: 1},
		Border: []excelize.Border{
			{Type: "bottom", Color: "#000000", Style: 1},
		},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return err
	}

	// Write header
	headers := []string{
		"Email",
		"Normalized Email",
		"Source",
		"Status",
		"Valid",
		"Reason",
	}

	for i, header := range headers {
		cell := fmt.Sprintf("%s1", string('A'+i))
		f.SetCellValue(resultsSheet, cell, header)
	}

	// Apply header style
	f.SetCellStyle(resultsSheet, "A1", string('A'+len(headers)-1)+"1", headerStyle)

	// Write matching emails
	row := 2
	for _, entry := range matching {
		f.SetCellValue(resultsSheet, fmt.Sprintf("A%d", row), entry.Email)
		f.SetCellValue(resultsSheet, fmt.Sprintf("B%d", row), entry.NormalizedEmail)
		f.SetCellValue(resultsSheet, fmt.Sprintf("C%d", row), "Both")
		f.SetCellValue(resultsSheet, fmt.Sprintf("D%d", row), "Matching")
		f.SetCellValue(resultsSheet, fmt.Sprintf("E%d", row), fmtBool(entry.IsValid))
		f.SetCellValue(resultsSheet, fmt.Sprintf("I%d", row), entry.Reason)
		row++
	}

	// Write emails missing in first file
	for _, entry := range missingInFirst {
		f.SetCellValue(resultsSheet, fmt.Sprintf("A%d", row), entry.Email)
		f.SetCellValue(resultsSheet, fmt.Sprintf("B%d", row), entry.NormalizedEmail)
		f.SetCellValue(resultsSheet, fmt.Sprintf("C%d", row), "Second File Only")
		f.SetCellValue(resultsSheet, fmt.Sprintf("D%d", row), "Missing in First File")
		f.SetCellValue(resultsSheet, fmt.Sprintf("E%d", row), fmtBool(entry.IsValid))
		f.SetCellValue(resultsSheet, fmt.Sprintf("I%d", row), entry.Reason)
		row++
	}

	// Write emails missing in second file
	for _, entry := range missingInSecond {
		f.SetCellValue(resultsSheet, fmt.Sprintf("A%d", row), entry.Email)
		f.SetCellValue(resultsSheet, fmt.Sprintf("B%d", row), entry.NormalizedEmail)
		f.SetCellValue(resultsSheet, fmt.Sprintf("C%d", row), "First File Only")
		f.SetCellValue(resultsSheet, fmt.Sprintf("D%d", row), "Missing in Second File")
		f.SetCellValue(resultsSheet, fmt.Sprintf("E%d", row), fmtBool(entry.IsValid))
		f.SetCellValue(resultsSheet, fmt.Sprintf("I%d", row), entry.Reason)
		row++
	}

	// Create a summary sheet
	summarySheet := "Summary"
	_, err = f.NewSheet(summarySheet)
	if err != nil {
		return err
	}

	// Write summary headers
	f.SetCellValue(summarySheet, "A1", "Metric")
	f.SetCellValue(summarySheet, "B1", "Value")
	f.SetCellStyle(summarySheet, "A1", "B1", headerStyle)

	// Write summary data
	summaryData := [][]interface{}{
		{"Total Emails in First File", summary.TotalEmailsFirstFile},
		{"Total Emails in Second File", summary.TotalEmailsSecondFile},
		{"Valid Emails in First File", summary.ValidEmailsFirstFile},
		{"Valid Emails in Second File", summary.ValidEmailsSecondFile},
		{"Matching Emails", summary.MatchingCount},
		{"Emails Missing in First File", summary.MissingInFirstCount},
		{"Emails Missing in Second File", summary.MissingInSecondCount},
		{"Disposable Emails", summary.DisposableEmailsCount},
	}

	for i, row := range summaryData {
		f.SetCellValue(summarySheet, fmt.Sprintf("A%d", i+2), row[0])
		f.SetCellValue(summarySheet, fmt.Sprintf("B%d", i+2), row[1])
	}

	// Auto-fit columns in both sheets
	for _, col := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"} {
		f.SetColWidth(resultsSheet, col, col, 20)
	}

	f.SetColWidth(summarySheet, "A", "A", 30)
	f.SetColWidth(summarySheet, "B", "B", 15)

	// Delete default sheet
	f.DeleteSheet("Sheet1")

	// Save the file
	if err := f.SaveAs(outputPath); err != nil {
		return err
	}

	return nil
}
