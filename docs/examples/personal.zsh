# Add to ~/.zshrc before the direnv hook. Adapt ports to your installation.
export CODEX_HOME="$HOME/.codex"
export CODEX_REMOTE="ws://127.0.0.1:4501"

# To follow the installer's saved mapping, replace both exports with:
# source "$HOME/.codex/phatmon.env"
# With the codex.zsh function in ~/.zshrc, just run: codex
# Without the function: codex --remote "$CODEX_REMOTE" --cd "$PWD"
