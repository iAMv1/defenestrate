// Package installer finds leftover installation files (setup exes, MSI,
// MSIX bundles and large archives) sitting in Downloads/Desktop-style
// folders long after they did their job.
package installer

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

const minInstallerBytes = 50 << 20 // 50 MB

var installerExts = map[string]bool{
	".exe": true, ".msi": true, ".msix": true, ".msixbundle": true, ".msp": true,
}

var archiveExts = map[string]bool{
	".zip": true, ".7z": true, ".rar": true,
}

type Finding struct {
	Path   string
	Name   string
	Size   int64
	Source string // "Downloads", "Desktop", "Temp"…
	IsArc  bool
}

func roots() []struct {
	label string
	dir   string
} {
	local := os.Getenv("LOCALAPPDATA")
	return []struct {
		label string
		dir   string
	}{
		{"Downloads", filepath.Join(os.Getenv("USERPROFILE"), "Downloads")},
		{"Desktop", filepath.Join(os.Getenv("USERPROFILE"), "Desktop")},
		{"Temp installers", filepath.Join(local, "Temp")},
	}
}

// Scan collects installer candidates from the standard drop zones.
// Executables living anywhere else are NOT our business.
func Scan() []Finding {
	var out []Finding
	for _, r := range roots() {
		ents, err := os.ReadDir(r.dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			isInst := installerExts[ext]
			isArc := archiveExts[ext]
			if !isInst && !isArc {
				continue
			}
			info, ierr := e.Info()
			if ierr != nil {
				continue
			}
			min := int64(minInstallerBytes)
			if isArc {
				min = 100 << 20 // archives must be bigger to be interesting
			}
			if info.Size() < min {
				continue
			}
			out = append(out, Finding{
				Path:   filepath.Join(r.dir, e.Name()),
				Name:   e.Name(),
				Size:   info.Size(),
				Source: r.label,
				IsArc:  isArc,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}

// Run implements `DEFENESTRATE installer [--dry-run]`.
func Run(args []string) error {
	fsI := flag.NewFlagSet("installer", flag.ContinueOnError)
	yes := fsI.Bool("y", false, "skip confirmation")
	limit := fsI.Int("top", 25, "how many to list")
	if err := fsI.Parse(args); err != nil {
		return err
	}

	fmt.Println(ui.Title("DEFENESTRATE installer sweep") + ui.If(safety.DryRun(), ui.Warn("  [DRY RUN]")))
	findings := Scan()
	if len(findings) == 0 {
		fmt.Println(ui.Good("\nNo leftover installers found."))
		return nil
	}

	var total int64
	shown := findings
	if len(shown) > *limit {
		shown = shown[:*limit]
	}
	fmt.Println(ui.Section(fmt.Sprintf("%d installers (%s shown)", len(findings), ui.HumanBytes(shown[0].Size))))
	for i, f := range shown {
		arc := ""
		if f.IsArc {
			arc = ui.StyleDim.Render(" archive")
		}
		p := f.Path
		if len(p) > 70 {
			p = "…" + p[len(p)-69:]
		}
		fmt.Printf("  %3d. %10s  %s%s\n     %s\n",
			i+1, ui.HumanBytes(f.Size), p, arc,
			ui.StyleDim.Render(f.Source))
		_ = total
	}
	total = 0
	for _, f := range findings {
		total += f.Size
	}
	fmt.Println(ui.Rule())
	fmt.Printf("%s across %d files\n", ui.Bold(ui.HumanBytes(total)), len(findings))

	if safety.DryRun() {
		fmt.Println(ui.Warn("\nDry run — nothing deleted."))
		return nil
	}
	if !*yes && !confirm(fmt.Sprintf("Recycle all %d listed (%s)?", len(shown), ui.HumanBytes(total))) {
		fmt.Println(ui.Dim("Cancelled."))
		return nil
	}
	freed := int64(0)
	okCount := 0
	for _, f := range shown {
		if err := safety.Recycle([]string{f.Path}); err != nil {
			fmt.Println(ui.Bad("skip:", f.Path, "-", err))
			continue
		}
		freed += f.Size
		safety.Logf("installer-recycle", f.Path, f.Size)
		okCount++
	}
	fmt.Println(ui.Rule())
	fmt.Println(ui.Good(fmt.Sprintf("Freed %s from %d installers.", ui.HumanBytes(freed), okCount)))
	return nil
}

func confirm(q string) bool {
	fmt.Printf("%s [y/N] ", q)
	var in string
	fmt.Scanln(&in)
	in = strings.TrimSpace(strings.ToLower(in))
	return in == "y" || in == "yes"
}
