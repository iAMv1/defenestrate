package purge

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// confirmP asks a y/N question on stdin.
func confirmP(q string) bool {
	fmt.Printf("%s [y/N] ", q)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func truncateP(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
