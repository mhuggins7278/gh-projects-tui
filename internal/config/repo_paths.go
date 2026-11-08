package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var repoPartPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func validRepoPart(part string) bool {
	return part != "." && part != ".." && repoPartPattern.MatchString(part)
}

func validateRepoName(name string, wildcard bool) bool {
	parts := strings.Split(name, "/")
	return len(parts) == 2 && validRepoPart(parts[0]) && (validRepoPart(parts[1]) || (wildcard && parts[1] == "*"))
}

func validConfiguredPath(path string) bool {
	return filepath.IsAbs(path) || path == "~" || strings.HasPrefix(path, "~/") || (filepath.Separator == '\\' && strings.HasPrefix(path, `~\`))
}

// ValidateRepoPaths supports exact owner/repository and owner/* mappings.
// Wildcard destinations contain one '*' which is replaced by the repository name.
func ValidateRepoPaths(paths map[string]string, roots []string) error {
	seen := map[string]bool{}
	for key, path := range paths {
		if !validateRepoName(key, true) {
			return fmt.Errorf("repoPaths: invalid repository pattern %q", key)
		}
		lower := strings.ToLower(key)
		if seen[lower] {
			return fmt.Errorf("repoPaths: duplicate pattern %q", key)
		}
		seen[lower] = true
		stars := strings.Count(path, "*")
		if !validConfiguredPath(path) || (strings.HasSuffix(key, "/*") && stars != 1) || (!strings.HasSuffix(key, "/*") && stars != 0) {
			return fmt.Errorf("repoPaths: %q must map to an absolute or ~/ path with a matching wildcard", key)
		}
	}
	for _, root := range roots {
		if !validConfiguredPath(root) || strings.Contains(root, "*") {
			return fmt.Errorf("repoSearchRoots: %q must be an absolute or ~/ directory", root)
		}
	}
	return nil
}

func expandRepoPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") || (filepath.Separator == '\\' && strings.HasPrefix(path, `~\`)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute: %s", path)
	}
	return filepath.Clean(path), nil
}

// ResolveRepoPath uses explicit mappings, then the current checkout, then
// repositories directly under configured roots or roots/owner. A missing
// mapped path is returned so a user action may create or clone it.
func ResolveRepoPath(ctx context.Context, host, repo, cwd string, paths map[string]string, roots []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !validateRepoName(repo, false) {
		return "", fmt.Errorf("invalid repository name %q", repo)
	}
	if strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("GitHub host is unavailable")
	}
	if err := ValidateRepoPaths(paths, roots); err != nil {
		return "", err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required to resolve repository paths: %w", err)
	}
	owner, name, _ := strings.Cut(repo, "/")
	var exact, wildcard string
	for pattern, path := range paths {
		if strings.EqualFold(pattern, repo) {
			exact = path
		}
		if strings.EqualFold(pattern, owner+"/*") {
			wildcard = path
		}
	}
	mapped := exact
	if mapped == "" && wildcard != "" {
		mapped = strings.Replace(wildcard, "*", name, 1)
	}
	if mapped != "" {
		path, err := expandRepoPath(mapped)
		if err != nil {
			return "", err
		}
		_, err = os.Stat(path)
		if os.IsNotExist(err) {
			return path, nil
		}
		if err != nil {
			return "", fmt.Errorf("inspect mapped path %s: %w", path, err)
		}
		root, match, err := matchingCheckout(ctx, path, host, repo)
		if err != nil {
			return "", err
		}
		if !match {
			return "", fmt.Errorf("mapped path %s is not a checkout with a remote for %s on %s", path, repo, host)
		}
		return root, nil
	}
	if cwd != "" {
		root, match, err := matchingCheckout(ctx, cwd, host, repo)
		if err != nil {
			return "", err
		}
		if match {
			return root, nil
		}
	}
	matches := map[string]string{}
	for _, configured := range roots {
		root, err := expandRepoPath(configured)
		if err != nil {
			return "", err
		}
		candidates, err := searchRootCandidates(root, owner)
		if err != nil {
			return "", err
		}
		for _, candidate := range candidates {
			resolved, match, err := matchingCheckout(ctx, candidate, host, repo)
			if err != nil {
				return "", err
			}
			if match {
				identity, err := filepath.EvalSymlinks(resolved)
				if err != nil {
					identity = resolved
				}
				matches[identity] = resolved
			}
		}
	}
	if len(matches) == 1 {
		for _, path := range matches {
			return path, nil
		}
	}
	if len(matches) > 1 {
		var options []string
		for _, path := range matches {
			options = append(options, path)
		}
		sort.Strings(options)
		return "", fmt.Errorf("multiple checkouts match %s: %s; add an exact repoPaths entry", repo, strings.Join(options, ", "))
	}
	return "", fmt.Errorf("no checkout found for %s on %s; add a repoPaths mapping or repoSearchRoots", repo, host)
}

func searchRootCandidates(root, owner string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read repository search root %s: %w", root, err)
	}
	var result []string
	addCheckout := func(path string) {
		// Normal clones have a directory here; linked worktrees have a file.
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			result = append(result, path)
		}
	}
	addCheckout(root)
	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		addCheckout(path)
		if strings.EqualFold(entry.Name(), owner) {
			children, err := os.ReadDir(path)
			if err != nil {
				return nil, fmt.Errorf("read repository owner directory %s: %w", path, err)
			}
			for _, child := range children {
				childPath := filepath.Join(path, child.Name())
				info, err := os.Stat(childPath)
				if err == nil && info.IsDir() {
					addCheckout(childPath)
				}
			}
		}
	}
	return result, nil
}

func matchingCheckout(ctx context.Context, path, host, repo string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return "", false, nil
	}
	command := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--show-toplevel")
	data, err := command.Output()
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	if err != nil {
		return "", false, nil
	}
	root := strings.TrimSuffix(string(data), "\n")
	root = strings.TrimSuffix(root, "\r")
	if root == "" {
		return "", false, nil
	}
	command = exec.CommandContext(ctx, "git", "-C", root, "remote", "-v")
	data, err = command.Output()
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	if err != nil {
		return "", false, nil
	}
	checkedSSH := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		remote := parseRepoRemote(fields[1])
		if !strings.EqualFold(remote.repo, repo) {
			continue
		}
		if strings.EqualFold(remote.host, host) {
			return root, true, nil
		}
		if remote.sshDestination == "" || checkedSSH[remote.sshDestination] {
			continue
		}
		checkedSSH[remote.sshDestination] = true
		resolvedHost := sshConfiguredHost(ctx, remote.sshDestination)
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		if strings.EqualFold(resolvedHost, host) {
			return root, true, nil
		}
	}
	return "", false, nil
}

type repoRemote struct {
	host, repo, sshDestination string
}

func parseRepoRemote(raw string) repoRemote {
	var host, path, destination string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return repoRemote{}
		}
		host, path = u.Host, strings.TrimPrefix(u.Path, "/")
		if u.Scheme == "ssh" && validRepoPart(u.Hostname()) {
			user := u.User.Username()
			if user == "" || validRepoPart(user) {
				target := url.URL{Scheme: "ssh", Host: u.Host}
				if user != "" {
					target.User = url.User(user)
				}
				destination = target.String()
			}
		}
	} else {
		left, right, ok := strings.Cut(raw, ":")
		if !ok {
			return repoRemote{}
		}
		var user string
		host, path = left, right
		if index := strings.LastIndex(left, "@"); index >= 0 {
			user, host = left[:index], left[index+1:]
		}
		if validRepoPart(host) && (user == "" || validRepoPart(user)) {
			destination = left
		}
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	if !validateRepoName(path, false) {
		return repoRemote{}
	}
	return repoRemote{host: host, repo: path, sshDestination: destination}
}

func sshConfiguredHost(ctx context.Context, destination string) string {
	// -G prints the effective SSH configuration without connecting.
	data, err := exec.CommandContext(ctx, "ssh", "-G", "--", destination).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "hostname" {
			return fields[1]
		}
	}
	return ""
}
