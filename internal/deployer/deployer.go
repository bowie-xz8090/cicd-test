package deployer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"auto-deploy-platform/internal/config"

	"golang.org/x/crypto/ssh"
)

// Deployer defines the interface for deploying build artifacts to target servers via SSH.
type Deployer interface {
	// Upload transfers a local file to the target server's deploy path via SFTP-like SCP.
	Upload(localPath string, server config.ServerConfig) error

	// Execute runs a script on the target server via SSH and returns the stdout output.
	Execute(server config.ServerConfig, script string) (string, error)
}

// deployer is the concrete implementation of Deployer using golang.org/x/crypto/ssh.
type deployer struct{}

// NewDeployer creates a new Deployer instance.
func NewDeployer() Deployer {
	return &deployer{}
}

// newSSHClient creates an SSH client connection to the target server using password authentication.
func newSSHClient(server config.ServerConfig) (*ssh.Client, error) {
	sshConfig := &ssh.ClientConfig{
		User: server.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(server.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH connection to %s failed: %w", addr, err)
	}
	return client, nil
}

// Upload transfers localPath to the target server's deploy path using SCP protocol over SSH.
func (d *deployer) Upload(localPath string, server config.ServerConfig) error {
	client, err := newSSHClient(server)
	if err != nil {
		return err
	}
	defer client.Close()

	// Read local file
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local file %s: %w", localPath, err)
	}

	fileData, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file %s: %w", localPath, err)
	}

	// Open SSH session for SCP
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Use SCP protocol to send the file
	remotePath := filepath.Join(server.DeployPath, filepath.Base(localPath))

	// Set up stdin pipe for SCP data
	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	var stderr bytes.Buffer
	session.Stderr = &stderr

	// Start SCP receive command on remote
	if err := session.Start(fmt.Sprintf("scp -t %s", server.DeployPath)); err != nil {
		return fmt.Errorf("failed to start remote scp: %w", err)
	}

	// Send SCP protocol header: C<mode> <size> <filename>
	fmt.Fprintf(stdin, "C0644 %d %s\n", fileInfo.Size(), filepath.Base(localPath))

	// Send file content
	if _, err := io.Copy(stdin, bytes.NewReader(fileData)); err != nil {
		return fmt.Errorf("failed to send file data: %w", err)
	}

	// Send SCP end-of-file marker
	fmt.Fprint(stdin, "\x00")
	stdin.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("scp upload to %s failed: %s: %w", remotePath, stderr.String(), err)
	}

	return nil
}

// Execute runs the given script on the target server via SSH and returns stdout.
func (d *deployer) Execute(server config.ServerConfig, script string) (string, error) {
	client, err := newSSHClient(server)
	if err != nil {
		return "", err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(script); err != nil {
		// Check if it's a network error vs command error
		if _, ok := err.(*net.OpError); ok {
			return "", fmt.Errorf("SSH connection lost during execution: %w", err)
		}
		return "", fmt.Errorf("remote command failed: %s%s: %w", stdout.String(), stderr.String(), err)
	}

	return stdout.String(), nil
}
