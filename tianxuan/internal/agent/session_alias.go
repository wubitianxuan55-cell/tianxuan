package agent

import "tianxuan/internal/agent/session"

// Session is an alias for session.Session, kept for internal backward
// compatibility during the phased split. New code should import the
// session sub-package directly.
type Session = session.Session

// SessionInfo is an alias for session.Info.
type SessionInfo = session.Info

// BranchMeta is an alias for session.BranchMeta.
type BranchMeta = session.BranchMeta

// BranchInfo is an alias for session.BranchInfo.
type BranchInfo = session.BranchInfo

// Re-exported functions from session sub-package for internal use.
var (
	NewSession        = session.New
	LoadSession       = session.Load
	ListSessions      = session.List
	ArchiveSession    = session.Archive
	UnarchiveSession  = session.Unarchive
	NewSessionPath    = session.NewPath
	BranchID          = session.BranchID
	BranchMetaPath    = session.BranchMetaPath
	LoadBranchMeta    = session.LoadBranchMeta
	SaveBranchMeta    = session.SaveBranchMeta
	EnsureBranchMeta  = session.EnsureBranchMeta
	TouchBranchMeta   = session.TouchBranchMeta
	ListBranches      = session.ListBranches
)
