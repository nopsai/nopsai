package credentials

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	Scheme           = "credential"
	DefaultNamespace = "system"
)

var (
	ErrInvalidReference = errors.New("invalid credential reference")
	ErrNotFound         = errors.New("credential not found")
	ErrUnavailable      = errors.New("credential unavailable")
	ErrDisabled         = errors.New("credential disabled")
	ErrExpired          = errors.New("credential expired")
	ErrActiveVersion    = errors.New("active credential version cannot be deleted")
	ErrLastVersion      = errors.New("credential must retain at least one version")

	credentialSegmentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
)

type Reference struct {
	Namespace string
	Name      string
}

func ParseReference(raw string) (Reference, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(parsed.Scheme, Scheme) || parsed.Host == "" {
		return Reference{}, fmt.Errorf("%w: expected credential://namespace/name", ErrInvalidReference)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return Reference{}, fmt.Errorf("%w: query, fragment, and user information are not allowed", ErrInvalidReference)
	}
	namespace := strings.ToLower(strings.TrimSpace(parsed.Host))
	name := strings.Trim(strings.ToLower(strings.TrimSpace(parsed.Path)), "/")
	if err := validatePath(namespace, false); err != nil {
		return Reference{}, fmt.Errorf("%w: namespace: %v", ErrInvalidReference, err)
	}
	if err := validatePath(name, true); err != nil {
		return Reference{}, fmt.Errorf("%w: name: %v", ErrInvalidReference, err)
	}
	return Reference{Namespace: namespace, Name: name}, nil
}

func NewReference(namespace, name string) (Reference, error) {
	namespace = strings.ToLower(strings.TrimSpace(namespace))
	if namespace == "" {
		namespace = DefaultNamespace
	}
	name = strings.Trim(strings.ToLower(strings.TrimSpace(name)), "/")
	return ParseReference(Scheme + "://" + namespace + "/" + name)
}

func (r Reference) String() string {
	if r.Namespace == "" || r.Name == "" {
		return ""
	}
	return Scheme + "://" + r.Namespace + "/" + r.Name
}

func (r Reference) ResourceID() string {
	return strings.TrimSpace(r.Namespace) + "/" + strings.Trim(strings.TrimSpace(r.Name), "/")
}

func validatePath(value string, allowSlash bool) error {
	if value == "" {
		return errors.New("value is required")
	}
	parts := []string{value}
	if allowSlash {
		parts = strings.Split(value, "/")
	}
	for _, part := range parts {
		if !credentialSegmentPattern.MatchString(part) {
			return fmt.Errorf("segment %q must use lowercase letters, numbers, dots, underscores, or hyphens", part)
		}
	}
	return nil
}

type Purpose struct {
	ConsumerService string
	Operation       string
	SubjectType     string
	SubjectID       string
	CorrelationID   string
}

func (p Purpose) Normalize() (Purpose, error) {
	p.ConsumerService = strings.ToLower(strings.TrimSpace(p.ConsumerService))
	p.Operation = strings.ToLower(strings.TrimSpace(p.Operation))
	p.SubjectType = strings.ToLower(strings.TrimSpace(p.SubjectType))
	p.SubjectID = strings.TrimSpace(p.SubjectID)
	p.CorrelationID = strings.TrimSpace(p.CorrelationID)
	if p.ConsumerService == "" {
		return Purpose{}, errors.New("credential consumer service is required")
	}
	if p.Operation == "" {
		return Purpose{}, errors.New("credential operation is required")
	}
	return p, nil
}

type Value struct {
	CredentialID uuid.UUID
	Version      int
	bytes        []byte
}

func NewValue(credentialID uuid.UUID, version int, plaintext []byte) Value {
	return Value{
		CredentialID: credentialID,
		Version:      version,
		bytes:        append([]byte(nil), plaintext...),
	}
}

func (v Value) Bytes() []byte {
	return append([]byte(nil), v.bytes...)
}

func (v Value) Text() string {
	return string(v.bytes)
}

type Resolver interface {
	Resolve(ctx context.Context, ref Reference, purpose Purpose) (Value, error)
}
