_canhazgpu_complete() {
    local cur prev words cword
    _init_completion || return

    # Find position of '--'
    local passthrough_index=-1
    for i in "${!words[@]}"; do
        if [[ "${words[$i]}" == "--" ]]; then
            passthrough_index=$i
            break
        fi
    done

    if [[ $passthrough_index -ge 0 && $COMP_CWORD -gt $passthrough_index ]]; then
        # After '--' — delegate to the next command's completion function
        local cmd="${words[$((passthrough_index + 1))]}"
        if command -v "$cmd" >/dev/null; then
            # Set up COMP_WORDS and COMP_CWORD for the delegated command
            local subcmd_words=("${words[@]:$((passthrough_index + 1))}")
            local subcmd_cword=$((COMP_CWORD - passthrough_index - 1))
            COMP_WORDS=("${subcmd_words[@]}")
            COMP_CWORD=$subcmd_cword
            # Load the appropriate completion function
            local completion_loader
            completion_loader=$(complete -p "$cmd" 2>/dev/null | sed -n "s/.*-F \([a-zA-Z0-9_]*\).*/\1/p")
            if [[ -n "$completion_loader" && $(type -t "$completion_loader") == "function" ]]; then
                "$completion_loader"
                return
            fi
        fi
        # Fallback to filename completion
        compopt -o nospace
        COMPREPLY=( $(compgen -f -- "$cur") )
        return
    fi

    # Before '--', provide completion for canhazgpu itself
    case "$prev" in
        canhazgpu)
            COMPREPLY=( $(compgen -W "admin reserve release run status report help --help --redis-host --redis-port --redis-db" -- "$cur") )
            ;;
        admin)
            COMPREPLY=( $(compgen -W "--gpus --force --help" -- "$cur") )
            ;;
        reserve)
            COMPREPLY=( $(compgen -W "--gpus --duration --help" -- "$cur") )
            ;;
        release)
            COMPREPLY=( $(compgen -W "--help" -- "$cur") )
            ;;
        run)
            COMPREPLY=( $(compgen -W "--gpus --help --" -- "$cur") )
            ;;
        status)
            COMPREPLY=( $(compgen -W "--help" -- "$cur") )
            ;;
        report)
            COMPREPLY=( $(compgen -W "--days --help" -- "$cur") )
            ;;
        --duration)
            COMPREPLY=( $(compgen -W "30m 1h 2h 4h 8h 1d 2d" -- "$cur") )
            ;;
        --days)
            COMPREPLY=( $(compgen -W "1 3 7 14 30 60 90" -- "$cur") )
            ;;
        *)
            COMPREPLY=( $(compgen -W "admin reserve release run status report help --help --redis-host --redis-port --redis-db" -- "$cur") )
            ;;
    esac
}
complete -F _canhazgpu_complete -o default -o bashdefault canhazgpu
