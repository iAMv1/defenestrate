package safety

import "golang.org/x/sys/windows"

// IsReparsePoint reports whether path carries a reparse point (symlink or
// junction). Deletion walkers must SKIP these during traversal: following a
// junction can exit the intended tree (path containment violation), and
// recycling through a link destroys data living elsewhere. The link itself
// may still be recycled as a single entry.
func IsReparsePoint(path string) bool {
	attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(path))
	if err != nil {
		return false // unreadable: treat as opaque; stat errors surface elsewhere
	}
	return attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
