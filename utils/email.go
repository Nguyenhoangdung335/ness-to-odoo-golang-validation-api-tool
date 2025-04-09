package utils

import (
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Regular expression for validating email addresses
// This is a more comprehensive regex that follows RFC 5322 standards
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9.!#$%&'*+/=?^_\x60{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

// Common disposable email domains
var disposableDomains = map[string]bool{
	"mailinator.com":    true,
	"tempmail.com":      true,
	"temp-mail.org":     true,
	"guerrillamail.com": true,
	"10minutemail.com":  true,
	"yopmail.com":       true,
	"sharklasers.com":   true,
	"throwawaymail.com": true,
	"dispostable.com":   true,
	"mailnesia.com":     true,
	"mailcatch.com":     true,
	"trashmail.com":     true,
	"getnada.com":       true,
	"temp-mail.ru":      true,
	"fakeinbox.com":     true,
	"tempinbox.com":     true,
	"emailfake.com":     true,
}

// Cache for domain validation results
var (
	domainCache         = NewCache()
	domainCacheTTL      = 24 * time.Hour // Cache domain validation results for 24 hours
	emailValidationPool sync.Pool
)

// EmailValidationResult contains detailed validation results for an email
type EmailValidationResult struct {
	Email           string `json:"email"`
	IsValid         bool   `json:"isValid"`
	IsDisposable    bool   `json:"isDisposable"`
	NormalizedEmail string `json:"normalizedEmail"`
	Reason          string `json:"reason,omitempty"`
}

// IsValidEmail checks if a string is a valid email address
func IsValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	return emailRegex.MatchString(email)
}

// Initialize the email validation pool
func init() {
	emailValidationPool = sync.Pool{
		New: func() interface{} {
			return &EmailValidationResult{}
		},
	}
}

// ValidateEmailDetailed performs a simplified validation of an email address
// This version uses object pooling for better performance and skips MX record checks
func ValidateEmailDetailed(email string) EmailValidationResult {
	defer LogExecutionTime("ValidateEmailDetailed")()
	email = strings.TrimSpace(email)

	// Get a result object from the pool
	resultPtr := emailValidationPool.Get().(*EmailValidationResult)
	defer emailValidationPool.Put(resultPtr) // Return to pool when done

	// Reset the result object
	*resultPtr = EmailValidationResult{
		Email:           email,
		IsValid:         false,
		IsDisposable:    false,
		NormalizedEmail: NormalizeEmail(email),
	}

	result := *resultPtr // Work with a copy to avoid modifying the pooled object

	// Fast path for empty emails
	if email == "" {
		result.Reason = "Email cannot be empty"
		return result
	}

	// Basic check - just verify it contains @ symbol
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		result.Reason = "Email must contain exactly one @ symbol"
		return result
	}

	//domain := parts[1]

	// Check if domain is disposable - this is a fast map lookup
	//if isDisposableDomain(domain) {
	//	result.IsDisposable = true
	//	GetLogger().Debug("Email %s has disposable domain %s", email, domain)
	//	// We don't set result.Reason here because disposable emails are still valid
	//}

	// All emails are considered valid as long as they have an @ symbol
	result.IsValid = true
	GetLogger().Debug("Email %s validated successfully", email)
	return result
}

// ValidateEmailsBatch validates multiple emails concurrently for better performance
func ValidateEmailsBatch(emails []string) []EmailValidationResult {
	defer LogExecutionTime("ValidateEmailsBatch")()
	logger := GetLogger()
	logger.Info("Starting batch validation of %d emails", len(emails))

	results := make([]EmailValidationResult, len(emails))

	// Use a worker pool to process emails concurrently
	workerCount := min(len(emails), 10) // Limit to 10 workers max
	logger.Debug("Using %d workers for email validation", workerCount)

	jobs := make(chan int, len(emails))
	wg := sync.WaitGroup{}

	// Start workers
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			logger.Debug("Worker %d started", workerID)
			processedCount := 0

			for idx := range jobs {
				results[idx] = ValidateEmailDetailed(emails[idx])
				processedCount++
			}

			logger.Debug("Worker %d finished, processed %d emails", workerID, processedCount)
		}(w)
	}

	// Send jobs to workers
	logger.Debug("Sending %d jobs to worker pool", len(emails))
	for i := range emails {
		jobs <- i
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()

	// Count validation results
	validCount := 0
	disposableCount := 0
	for _, result := range results {
		if result.IsValid {
			validCount++
		}
		if result.IsDisposable {
			disposableCount++
		}
	}

	logger.Info("Batch validation completed: %d/%d valid, %d disposable",
		validCount, len(emails), disposableCount)

	return results
}

// NormalizeEmail normalizes an email address by trimming spaces and converting to lowercase
func NormalizeEmail(email string) string {
	email = strings.TrimSpace(email)
	email = strings.ToLower(email)

	// Handle Gmail's dot-ignoring feature
	parts := strings.Split(email, "@")
	if len(parts) == 2 && parts[1] == "gmail.com" {
		// Remove dots from username part for Gmail
		username := strings.Replace(parts[0], ".", "", -1)
		// Remove anything after + in username
		if plusIndex := strings.Index(username, "+"); plusIndex > 0 {
			username = username[:plusIndex]
		}
		return username + "@gmail.com"
	}

	return email
}

// isDisposableDomain checks if a domain is a known disposable email domain
func isDisposableDomain(domain string) bool {
	domain = strings.ToLower(domain)
	return disposableDomains[domain]
}

// hasMXRecordCached checks if a domain has valid MX records using cache for performance
func hasMXRecordCached(domain string) bool {
	// Check cache first
	if cachedResult, found := domainCache.Get(domain); found {
		return cachedResult.(bool)
	}

	// Not in cache, perform the actual check
	result := hasMXRecord(domain)

	// Store in cache
	domainCache.Set(domain, result, domainCacheTTL)

	return result
}

// hasMXRecord checks if a domain has valid MX records
func hasMXRecord(domain string) bool {
	// Skip actual MX lookup during development to avoid network calls
	// In production, uncomment the code below
	/*
		mxRecords, err := net.LookupMX(domain)
		if err != nil || len(mxRecords) == 0 {
			return false
		}
		return true
	*/

	// For now, just check if the domain looks valid
	_, err := net.LookupHost(domain)
	return err == nil
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
