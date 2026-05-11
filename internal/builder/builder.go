package builder

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// Builder defines the interface for code fetching and project building.
type Builder interface {
	// CloneOrPull clones a repository if the workDir doesn't exist,
	// or fetches and checks out the specified branch if it does.
	CloneOrPull(repoURL, branch, workDir string) error

	// Build executes the given build command in workDir with BUILD_ENV set to env.
	// Returns the combined stdout/stderr output on success, or an error with output on failure.
	Build(workDir string, buildCmd string, env string) (string, error)
}

// builder is the concrete implementation of Builder using os/exec to run git and shell commands.
type builder struct{}

// NewBuilder creates a new Builder instance.
func NewBuilder() Builder {
	return &builder{}
}

// CloneOrPull clones the repository into workDir if it doesn't exist,
// or fetches and checks out the specified branch if workDir already exists.
func (b *builder) CloneOrPull(repoURL, branch, workDir string) error {
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		// Directory doesn't exist — clone the repo.
		return b.cloneRepo(repoURL, branch, workDir)
	}
	// Directory exists — fetch, checkout, and pull.
	return b.pullRepo(branch, workDir)
}

// cloneRepo runs git clone for the specified branch into workDir.
func (b *builder) cloneRepo(repoURL, branch, workDir string) error {
	cmd := exec.Command("git", "clone", "--branch", branch, repoURL, workDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %s: %w", stderr.String(), err)
	}
	return nil
}

// pullRepo fetches, checks out, and pulls the specified branch inside workDir.
func (b *builder) pullRepo(branch, workDir string) error {
	// Ensure the remote fetch refspec covers all branches (in case the repo
	// was originally cloned with --single-branch, which restricts the refspec).
	configCmd := exec.Command("git", "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	configCmd.Dir = workDir
	configCmd.Run() // ignore errors — best-effort fix for legacy single-branch clones

	// git clean -fd — remove untracked files/dirs left by previous builds
	cleanCmd := exec.Command("git", "clean", "-fd")
	cleanCmd.Dir = workDir
	cleanCmd.Run() // ignore errors — best-effort cleanup

	// git fetch origin — fetch all branches to ensure remote refs are up to date
	fetchCmd := exec.Command("git", "fetch", "origin")
	fetchCmd.Dir = workDir
	var fetchStderr bytes.Buffer
	fetchCmd.Stderr = &fetchStderr
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("git fetch failed: %s: %w", fetchStderr.String(), err)
	}

	// git checkout -f -B <branch> origin/<branch>
	// -f force-discards any local changes (e.g. build artifacts from previous runs).
	// -B creates the branch from origin/<branch> if it doesn't exist locally,
	// or resets it to match origin/<branch> if it does (safe for CI/CD).
	checkoutCmd := exec.Command("git", "checkout", "-f", "-B", branch, "origin/"+branch)
	checkoutCmd.Dir = workDir
	var checkoutStderr bytes.Buffer
	checkoutCmd.Stderr = &checkoutStderr
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout failed: %s: %w", checkoutStderr.String(), err)
	}

	// git pull origin <branch>
	pullCmd := exec.Command("git", "pull", "origin", branch)
	pullCmd.Dir = workDir
	var pullStderr bytes.Buffer
	pullCmd.Stderr = &pullStderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("git pull failed: %s: %w", pullStderr.String(), err)
	}

	return nil
}

// Build executes the build command in workDir with BUILD_ENV set to the given env value.
// It returns the combined output on success, or an error containing the output on failure.
func (b *builder) Build(workDir string, buildCmd string, env string) (string, error) {
	cmd := exec.Command("sh", "-c", buildCmd)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("BUILD_ENV=%s", env))

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build command failed: %s: %w", output.String(), err)
	}

	return output.String(), nil
}
