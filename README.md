# saneshell

> A next-generation shell that learns from you — proactive completions, inline previews, and post-hoc error detection.

[![Go Version](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)](https://golang.org)
[![Java Version](https://img.shields.io/badge/Java-21-ED8B00?logo=openjdk)](https://openjdk.org)
[![GraalVM](https://img.shields.io/badge/GraalVM-23+-F05032?logo=graalvm)](https://www.graalvm.org)
[![Protocol](https://img.shields.io/badge/Protocol-v1-6A5ACD)](internal/ipc/protocol.go)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

---

## Architecture

```mermaid
graph TB
    A[saneshell Go Core] --> B[Line Editor<br/>(vi keys)]
    A --> C[PTY / Job Control]
    A --> D[IPC Client<br/>Unix Socket JSONL]
    D --> E[saneshell-intel<br/>Java/GraalVM]
    E --> F[Completions]
    E --> G[Previews]
    E --> H[Learning Engine]
    E --> I[Ollama LLM]
```

<details>
<summary>ASCII fallback (for terminal viewing)</summary>

```
┌─────────────────────────────────────────────────────────────┐
│                        saneshell (Go)                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Line Editor │  │  PTY/Job    │  │ IPC Client (Unix    │  │
│  │  (vi keys)  │  │  Control    │  │  Socket JSONL)      │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
└─────────┼────────────────┼─────────────────────┼─────────────┘
          │                │                     │
          ▼                ▼                     ▼
   ┌──────────────┐ ┌─────────────┐      ┌──────────────────┐
   │  Commands    │ │ Interactive │      │ saneshell-intel  │
   │  (ls, cd,    │ │ (vim, less, │      │  (Java/GraalVM)  │
   │   git, ...)  │ │  top, ...)  │      │                  │
   └──────────────┘ └─────────────┘      │  • Completions   │
                                          │  • Previews      │
                                          │  • Learning      │
                                          │  • Ollama LLM    │
                                          └──────────────────┘
```
</details>

---

## Features

| Feature | Status | Description |
|---------|--------|-------------|
| Line editor (vi keys) | ✅ | History, tab completion, ghost text, vi normal/insert mode |
| `cd`/`pushd`/`popd` builtins | ✅ | Native, directory stack for pushd/popd |
| PTY passthrough | ✅ | vim, less, top work natively |
| Native completions | ✅ | PATH commands + filesystem |
| IPC protocol | ✅ | JSONL over Unix socket |
| Intelligence daemon | 🚧 | Java/GraalVM (completions, preview, learn) |
| Pre-check (⏎⏎) | 📋 | Inline risk analysis before execute |
| Post-hoc fix | 📋 | Learns from errors via Ollama |

---

## Quick Start

```bash
# Build (requires Go 1.22+, Gradle/GraalVM optional for intel daemon)
make build-go

# Run
./dist/saneshell

# With intel daemon (separate terminal):
make build-java
./dist/saneshell-intel
```

> **Version:** `0.1.0`

---

## Requirements

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.22+ | Core shell binary |
| Gradle | 8.5+ | Java daemon build (optional) |
| GraalVM | 23.1+ (Java 21) | Native-image compilation (optional) |
| glibc-devel / zlib-devel | — | Static linking for native-image (optional) |
| Ollama | 0.30+ | Optional LLM backend |

---

## Configuration

`~/.config/saneshell/config.toml`:

```toml
[editor]
mode = "vi"          # "vi" or "emacs"
prompt = "\033[32m{{.User}}@\033[36m{{.Host}}:\033[34m{{.CWD}}\033[32m$ \033[0m"
ghost_color = "\033[90m"

[intel]
enabled = false
socket_path = ""     # default: /tmp/saneshell-$UID.sock
timeout_ms = 5000
```

---

## Related

- **sanityshell** — Python REPL wrapper (stable, in `../sanityshell/`)
- Protocol spec — `internal/ipc/protocol.go`

---

## License

MIT
