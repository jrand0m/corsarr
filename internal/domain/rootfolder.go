package domain

import "time"

// RootFolderProblem is why a root folder cannot take imports.
type RootFolderProblem string

const (
	RootOK       RootFolderProblem = ""
	RootMissing  RootFolderProblem = "missing"   // absent or not a directory
	RootReadOnly RootFolderProblem = "read-only" // corsarr may not write
	RootFull     RootFolderProblem = "full"      // free space below the reserve
)

// RootFolder is a library root with its last checked state.
type RootFolder struct {
	Path        string            `json:"path"`
	Kind        MediaKind         `json:"kind"`
	FreeSpace   int64             `json:"freeSpace"`
	Accessible  bool              `json:"accessible"`
	Writable    bool              `json:"writable"`
	Problem     RootFolderProblem `json:"problem,omitempty"`
	Managed     bool              `json:"managed"` // from the config (not UI-created)
	LastChecked *time.Time        `json:"lastChecked,omitempty"`
}
