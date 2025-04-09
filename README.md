# Enhanced Email Validation API

An API server that validates and compares emails from two different sources (CSV/Excel files) with advanced validation features.

## Features

- Upload two CSV/Excel files containing emails
- Advanced email validation including:
  - Format validation using RFC 5322 standards
  - Domain validation with MX record checking
  - Disposable email detection
  - Email normalization (e.g., handling Gmail's dot-ignoring feature)
- Detailed comparison of emails from both sources
- Generate comprehensive CSV/Excel reports with validation results
- Summary statistics of validation results
- Swagger documentation for easy API exploration

## API Endpoints

### Validate Emails

```
POST /api/v1/validate-emails
```

**Parameters:**
- `firstFile` (required): First CSV/Excel file containing emails
- `secondFile` (required): Second CSV/Excel file containing emails
- `outputFormat` (optional): Output format (csv or excel, default: csv)

**Response:**
```json
{
  "matchingEmails": ["email1@example.com", "email2@example.com"],
  "missingInFirstFile": ["email3@example.com"],
  "missingInSecondFile": ["email4@example.com"],
  "outputFileURL": "/api/v1/download/validation_result_20230101_120000.csv",
  "summary": {
    "totalEmailsFirstFile": 3,
    "totalEmailsSecondFile": 3,
    "validEmailsFirstFile": 3,
    "validEmailsSecondFile": 2,
    "matchingCount": 2,
    "missingInFirstCount": 1,
    "missingInSecondCount": 1,
    "disposableEmailsCount": 1
  }
}
```

### Download Result File

```
GET /api/v1/download/{filename}
```

**Parameters:**
- `filename` (required): Name of the file to download

**Response:**
- The file content with appropriate content type headers

## Getting Started

### Prerequisites

- Go 1.21 or higher

### Installation

1. Clone the repository
2. Install dependencies:
   ```
   go mod tidy
   ```
3. Run the server:
   ```
   go run main.go
   ```

### Swagger Documentation

Access the Swagger UI at:
```
http://localhost:8080/swagger/index.html
```

## File Format Requirements

- Supported file formats: CSV, Excel (.xlsx, .xls)
- The files should have emails in the first column
- The first row is assumed to be a header row

## Enhanced Validation Features

### Email Validation
- **Format Validation**: Validates email format according to RFC 5322 standards
- **Domain Validation**: Checks if the email domain has valid MX records
- **Disposable Email Detection**: Identifies emails from known disposable email providers
- **Email Normalization**: Normalizes emails for better comparison (e.g., handling Gmail's dot-ignoring feature)

### Performance Optimizations
- **Concurrent Processing**: Processes files and validates emails in parallel
- **Streaming File Processing**: Handles large files efficiently without loading everything into memory
- **Domain Validation Caching**: Caches domain validation results to avoid repeated network lookups
- **Object Pooling**: Reuses objects to reduce memory allocations and garbage collection
- **Pre-allocated Data Structures**: Reduces memory reallocations for better performance
- **Batch Processing**: Processes emails in batches for better throughput

### Comparison Logic
- **Normalized Comparison**: Uses normalized email addresses for more accurate matching
- **Detailed Categorization**:
  - Matching emails (present in both files)
  - Emails missing in the first file (present only in the second file)
  - Emails missing in the second file (present only in the first file)

### Output Report
The generated output file contains:
- Email address
- Normalized email address
- Source information
- Validation status
- Detailed validation results (format validity, domain validity, etc.)
- Reason for invalid emails
- Summary statistics

