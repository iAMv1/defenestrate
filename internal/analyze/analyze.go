// Package analyze is the disk space analyzer: a concurrent size walk with a
// visual tree, largest-file list, optional JSON output and Recycle-Bin
// deletion of listed items.
package analyze

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/iAMv1/defenestrate/internal/safety"
	"github.com/iAMv1/defenestrate/internal/ui"
)

// Entry is one child in the tree.
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// File is one large item candidate.
type File struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Result is the analysis of one root.
type Result struct {
	Path       string  `json:"path"`
	Children   []Entry `json:"entries"`
	LargeFiles []File  `json:"large_files"`
	TotalBytes int64   `json:"total_size"`
	TotalFiles int     `json:"total_files"`
}

const (
	largeFileMin = 64 << 20 // report files/dirs at or above 64 MB
	walkWorkers  = 16
)

type walkOut struct {
	size      int64
	files     int
	largeList []File // large FILES found anywhere under a child dir
}

// Walk sizes every direct child of root concurrently and collects large items.
func Walk(root string, maxLarge int) (*Result, error) {
	res := &Result{Path: root}
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, walkWorkers)
		sizes []Entry
	)

	for _, e := range ents {
		e := e
		p := filepath.Join(root, e.Name())
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		if !e.IsDir() {
			mu.Lock()
			res.TotalBytes += info.Size()
			res.TotalFiles++
			if info.Size() >= largeFileMin {
				res.LargeFiles = append(res.LargeFiles, File{e.Name(), p, info.Size()})
			}
			sizes = append(sizes, Entry{Name: e.Name(), Path: p, Size: info.Size()})
			mu.Unlock()
			continue
		}
		wg.Add(1)
		go func(p, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var out walkOut
			filepath.WalkDir(p, func(fp string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil // skip unreadable; only files carry bytes
				}
				if safety.IsReparsePoint(fp) {
					return nil // never follow links/junctions
				}
				info, err2 := d.Info()
				if err2 != nil {
					return nil
				}
				out.size += info.Size()
				out.files++
				if info.Size() >= largeFileMin {
					out.largeList = append(out.largeList, File{d.Name(), fp, info.Size()})
				}
				return nil
			})
			mu.Lock()
			defer mu.Unlock()
			res.TotalBytes += out.size
			res.TotalFiles += out.files
			res.LargeFiles = append(res.LargeFiles, out.largeList...)
			if out.size >= largeFileMin {
				res.LargeFiles = append(res.LargeFiles, File{name + string(os.PathSeparator) + " (whole dir)", p, out.size})
			}
			sizes = append(sizes, Entry{Name: name, Path: p, Size: out.size, IsDir: true})
		}(p, e.Name())
	}
	wg.Wait()

	sort.Slice(sizes, func(i, j int) bool { return sizes[i].Size > sizes[j].Size })
	res.Children = sizes

	sort.Slice(res.LargeFiles, func(i, j int) bool { return res.LargeFiles[i].Size > res.LargeFiles[j].Size })
	if len(res.LargeFiles) > maxLarge {
		res.LargeFiles = res.LargeFiles[:maxLarge]
	}
	return res, nil
}

// Run implements `DEFENESTRATE analyze [path] [--json] [--top N] [--delete <path>]`.
func Run(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "machine-readable output")
	top := fs.Int("top", 20, "large items to list")
	del := fs.String("delete", "", "recycle this exact path after listing")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: DEFENESTRATE analyze [path] [--json] [--top N] [--delete <path>]")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	// Deletion-only invocation skips the walk entirely.
	if *del != "" {
		dabs, derr := filepath.Abs(*del)
		if derr != nil {
			return derr
		}
		fmt.Println(ui.Title("Recycling " + dabs) + ui.If(safety.DryRun(), ui.Warn("  [DRY RUN]")))
		return safety.Recycle([]string{dabs})
	}

	fmt.Println(ui.Title("Analyzing "+abs) + ui.Dim("  (concurrent walk; unreadable entries skipped)"))
	res, err := Walk(abs, *top)
	if err != nil {
		return err
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	fmt.Printf("\n%s  %s free\n\n",
		ui.Bold("Total: "+ui.HumanBytes(res.TotalBytes)), ui.HumanBytesU(ui.FreeBytes(abs)))

	fmt.Println(ui.Section("Largest children"))
	max := int64(1)
	for _, c := range res.Children {
		if c.Size > max {
			max = c.Size
		}
	}
	shown := res.Children
	if len(shown) > 15 {
		shown = shown[:15]
	}
	for _, c := range shown {
		frac := float64(c.Size) / float64(max)
		label := c.Name
		if len(label) > 36 {
			label = label[:35] + "…"
		}
		kind := "dir "
		if !c.IsDir {
			kind = "file"
		}
		fmt.Printf("  %-4s %-37s %s %10s\n", ui.Dim(kind), label, ui.Bar(frac, 24), ui.HumanBytes(c.Size))
	}

	fmt.Println(ui.Section(fmt.Sprintf("Large items (>=%s)", ui.HumanBytes(largeFileMin))))
	if len(res.LargeFiles) == 0 {
		fmt.Println(ui.Dim("  none found"))
	}
	for i, f := range res.LargeFiles {
		p := f.Path
		if len(p) > 90 {
			p = "…" + p[len(p)-89:]
		}
		fmt.Printf("  %3d. %10s  %s\n", i+1, ui.HumanBytes(f.Size), p)
	}

	if safety.DryRun() && *del == "" {
		fmt.Println(ui.Warn("\nDry run: nothing deleted. Use --delete to recycle one item."))
	} else if !safety.DryRun() {
		fmt.Println(ui.Dim("\nTip: recycle an item with  DEFENESTRATE analyze --delete \"<path>\"  (goes to Recycle Bin)"))
	}
	return nil
}
