package main

import (
	"fmt"

	"github.com/open-code-review/open-code-review/internal/reviewbundle"
)

type validationFailedError struct{}

func (validationFailedError) Error() string {
	return "comment validation failed"
}

type invalidValidationReportError struct{}

func (invalidValidationReportError) Error() string {
	return "validation result is invalid"
}

func requireValidationReport(path string) (*reviewbundle.ValidationResult, error) {
	if path == "" {
		return nil, fmt.Errorf("--validation is required; run validate-comments first")
	}
	result, err := loadValidationResult(path)
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Valid {
		return result, invalidValidationReportError{}
	}
	return result, nil
}
