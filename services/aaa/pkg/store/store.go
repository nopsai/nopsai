package store

import "errors"

var (
	ErrSubjectNotFound  = errors.New("subject not found")
	ErrSubjectInactive  = errors.New("subject inactive")
	ErrResourceNotFound = errors.New("resource not found")
)
