# Indexia - Telegram Alphabetical Channel Directory Bot

Indexia is a Go-powered Telegram bot that automatically organizes links and directory entries into dedicated, alphabetically sorted messages in a Telegram channel.

---

## 🌟 Key Features

- **Alphabetical Auto-Sorting**: Entries are grouped by letter (`A` through `Z`, plus `#` for numbers/symbols) and sorted alphabetically by name.
- **Dynamic Message Cascading**: 
  - Each alphabet letter gets its own message(s) in the target Telegram channel.
  - If a letter group exceeds Telegram's message limit (~3800 characters), it automatically splits into multi-part messages (e.g. `A (Part 1)`, `A (Part 2)`).
  - Subsequent letter messages shift gracefully.
- **Multi-Footer Media Support**:
  - Add custom text, stickers, photos, videos, GIFs, or document footers below all alphabet messages.
  - Footers stay at the very bottom and are only re-positioned when page cascading occurs or when footers change.
- **Admin Setup & Control**:
  - The first user to message the bot (`/start`) automatically becomes the **Initial Super Admin**.
  - Admins can add or remove other administrators via Telegram User ID or Username.
- **Pure Go CGO-Free SQLite**: Uses GORM with `glebarez/go-sqlite` for seamless cross-compilation on Linux, Windows, and macOS without requiring a C compiler.
- **GitHub Release Workflow & Versioning Helper**: Built-in GitHub Action and helper scripts to automate tagging and releasing binaries for all OS architectures.

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
# Run the bot
go run main.go
```

---

## 👑 Command Reference

| Command | Description |
|---|---|
| `/add Name \| https://link.com` | Add a new entry (or simply send `Name \| URL`) |
| `/delete <ID>` | Delete an entry by its database ID |
| `/list` | Display all entries in the database |
| `/setchannel <channel_id>` | Configure target channel ID |
| `/addfooter <text>` | Add footer message (or reply to text/sticker/photo/video/GIF with `/addfooter`) |
| `/listfooters` | List all active footer messages |
| `/delfooter <ID>` | Delete a specific footer message by ID |
| `/clearfooters` | Delete all footer messages |
| `/sync` | Force channel refresh & cascading message update |
| `/addadmin <id_or_username>` | Add a new administrator |
| `/removeadmin <id_or_username>` | Remove an administrator (Super Admin only) |
| `/admins` | List all current administrators |

---

## 🏷️ Versioning & Automated GitHub Releases

Use the included helper scripts to bump versions and create Git tags:

### On Windows (PowerShell):
```powershell
# Bump patch version (v1.0.0 -> v1.0.1)
.\scripts\version.ps1 -Type patch

# Bump minor version (v1.0.0 -> v1.1.0)
.\scripts\version.ps1 -Type minor

# Bump major version (v1.0.0 -> v2.0.0)
.\scripts\version.ps1 -Type major

# Push tag to GitHub to trigger automatic release build
git push origin --tags
```

### On Linux / macOS / Git Bash:
```bash
./scripts/version.sh patch
./scripts/version.sh minor
git push origin --tags
```
