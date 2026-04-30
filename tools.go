//go:build tools

package tools

// This file ensures tool and test dependencies are tracked in go.mod.
// These imports are used by various internal packages.
import (
	_ "github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/stretchr/testify/assert"
	_ "golang.org/x/crypto/ssh"
	_ "pgregory.net/rapid"
)
