package clean

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

var timeNow = time.Now

// confirm asks a y/N question on stdin.
func confirm(q string) bool {
	fmt.Printf("%s [y/N] ", q)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}
