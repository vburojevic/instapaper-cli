package main

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/vburojevic/instapaper-cli/internal/instapaper"
)

const (
	ErrCodeUnknown         = "unknown"
	ErrCodeInvalidUsage    = "invalid_usage"
	ErrCodeRateLimited     = "rate_limited"
	ErrCodePremiumRequired = "premium_required"
	ErrCodeAppSuspended    = "app_suspended"
	ErrCodeInvalidRequest  = "invalid_request"
	ErrCodeServerError     = "server_error"
	ErrCodeAPIError        = "api_error"
	ErrCodeTimeout         = "timeout"
	ErrCodeNetwork         = "network_error"
	ErrCodeAuth            = "auth_error"
	ErrCodeConfig          = "config_error"
)

func errorCodeForError(err error) string {
	if err == nil {
		return ErrCodeUnknown
	}
	var apiErr *instapaper.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case 1040:
			return ErrCodeRateLimited
		case 1041:
			return ErrCodePremiumRequired
		case 1042:
			return ErrCodeAppSuspended
		case 1240, 1241, 1242, 1243, 1244, 1245, 1250, 1251, 1252, 1600, 1601, 1220, 1221:
			return ErrCodeInvalidRequest
		case 1500, 1550:
			return ErrCodeServerError
		default:
			return ErrCodeAPIError
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrCodeTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrCodeTimeout
		}
		return ErrCodeNetwork
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not logged in"):
		return ErrCodeAuth
	case strings.Contains(msg, "missing consumer key/secret"):
		return ErrCodeConfig
	case strings.Contains(msg, "parse config"):
		return ErrCodeConfig
	case strings.Contains(msg, "config "):
		return ErrCodeConfig
	}
	return ErrCodeUnknown
}
