#!/usr/bin/env bash
# install.sh — install GoGitIt as a systemd service that starts on boot.
#
#   ./install.sh               # per-user service (default) — runs as you, so
#                              # your SSH keys / gh token / git config work
#   sudo ./install.sh --system # system-wide service under a dedicated user
#   ./install.sh --uninstall   # (add --system for the system service)
#
# User mode (default) installs under ~/.local and enables lingering so the
# service comes up on boot without an interactive login. It runs as the current
# user, so git pulls use the same credentials as your interactive shell.
# System mode installs to /usr/local/bin, config in /etc/gogitit, state in
# /var/lib/gogitit, running as a dedicated `gogitit` user (no access to your
# $HOME credentials — configure PAT/keys for that user separately).
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_SRC="$REPO_DIR/bin/gogitit"
MODE="user"
ACTION="install"

for arg in "$@"; do
  case "$arg" in
    --system)    MODE="system" ;;
    --uninstall) ACTION="uninstall" ;;
    -h|--help)   grep '^#' "$0" | cut -c3-; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; exit 1 ;;
  esac
done

command -v systemctl >/dev/null || { echo "systemd not found; this installer only supports systemd." >&2; exit 1; }

# ---- uninstall -----------------------------------------------------------
if [[ "$ACTION" == "uninstall" ]]; then
  if [[ "$MODE" == "user" ]]; then
    systemctl --user disable --now gogitit.service 2>/dev/null || true
    rm -f "$HOME/.config/systemd/user/gogitit.service"
    systemctl --user daemon-reload
  else
    [[ $EUID -eq 0 ]] || { echo "system uninstall needs root (use sudo)." >&2; exit 1; }
    systemctl disable --now gogitit.service 2>/dev/null || true
    rm -f /etc/systemd/system/gogitit.service
    systemctl daemon-reload
  fi
  echo "GoGitIt service removed. Binary, config, and state left in place."
  exit 0
fi

# ---- build if needed -----------------------------------------------------
if [[ ! -x "$BIN_SRC" ]]; then
  echo "No built binary at $BIN_SRC — running 'make build'..."
  make -C "$REPO_DIR" build
fi

# ---- user mode -----------------------------------------------------------
if [[ "$MODE" == "user" ]]; then
  BIN_DIR="$HOME/.local/bin"
  WORK_DIR="$HOME/.local/share/gogitit"
  UNIT_DIR="$HOME/.config/systemd/user"
  mkdir -p "$BIN_DIR" "$WORK_DIR" "$UNIT_DIR"
  install -m755 "$BIN_SRC" "$BIN_DIR/gogitit"
  # Config lives in WORK_DIR — the binary reads config.yaml from its CWD.
  [[ -f "$WORK_DIR/config.yaml" ]] || install -m644 "$REPO_DIR/config.example.yaml" "$WORK_DIR/config.yaml"

  cat > "$UNIT_DIR/gogitit.service" <<EOF
[Unit]
Description=GoGitIt headless Git client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$WORK_DIR
ExecStart=$BIN_DIR/gogitit
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

  systemctl --user daemon-reload
  systemctl --user enable --now gogitit.service
  # Linger keeps the user manager running so the service survives logout / boots at startup.
  loginctl enable-linger "$USER" 2>/dev/null || echo "note: could not enable linger; service starts on next login."
  echo "Installed user service. Status: systemctl --user status gogitit"
  exit 0
fi

# ---- system mode ---------------------------------------------------------
[[ $EUID -eq 0 ]] || { echo "system install needs root (use sudo), or pass --user for a rootless service." >&2; exit 1; }

SVC_USER="gogitit"
BIN_DST="/usr/local/bin/gogitit"
CFG_DIR="/etc/gogitit"
WORK_DIR="/var/lib/gogitit"

id "$SVC_USER" &>/dev/null || useradd --system --home-dir "$WORK_DIR" --shell /usr/sbin/nologin "$SVC_USER"
install -m755 "$BIN_SRC" "$BIN_DST"
mkdir -p "$CFG_DIR" "$WORK_DIR"
[[ -f "$CFG_DIR/config.yaml" ]] || install -m644 "$REPO_DIR/config.example.yaml" "$CFG_DIR/config.yaml"
chown -R "$SVC_USER:$SVC_USER" "$WORK_DIR"

cat > /etc/systemd/system/gogitit.service <<EOF
[Unit]
Description=GoGitIt headless Git client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SVC_USER
Group=$SVC_USER
WorkingDirectory=$WORK_DIR
ExecStart=$BIN_DST
Restart=on-failure
RestartSec=3
# Light hardening. Not sandboxing the filesystem: GoGitIt manages arbitrary
# git repos on disk, so ProtectHome/ProtectSystem=strict would break it.
# ponytail: expand hardening if the repos live in a known fixed subtree.
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now gogitit.service
echo "Installed system service running as '$SVC_USER'."
echo "Config: $CFG_DIR/config.yaml   State: $WORK_DIR   Status: systemctl status gogitit"
