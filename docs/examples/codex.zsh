# Add this block to ~/.zshrc after your CODEX_HOME/CODEX_REMOTE defaults.
# >>> phatmon codex shortcut >>>
function codex {
  # Administrative and noninteractive commands use the original CLI directly.
  case "${1-}" in
    agents|exec|e|review|login|logout|mcp|plugin|app-server|remote-control|completion|update|doctor|sandbox|debug|apply|a|queue|archive|delete|migrate-rollouts|unarchive|cloud|exec-server|features|help|-h|--help|-V|--version)
      command codex "$@"
      return $?
      ;;
  esac

  local arg previous='' explicit_remote=0 explicit_cd=0
  local -a defaults=()
  for arg in "$@"; do
    # Skip option values so a prompt/config value cannot act as a flag.
    if [[ -n "$previous" ]]; then
      previous=''
      continue
    fi
    case "$arg" in
      --) break ;;
      --remote) explicit_remote=1; previous=value ;;
      --remote=*) explicit_remote=1 ;;
      --cd|-C) explicit_cd=1; previous=value ;;
      --cd=*|-C?*) explicit_cd=1 ;;
      -c|--config|-i|--image|-m|--model|-p|--profile|-s|--sandbox|-a|--ask-for-approval|--enable|--disable|--local-provider|--add-dir|--remote-auth-token-env)
        previous=value ;;
    esac
  done
  if (( ! explicit_remote )) && [[ -n "${CODEX_REMOTE-}" ]]; then
    defaults+=(--remote "$CODEX_REMOTE")
  fi
  if (( ! explicit_cd )); then
    defaults+=(--cd "$PWD")
  fi
  command codex "${defaults[@]}" "$@"
}
# <<< phatmon codex shortcut <<<
