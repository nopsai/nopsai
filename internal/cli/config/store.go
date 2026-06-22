package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	currentVersion      = 1
	configFileName      = "config.yaml"
	credentialsFileName = "credentials.yaml"
)

var contextNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

type Context struct {
	API string `yaml:"api" json:"api"`
}

type File struct {
	Version        int                `yaml:"version" json:"version"`
	CurrentContext string             `yaml:"current_context,omitempty" json:"current_context,omitempty"`
	Contexts       map[string]Context `yaml:"contexts" json:"contexts"`
}

type credentialsFile struct {
	Version int               `yaml:"version"`
	Tokens  map[string]string `yaml:"tokens"`
}

type Store struct {
	dir string
}

func DefaultDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("NOPSAI_CONFIG_DIR")); configured != "" {
		return filepath.Abs(configured)
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(root, "nopsai"), nil
}

func NewStore(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		var err error
		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve config directory: %w", err)
	}
	return &Store{dir: abs}, nil
}

func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) ConfigPath() string {
	return filepath.Join(s.dir, configFileName)
}

func (s *Store) CredentialsPath() string {
	return filepath.Join(s.dir, credentialsFileName)
}

func (s *Store) Load() (File, error) {
	var cfg File
	found, err := readYAML(s.ConfigPath(), false, &cfg)
	if err != nil {
		return File{}, err
	}
	if !found {
		return emptyFile(), nil
	}
	if err := validateFile(&cfg); err != nil {
		return File{}, fmt.Errorf("validate %s: %w", s.ConfigPath(), err)
	}
	return cfg, nil
}

func (s *Store) AddContext(name, api string) (Context, error) {
	if err := ValidateContextName(name); err != nil {
		return Context{}, err
	}
	normalizedAPI, err := NormalizeAPIURL(api)
	if err != nil {
		return Context{}, err
	}
	cfg, err := s.Load()
	if err != nil {
		return Context{}, err
	}
	if existing, ok := cfg.Contexts[name]; ok && existing.API != normalizedAPI {
		// Remove the old credential first so a failed config write cannot leave
		// it attached to a different API endpoint.
		if err := s.DeleteToken(name); err != nil {
			return Context{}, fmt.Errorf("remove credential for changed context: %w", err)
		}
	}
	ctx := Context{API: normalizedAPI}
	cfg.Contexts[name] = ctx
	if cfg.CurrentContext == "" {
		cfg.CurrentContext = name
	}
	if err := s.saveConfig(cfg); err != nil {
		return Context{}, err
	}
	return ctx, nil
}

func (s *Store) UseContext(name string) error {
	if err := ValidateContextName(name); err != nil {
		return err
	}
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context %q does not exist", name)
	}
	cfg.CurrentContext = name
	return s.saveConfig(cfg)
}

func (s *Store) DeleteContext(name string) error {
	if err := ValidateContextName(name); err != nil {
		return err
	}
	cfg, err := s.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Contexts[name]; !ok {
		return fmt.Errorf("context %q does not exist", name)
	}
	delete(cfg.Contexts, name)
	if cfg.CurrentContext == name {
		cfg.CurrentContext = ""
	}
	if err := s.saveConfig(cfg); err != nil {
		return err
	}
	return s.DeleteToken(name)
}

func (s *Store) ResolveContext(name string) (string, Context, error) {
	cfg, err := s.Load()
	if err != nil {
		return "", Context{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = cfg.CurrentContext
	}
	if name == "" {
		return "", Context{}, errors.New("no current context; run `nopsai context add NAME --api URL`")
	}
	ctx, ok := cfg.Contexts[name]
	if !ok {
		return "", Context{}, fmt.Errorf("context %q does not exist", name)
	}
	return name, ctx, nil
}

func (s *Store) SaveToken(contextName, token string) error {
	if err := ValidateContextName(contextName); err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token cannot be empty")
	}
	if strings.ContainsAny(token, "\r\n") {
		return errors.New("token cannot contain a newline")
	}
	credentials, err := s.loadCredentials()
	if err != nil {
		return err
	}
	credentials.Tokens[contextName] = token
	return writeYAML(s.CredentialsPath(), credentials)
}

func (s *Store) Token(contextName string) (string, error) {
	if err := ValidateContextName(contextName); err != nil {
		return "", err
	}
	credentials, err := s.loadCredentials()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(credentials.Tokens[contextName]), nil
}

func (s *Store) DeleteToken(contextName string) error {
	credentials, err := s.loadCredentials()
	if err != nil {
		return err
	}
	if _, ok := credentials.Tokens[contextName]; !ok {
		return nil
	}
	delete(credentials.Tokens, contextName)
	return writeYAML(s.CredentialsPath(), credentials)
}

func ValidateContextName(name string) error {
	if !contextNamePattern.MatchString(strings.TrimSpace(name)) {
		return errors.New("context name must be 1-63 characters using letters, numbers, dot, underscore, or hyphen")
	}
	return nil
}

func NormalizeAPIURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse API URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("API URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("API URL must include a host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("API URL cannot include credentials, a query, or a fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return parsed.String(), nil
}

func (s *Store) saveConfig(cfg File) error {
	if err := validateFile(&cfg); err != nil {
		return err
	}
	return writeYAML(s.ConfigPath(), cfg)
}

func (s *Store) loadCredentials() (credentialsFile, error) {
	credentials := credentialsFile{Version: currentVersion, Tokens: map[string]string{}}
	found, err := readYAML(s.CredentialsPath(), true, &credentials)
	if err != nil {
		return credentialsFile{}, err
	}
	if !found {
		return credentials, nil
	}
	if credentials.Version != currentVersion {
		return credentialsFile{}, fmt.Errorf("unsupported credentials version %d", credentials.Version)
	}
	if credentials.Tokens == nil {
		credentials.Tokens = map[string]string{}
	}
	return credentials, nil
}

func emptyFile() File {
	return File{Version: currentVersion, Contexts: map[string]Context{}}
}

func validateFile(cfg *File) error {
	if cfg.Version != currentVersion {
		return fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if cfg.Contexts == nil {
		cfg.Contexts = map[string]Context{}
	}
	for name, ctx := range cfg.Contexts {
		if err := ValidateContextName(name); err != nil {
			return fmt.Errorf("context %q: %w", name, err)
		}
		normalized, err := NormalizeAPIURL(ctx.API)
		if err != nil {
			return fmt.Errorf("context %q: %w", name, err)
		}
		ctx.API = normalized
		cfg.Contexts[name] = ctx
	}
	if cfg.CurrentContext != "" {
		if _, ok := cfg.Contexts[cfg.CurrentContext]; !ok {
			return fmt.Errorf("current context %q does not exist", cfg.CurrentContext)
		}
	}
	return nil
}

func readYAML(path string, sensitive bool, target any) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 && !sensitive {
		info, err = os.Stat(path)
		if err != nil {
			return false, fmt.Errorf("inspect config target %s: %w", path, err)
		}
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s must be a regular file", path)
	}
	if sensitive && info.Mode().Perm()&0o077 != 0 {
		return false, fmt.Errorf("%s permissions are too broad; expected 0600", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return false, fmt.Errorf("decode %s: %w", path, err)
	}
	return true, nil
}

func writeYAML(path string, value any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil { // #nosec G302 -- directories require owner execute permission.
		return fmt.Errorf("secure config directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".nopsai-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("secure temporary config file: %w", err)
	}
	encoder := yaml.NewEncoder(temp)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		temp.Close()
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := encoder.Close(); err != nil {
		temp.Close()
		return fmt.Errorf("close YAML encoder: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure %s: %w", path, err)
	}
	return nil
}
