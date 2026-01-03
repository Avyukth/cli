# Sessions and Checkpoints

## Overview

Entire CLI creates checkpoints for AI coding sessions. The system is agent-agnostic - it works with Claude Code, Cursor, Copilot, or any tool that triggers Entire hooks.

## Domain Model

### Session

A **Session** is a unit of work. Sessions can be nested - when a subagent runs, it creates a sub-session.

```go
type Session struct {
    ID             string
    FirstPrompt    string       // Raw first user prompt (immutable)
    Description    string       // Display text (derived or editable)
    StartTime      time.Time
    AgentType      string       // "claude-code", "cursor", etc.
    AgentSessionID string       // The agent's session identifier

    Checkpoints    []Checkpoint
    SubSessions    []Session    // Nested sessions (subagent work)

    // Empty for top-level sessions
    ParentID       string       // Parent session ID
    ToolUseID      string       // Tool invocation that spawned this
}

func (s *Session) IsSubSession() bool {
    return s.ParentID != ""
}
```

### Checkpoint

A **Checkpoint** captures a point-in-time within a session.

```go
type Checkpoint struct {
    ID        string
    SessionID string
    Timestamp time.Time
    Type      CheckpointType
    Message   string
}

type CheckpointType int

const (
    Temporary CheckpointType = iota // Full state snapshot, shadow branch
    Committed                        // Metadata + commit ref, entire/sessions
)
```

### Checkpoint Types

| Type | Contents | Use Case |
|------|----------|----------|
| Temporary | Full state (code + metadata) | Intra-session rewind, pre-commit |
| Committed | Metadata + commit reference | Permanent record, post-commit rewind |

### Session Nesting

```
Session (top-level, ParentID="")
├── Checkpoints: [c1, c2, c3]
└── SubSessions:
    └── Session (ParentID=<parent>, ToolUseID="toolu_abc")
        ├── Checkpoints: [c4, c5]
        └── SubSessions: [...] (can nest further)
```

Each session - top-level or nested - has its own FirstPrompt, Description, and Checkpoints.

## Interface

### Session Operations

```go
type Sessions interface {
    Create(ctx context.Context, opts CreateSessionOptions) (*Session, error)
    Get(ctx context.Context, sessionID string) (*Session, error)
    List(ctx context.Context) ([]Session, error) // Top-level sessions only
}
```

### Checkpoint Storage (Low-Level)

Primitives for reading/writing checkpoints. Used by strategies.

```go
type CheckpointStore interface {
    // Temporary checkpoint operations (shadow branches - full state)
    WriteTemporary(ctx context.Context, sessionID string, snapshot TemporaryCheckpoint) error
    ReadTemporary(ctx context.Context, sessionID string) (*TemporaryCheckpoint, error)
    ListTemporary(ctx context.Context) ([]CheckpointInfo, error)

    // Committed checkpoint operations (entire/sessions branch - metadata only)
    WriteCommitted(ctx context.Context, checkpoint CommittedCheckpoint) error
    ReadCommitted(ctx context.Context, checkpointID string) (*CommittedCheckpoint, error)
    ListCommitted(ctx context.Context) ([]CheckpointInfo, error)
}

type TemporaryCheckpoint struct {
    SessionID  string
    CodeTree   plumbing.Hash // Full worktree snapshot
    Transcript []byte
    Prompts    []string
    Context    []byte
}

type CommittedCheckpoint struct {
    ID         string
    SessionID  string
    CommitRef  plumbing.Hash // Reference to user's/auto-commit's code commit
    Transcript []byte
    Prompts    []string
    Context    []byte
    CreatedAt  time.Time
}
```

### Strategy-Level Operations

Strategies compose low-level primitives into higher-level workflows.

**Manual-commit** has condensation logic:

```go
// Condense reads accumulated temporary state and writes a committed checkpoint.
// Handles incremental extraction (since last condense) and derived data generation.
func (s *ManualCommitStrategy) Condense(ctx context.Context, sessionID string, commitRef plumbing.Hash) (*Checkpoint, error)
```

**Auto-commit** writes committed checkpoints directly:

```go
// SaveChanges writes directly to committed storage (no temporary phase).
func (s *AutoCommitStrategy) SaveChanges(ctx context.Context, ...) error
```

## Storage

| Type | Location | Contents |
|------|----------|----------|
| Session State | `.git/entire-sessions/<id>.json` | Active session tracking |
| Temporary | `entire/<commit-hash>` branch | Full state (code + metadata) |
| Committed | `entire/sessions` branch (sharded) | Metadata + commit reference |

### Session State

Location: `.git/entire-sessions/<session-id>.json`

Stored in git common dir (shared across worktrees). Tracks active session info.

### Temporary Checkpoints

Branch: `entire/<base-commit-hash>`

Contains full worktree snapshot plus metadata overlay:

```
<worktree files...>
.entire/metadata/<session-id>/
├── full.jsonl           # Session transcript
├── prompt.txt           # User prompts
├── context.md           # Generated context
└── subsessions/<id>/    # Nested session data
```

Tied to a base commit. Condensed to committed on user commit.

### Committed Checkpoints

Branch: `entire/sessions`

Metadata only, sharded by checkpoint ID:

```
<id[:2]>/<id[2:]>/
├── metadata.json        # Checkpoint info (includes commit reference)
├── full.jsonl           # Session transcript
├── prompt.txt
├── context.md
└── subsessions/<id>/    # Nested session data
```

### Package Structure

```
session/
├── session.go           # Session type
├── state.go             # Active session state

checkpoint/
├── checkpoint.go        # Checkpoint type
├── store.go             # CheckpointStore interface
├── temporary.go         # Shadow branch storage
├── committed.go         # Metadata branch storage
```

Strategies use `CheckpointStore` primitives - storage details are encapsulated.

## Strategy Role

Strategies determine checkpoint timing and type:

| Strategy | On Save | On SubSession Complete | On User Commit |
|----------|---------|------------------------|----------------|
| Manual-commit | Temporary | Temporary | Condense → Committed |
| Auto-commit | Committed | Committed | — |

## Rewind

Rewind is limited to top-level sessions for simplicity. Subsession rewind out of scope for now.

---

## Appendix: Legacy Names

| Current | Legacy |
|---------|--------|
| Manual-commit | Shadow |
| Auto-commit | Dual |
