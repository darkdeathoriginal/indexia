# Indexia - Telegram Alphabetical Channel Directory Bot

Indexia is a Go-powered Telegram bot that automatically organizes links and directory entries into dedicated, alphabetically sorted messages in a Telegram channel.

---

## 🌟 Key Features

- **Alphabetical Auto-Sorting**: Entries are grouped by letter (`A` through `Z`, plus `#` for numbers/symbols) and sorted alphabetically by name.
- **Dynamic Message Cascading**: 
  - Each alphabet letter gets its own message(s) in the target Telegram channel.
  - If a letter group exceeds Telegram's message limit (~3800 characters), it automatically splits into multi-part messages (e.g. `A (Part 1)`, `A (Part 2)`).
  - Subsequent letter messages and channel message positions shift gracefully.
- **Persistent Footer Messages**:
  - Add custom footer/pinned notes below all alphabet messages.
  - When cascading occurs, footer messages are automatically deleted and re-posted at the very bottom of the channel after `Z`.
- **Admin Setup & Control**:
  - The first user to message the bot (`/start`) automatically becomes the **Initial Super Admin**.
  - Admins can add or remove other administrators via Telegram User ID or Username.
- **Pure Go CGO-Free SQLite**: Uses GORM with `glebarez/go-sqlite` for seamless cross-compilation on Linux, Windows, and macOS without requiring a C compiler.
- **GitHub Release Workflow**: Built-in GitHub Action to compile release binaries for all major operating systems and architectures.

---

## 🚀 Quick Start

### 1. Prerequisites
- Go 1.22 or higher installed
- Telegram Bot Token from [@BotFather](https://t.me/BotFather)
- A Telegram Channel where the bot is added as an **Administrator** with permission to post and edit messages.

### 2. Configuration
Copy `.env.example` to `.env` and fill in your details:

```env
BOT_TOKEN=123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ
CHANNEL_ID=-1001234567890
DB_PATH=indexia.db
```

### 3. Run Locally

```bash
# Download dependencies
go mod tidy

# Run the bot
go run main.go
```

---

## 👑 First-Time Setup & Admin Commands

1. Send `/start` to your bot in a private message.
2. The bot will automatically register you as the **Initial Super Admin**.
3. Set your target channel ID using `/setchannel -1001234567890` (or set `CHANNEL_ID` in `.env`).

### Command Reference

| Command | Description |
|---|---|
| `/add Name \| https://link.com` | Add a new entry (or simply send `Name \| URL`) |
| `/delete <ID>` | Delete an entry by its database ID |
| `/list` | Display all entries in the database |
| `/setchannel <channel_id>` | Configure target channel ID |
| `/addfooter <text>` | Add custom footer message below all alphabets |
| `/clearfooters` | Delete all footer messages |
| `/sync` | Force channel refresh & cascading message update |
| `/addadmin <id_or_username>` | Add a new administrator |
| `/removeadmin <id_or_username>` | Remove an administrator (Super Admin only) |
| `/admins` | List all current administrators |

---

## 🛠️ Cross-Platform Building

To compile the binary locally for your operating system:

```bash
# Linux
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o indexia-linux main.go

# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o indexia-windows.exe main.go

# macOS
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o indexia-mac-arm64 main.go
```

---

## 📦 GitHub Release Workflow

Pushing a tag matching `v*` (e.g. `v1.0.0`) triggers the `.github/workflows/release.yml` workflow, automatically building and attaching binaries for:
- Linux (`amd64`, `arm64`)
- Windows (`amd64`, `arm64`)
- macOS (`amd64`, `arm64`)
