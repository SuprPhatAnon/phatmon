# Add to ~/.zshrc before the direnv hook. Adapt ports to your installation.
export CODEX_HOME="$HOME/.codex"
export CODEX_REMOTE="ws://127.0.0.1:4501"

# To follow the installer's saved mapping, replace both exports with:
# source "$HOME/.codex/phatmon.env"
# Start the client with: codex --remote "$CODEX_REMOTE"
