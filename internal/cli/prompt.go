package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// promptInput is a package variable (rather than a hardcoded os.Stdin) so
// tests can substitute a fake reader without spawning a real process.
var promptInput io.Reader = os.Stdin

func confirm(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(promptInput)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
