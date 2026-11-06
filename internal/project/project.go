// Package project obtains filesystem identity and read-only Git context. It never reads source.
package project

import (
	"bytes"
	"context"
	"errors"
	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/localpath"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

type Error = domain.Error

func fail(code, message string) error { return &Error{Code: code, Message: message} }

type Observation struct {
	ResolvedPath   string  `json:"resolvedPath"`
	RepositoryRoot string  `json:"repositoryRoot"`
	Branch         *string `json:"branch"`
	Commit         *string `json:"commit"`
	ObservedAt     string  `json:"observedAt"`
}
type Inspection struct {
	Observation Observation
	Identity    string
}

func validPath(path string) bool {
	return path != "" && utf8.ValidString(path) && !strings.ContainsRune(path, 0) && platformPathValid(path)
}

// Inspect resolves against the explicit caller base; it never expands shell syntax or ~.
func Inspect(ctx context.Context, selectedPath, basePath string) (Inspection, error) {
	var result Inspection
	if !validPath(selectedPath) || !validPath(basePath) || !filepath.IsAbs(basePath) {
		return result, fail("invalid_request", "Select a losslessly representable folder and an absolute base path.")
	}
	path := selectedPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(basePath, path)
	}
	path = filepath.Clean(path)
	resolved, err := localpath.Resolve(path)
	if err != nil {
		return result, fail("project_unavailable", "The selected folder is missing or inaccessible. Select an accessible local Git project.")
	}
	i, err := os.Stat(resolved)
	if err != nil || !i.IsDir() {
		return result, fail("project_unavailable", "Select an accessible existing local folder.")
	}
	identity, err := physicalIdentity(resolved)
	if err != nil {
		return result, fail("project_unavailable", "Cannot establish the selected folder's physical identity.")
	}
	gitName := "git"
	if runtime.GOOS == "windows" {
		gitName = "git.exe"
	}
	gitPath, err := exec.LookPath(gitName)
	if err != nil {
		return result, fail("prerequisite_missing", "Install Git and make its native executable available on PATH.")
	}
	inside, err := gitMetadata(ctx, gitPath, resolved, "rev-parse", "--is-inside-work-tree")
	if err != nil || inside != "true" {
		return result, fail("not_git_project", "Select a folder inside a non-bare Git worktree.")
	}
	repo, err := gitMetadata(ctx, gitPath, resolved, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil || !validPath(repo) {
		return result, fail("project_unavailable", "Cannot inspect the Git worktree root.")
	}
	repo, err = localpath.Resolve(filepath.FromSlash(repo))
	if err != nil {
		return result, fail("project_unavailable", "Cannot resolve the Git worktree root.")
	}
	branchText, branchErr := gitMetadata(ctx, gitPath, resolved, "symbolic-ref", "--quiet", "--short", "HEAD")
	var branch *string
	if branchErr == nil {
		if branchText == "" {
			return result, fail("project_unavailable", "Cannot inspect the Git branch.")
		}
		branch = &branchText
	} else if !exitStatus(branchErr, 1) {
		return result, fail("project_unavailable", "Cannot inspect the Git branch.")
	}
	commitText, commitErr := gitMetadata(ctx, gitPath, resolved, "rev-parse", "--verify", "--quiet", "HEAD")
	var commit *string
	if commitErr == nil {
		if !isObjectID(commitText) {
			return result, fail("project_unavailable", "Git returned an invalid commit identity.")
		}
		commit = &commitText
	} else if branch == nil || !exitStatus(commitErr, 1) {
		return result, fail("project_unavailable", "Cannot inspect the Git commit.")
	}
	if err := ValidateIdentity(resolved, identity); err != nil {
		return result, err
	}
	result = Inspection{Observation: Observation{ResolvedPath: resolved, RepositoryRoot: repo, Branch: branch, Commit: commit, ObservedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}, Identity: identity}
	return result, nil
}

// ValidateIdentity rejects a replaced location before its retained drafts can be attached.
func ValidateIdentity(path, evidence string) error {
	if !validPath(path) || !filepath.IsAbs(path) {
		return fail("project_unavailable", "The saved project folder is unavailable.")
	}
	identity, err := physicalIdentity(path)
	if err != nil {
		return fail("project_unavailable", "The saved project folder is missing or inaccessible.")
	}
	if evidence == "" || identity != evidence {
		return fail("project_identity_changed", "The saved project location now identifies a different physical folder. Restore the original folder before reopening it.")
	}
	return nil
}

func cleanGitEnvironment() []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
}

type limitedOutput struct {
	bytes.Buffer
	limit int
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("Git metadata exceeds its bound")
	}
	return b.Buffer.Write(p)
}
func gitMetadata(ctx context.Context, git, path string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	all := append([]string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-C", path}, args...)
	c := exec.CommandContext(ctx, git, all...)
	configureGitCommand(c)
	c.Env = cleanGitEnvironment()
	out := &limitedOutput{limit: 1024 * 1024}
	c.Stdout = out
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		return "", err
	}
	text := out.String()
	if strings.HasSuffix(text, "\n") {
		text = strings.TrimSuffix(text, "\n")
		if runtime.GOOS == "windows" {
			text = strings.TrimSuffix(text, "\r")
		}
	}
	if !utf8.ValidString(text) {
		return "", errors.New("Git metadata is not valid UTF-8")
	}
	return text, nil
}
func exitStatus(err error, status int) bool {
	var e *exec.ExitError
	return errors.As(err, &e) && e.ExitCode() == status
}
func isObjectID(s string) bool {
	if len(s) < 40 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
