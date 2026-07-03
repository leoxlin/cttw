//go:build tools

package tools

import (
	_ "github.com/BurntSushi/toml"
	_ "github.com/charmbracelet/bubbles"
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/lipgloss"
	_ "github.com/google/uuid"
	_ "github.com/spf13/cobra"
	_ "github.com/stretchr/testify"
	_ "modernc.org/sqlite"
)
