// Package errors provides standardized error types and codes for the application.
package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode represents a standardized error code.
type ErrorCode string

// Standard error codes as defined in PRD.
const (
	// 4xx Client Errors
	CodeWalletNotFound         ErrorCode = "WALLET_NOT_FOUND"
	CodeWalletAlreadyExists    ErrorCode = "WALLET_ALREADY_EXISTS"
	CodeWalletFrozen           ErrorCode = "WALLET_FROZEN"
	CodeInsufficientFunds      ErrorCode = "INSUFFICIENT_FUNDS"
	CodeDuplicateTransfer      ErrorCode = "DUPLICATE_TRANSFER"
	CodeTransferNotFound       ErrorCode = "TRANSFER_NOT_FOUND"
	CodeInvalidAmount          ErrorCode = "INVALID_AMOUNT"
	CodeInvalidCurrency        ErrorCode = "INVALID_CURRENCY"
	CodeConcurrentModification ErrorCode = "CONCURRENT_MODIFICATION"
	CodeUnauthorized           ErrorCode = "UNAUTHORIZED"
	CodeForbidden              ErrorCode = "FORBIDDEN"
	CodeConflict               ErrorCode = "CONFLICT"
	CodeValidationFailed       ErrorCode = "VALIDATION_FAILED"
	CodeRateLimitExceeded      ErrorCode = "RATE_LIMIT_EXCEEDED"

	// 5xx Server Errors
	CodeServiceUnavailable ErrorCode = "SERVICE_UNAVAILABLE"
	CodeInternalError      ErrorCode = "INTERNAL_ERROR"
	CodeKafkaError         ErrorCode = "KAFKA_ERROR"
	CodeDatabaseError      ErrorCode = "DATABASE_ERROR"
)

// httpStatusMap maps error codes to HTTP status codes.
var httpStatusMap = map[ErrorCode]int{
	CodeWalletNotFound:         http.StatusNotFound,
	CodeWalletAlreadyExists:    http.StatusConflict,
	CodeWalletFrozen:           http.StatusForbidden,
	CodeInsufficientFunds:      http.StatusBadRequest,
	CodeDuplicateTransfer:      http.StatusConflict,
	CodeTransferNotFound:       http.StatusNotFound,
	CodeInvalidAmount:          http.StatusBadRequest,
	CodeInvalidCurrency:        http.StatusBadRequest,
	CodeConcurrentModification: http.StatusConflict,
	CodeUnauthorized:           http.StatusUnauthorized,
	CodeForbidden:              http.StatusForbidden,
	CodeConflict:               http.StatusConflict,
	CodeValidationFailed:       http.StatusBadRequest,
	CodeRateLimitExceeded:      http.StatusTooManyRequests,
	CodeServiceUnavailable:     http.StatusServiceUnavailable,
	CodeInternalError:          http.StatusInternalServerError,
	CodeKafkaError:             http.StatusInternalServerError,
	CodeDatabaseError:          http.StatusInternalServerError,
}

// AppError represents a structured application error.
type AppError struct {
	Code      ErrorCode              `json:"code"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	Err       error                  `json:"-"` // Internal error, not serialized
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error.
func (e *AppError) Unwrap() error {
	return e.Err
}

// HTTPStatus returns the appropriate HTTP status code for this error.
func (e *AppError) HTTPStatus() int {
	if status, ok := httpStatusMap[e.Code]; ok {
		return status
	}
	return http.StatusInternalServerError
}

// WithDetails adds details to the error.
func (e *AppError) WithDetails(key string, value interface{}) *AppError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// WithRequestID adds a request ID to the error.
func (e *AppError) WithRequestID(requestID string) *AppError {
	e.RequestID = requestID
	return e
}

// New creates a new AppError.
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an existing error with an AppError.
func Wrap(code ErrorCode, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Convenience constructors for common errors

func WalletNotFound(walletID string) *AppError {
	return New(CodeWalletNotFound, "Wallet not found").
		WithDetails("wallet_id", walletID)
}

func WalletAlreadyExists(msg string) *AppError {
	return New(CodeWalletAlreadyExists, msg)
}

func WalletFrozen(walletID string) *AppError {
	return New(CodeWalletFrozen, "Wallet is frozen or suspended").
		WithDetails("wallet_id", walletID)
}

func InsufficientFunds(walletID string, required, available string) *AppError {
	return New(CodeInsufficientFunds, "Insufficient funds for this operation").
		WithDetails("wallet_id", walletID).
		WithDetails("required", required).
		WithDetails("available", available)
}

func DuplicateTransfer(idempotencyKey string) *AppError {
	return New(CodeDuplicateTransfer, "Transfer with this idempotency key already exists").
		WithDetails("idempotency_key", idempotencyKey)
}

func TransferNotFound(transferID string) *AppError {
	return New(CodeTransferNotFound, "Transfer not found").
		WithDetails("transfer_id", transferID)
}

func InvalidAmount(amount string) *AppError {
	return New(CodeInvalidAmount, "Amount must be positive").
		WithDetails("amount", amount)
}

func ConcurrentModification(resource string) *AppError {
	return New(CodeConcurrentModification, "Resource was modified by another request, please retry").
		WithDetails("resource", resource)
}

func Conflict(msg string) *AppError {
	return New(CodeConflict, msg)
}

func InternalError(err error) *AppError {
	return Wrap(CodeInternalError, "An internal error occurred", err)
}

func DatabaseError(err error) *AppError {
	return Wrap(CodeDatabaseError, "Database operation failed", err)
}

func KafkaError(err error) *AppError {
	return Wrap(CodeKafkaError, "Kafka operation failed", err)
}

// ErrorResponse is the JSON structure returned to clients.
type ErrorResponse struct {
	Error *AppError `json:"error"`
}
