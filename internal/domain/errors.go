package domain

import (
	"errors"
	"fmt"
)

var (
	ErrInvalid      = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrExpired      = errors.New("expired")
	ErrCanceled     = errors.New("operation canceled")
)

type FieldError struct {
	Field  string
	Reason string
}

func (e *FieldError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Reason) }
func (e *FieldError) Unwrap() error { return ErrInvalid }

type StateError struct {
	Entity string
	From   string
	To     string
	Reason string
}

func (e *StateError) Error() string {
	return fmt.Sprintf("%s cannot transition from %s to %s: %s", e.Entity, e.From, e.To, e.Reason)
}
func (e *StateError) Unwrap() error { return ErrConflict }

type VersionConflict struct {
	Entity  string
	ID      string
	Version int64
}

func (e *VersionConflict) Error() string {
	return fmt.Sprintf("%s %s no longer has version %d", e.Entity, e.ID, e.Version)
}
func (e *VersionConflict) Unwrap() error { return ErrConflict }

func Required(field, value string) error {
	if value == "" {
		return &FieldError{Field: field, Reason: "is required"}
	}
	return nil
}
